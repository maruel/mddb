package handlers

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestRegister(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	userService, err := identity.NewUserService(filepath.Join(tempDir, "users.jsonl"))
	if err != nil {
		t.Fatalf("NewUserService failed: %v", err)
	}

	orgService, err := identity.NewOrganizationService(filepath.Join(tempDir, "organizations.jsonl"))
	if err != nil {
		t.Fatalf("NewOrganizationService failed: %v", err)
	}

	memService, err := identity.NewMembershipService(filepath.Join(tempDir, "memberships.jsonl"), userService, orgService)
	if err != nil {
		t.Fatalf("NewMembershipService failed: %v", err)
	}

	sessionService, err := identity.NewSessionService(filepath.Join(tempDir, "sessions.jsonl"))
	if err != nil {
		t.Fatalf("NewSessionService failed: %v", err)
	}

	gitService, err := git.New(ctx, tempDir, "test", "test@test.com")
	if err != nil {
		t.Fatalf("git.New failed: %v", err)
	}

	fileStore, err := content.NewFileStore(tempDir, gitService, orgService)
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}

	authHandler := NewAuthHandler(userService, memService, orgService, sessionService, fileStore, "secret")

	// Register Joe - should not create organization (frontend handles that)
	req1 := &dto.RegisterRequest{
		Email:    "joe@example.com",
		Password: "password",
		Name:     "Joe",
	}
	resp1, err := authHandler.Register(ctx, req1)
	if err != nil {
		t.Fatalf("Failed to register Joe: %v", err)
	}

	if resp1.User.Name != "Joe" {
		t.Errorf("Expected name Joe, got %s", resp1.User.Name)
	}

	// New users should have no memberships (org creation is frontend-driven)
	if len(resp1.User.Memberships) != 0 {
		t.Errorf("Expected Joe to have no memberships after registration, got %d", len(resp1.User.Memberships))
	}

	// Register Alice
	req2 := &dto.RegisterRequest{
		Email:    "alice@example.com",
		Password: "password",
		Name:     "Alice",
	}
	resp2, err := authHandler.Register(ctx, req2)
	if err != nil {
		t.Fatalf("Failed to register Alice: %v", err)
	}

	if resp2.User.Name != "Alice" {
		t.Errorf("Expected name Alice, got %s", resp2.User.Name)
	}

	// Alice should also have no memberships
	if len(resp2.User.Memberships) != 0 {
		t.Errorf("Expected Alice to have no memberships after registration, got %d", len(resp2.User.Memberships))
	}

	// Verify token is returned
	if resp1.Token == "" {
		t.Error("Expected Joe to receive a token")
	}
	if resp2.Token == "" {
		t.Error("Expected Alice to receive a token")
	}
}
