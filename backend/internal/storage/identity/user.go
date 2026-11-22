// Manages user accounts, authentication, and profiles.

// Package identity provides services for user authentication and organization management.
//
// This package handles internal database tables (JSONL-backed) for:
//   - User accounts and authentication
//   - Organizations (multi-tenant workspaces)
//   - Memberships (user-organization relationships)
//   - Invitations (pending organization invites)
package identity

import (
	"errors"
	"fmt"
	"iter"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"golang.org/x/crypto/bcrypt"
)

// PreferredEmail returns the best email for external attribution
// (e.g. git commits). Priority: GitHub > Google > Microsoft > raw email.
// Identities without an email are skipped.
func PreferredEmail(u *identityv1.User) string {
	best := u.Email
	bestRank := 4
	for i := range u.OAuthIdentities {
		if u.OAuthIdentities[i].Email == "" {
			continue
		}
		var rank int
		switch u.OAuthIdentities[i].Provider {
		case identityv1.OAuthProviderGitHub:
			rank = 1
		case identityv1.OAuthProviderGoogle:
			rank = 2
		case identityv1.OAuthProviderMicrosoft:
			rank = 3
		default:
			continue
		}
		if rank < bestRank {
			best = u.OAuthIdentities[i].Email
			bestRank = rank
		}
	}
	return best
}

// UserService handles user management and authentication.
type UserService struct {
	table      *jsonldb.Table[*identityv1.StoredUser]
	byEmail    *jsonldb.UniqueIndex[string, *identityv1.StoredUser]
	byOAuth    *oauthIndex
	bcryptCost int
}

// NewUserService creates a new user service.
func NewUserService(tablePath string) (*UserService, error) {
	table, err := jsonldb.NewTable[*identityv1.StoredUser](tablePath)
	if err != nil {
		return nil, err
	}
	byEmail := jsonldb.NewUniqueIndex(table, func(u *identityv1.StoredUser) string { return u.Email })
	byOAuth := newOAuthIndex(table)
	return &UserService{table: table, byEmail: byEmail, byOAuth: byOAuth}, nil
}

// SetBcryptCostForTesting overrides the bcrypt cost for password hashing in tests.
func (s *UserService) SetBcryptCostForTesting(cost int) {
	s.bcryptCost = cost
}

func (s *UserService) cost() int {
	if s.bcryptCost > 0 {
		return s.bcryptCost
	}
	return bcrypt.DefaultCost
}

// Create creates a new user.
// The first user created becomes a global admin.
func (s *UserService) Create(email, password, name string) (*identityv1.User, error) {
	if email == "" || password == "" {
		return nil, errEmailPwdRequired
	}
	// Check if user already exists (direct index check, no copy)
	if s.byEmail.Get(email) != nil {
		return nil, errUserExists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost())
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}
	// First user becomes global admin
	isFirstUser := s.table.Len() == 0
	id := ksid.NewID()
	now := storagev1.Now()
	stored := &identityv1.StoredUser{
		User: identityv1.User{
			ID:            id,
			Email:         email,
			Name:          name,
			IsGlobalAdmin: isFirstUser,
			Quotas: identityv1.UserQuota{
				MaxOrganizations: 3,
			},
			Created:  now,
			Modified: now,
		},
		PasswordHash: string(hash),
	}
	if err := s.table.Append(stored, nil); err != nil {
		return nil, err
	}
	user := stored.User
	return &user, nil
}

// Get retrieves a user by ID.
func (s *UserService) Get(id ksid.ID) (*identityv1.User, error) {
	if id.IsZero() {
		return nil, identityv1.ErrUserIDEmpty
	}
	stored := s.table.Get(id)
	if stored == nil {
		return nil, errUserNotFound
	}
	user := stored.User
	return &user, nil
}

// Count returns the total number of users.
func (s *UserService) Count() int {
	return s.table.Len()
}

// GetByEmail retrieves a user by email. O(1) via index.
func (s *UserService) GetByEmail(email string) (*identityv1.User, error) {
	stored := s.byEmail.Get(email)
	if stored == nil {
		return nil, errUserNotFound
	}
	user := stored.User
	return &user, nil
}

// Authenticate verifies user credentials. O(1) lookup via index.
func (s *UserService) Authenticate(email, password string) (*identityv1.User, error) {
	stored := s.byEmail.Get(email)
	if stored == nil {
		return nil, errInvalidCreds
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte(password)); err != nil {
		return nil, errInvalidCreds
	}
	user := stored.User
	return &user, nil
}

// GetByOAuth retrieves a user by their OAuth identity. O(1) via index.
func (s *UserService) GetByOAuth(provider identityv1.OAuthProvider, providerID string) (*identityv1.User, error) {
	stored := s.byOAuth.Get(provider, providerID)
	if stored == nil {
		return nil, errUserNotFound
	}
	user := stored.User
	return &user, nil
}

// Modify atomically modifies a user.
func (s *UserService) Modify(id ksid.ID, fn func(user *identityv1.User) error) (*identityv1.User, error) {
	if id.IsZero() {
		return nil, identityv1.ErrUserIDEmpty
	}
	stored, err := s.table.Modify(id, func(row *identityv1.StoredUser) error {
		return fn(&row.User)
	})
	if err != nil {
		return nil, err
	}
	user := stored.User
	return &user, nil
}

// Iter iterates over users with ID greater than startID. Pass 0 to iterate from the beginning.
func (s *UserService) Iter(startID ksid.ID) iter.Seq[*identityv1.User] {
	return func(yield func(*identityv1.User) bool) {
		for stored := range s.table.Iter(startID) {
			user := stored.User
			if !yield(&user) {
				return
			}
		}
	}
}

// HasPassword checks if the user has a password set.
func (s *UserService) HasPassword(id ksid.ID) bool {
	if id.IsZero() {
		return false
	}
	stored := s.table.Get(id)
	if stored == nil {
		return false
	}
	return stored.PasswordHash != ""
}

// VerifyPassword checks if the provided password matches the user's stored hash.
func (s *UserService) VerifyPassword(id ksid.ID, password string) bool {
	if id.IsZero() || password == "" {
		return false
	}
	stored := s.table.Get(id)
	if stored == nil || stored.PasswordHash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte(password)) == nil
}

// SetPassword sets or updates the user's password.
// If the user already has a password, currentPassword must match.
// If the user is OAuth-only (no password), currentPassword is ignored.
func (s *UserService) SetPassword(id ksid.ID, currentPassword, newPassword string) error {
	if id.IsZero() {
		return identityv1.ErrUserIDEmpty
	}
	if newPassword == "" {
		return errPasswordRequired
	}

	stored := s.table.Get(id)
	if stored == nil {
		return errUserNotFound
	}

	// If user already has a password, verify the current password
	if stored.PasswordHash != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte(currentPassword)); err != nil {
			return errInvalidCreds
		}
	}

	// Hash the new password
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), s.cost())
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update password in storage
	_, err = s.table.Modify(id, func(row *identityv1.StoredUser) error {
		row.PasswordHash = string(hash)
		row.Modified = storagev1.Now()
		return nil
	})
	return err
}

//

var (
	errEmailPwdRequired = errors.New("email and password are required")
	errUserExists       = errors.New("user already exists")
	errInvalidCreds     = errors.New("invalid credentials")
	errPasswordRequired = errors.New("new password is required")
)

// oauthKey is a composite key for OAuth identity lookups.
type oauthKey struct {
	Provider   identityv1.OAuthProvider
	ProviderID string
}

// oauthIndex indexes users by their OAuth identities (multi-valued).
type oauthIndex struct {
	table *jsonldb.Table[*identityv1.StoredUser]
	byKey map[oauthKey]ksid.ID
}

func newOAuthIndex(table *jsonldb.Table[*identityv1.StoredUser]) *oauthIndex {
	idx := &oauthIndex{table: table, byKey: make(map[oauthKey]ksid.ID)}
	table.AddObserver(idx)
	return idx
}

func (idx *oauthIndex) Get(provider identityv1.OAuthProvider, providerID string) *identityv1.StoredUser {
	id, ok := idx.byKey[oauthKey{Provider: provider, ProviderID: providerID}]
	if !ok {
		return nil
	}
	return idx.table.Get(id)
}

func (idx *oauthIndex) OnAppend(row *identityv1.StoredUser) {
	for _, ident := range row.OAuthIdentities {
		idx.byKey[oauthKey{Provider: ident.Provider, ProviderID: ident.ProviderID}] = row.ID
	}
}

func (idx *oauthIndex) OnUpdate(prev, curr *identityv1.StoredUser) {
	// Remove old keys
	for _, ident := range prev.OAuthIdentities {
		delete(idx.byKey, oauthKey{Provider: ident.Provider, ProviderID: ident.ProviderID})
	}
	// Add new keys
	for _, ident := range curr.OAuthIdentities {
		idx.byKey[oauthKey{Provider: ident.Provider, ProviderID: ident.ProviderID}] = curr.ID
	}
}

func (idx *oauthIndex) OnDelete(row *identityv1.StoredUser) {
	for _, ident := range row.OAuthIdentities {
		delete(idx.byKey, oauthKey{Provider: ident.Provider, ProviderID: ident.ProviderID})
	}
}
