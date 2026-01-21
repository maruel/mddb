package identity

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func TestInvitationService(t *testing.T) {
	service, err := NewInvitationService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	orgID := jsonldb.ID(100)
	email := "invitee@example.com"
	role := UserRoleEditor

	// Test CreateInvitation
	inv, err := service.Create(email, orgID, role)
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
	found, err := service.GetByToken(inv.Token)
	if err != nil {
		t.Fatalf("Failed to find invitation by token: %v", err)
	}
	if found.ID != inv.ID {
		t.Errorf("Expected ID %v, got %v", inv.ID, found.ID)
	}

	// Test Iter
	iter, err := service.Iter(orgID)
	if err != nil {
		t.Fatalf("Failed to iterate invitations: %v", err)
	}
	count := 0
	for range iter {
		count++
	}
	if count != 1 {
		t.Errorf("Expected 1 invitation, got %d", count)
	}

	// Test DeleteInvitation
	err = service.Delete(inv.ID)
	if err != nil {
		t.Fatalf("Failed to delete invitation: %v", err)
	}

	_, err = service.GetByToken(inv.Token)
	if err == nil {
		t.Error("Expected invitation to be deleted")
	}
}
