package storage

import (
	"testing"

	"github.com/maruel/mddb/internal/models"
)

func TestInvitationService(t *testing.T) {
	tempDir := t.TempDir()

	service, err := NewInvitationService(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	orgID := "org1"
	email := "invitee@example.com"
	role := models.UserRoleEditor

	// Test CreateInvitation
	inv, err := service.CreateInvitation(email, orgID, role)
	if err != nil {
		t.Fatalf("Failed to create invitation: %v", err)
	}

	if inv.Email != email {
		t.Errorf("Expected email %s, got %s", email, inv.Email)
	}
	if inv.OrganizationID != orgID {
		t.Errorf("Expected orgID %s, got %s", orgID, inv.OrganizationID)
	}
	if inv.Role != role {
		t.Errorf("Expected role %s, got %s", role, inv.Role)
	}
	if inv.Token == "" {
		t.Error("Expected token to be generated")
	}

	// Test GetInvitationByToken
	found, err := service.GetInvitationByToken(inv.Token)
	if err != nil {
		t.Fatalf("Failed to find invitation by token: %v", err)
	}
	if found.ID != inv.ID {
		t.Errorf("Expected ID %s, got %s", inv.ID, found.ID)
	}

	// Test ListByOrganization
	list, err := service.ListByOrganization(orgID)
	if err != nil {
		t.Fatalf("Failed to list invitations: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("Expected 1 invitation, got %d", len(list))
	}

	// Test DeleteInvitation
	err = service.DeleteInvitation(inv.ID)
	if err != nil {
		t.Fatalf("Failed to delete invitation: %v", err)
	}

	_, err = service.GetInvitationByToken(inv.Token)
	if err == nil {
		t.Error("Expected invitation to be deleted")
	}
}
