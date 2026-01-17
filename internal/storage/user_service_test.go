package storage

import (
	"os"
	"testing"

	"github.com/maruel/mddb/internal/models"
)

func TestUserService(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-user-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	memService, err := NewMembershipService(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	service, err := NewUserService(tempDir, memService)
	if err != nil {
		t.Fatal(err)
	}

	// Test CreateUser (First user should be Admin in handler logic, but service just takes the role)
	user, err := service.CreateUser("test@example.com", "password123", "Test User", models.RoleAdmin)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if user.Email != "test@example.com" {
		t.Errorf("Expected email test@example.com, got %s", user.Email)
	}
	if user.Role != models.RoleAdmin {
		t.Errorf("Expected role admin, got %s", user.Role)
	}

	// Test Authenticate
	authenticatedUser, err := service.Authenticate("test@example.com", "password123")
	if err != nil {
		t.Fatalf("Authentication failed: %v", err)
	}
	if authenticatedUser.ID != user.ID {
		t.Errorf("Expected user ID %s, got %s", user.ID, authenticatedUser.ID)
	}

	// Test Authenticate with wrong password
	_, err = service.Authenticate("test@example.com", "wrongpassword")
	if err == nil {
		t.Error("Expected authentication to fail with wrong password")
	}

	// Test CountUsers
	count, err := service.CountUsers()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("Expected 1 user, got %d", count)
	}

	// Test ListUsers
	users, err := service.ListUsers()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Errorf("Expected 1 user in list, got %d", len(users))
	}

	// Test UpdateUserRole
	err = service.UpdateUserRole(user.ID, models.RoleEditor)
	if err != nil {
		t.Fatalf("Failed to update user role: %v", err)
	}

	updatedUser, err := service.GetUser(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedUser.Role != models.RoleEditor {
		t.Errorf("Expected role editor, got %s", updatedUser.Role)
	}
}
