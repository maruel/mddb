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
	"os"
	"path/filepath"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"golang.org/x/crypto/bcrypt"
)

var (
	errUserIDRequired   = errors.New("id is required")
	errEmailPwdRequired = errors.New("email and password are required")
	errUserExists       = errors.New("user already exists")
	errInvalidCreds     = errors.New("invalid credentials")
)

// UserService handles user management and authentication.
type UserService struct {
	table *jsonldb.Table[*userStorage]
}

// NewUserService creates a new user service.
func NewUserService(rootDir string) (*UserService, error) {
	dbDir := filepath.Join(rootDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	tablePath := filepath.Join(dbDir, "users.jsonl")
	table, err := jsonldb.NewTable[*userStorage](tablePath)
	if err != nil {
		return nil, err
	}

	return &UserService{table: table}, nil
}

type userStorage struct {
	entity.User
	PasswordHash string `json:"password_hash" jsonschema:"description=Bcrypt-hashed password"`
}

func (u *userStorage) Clone() *userStorage {
	c := *u
	if u.OAuthIdentities != nil {
		c.OAuthIdentities = make([]entity.OAuthIdentity, len(u.OAuthIdentities))
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
	return nil
}

// Create creates a new user.
func (s *UserService) Create(email, password, name string) (*entity.User, error) {
	if email == "" || password == "" {
		return nil, errEmailPwdRequired
	}

	// Check if user already exists
	if _, err := s.GetByEmail(email); err == nil {
		return nil, errUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	id := jsonldb.NewID()
	now := time.Now()
	stored := &userStorage{
		User: entity.User{
			ID:       id,
			Email:    email,
			Name:     name,
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
func (s *UserService) Get(id jsonldb.ID) (*entity.User, error) {
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

// GetByEmail retrieves a user by email.
func (s *UserService) GetByEmail(email string) (*entity.User, error) {
	for stored := range s.table.Iter(0) {
		if stored.Email == email {
			user := stored.User
			return &user, nil
		}
	}
	return nil, errUserNotFound
}

// Authenticate verifies user credentials.
func (s *UserService) Authenticate(email, password string) (*entity.User, error) {
	for stored := range s.table.Iter(0) {
		if stored.Email == email {
			if err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte(password)); err != nil {
				return nil, errInvalidCreds
			}
			user := stored.User
			return &user, nil
		}
	}
	return nil, errInvalidCreds
}

// GetByOAuth retrieves a user by their OAuth identity.
func (s *UserService) GetByOAuth(provider, providerID string) (*entity.User, error) {
	for stored := range s.table.Iter(0) {
		for _, identity := range stored.OAuthIdentities {
			if identity.Provider == provider && identity.ProviderID == providerID {
				user := stored.User
				return &user, nil
			}
		}
	}
	return nil, errUserNotFound
}

// LinkOAuth links an OAuth identity to a user.
func (s *UserService) LinkOAuth(userID jsonldb.ID, identity entity.OAuthIdentity) error {
	if userID.IsZero() {
		return errUserIDEmpty
	}

	stored := s.table.Get(userID)
	if stored == nil {
		return errUserNotFound
	}

	// Check if already linked
	found := false
	for i, id := range stored.OAuthIdentities {
		if id.Provider == identity.Provider && id.ProviderID == identity.ProviderID {
			stored.OAuthIdentities[i].LastLogin = time.Now()
			found = true
			break
		}
	}

	if !found {
		stored.OAuthIdentities = append(stored.OAuthIdentities, identity)
	}

	stored.Modified = time.Now()
	_, err := s.table.Update(stored)
	return err
}

// UpdateSettings updates user global settings.
func (s *UserService) UpdateSettings(id jsonldb.ID, settings entity.UserSettings) error {
	if id.IsZero() {
		return errUserIDEmpty
	}

	stored := s.table.Get(id)
	if stored == nil {
		return errUserNotFound
	}

	stored.Settings = settings
	stored.Modified = time.Now()
	_, err := s.table.Update(stored)
	return err
}

// Iter iterates over all users.
func (s *UserService) Iter() iter.Seq[*entity.User] {
	return func(yield func(*entity.User) bool) {
		for stored := range s.table.Iter(0) {
			user := stored.User
			if !yield(&user) {
				return
			}
		}
	}
}
