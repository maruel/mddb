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

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// User represents a system user (persistent fields only).
type User struct {
	ID              jsonldb.ID      `json:"id" jsonschema:"description=Unique user identifier"`
	Email           string          `json:"email" jsonschema:"description=User email address"`
	EmailVerified   bool            `json:"email_verified,omitempty" jsonschema:"description=Whether the email has been verified"`
	Name            string          `json:"name" jsonschema:"description=User display name"`
	IsGlobalAdmin   bool            `json:"is_global_admin,omitempty" jsonschema:"description=Whether user has server-wide administrative access"`
	OAuthIdentities []OAuthIdentity `json:"oauth_identities,omitempty" jsonschema:"description=Linked OAuth provider accounts"`
	Quotas          UserQuota       `json:"quotas" jsonschema:"description=Resource limits for the user"`
	Settings        UserSettings    `json:"settings" jsonschema:"description=Global user preferences"`
	Created         storage.Time    `json:"created" jsonschema:"description=Account creation timestamp"`
	Modified        storage.Time    `json:"modified" jsonschema:"description=Last modification timestamp"`
}

// GetID returns the User's ID.
func (u *User) GetID() jsonldb.ID {
	return u.ID
}

// UserSettings represents global user preferences.
type UserSettings struct {
	Theme                string       `json:"theme" jsonschema:"description=UI theme preference (light/dark/system)"`
	Language             string       `json:"language" jsonschema:"description=Preferred language code (en/fr/etc)"`
	LastActiveWorkspaces []jsonldb.ID `json:"last_active_workspaces,omitempty" jsonschema:"description=Recently used workspace IDs in LRU order (most recent first)"`
}

// UserQuota defines limits for a user.
type UserQuota struct {
	MaxOrganizations int `json:"max_organizations" jsonschema:"description=Maximum number of organizations the user can create"`
}

// OAuthProvider represents a supported OAuth2 provider.
type OAuthProvider string

const (
	// OAuthProviderGoogle represents Google OAuth.
	OAuthProviderGoogle OAuthProvider = "google"
	// OAuthProviderMicrosoft represents Microsoft OAuth.
	OAuthProviderMicrosoft OAuthProvider = "microsoft"
	// OAuthProviderGitHub represents GitHub OAuth.
	OAuthProviderGitHub OAuthProvider = "github"
)

// IsValid checks if the provider is a known valid value.
func (p OAuthProvider) IsValid() bool {
	switch p {
	case OAuthProviderGoogle, OAuthProviderMicrosoft, OAuthProviderGitHub:
		return true
	}
	return false
}

// OAuthIdentity represents a link between a local user and an OAuth2 provider.
type OAuthIdentity struct {
	Provider   OAuthProvider `json:"provider" jsonschema:"description=OAuth provider name (google/microsoft/github)"`
	ProviderID string        `json:"provider_id" jsonschema:"description=User ID at the OAuth provider"`
	Email      string        `json:"email" jsonschema:"description=Email address from OAuth provider"`
	AvatarURL  string        `json:"avatar_url,omitempty" jsonschema:"description=Profile picture URL from OAuth provider"`
	LastLogin  storage.Time  `json:"last_login" jsonschema:"description=Last login timestamp via this provider"`
}

// UserService handles user management and authentication.
type UserService struct {
	table   *jsonldb.Table[*userStorage]
	byEmail *jsonldb.UniqueIndex[string, *userStorage]
	byOAuth *oauthIndex
}

// NewUserService creates a new user service.
func NewUserService(tablePath string) (*UserService, error) {
	table, err := jsonldb.NewTable[*userStorage](tablePath)
	if err != nil {
		return nil, err
	}
	byEmail := jsonldb.NewUniqueIndex(table, func(u *userStorage) string { return u.Email })
	byOAuth := newOAuthIndex(table)
	return &UserService{table: table, byEmail: byEmail, byOAuth: byOAuth}, nil
}

// Create creates a new user.
// The first user created becomes a global admin.
func (s *UserService) Create(email, password, name string) (*User, error) {
	if email == "" || password == "" {
		return nil, errEmailPwdRequired
	}
	// Check if user already exists (direct index check, no copy)
	if s.byEmail.Get(email) != nil {
		return nil, errUserExists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}
	// First user becomes global admin
	isFirstUser := s.table.Len() == 0
	id := jsonldb.NewID()
	now := storage.Now()
	stored := &userStorage{
		User: User{
			ID:            id,
			Email:         email,
			Name:          name,
			IsGlobalAdmin: isFirstUser,
			Quotas: UserQuota{
				MaxOrganizations: 3,
			},
			Created:  now,
			Modified: now,
		},
		PasswordHash: string(hash),
	}
	if err := s.table.Append(stored); err != nil {
		return nil, err
	}
	user := stored.User
	return &user, nil
}

// Get retrieves a user by ID.
func (s *UserService) Get(id jsonldb.ID) (*User, error) {
	if id.IsZero() {
		return nil, errUserIDEmpty
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
func (s *UserService) GetByEmail(email string) (*User, error) {
	stored := s.byEmail.Get(email)
	if stored == nil {
		return nil, errUserNotFound
	}
	user := stored.User
	return &user, nil
}

// Authenticate verifies user credentials. O(1) lookup via index.
func (s *UserService) Authenticate(email, password string) (*User, error) {
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
func (s *UserService) GetByOAuth(provider OAuthProvider, providerID string) (*User, error) {
	stored := s.byOAuth.Get(provider, providerID)
	if stored == nil {
		return nil, errUserNotFound
	}
	user := stored.User
	return &user, nil
}

// Modify atomically modifies a user.
func (s *UserService) Modify(id jsonldb.ID, fn func(user *User) error) (*User, error) {
	if id.IsZero() {
		return nil, errUserIDEmpty
	}
	stored, err := s.table.Modify(id, func(row *userStorage) error {
		return fn(&row.User)
	})
	if err != nil {
		return nil, err
	}
	user := stored.User
	return &user, nil
}

// Iter iterates over users with ID greater than startID. Pass 0 to iterate from the beginning.
func (s *UserService) Iter(startID jsonldb.ID) iter.Seq[*User] {
	return func(yield func(*User) bool) {
		for stored := range s.table.Iter(startID) {
			user := stored.User
			if !yield(&user) {
				return
			}
		}
	}
}

// HasPassword checks if the user has a password set.
func (s *UserService) HasPassword(id jsonldb.ID) bool {
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
func (s *UserService) VerifyPassword(id jsonldb.ID, password string) bool {
	if id.IsZero() || password == "" {
		return false
	}
	stored := s.table.Get(id)
	if stored == nil || stored.PasswordHash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte(password)) == nil
}

//

var (
	errUserIDRequired   = errors.New("id is required")
	errEmailPwdRequired = errors.New("email and password are required")
	errUserExists       = errors.New("user already exists")
	errInvalidCreds     = errors.New("invalid credentials")
	errInvalidUserQuota = errors.New("invalid user quota")
)

// oauthKey is a composite key for OAuth identity lookups.
type oauthKey struct {
	Provider   OAuthProvider
	ProviderID string
}

// oauthIndex indexes users by their OAuth identities (multi-valued).
type oauthIndex struct {
	table *jsonldb.Table[*userStorage]
	byKey map[oauthKey]jsonldb.ID
}

func newOAuthIndex(table *jsonldb.Table[*userStorage]) *oauthIndex {
	idx := &oauthIndex{table: table, byKey: make(map[oauthKey]jsonldb.ID)}
	table.AddObserver(idx)
	return idx
}

func (idx *oauthIndex) Get(provider OAuthProvider, providerID string) *userStorage {
	id, ok := idx.byKey[oauthKey{Provider: provider, ProviderID: providerID}]
	if !ok {
		return nil
	}
	return idx.table.Get(id)
}

func (idx *oauthIndex) OnAppend(row *userStorage) {
	for _, ident := range row.OAuthIdentities {
		idx.byKey[oauthKey{Provider: ident.Provider, ProviderID: ident.ProviderID}] = row.ID
	}
}

func (idx *oauthIndex) OnUpdate(prev, curr *userStorage) {
	// Remove old keys
	for _, ident := range prev.OAuthIdentities {
		delete(idx.byKey, oauthKey{Provider: ident.Provider, ProviderID: ident.ProviderID})
	}
	// Add new keys
	for _, ident := range curr.OAuthIdentities {
		idx.byKey[oauthKey{Provider: ident.Provider, ProviderID: ident.ProviderID}] = curr.ID
	}
}

func (idx *oauthIndex) OnDelete(row *userStorage) {
	for _, ident := range row.OAuthIdentities {
		delete(idx.byKey, oauthKey{Provider: ident.Provider, ProviderID: ident.ProviderID})
	}
}

type userStorage struct {
	User
	PasswordHash string `json:"password_hash" jsonschema:"description=Bcrypt-hashed password"`
}

func (u *userStorage) Clone() *userStorage {
	c := *u
	if u.OAuthIdentities != nil {
		c.OAuthIdentities = make([]OAuthIdentity, len(u.OAuthIdentities))
		copy(c.OAuthIdentities, u.OAuthIdentities)
	}
	return &c
}

// GetID returns the userStorage's ID.
func (u *userStorage) GetID() jsonldb.ID {
	return u.ID
}

// Validate checks that the userStorage is valid.
func (u *userStorage) Validate() error {
	if u.ID.IsZero() {
		return errUserIDRequired
	}
	if u.Email == "" {
		return errEmailEmpty
	}
	if u.Quotas.MaxOrganizations <= 0 {
		return errInvalidUserQuota
	}
	return nil
}
