package storage

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/maruel/mddb/internal/jsonldb"
	"github.com/maruel/mddb/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// UserService handles user management and authentication.
type UserService struct {
	rootDir    string
	table      *jsonldb.Table[userStorage]
	memService *MembershipService
	orgService *OrganizationService
	mu         sync.RWMutex
	byID       map[string]*userStorage
	byEmail    map[string]*userStorage
}

// NewUserService creates a new user service.
func NewUserService(rootDir string, memService *MembershipService, orgService *OrganizationService) (*UserService, error) {
	tablePath := filepath.Join(rootDir, "db", "users.jsonl")
	table, err := jsonldb.NewTable[userStorage](tablePath)
	if err != nil {
		return nil, err
	}

	s := &UserService{
		rootDir:    rootDir,
		table:      table,
		memService: memService,
		orgService: orgService,
		byID:       make(map[string]*userStorage),
		byEmail:    make(map[string]*userStorage),
	}

	for i := range table.Rows {
		ptr := &table.Rows[i]
		s.byID[ptr.ID] = ptr
		s.byEmail[ptr.Email] = ptr
	}

	return s, nil
}

type userStorage struct {
	models.User
	PasswordHash string `json:"password_hash"`
}

// CreateUser creates a new user.
func (s *UserService) CreateUser(email, password, name string, role models.UserRole) (*models.User, error) {
	if email == "" || password == "" {
		return nil, fmt.Errorf("email and password are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if user already exists
	if _, ok := s.byEmail[email]; ok {
		return nil, fmt.Errorf("user already exists")
	}

	id := generateShortID()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now()
	user := &models.User{
		ID:       id,
		Email:    email,
		Name:     name,
		Created:  now,
		Modified: now,
	}

	stored := userStorage{
		User:         *user,
		PasswordHash: string(hash),
	}

	if err := s.table.Append(stored); err != nil {
		return nil, err
	}

	// Update local cache
	s.table.Mu.RLock()
	newStored := &s.table.Rows[len(s.table.Rows)-1]
	s.table.Mu.RUnlock()

	s.byID[id] = newStored
	s.byEmail[email] = newStored

	return user, nil
}

// CountUsers returns the total number of users.
func (s *UserService) CountUsers() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.byID), nil
}

// UpdateUserRole updates the role of a user in a specific organization.
func (s *UserService) UpdateUserRole(userID, orgID string, role models.UserRole) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.memService == nil {
		return fmt.Errorf("membership service not initialized")
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
func (s *UserService) GetUser(id string) (*models.User, error) {
	s.mu.RLock()
	stored, ok := s.byID[id]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("user not found")
	}

	user := stored.User
	s.populateMemberships(&user)
	return &user, nil
}

func (s *UserService) populateMemberships(user *models.User) {
	if s.memService != nil {
		mems, err := s.memService.ListByUser(user.ID)
		if err == nil {
			if s.orgService != nil {
				for i := range mems {
					org, err := s.orgService.GetOrganization(mems[i].OrganizationID)
					if err == nil {
						mems[i].OrganizationName = org.Name
					}
				}
			}
			user.Memberships = mems
		}
	}
}

// GetUserByEmail retrieves a user by email.
func (s *UserService) GetUserByEmail(email string) (*models.User, error) {
	s.mu.RLock()
	stored, ok := s.byEmail[email]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("user not found")
	}

	user := stored.User
	s.populateMemberships(&user)
	return &user, nil
}

// Authenticate verifies user credentials.
func (s *UserService) Authenticate(email, password string) (*models.User, error) {
	s.mu.RLock()
	stored, ok := s.byEmail[email]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("invalid credentials")
	}

	err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte(password))
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	user := stored.User
	s.populateMemberships(&user)
	return &user, nil
}

// GetUserByOAuth retrieves a user by their OAuth identity.
func (s *UserService) GetUserByOAuth(provider, providerID string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, stored := range s.byID {
		for _, identity := range stored.OAuthIdentities {
			if identity.Provider == provider && identity.ProviderID == providerID {
				user := stored.User
				s.populateMemberships(&user)
				return &user, nil
			}
		}
	}

	return nil, fmt.Errorf("user not found")
}

// LinkOAuthIdentity links an OAuth identity to a user.
func (s *UserService) LinkOAuthIdentity(userID string, identity models.OAuthIdentity) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.byID[userID]
	if !ok {
		return fmt.Errorf("user not found")
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
	return s.table.Replace(s.getAllFromCache())
}

// UpdateSettings updates user global settings.
func (s *UserService) UpdateSettings(id string, settings models.UserSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("user not found")
	}

	stored.Settings = settings
	stored.Modified = time.Now()

	return s.table.Replace(s.getAllFromCache())
}

func (s *UserService) getAllFromCache() []userStorage {
	rows := make([]userStorage, 0, len(s.byID))
	for _, v := range s.byID {
		rows = append(rows, *v)
	}
	return rows
}

// ListUsers returns all users.
func (s *UserService) ListUsers() ([]*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]*models.User, 0, len(s.byID))
	for _, stored := range s.byID {
		user := stored.User
		s.populateMemberships(&user)
		users = append(users, &user)
	}
	return users, nil
}

// generateShortID is a placeholder for a real ID generator if not available in utils.go
func generateShortID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
