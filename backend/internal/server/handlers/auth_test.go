package handlers

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestRegister(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	memService, err := identity.NewMembershipService(filepath.Join(tempDir, "memberships.jsonl"))
	if err != nil {
		t.Fatalf("NewMembershipService failed: %v", err)
	}

	orgService, err := identity.NewOrganizationService(filepath.Join(tempDir, "organizations.jsonl"))
	if err != nil {
		t.Fatalf("NewOrganizationService failed: %v", err)
	}

	gitService, err := git.New(ctx, tempDir, "test", "test@test.com")
	if err != nil {
		t.Fatalf("git.New failed: %v", err)
	}

	fileStore, err := content.NewFileStore(tempDir, gitService, orgService)
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}

	userService, err := identity.NewUserService(filepath.Join(tempDir, "users.jsonl"))
	if err != nil {
		t.Fatalf("NewUserService failed: %v", err)
	}

	authHandler := NewAuthHandler(userService, memService, orgService, fileStore, "secret")

	// Register Joe
	req1 := dto.RegisterRequest{
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

	// Check Joe's membership
	if len(resp1.User.Memberships) == 0 {
		t.Error("Expected Joe to have a membership")
	}
	resp1OrgID := resp1.User.Memberships[0].OrganizationID
	if resp1OrgID == "" {
		t.Error("Expected Joe's membership to have an organization ID")
	}
	if resp1.User.Memberships[0].Role != dto.UserRoleAdmin {
		t.Errorf("Expected Joe to be admin in his org, got %s", resp1.User.Memberships[0].Role)
	}

	org1ID, err := jsonldb.DecodeID(resp1OrgID)
	if err != nil {
		t.Fatalf("Failed to decode Joe's organization ID: %v", err)
	}
	org1, err := orgService.Get(org1ID)
	if err != nil {
		t.Fatalf("Failed to get Joe's organization: %v", err)
	}
	if org1.Name != "Joe's Organization" {
		t.Errorf("Expected Joe's Organization, got %s", org1.Name)
	}

	// Register Alice
	req2 := dto.RegisterRequest{
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

	// Check Alice's membership
	if len(resp2.User.Memberships) == 0 {
		t.Error("Expected Alice to have a membership")
	}
	resp2OrgID := resp2.User.Memberships[0].OrganizationID
	if resp2OrgID == "" {
		t.Error("Expected Alice's membership to have an organization ID")
	}
	if resp2OrgID == resp1OrgID {
		t.Error("Expected Alice to have a different organization ID than Joe")
	}
	if resp2.User.Memberships[0].Role != dto.UserRoleAdmin {
		t.Errorf("Expected Alice to be admin in her org, got %s", resp2.User.Memberships[0].Role)
	}

	org2ID, err := jsonldb.DecodeID(resp2OrgID)
	if err != nil {
		t.Fatalf("Failed to decode Alice's organization ID: %v", err)
	}
	org2, err := orgService.Get(org2ID)
	if err != nil {
		t.Fatalf("Failed to get Alice's organization: %v", err)
	}
	if org2.Name != "Alice's Organization" {
		t.Errorf("Expected Alice's Organization, got %s", org2.Name)
	}
}
