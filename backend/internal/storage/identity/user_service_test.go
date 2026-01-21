package identity

import (
	"testing"
)

func TestUserService(t *testing.T) {
	tempDir := t.TempDir()

	service, err := NewUserService(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Test Create
	user, err := service.Create("test@example.com", "password123", "Test User")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if user.Email != "test@example.com" {
		t.Errorf("Expected email test@example.com, got %s", user.Email)
	}

	// Test Get
	retrieved, err := service.Get(user.ID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}
	if retrieved.ID != user.ID {
		t.Errorf("Expected user ID %s, got %s", user.ID, retrieved.ID)
	}

	// Test GetByEmail
	byEmail, err := service.GetByEmail("test@example.com")
	if err != nil {
		t.Fatalf("Failed to get user by email: %v", err)
	}
	if byEmail.ID != user.ID {
		t.Errorf("Expected user ID %s, got %s", user.ID, byEmail.ID)
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

	// Test duplicate user creation
	_, err = service.Create("test@example.com", "password456", "Another User")
	if err == nil {
		t.Error("Expected error when creating duplicate user")
	}
}
