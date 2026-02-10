package identity

import (
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/ksid"
)

func TestOrganizationInvitation(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			valid := &OrganizationInvitation{
				ID:             ksid.ID(1),
				Email:          "test@example.com",
				OrganizationID: ksid.ID(100),
				Role:           OrgRoleMember,
				Token:          "token123",
			}
			if err := valid.Validate(); err != nil {
				t.Errorf("Expected valid invitation, got error: %v", err)
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			zeroID := &OrganizationInvitation{
				ID:             ksid.ID(0),
				Email:          "test@example.com",
				OrganizationID: ksid.ID(100),
				Role:           OrgRoleMember,
				Token:          "token123",
			}
			if err := zeroID.Validate(); err == nil {
				t.Error("Expected error for zero ID")
			}
		})

		t.Run("empty email", func(t *testing.T) {
			emptyEmail := &OrganizationInvitation{
				ID:             ksid.ID(1),
				Email:          "",
				OrganizationID: ksid.ID(100),
				Role:           OrgRoleMember,
				Token:          "token123",
			}
			if err := emptyEmail.Validate(); err == nil {
				t.Error("Expected error for empty email")
			}
		})

		t.Run("zero org ID", func(t *testing.T) {
			zeroOrg := &OrganizationInvitation{
				ID:             ksid.ID(1),
				Email:          "test@example.com",
				OrganizationID: ksid.ID(0),
				Role:           OrgRoleMember,
				Token:          "token123",
			}
			if err := zeroOrg.Validate(); err == nil {
				t.Error("Expected error for zero org ID")
			}
		})

		t.Run("empty token", func(t *testing.T) {
			emptyToken := &OrganizationInvitation{
				ID:             ksid.ID(1),
				Email:          "test@example.com",
				OrganizationID: ksid.ID(100),
				Role:           OrgRoleMember,
				Token:          "",
			}
			if err := emptyToken.Validate(); err == nil {
				t.Error("Expected error for empty token")
			}
		})
	})

	t.Run("Clone", func(t *testing.T) {
		original := &OrganizationInvitation{
			ID:             ksid.ID(1),
			Email:          "test@example.com",
			OrganizationID: ksid.ID(100),
			Role:           OrgRoleAdmin,
			Token:          "token123",
		}

		clone := original.Clone()

		if clone.ID != original.ID {
			t.Error("Clone ID should match")
		}
		if clone.Email != original.Email {
			t.Error("Clone Email should match")
		}

		clone.Email = "modified@example.com"
		if original.Email == "modified@example.com" {
			t.Error("Clone should be independent of original")
		}
	})

	t.Run("GetID", func(t *testing.T) {
		inv := &OrganizationInvitation{ID: ksid.ID(42)}
		if inv.GetID() != ksid.ID(42) {
			t.Errorf("GetID() = %v, want %v", inv.GetID(), ksid.ID(42))
		}
	})
}

func TestOrganizationInvitationService(t *testing.T) {
	service, err := NewOrganizationInvitationService(filepath.Join(t.TempDir(), "org_invitations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	orgID := ksid.ID(100)
	inviterID := ksid.ID(1)
	email := "invitee@example.com"
	role := OrgRoleMember
	var inv, inv2 *OrganizationInvitation

	t.Run("Create", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			var createErr error
			inv, createErr = service.Create(email, orgID, role, inviterID)
			if createErr != nil {
				t.Fatalf("Failed to create invitation: %v", createErr)
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
			if inv.ExpiresAt.IsZero() {
				t.Error("Expected ExpiresAt to be set")
			}
		})

		t.Run("empty email", func(t *testing.T) {
			_, createErr := service.Create("", orgID, role, inviterID)
			if createErr == nil {
				t.Error("Expected error for empty email")
			}
		})

		t.Run("zero orgID", func(t *testing.T) {
			_, createErr := service.Create(email, ksid.ID(0), role, inviterID)
			if createErr == nil {
				t.Error("Expected error for zero orgID")
			}
		})

		t.Run("second invitation", func(t *testing.T) {
			var createErr error
			inv2, createErr = service.Create("second@example.com", orgID, OrgRoleAdmin, inviterID)
			if createErr != nil {
				t.Fatalf("Failed to create second invitation: %v", createErr)
			}
		})
	})

	t.Run("GetByToken", func(t *testing.T) {
		t.Run("existing", func(t *testing.T) {
			found, getErr := service.GetByToken(inv.Token)
			if getErr != nil {
				t.Fatalf("Failed to find invitation by token: %v", getErr)
			}
			if found.ID != inv.ID {
				t.Errorf("Expected ID %v, got %v", inv.ID, found.ID)
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			_, getErr := service.GetByToken("nonexistent-token")
			if getErr == nil {
				t.Error("Expected error for non-existent token")
			}
		})
	})

	t.Run("IterByOrg", func(t *testing.T) {
		t.Run("by org", func(t *testing.T) {
			count := 0
			for range service.IterByOrg(orgID) {
				count++
			}
			if count != 2 {
				t.Errorf("Expected 2 invitations, got %d", count)
			}
		})

		t.Run("different org", func(t *testing.T) {
			count := 0
			for range service.IterByOrg(ksid.ID(999)) {
				count++
			}
			if count != 0 {
				t.Errorf("Expected 0 invitations for different org, got %d", count)
			}
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("zero ID", func(t *testing.T) {
			delErr := service.Delete(ksid.ID(0))
			if delErr == nil {
				t.Error("Expected error for Delete with zero ID")
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			delErr := service.Delete(ksid.ID(99999))
			if delErr == nil {
				t.Error("Expected error for Delete with non-existent ID")
			}
		})

		t.Run("valid", func(t *testing.T) {
			delErr := service.Delete(inv.ID)
			if delErr != nil {
				t.Fatalf("Failed to delete invitation: %v", delErr)
			}

			_, getErr := service.GetByToken(inv.Token)
			if getErr == nil {
				t.Error("Expected invitation to be deleted")
			}

			// Second invitation should still exist
			found2, getErr := service.GetByToken(inv2.Token)
			if getErr != nil {
				t.Fatalf("Second invitation should still exist: %v", getErr)
			}
			if found2.ID != inv2.ID {
				t.Error("Wrong invitation returned")
			}
		})
	})
}

func TestWorkspaceInvitation(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			valid := &WorkspaceInvitation{
				ID:          ksid.ID(1),
				Email:       "test@example.com",
				WorkspaceID: ksid.ID(100),
				Role:        WSRoleEditor,
				Token:       "token123",
			}
			if err := valid.Validate(); err != nil {
				t.Errorf("Expected valid invitation, got error: %v", err)
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			zeroID := &WorkspaceInvitation{
				ID:          ksid.ID(0),
				Email:       "test@example.com",
				WorkspaceID: ksid.ID(100),
				Role:        WSRoleEditor,
				Token:       "token123",
			}
			if err := zeroID.Validate(); err == nil {
				t.Error("Expected error for zero ID")
			}
		})

		t.Run("empty email", func(t *testing.T) {
			emptyEmail := &WorkspaceInvitation{
				ID:          ksid.ID(1),
				Email:       "",
				WorkspaceID: ksid.ID(100),
				Role:        WSRoleEditor,
				Token:       "token123",
			}
			if err := emptyEmail.Validate(); err == nil {
				t.Error("Expected error for empty email")
			}
		})

		t.Run("zero workspace ID", func(t *testing.T) {
			zeroWS := &WorkspaceInvitation{
				ID:          ksid.ID(1),
				Email:       "test@example.com",
				WorkspaceID: ksid.ID(0),
				Role:        WSRoleEditor,
				Token:       "token123",
			}
			if err := zeroWS.Validate(); err == nil {
				t.Error("Expected error for zero workspace ID")
			}
		})

		t.Run("empty token", func(t *testing.T) {
			emptyToken := &WorkspaceInvitation{
				ID:          ksid.ID(1),
				Email:       "test@example.com",
				WorkspaceID: ksid.ID(100),
				Role:        WSRoleEditor,
				Token:       "",
			}
			if err := emptyToken.Validate(); err == nil {
				t.Error("Expected error for empty token")
			}
		})
	})

	t.Run("Clone", func(t *testing.T) {
		original := &WorkspaceInvitation{
			ID:          ksid.ID(1),
			Email:       "test@example.com",
			WorkspaceID: ksid.ID(100),
			Role:        WSRoleAdmin,
			Token:       "token123",
		}

		clone := original.Clone()

		if clone.ID != original.ID {
			t.Error("Clone ID should match")
		}
		if clone.Email != original.Email {
			t.Error("Clone Email should match")
		}

		clone.Email = "modified@example.com"
		if original.Email == "modified@example.com" {
			t.Error("Clone should be independent of original")
		}
	})

	t.Run("GetID", func(t *testing.T) {
		inv := &WorkspaceInvitation{ID: ksid.ID(42)}
		if inv.GetID() != ksid.ID(42) {
			t.Errorf("GetID() = %v, want %v", inv.GetID(), ksid.ID(42))
		}
	})
}

func TestWorkspaceInvitationService(t *testing.T) {
	service, err := NewWorkspaceInvitationService(filepath.Join(t.TempDir(), "ws_invitations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	wsID := ksid.ID(100)
	inviterID := ksid.ID(1)
	email := "invitee@example.com"
	role := WSRoleEditor
	var inv, inv2 *WorkspaceInvitation

	t.Run("Create", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			var createErr error
			inv, createErr = service.Create(email, wsID, role, inviterID)
			if createErr != nil {
				t.Fatalf("Failed to create invitation: %v", createErr)
			}

			if inv.Email != email {
				t.Errorf("Expected email %s, got %s", email, inv.Email)
			}
			if inv.WorkspaceID != wsID {
				t.Errorf("Expected wsID %v, got %v", wsID, inv.WorkspaceID)
			}
			if inv.Role != role {
				t.Errorf("Expected role %s, got %s", role, inv.Role)
			}
			if inv.Token == "" {
				t.Error("Expected token to be generated")
			}
			if inv.ExpiresAt.IsZero() {
				t.Error("Expected ExpiresAt to be set")
			}
		})

		t.Run("empty email", func(t *testing.T) {
			_, createErr := service.Create("", wsID, role, inviterID)
			if createErr == nil {
				t.Error("Expected error for empty email")
			}
		})

		t.Run("zero wsID", func(t *testing.T) {
			_, createErr := service.Create(email, ksid.ID(0), role, inviterID)
			if createErr == nil {
				t.Error("Expected error for zero wsID")
			}
		})

		t.Run("second invitation", func(t *testing.T) {
			var createErr error
			inv2, createErr = service.Create("second@example.com", wsID, WSRoleViewer, inviterID)
			if createErr != nil {
				t.Fatalf("Failed to create second invitation: %v", createErr)
			}
		})
	})

	t.Run("GetByToken", func(t *testing.T) {
		t.Run("existing", func(t *testing.T) {
			found, getErr := service.GetByToken(inv.Token)
			if getErr != nil {
				t.Fatalf("Failed to find invitation by token: %v", getErr)
			}
			if found.ID != inv.ID {
				t.Errorf("Expected ID %v, got %v", inv.ID, found.ID)
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			_, getErr := service.GetByToken("nonexistent-token")
			if getErr == nil {
				t.Error("Expected error for non-existent token")
			}
		})
	})

	t.Run("IterByWorkspace", func(t *testing.T) {
		t.Run("by workspace", func(t *testing.T) {
			count := 0
			for range service.IterByWorkspace(wsID) {
				count++
			}
			if count != 2 {
				t.Errorf("Expected 2 invitations, got %d", count)
			}
		})

		t.Run("different workspace", func(t *testing.T) {
			count := 0
			for range service.IterByWorkspace(ksid.ID(999)) {
				count++
			}
			if count != 0 {
				t.Errorf("Expected 0 invitations for different workspace, got %d", count)
			}
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("zero ID", func(t *testing.T) {
			delErr := service.Delete(ksid.ID(0))
			if delErr == nil {
				t.Error("Expected error for Delete with zero ID")
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			delErr := service.Delete(ksid.ID(99999))
			if delErr == nil {
				t.Error("Expected error for Delete with non-existent ID")
			}
		})

		t.Run("valid", func(t *testing.T) {
			delErr := service.Delete(inv.ID)
			if delErr != nil {
				t.Fatalf("Failed to delete invitation: %v", delErr)
			}

			_, getErr := service.GetByToken(inv.Token)
			if getErr == nil {
				t.Error("Expected invitation to be deleted")
			}

			// Second invitation should still exist
			found2, getErr := service.GetByToken(inv2.Token)
			if getErr != nil {
				t.Fatalf("Second invitation should still exist: %v", getErr)
			}
			if found2.ID != inv2.ID {
				t.Error("Wrong invitation returned")
			}
		})
	})
}
