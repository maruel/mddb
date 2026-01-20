package storage

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/entity"
)

func TestInvitationService(t *testing.T) {
	tempDir := t.TempDir()

	service, err := NewInvitationService(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	orgID := testID(100)
	email := "invitee@example.com"
	role := entity.UserRoleEditor

	// Test CreateInvitation
	inv, err := service.CreateInvitation(email, orgID.String(), role)
	if err != nil {
		t.Fatalf("Failed to create invitation: %v", err)
	}

	if inv.Email != email {
		t.Errorf("Expected email %s, got %s", email, inv.Email)
	}
	if inv.OrganizationID != orgID {
		t.Errorf("Expected orgID %v, got %v", orgID, inv.OrganizationID)
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
		t.Errorf("Expected ID %v, got %v", inv.ID, found.ID)
	}

	// Test ListByOrganization
	list, err := service.ListByOrganization(orgID.String())
	if err != nil {
		t.Fatalf("Failed to list invitations: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("Expected 1 invitation, got %d", len(list))
	}

	// Test DeleteInvitation
	err = service.DeleteInvitation(inv.ID.String())
	if err != nil {
		t.Fatalf("Failed to delete invitation: %v", err)
	}

	_, err = service.GetInvitationByToken(inv.Token)
	if err == nil {
		t.Error("Expected invitation to be deleted")
	}
}
