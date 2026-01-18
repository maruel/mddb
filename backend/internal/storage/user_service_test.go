package storage

import (
	"os"
	"testing"

	"github.com/maruel/mddb/backend/internal/models"
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

	service, err := NewUserService(tempDir, memService, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Test CreateUser
	user, err := service.CreateUser("test@example.com", "password123", "Test User", models.UserRoleAdmin)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if user.Email != "test@example.com" {
		t.Errorf("Expected email test@example.com, got %s", user.Email)
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
	orgID := "org123"
	err = service.UpdateUserRole(user.ID, orgID, models.UserRoleEditor)
	if err != nil {
		t.Fatalf("Failed to update user role: %v", err)
	}

	updatedUser, err := service.GetUser(user.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Check membership for role
	found := false
	for _, m := range updatedUser.Memberships {
		if m.OrganizationID == orgID && m.Role == models.UserRoleEditor {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected role editor in membership for org %s", orgID)
	}
}
