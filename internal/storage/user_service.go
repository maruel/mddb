package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/maruel/mddb/internal/models"
	"golang.org/x/crypto/bcrypt"
)

// UserService handles user management and authentication.
type UserService struct {
	rootDir    string
	usersDir   string
	memService *MembershipService
	orgService *OrganizationService
}

// NewUserService creates a new user service.
func NewUserService(rootDir string, memService *MembershipService, orgService *OrganizationService) (*UserService, error) {
	usersDir := filepath.Join(rootDir, "db", "users")
	if err := os.MkdirAll(usersDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create users directory: %w", err)
	}

	return &UserService{
		rootDir:    rootDir,
		usersDir:   usersDir,
		memService: memService,
		orgService: orgService,
	}, nil
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

	if role == "" {
		role = models.RoleViewer
	}

	// Check if user already exists
	if _, err := s.GetUserByEmail(email); err == nil {
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
		Role:     role,
		Created:  now,
		Modified: now,
	}

	if err := s.saveUser(user, string(hash)); err != nil {
		return nil, err
	}

	return user, nil
}

// CountUsers returns the total number of users.
func (s *UserService) CountUsers() (int, error) {
	entries, err := os.ReadDir(s.usersDir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			count++
		}
	}
	return count, nil
}

// UpdateUserRole updates the role of a user in their active organization.
func (s *UserService) UpdateUserRole(id string, role models.UserRole) error {
	user, hash, err := s.getUserWithHash(id)
	if err != nil {
		return err
	}

	user.Role = role
	user.Modified = time.Now()

	// Update membership if organization is set
	if user.OrganizationID != "" && s.memService != nil {
		if err := s.memService.UpdateRole(user.ID, user.OrganizationID, role); err != nil {
			// If membership doesn't exist, create it
			_, _ = s.memService.CreateMembership(user.ID, user.OrganizationID, role)
		}
	}

	return s.saveUser(user, hash)
}

// UpdateUserOrg updates the organization of a user and ensures membership exists.
func (s *UserService) UpdateUserOrg(id, orgID string) error {
	user, hash, err := s.getUserWithHash(id)
	if err != nil {
		return err
	}

	user.OrganizationID = orgID
	user.Modified = time.Now()

	// Ensure membership exists
	if orgID != "" && s.memService != nil {
		m, err := s.memService.GetMembership(user.ID, orgID)
		if err != nil {
			// Create membership if it doesn't exist
			_, err = s.memService.CreateMembership(user.ID, orgID, user.Role)
			if err != nil {
				return fmt.Errorf("failed to create membership: %w", err)
			}
		} else {
			// Sync user role with membership role for the active org
			user.Role = m.Role
		}
	}

	return s.saveUser(user, hash)
}

// GetUser retrieves a user by ID.
func (s *UserService) GetUser(id string) (*models.User, error) {
	user, _, err := s.getUserWithHash(id)
	if err != nil {
		return nil, err
	}
	s.populateMemberships(user)
	return user, nil
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

func (s *UserService) getUserWithHash(id string) (*models.User, string, error) {
	filePath := filepath.Join(s.usersDir, id+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("user not found")
	}

	var stored userStorage
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, "", fmt.Errorf("failed to parse user: %w", err)
	}

	return &stored.User, stored.PasswordHash, nil
}

// GetUserByEmail retrieves a user by email.
func (s *UserService) GetUserByEmail(email string) (*models.User, error) {
	entries, err := os.ReadDir(s.usersDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		user, err := s.GetUser(entry.Name()[:len(entry.Name())-5])
		if err == nil && user.Email == email {
			return user, nil
		}
	}

	return nil, fmt.Errorf("user not found")
}

// Authenticate verifies user credentials.
func (s *UserService) Authenticate(email, password string) (*models.User, error) {
	entries, err := os.ReadDir(s.usersDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		user, hash, err := s.getUserWithHash(entry.Name()[:len(entry.Name())-5])
		if err == nil && user.Email == email {
			err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
			if err != nil {
				return nil, fmt.Errorf("invalid credentials")
			}
			s.populateMemberships(user)
			return user, nil
		}
	}

	return nil, fmt.Errorf("invalid credentials")
}

// GetUserByOAuth retrieves a user by their OAuth identity.
func (s *UserService) GetUserByOAuth(provider, providerID string) (*models.User, error) {
	entries, err := os.ReadDir(s.usersDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		user, err := s.GetUser(entry.Name()[:len(entry.Name())-5])
		if err == nil {
			for _, identity := range user.OAuthIdentities {
				if identity.Provider == provider && identity.ProviderID == providerID {
					return user, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("user not found")
}

// LinkOAuthIdentity links an OAuth identity to a user.
func (s *UserService) LinkOAuthIdentity(userID string, identity models.OAuthIdentity) error {
	user, hash, err := s.getUserWithHash(userID)
	if err != nil {
		return err
	}

	// Check if already linked
	for i, id := range user.OAuthIdentities {
		if id.Provider == identity.Provider && id.ProviderID == identity.ProviderID {
			user.OAuthIdentities[i].LastLogin = time.Now()
			return s.saveUser(user, hash)
		}
	}

	user.OAuthIdentities = append(user.OAuthIdentities, identity)
	user.Modified = time.Now()
	return s.saveUser(user, hash)
}

func (s *UserService) saveUser(user *models.User, passwordHash string) error {
	stored := userStorage{
		User:         *user,
		PasswordHash: passwordHash,
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}

	filePath := filepath.Join(s.usersDir, user.ID+".json")
	return os.WriteFile(filePath, data, 0o600)
}

// ListUsers returns all users.
func (s *UserService) ListUsers() ([]*models.User, error) {
	entries, err := os.ReadDir(s.usersDir)
	if err != nil {
		return nil, err
	}

	var users []*models.User
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		user, err := s.GetUser(entry.Name()[:len(entry.Name())-5])
		if err == nil {
			users = append(users, user)
		}
	}
	return users, nil
}

// generateShortID is a placeholder for a real ID generator if not available in utils.go
func generateShortID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
