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
	rootDir  string
	usersDir string
}

// NewUserService creates a new user service.
func NewUserService(rootDir string) (*UserService, error) {
	usersDir := filepath.Join(rootDir, "users")
	if err := os.MkdirAll(usersDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create users directory: %w", err)
	}

	return &UserService{
		rootDir:  rootDir,
		usersDir: usersDir,
	}, nil
}

// CreateUser creates a new user.
func (s *UserService) CreateUser(email, password, name string) (*models.User, error) {
	if email == "" || password == "" {
		return nil, fmt.Errorf("email and password are required")
	}

	// Check if user already exists (by scanning directory for now, or using a secondary index if needed)
	// For simplicity, we use email as a filename-safe ID or hash it.
	id := generateShortID() // We can use the same utility as for records if available

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now()
	user := &models.User{
		ID:           id,
		Email:        email,
		PasswordHash: string(hash),
		Name:         name,
		Role:         models.RoleViewer, // Default role
		Created:      now,
		Modified:     now,
	}

	if err := s.saveUser(user); err != nil {
		return nil, err
	}

	return user, nil
}

// GetUser retrieves a user by ID.
func (s *UserService) GetUser(id string) (*models.User, error) {
	filePath := filepath.Join(s.usersDir, id+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	var user models.User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("failed to parse user: %w", err)
	}

	return &user, nil
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
	user, err := s.GetUserByEmail(email)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	return user, nil
}

func (s *UserService) saveUser(user *models.User) error {
	data, err := json.MarshalIndent(user, "", "  ")
	if err != nil {
		return err
	}

	filePath := filepath.Join(s.usersDir, user.ID+".json")
	return os.WriteFile(filePath, data, 0o600)
}

// generateShortID is a placeholder for a real ID generator if not available in utils.go
func generateShortID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
