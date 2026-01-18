package handlers

import (
	"context"
	"os"
	"testing"

	"github.com/maruel/mddb/backend/internal/models"
	"github.com/maruel/mddb/backend/internal/storage"
)

func TestRegister(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-auth-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	fileStore, _ := storage.NewFileStore(tempDir)
	memService, _ := storage.NewMembershipService(tempDir)
	orgService, _ := storage.NewOrganizationService(tempDir, fileStore, nil)
	userService, _ := storage.NewUserService(tempDir, memService, orgService)
	authHandler := NewAuthHandler(userService, orgService, "secret")

	ctx := context.Background()

	// Register Joe
	req1 := models.RegisterRequest{
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
	if resp1.User.Memberships[0].Role != models.UserRoleAdmin {
		t.Errorf("Expected Joe to be admin in his org, got %s", resp1.User.Memberships[0].Role)
	}

	org1, err := orgService.GetOrganization(resp1OrgID)
	if err != nil {
		t.Fatalf("Failed to get Joe's organization: %v", err)
	}
	if org1.Name != "Joe's Organization" {
		t.Errorf("Expected Joe's Organization, got %s", org1.Name)
	}

	// Register Alice
	req2 := models.RegisterRequest{
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
	if resp2.User.Memberships[0].Role != models.UserRoleAdmin {
		t.Errorf("Expected Alice to be admin in her org, got %s", resp2.User.Memberships[0].Role)
	}

	org2, err := orgService.GetOrganization(resp2OrgID)
	if err != nil {
		t.Fatalf("Failed to get Alice's organization: %v", err)
	}
	if org2.Name != "Alice's Organization" {
		t.Errorf("Expected Alice's Organization, got %s", org2.Name)
	}
}
