package identity

import (
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/ksid"
)

func TestOrganizationMembership(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			valid := &OrganizationMembership{
				ID:             ksid.ID(1),
				UserID:         ksid.ID(100),
				OrganizationID: ksid.ID(200),
				Role:           OrgRoleAdmin,
			}
			if err := valid.Validate(); err != nil {
				t.Errorf("Expected valid membership, got error: %v", err)
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			zeroID := &OrganizationMembership{
				ID:             ksid.ID(0),
				UserID:         ksid.ID(100),
				OrganizationID: ksid.ID(200),
				Role:           OrgRoleAdmin,
			}
			if err := zeroID.Validate(); err == nil {
				t.Error("Expected error for zero ID")
			}
		})

		t.Run("zero UserID", func(t *testing.T) {
			zeroUser := &OrganizationMembership{
				ID:             ksid.ID(1),
				UserID:         ksid.ID(0),
				OrganizationID: ksid.ID(200),
				Role:           OrgRoleAdmin,
			}
			if err := zeroUser.Validate(); err == nil {
				t.Error("Expected error for zero UserID")
			}
		})

		t.Run("zero OrganizationID", func(t *testing.T) {
			zeroOrg := &OrganizationMembership{
				ID:             ksid.ID(1),
				UserID:         ksid.ID(100),
				OrganizationID: ksid.ID(0),
				Role:           OrgRoleAdmin,
			}
			if err := zeroOrg.Validate(); err == nil {
				t.Error("Expected error for zero OrganizationID")
			}
		})

		t.Run("invalid role", func(t *testing.T) {
			invalidRole := &OrganizationMembership{
				ID:             ksid.ID(1),
				UserID:         ksid.ID(100),
				OrganizationID: ksid.ID(200),
				Role:           "invalid",
			}
			if err := invalidRole.Validate(); err == nil {
				t.Error("Expected error for invalid role")
			}
		})
	})

	t.Run("Clone", func(t *testing.T) {
		original := &OrganizationMembership{
			ID:             ksid.ID(1),
			UserID:         ksid.ID(100),
			OrganizationID: ksid.ID(200),
			Role:           OrgRoleOwner,
		}

		clone := original.Clone()

		if clone.ID != original.ID {
			t.Error("Clone ID should match")
		}
		if clone.UserID != original.UserID {
			t.Error("Clone UserID should match")
		}

		clone.Role = OrgRoleMember
		if original.Role == OrgRoleMember {
			t.Error("Clone should be independent of original")
		}
	})

	t.Run("GetID", func(t *testing.T) {
		m := &OrganizationMembership{ID: ksid.ID(42)}
		if m.GetID() != ksid.ID(42) {
			t.Errorf("GetID() = %v, want %v", m.GetID(), ksid.ID(42))
		}
	})
}

func TestOrganizationRole(t *testing.T) {
	t.Run("IsValid", func(t *testing.T) {
		if !OrgRoleOwner.IsValid() {
			t.Error("OrgRoleOwner should be valid")
		}
		if !OrgRoleAdmin.IsValid() {
			t.Error("OrgRoleAdmin should be valid")
		}
		if !OrgRoleMember.IsValid() {
			t.Error("OrgRoleMember should be valid")
		}
		if OrganizationRole("invalid").IsValid() {
			t.Error("invalid role should not be valid")
		}
	})

	t.Run("CanManageMembers", func(t *testing.T) {
		if !OrgRoleOwner.CanManageMembers() {
			t.Error("Owner should be able to manage members")
		}
		if !OrgRoleAdmin.CanManageMembers() {
			t.Error("Admin should be able to manage members")
		}
		if OrgRoleMember.CanManageMembers() {
			t.Error("Member should not be able to manage members")
		}
	})

	t.Run("CanManageWorkspaces", func(t *testing.T) {
		if !OrgRoleOwner.CanManageWorkspaces() {
			t.Error("Owner should be able to manage workspaces")
		}
		if !OrgRoleAdmin.CanManageWorkspaces() {
			t.Error("Admin should be able to manage workspaces")
		}
		if OrgRoleMember.CanManageWorkspaces() {
			t.Error("Member should not be able to manage workspaces")
		}
	})

	t.Run("CanManageBilling", func(t *testing.T) {
		if !OrgRoleOwner.CanManageBilling() {
			t.Error("Owner should be able to manage billing")
		}
		if OrgRoleAdmin.CanManageBilling() {
			t.Error("Admin should not be able to manage billing")
		}
		if OrgRoleMember.CanManageBilling() {
			t.Error("Member should not be able to manage billing")
		}
	})
}

func TestWorkspaceMembership(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			valid := &WorkspaceMembership{
				ID:          ksid.ID(1),
				UserID:      ksid.ID(100),
				WorkspaceID: ksid.ID(200),
				Role:        WSRoleEditor,
			}
			if err := valid.Validate(); err != nil {
				t.Errorf("Expected valid membership, got error: %v", err)
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			zeroID := &WorkspaceMembership{
				ID:          ksid.ID(0),
				UserID:      ksid.ID(100),
				WorkspaceID: ksid.ID(200),
				Role:        WSRoleEditor,
			}
			if err := zeroID.Validate(); err == nil {
				t.Error("Expected error for zero ID")
			}
		})

		t.Run("zero UserID", func(t *testing.T) {
			zeroUser := &WorkspaceMembership{
				ID:          ksid.ID(1),
				UserID:      ksid.ID(0),
				WorkspaceID: ksid.ID(200),
				Role:        WSRoleEditor,
			}
			if err := zeroUser.Validate(); err == nil {
				t.Error("Expected error for zero UserID")
			}
		})

		t.Run("zero WorkspaceID", func(t *testing.T) {
			zeroWS := &WorkspaceMembership{
				ID:          ksid.ID(1),
				UserID:      ksid.ID(100),
				WorkspaceID: ksid.ID(0),
				Role:        WSRoleEditor,
			}
			if err := zeroWS.Validate(); err == nil {
				t.Error("Expected error for zero WorkspaceID")
			}
		})

		t.Run("invalid role", func(t *testing.T) {
			invalidRole := &WorkspaceMembership{
				ID:          ksid.ID(1),
				UserID:      ksid.ID(100),
				WorkspaceID: ksid.ID(200),
				Role:        "invalid",
			}
			if err := invalidRole.Validate(); err == nil {
				t.Error("Expected error for invalid role")
			}
		})
	})

	t.Run("Clone", func(t *testing.T) {
		original := &WorkspaceMembership{
			ID:          ksid.ID(1),
			UserID:      ksid.ID(100),
			WorkspaceID: ksid.ID(200),
			Role:        WSRoleAdmin,
		}

		clone := original.Clone()

		if clone.ID != original.ID {
			t.Error("Clone ID should match")
		}
		if clone.UserID != original.UserID {
			t.Error("Clone UserID should match")
		}

		clone.Role = WSRoleViewer
		if original.Role == WSRoleViewer {
			t.Error("Clone should be independent of original")
		}
	})

	t.Run("GetID", func(t *testing.T) {
		m := &WorkspaceMembership{ID: ksid.ID(42)}
		if m.GetID() != ksid.ID(42) {
			t.Errorf("GetID() = %v, want %v", m.GetID(), ksid.ID(42))
		}
	})
}

func TestWorkspaceRole(t *testing.T) {
	t.Run("IsValid", func(t *testing.T) {
		if !WSRoleAdmin.IsValid() {
			t.Error("WSRoleAdmin should be valid")
		}
		if !WSRoleEditor.IsValid() {
			t.Error("WSRoleEditor should be valid")
		}
		if !WSRoleViewer.IsValid() {
			t.Error("WSRoleViewer should be valid")
		}
		if WorkspaceRole("invalid").IsValid() {
			t.Error("invalid role should not be valid")
		}
	})

	t.Run("CanEdit", func(t *testing.T) {
		if !WSRoleAdmin.CanEdit() {
			t.Error("Admin should be able to edit")
		}
		if !WSRoleEditor.CanEdit() {
			t.Error("Editor should be able to edit")
		}
		if WSRoleViewer.CanEdit() {
			t.Error("Viewer should not be able to edit")
		}
	})

	t.Run("CanManageMembers", func(t *testing.T) {
		if !WSRoleAdmin.CanManageMembers() {
			t.Error("Admin should be able to manage members")
		}
		if WSRoleEditor.CanManageMembers() {
			t.Error("Editor should not be able to manage members")
		}
		if WSRoleViewer.CanManageMembers() {
			t.Error("Viewer should not be able to manage members")
		}
	})
}

func TestOrganizationMembershipService(t *testing.T) {
	// Create services needed for membership service
	tempDir := t.TempDir()
	userService, err := NewUserService(filepath.Join(tempDir, "users.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	orgService, err := NewOrganizationService(filepath.Join(tempDir, "orgs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewOrganizationMembershipService(filepath.Join(tempDir, "org_memberships.jsonl"), userService, orgService)
	if err != nil {
		t.Fatal(err)
	}

	// Create test user and org
	user, err := userService.Create("test@example.com", "password", "Test User")
	if err != nil {
		t.Fatal(err)
	}
	org, err := orgService.Create(t.Context(), "Test Org", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	var mem *OrganizationMembership

	t.Run("Create", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			var createErr error
			mem, createErr = service.Create(user.ID, org.ID, OrgRoleOwner)
			if createErr != nil {
				t.Fatalf("Failed to create membership: %v", createErr)
			}

			if mem.UserID != user.ID {
				t.Errorf("Expected UserID %v, got %v", user.ID, mem.UserID)
			}
			if mem.OrganizationID != org.ID {
				t.Errorf("Expected OrganizationID %v, got %v", org.ID, mem.OrganizationID)
			}
			if mem.Role != OrgRoleOwner {
				t.Errorf("Expected role %s, got %s", OrgRoleOwner, mem.Role)
			}
		})

		t.Run("duplicate", func(t *testing.T) {
			_, createErr := service.Create(user.ID, org.ID, OrgRoleAdmin)
			if createErr == nil {
				t.Error("Expected error for duplicate membership")
			}
		})

		t.Run("empty userID", func(t *testing.T) {
			_, createErr := service.Create(ksid.ID(0), org.ID, OrgRoleAdmin)
			if createErr == nil {
				t.Error("Expected error for empty userID")
			}
		})

		t.Run("empty orgID", func(t *testing.T) {
			_, createErr := service.Create(user.ID, ksid.ID(0), OrgRoleAdmin)
			if createErr == nil {
				t.Error("Expected error for empty orgID")
			}
		})
	})

	t.Run("Get", func(t *testing.T) {
		t.Run("existing", func(t *testing.T) {
			found, getErr := service.Get(user.ID, org.ID)
			if getErr != nil {
				t.Fatalf("Failed to get membership: %v", getErr)
			}
			if found.ID != mem.ID {
				t.Errorf("Expected ID %v, got %v", mem.ID, found.ID)
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			_, getErr := service.Get(ksid.ID(99999), org.ID)
			if getErr == nil {
				t.Error("Expected error for non-existent membership")
			}
		})
	})

	t.Run("IterByUser", func(t *testing.T) {
		count := 0
		for range service.IterByUser(user.ID) {
			count++
		}
		if count != 1 {
			t.Errorf("Expected 1 membership, got %d", count)
		}
	})

	t.Run("IterByOrg", func(t *testing.T) {
		count := 0
		for range service.IterByOrg(org.ID) {
			count++
		}
		if count != 1 {
			t.Errorf("Expected 1 membership, got %d", count)
		}
	})

	t.Run("CountUserMemberships", func(t *testing.T) {
		count := service.CountUserMemberships(user.ID)
		if count != 1 {
			t.Errorf("Expected count 1, got %d", count)
		}
	})

	t.Run("CountOrgMemberships", func(t *testing.T) {
		count := service.CountOrgMemberships(org.ID)
		if count != 1 {
			t.Errorf("Expected count 1, got %d", count)
		}
	})

	t.Run("HasOwner", func(t *testing.T) {
		if !service.HasOwner(org.ID) {
			t.Error("Expected org to have an owner")
		}
	})

	t.Run("Modify", func(t *testing.T) {
		modified, modErr := service.Modify(mem.ID, func(m *OrganizationMembership) error {
			m.Role = OrgRoleAdmin
			return nil
		})
		if modErr != nil {
			t.Fatalf("Failed to modify membership: %v", modErr)
		}
		if modified.Role != OrgRoleAdmin {
			t.Errorf("Expected role %s, got %s", OrgRoleAdmin, modified.Role)
		}
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
	})
}

func TestWorkspaceMembershipService(t *testing.T) {
	// Create services needed for membership service
	tempDir := t.TempDir()
	userService, err := NewUserService(filepath.Join(tempDir, "users.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	orgService, err := NewOrganizationService(filepath.Join(tempDir, "orgs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	wsService, err := NewWorkspaceService(filepath.Join(tempDir, "workspaces.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewWorkspaceMembershipService(filepath.Join(tempDir, "ws_memberships.jsonl"), wsService, orgService)
	if err != nil {
		t.Fatal(err)
	}

	// Create test users and orgs
	user, err := userService.Create("test@example.com", "password", "Test User")
	if err != nil {
		t.Fatal(err)
	}
	org1, err := orgService.Create(t.Context(), "Org 1", "org1@example.com")
	if err != nil {
		t.Fatal(err)
	}
	org2, err := orgService.Create(t.Context(), "Org 2", "org2@example.com")
	if err != nil {
		t.Fatal(err)
	}

	// Create workspaces in each org
	ws1, err := wsService.Create(t.Context(), org1.ID, "Workspace 1")
	if err != nil {
		t.Fatal(err)
	}
	ws2, err := wsService.Create(t.Context(), org1.ID, "Workspace 2")
	if err != nil {
		t.Fatal(err)
	}
	ws3, err := wsService.Create(t.Context(), org2.ID, "Workspace 3")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("DeleteByUserInOrg", func(t *testing.T) {
		// Create memberships for user in all workspaces
		mem1, err := service.Create(user.ID, ws1.ID, WSRoleEditor)
		if err != nil {
			t.Fatalf("Failed to create membership 1: %v", err)
		}
		mem2, err := service.Create(user.ID, ws2.ID, WSRoleViewer)
		if err != nil {
			t.Fatalf("Failed to create membership 2: %v", err)
		}
		mem3, err := service.Create(user.ID, ws3.ID, WSRoleAdmin)
		if err != nil {
			t.Fatalf("Failed to create membership 3: %v", err)
		}

		// Verify user has 3 memberships
		count := 0
		for range service.IterByUser(user.ID) {
			count++
		}
		if count != 3 {
			t.Errorf("Expected 3 memberships, got %d", count)
		}

		// Delete memberships for user in org1
		if err := service.DeleteByUserInOrg(user.ID, org1.ID); err != nil {
			t.Fatalf("DeleteByUserInOrg failed: %v", err)
		}

		// Verify memberships in org1 are deleted
		if _, err := service.GetByID(mem1.ID); err == nil {
			t.Error("Expected membership 1 to be deleted")
		}
		if _, err := service.GetByID(mem2.ID); err == nil {
			t.Error("Expected membership 2 to be deleted")
		}

		// Verify membership in org2 still exists
		if _, err := service.GetByID(mem3.ID); err != nil {
			t.Error("Expected membership 3 to still exist")
		}

		// Verify user now has 1 membership
		count = 0
		for range service.IterByUser(user.ID) {
			count++
		}
		if count != 1 {
			t.Errorf("Expected 1 membership remaining, got %d", count)
		}
	})
}
