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
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"golang.org/x/crypto/bcrypt"
)

var (
	errUserIDRequired   = errors.New("id is required")
	errEmailPwdRequired = errors.New("email and password are required")
	errUserExists       = errors.New("user already exists")
	errMemSvcNotInit    = errors.New("membership service not initialized")
	errInvalidCreds     = errors.New("invalid credentials")
)

// UserService handles user management and authentication.
type UserService struct {
	table      *jsonldb.Table[*userStorage]
	memService *MembershipService
	orgService *OrganizationService
	mu         sync.RWMutex
	byID       map[jsonldb.ID]*userStorage
	byEmail    map[string]*userStorage
}

// NewUserService creates a new user service.
func NewUserService(rootDir string, memService *MembershipService, orgService *OrganizationService) (*UserService, error) {
	dbDir := filepath.Join(rootDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	tablePath := filepath.Join(dbDir, "users.jsonl")
	table, err := jsonldb.NewTable[*userStorage](tablePath)
	if err != nil {
		return nil, err
	}

	s := &UserService{
		table:      table,
		memService: memService,
		orgService: orgService,
		byID:       make(map[jsonldb.ID]*userStorage),
		byEmail:    make(map[string]*userStorage),
	}

	for u := range table.Iter(0) {
		s.byID[u.ID] = u
		s.byEmail[u.Email] = u
	}

	return s, nil
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

// CreateUser creates a new user.
func (s *UserService) CreateUser(email, password, name string, role entity.UserRole) (*entity.User, error) {
	if email == "" || password == "" {
		return nil, errEmailPwdRequired
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if user already exists
	if _, ok := s.byEmail[email]; ok {
		return nil, errUserExists
	}

	id := jsonldb.NewID()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now()
	user := &entity.User{
		ID:       id,
		Email:    email,
		Name:     name,
		Created:  now,
		Modified: now,
	}

	stored := &userStorage{
		User:         *user,
		PasswordHash: string(hash),
	}

	if err := s.table.Append(stored); err != nil {
		return nil, err
	}

	s.byID[id] = stored
	s.byEmail[email] = stored

	return user, nil
}

// UpdateUserRole updates the role of a user in a specific organization.
func (s *UserService) UpdateUserRole(userID, orgID jsonldb.ID, role entity.UserRole) error {
	if userID.IsZero() {
		return errUserIDEmpty
	}
	if orgID.IsZero() {
		return errOrgIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.memService == nil {
		return errMemSvcNotInit
	}

	// Update membership
	if err := s.memService.UpdateRole(userID, orgID, role); err != nil {
		// If membership doesn't exist, create it
		_, err = s.memService.CreateMembership(userID, orgID, role)
		return err
	}

	return nil
}

// GetUser retrieves a user by ID.
func (s *UserService) GetUser(id jsonldb.ID) (*entity.User, error) {
	if id.IsZero() {
		return nil, errUserIDEmpty
	}

	s.mu.RLock()
	stored, ok := s.byID[id]
	s.mu.RUnlock()

	if !ok {
		return nil, errUserNotFound
	}

	user := stored.User
	return &user, nil
}

// MembershipWithOrgName wraps a membership with its organization name.
type MembershipWithOrgName struct {
	entity.Membership
	OrganizationName string
}

// GetMembershipsForUser returns memberships with organization names populated.
func (s *UserService) GetMembershipsForUser(userID jsonldb.ID) ([]MembershipWithOrgName, error) {
	if s.memService == nil {
		return nil, nil
	}

	mems, err := s.memService.ListByUser(userID)
	if err != nil {
		return nil, err
	}

	results := make([]MembershipWithOrgName, 0, len(mems))
	for _, m := range mems {
		result := MembershipWithOrgName{Membership: m}
		if s.orgService != nil {
			org, err := s.orgService.GetOrganization(m.OrganizationID)
			if err == nil {
				result.OrganizationName = org.Name
			}
		}
		results = append(results, result)
	}
	return results, nil
}

// UserWithMemberships wraps a user with their memberships.
type UserWithMemberships struct {
	User        *entity.User
	Memberships []MembershipWithOrgName
}

// GetUserWithMemberships retrieves a user by ID with their memberships.
func (s *UserService) GetUserWithMemberships(id jsonldb.ID) (*UserWithMemberships, error) {
	user, err := s.GetUser(id)
	if err != nil {
		return nil, err
	}

	// Memberships are supplementary; continue with empty list on error.
	mems, _ := s.GetMembershipsForUser(id)

	return &UserWithMemberships{
		User:        user,
		Memberships: mems,
	}, nil
}

// GetUserByEmail retrieves a user by email.
func (s *UserService) GetUserByEmail(email string) (*entity.User, error) {
	s.mu.RLock()
	stored, ok := s.byEmail[email]
	s.mu.RUnlock()

	if !ok {
		return nil, errUserNotFound
	}

	user := stored.User
	return &user, nil
}

// Authenticate verifies user credentials.
func (s *UserService) Authenticate(email, password string) (*entity.User, error) {
	s.mu.RLock()
	stored, ok := s.byEmail[email]
	s.mu.RUnlock()

	if !ok {
		return nil, errInvalidCreds
	}

	err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte(password))
	if err != nil {
		return nil, errInvalidCreds
	}

	user := stored.User
	return &user, nil
}

// GetUserByOAuth retrieves a user by their OAuth identity.
func (s *UserService) GetUserByOAuth(provider, providerID string) (*entity.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, stored := range s.byID {
		for _, identity := range stored.OAuthIdentities {
			if identity.Provider == provider && identity.ProviderID == providerID {
				user := stored.User
				return &user, nil
			}
		}
	}

	return nil, errUserNotFound
}

// LinkOAuthIdentity links an OAuth identity to a user.
func (s *UserService) LinkOAuthIdentity(userID jsonldb.ID, identity entity.OAuthIdentity) error {
	if userID.IsZero() {
		return errUserIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.byID[userID]
	if !ok {
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
	if _, err := s.table.Update(stored); err != nil {
		return err
	}
	return nil
}

// UpdateSettings updates user global settings.
func (s *UserService) UpdateSettings(id jsonldb.ID, settings entity.UserSettings) error {
	if id.IsZero() {
		return errUserIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.byID[id]
	if !ok {
		return errUserNotFound
	}

	stored.Settings = settings
	stored.Modified = time.Now()

	if _, err := s.table.Update(stored); err != nil {
		return err
	}
	return nil
}

// ListUsers returns all users (domain models without runtime fields).
func (s *UserService) ListUsers() ([]*entity.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]*entity.User, 0, len(s.byID))
	for _, stored := range s.byID {
		user := stored.User
		users = append(users, &user)
	}
	return users, nil
}

// ListUsersWithMemberships returns all users with their memberships.
func (s *UserService) ListUsersWithMemberships() ([]UserWithMemberships, error) {
	users, err := s.ListUsers()
	if err != nil {
		return nil, err
	}

	results := make([]UserWithMemberships, 0, len(users))
	for _, user := range users {
		// Memberships are supplementary; continue with empty list on error.
		mems, _ := s.GetMembershipsForUser(user.ID)
		results = append(results, UserWithMemberships{
			User:        user,
			Memberships: mems,
		})
	}
	return results, nil
}
