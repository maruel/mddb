package handlers

import (
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestRegister(t *testing.T) {
	ctx := t.Context()
	tempDir := t.TempDir()

	userService, err := identity.NewUserService(filepath.Join(tempDir, "users.jsonl"))
	if err != nil {
		t.Fatalf("NewUserService failed: %v", err)
	}

	orgService, err := identity.NewOrganizationService(filepath.Join(tempDir, "organizations.jsonl"))
	if err != nil {
		t.Fatalf("NewOrganizationService failed: %v", err)
	}

	wsService, err := identity.NewWorkspaceService(filepath.Join(tempDir, "workspaces.jsonl"))
	if err != nil {
		t.Fatalf("NewWorkspaceService failed: %v", err)
	}

	orgMemService, err := identity.NewOrganizationMembershipService(filepath.Join(tempDir, "org_memberships.jsonl"), userService, orgService)
	if err != nil {
		t.Fatalf("NewOrganizationMembershipService failed: %v", err)
	}

	wsMemService, err := identity.NewWorkspaceMembershipService(filepath.Join(tempDir, "ws_memberships.jsonl"), wsService, orgService)
	if err != nil {
		t.Fatalf("NewWorkspaceMembershipService failed: %v", err)
	}

	sessionService, err := identity.NewSessionService(filepath.Join(tempDir, "sessions.jsonl"))
	if err != nil {
		t.Fatalf("NewSessionService failed: %v", err)
	}

	gitMgr := git.NewManager(tempDir, "test", "test@test.com")

	fileStore, err := content.NewFileStoreService(tempDir, gitMgr, wsService, orgService)
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}

	authHandler := NewAuthHandler(userService, orgMemService, wsMemService, orgService, wsService, sessionService, nil, nil, fileStore, "secret", "http://localhost:8080", 0, 0, 0)

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

	// New users should have no org/workspace memberships
	if len(resp1.User.Organizations) != 0 {
		t.Errorf("Expected Joe to have no org memberships after registration, got %d", len(resp1.User.Organizations))
	}
	if len(resp1.User.Workspaces) != 0 {
		t.Errorf("Expected Joe to have no workspace memberships after registration, got %d", len(resp1.User.Workspaces))
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
	if len(resp2.User.Organizations) != 0 {
		t.Errorf("Expected Alice to have no org memberships after registration, got %d", len(resp2.User.Organizations))
	}

	// Verify token is returned
	if resp1.Token == "" {
		t.Error("Expected Joe to receive a token")
	}
	if resp2.Token == "" {
		t.Error("Expected Alice to receive a token")
	}
}
