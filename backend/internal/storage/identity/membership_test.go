package identity

import (
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func TestMembership(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			valid := &Membership{
				ID:             jsonldb.ID(1),
				UserID:         jsonldb.ID(100),
				OrganizationID: jsonldb.ID(200),
				Role:           UserRoleAdmin,
			}
			if err := valid.Validate(); err != nil {
				t.Errorf("Expected valid membership, got error: %v", err)
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			zeroID := &Membership{
				ID:             jsonldb.ID(0),
				UserID:         jsonldb.ID(100),
				OrganizationID: jsonldb.ID(200),
				Role:           UserRoleAdmin,
			}
			if err := zeroID.Validate(); err == nil {
				t.Error("Expected error for zero ID")
			}
		})

		t.Run("zero UserID", func(t *testing.T) {
			zeroUser := &Membership{
				ID:             jsonldb.ID(1),
				UserID:         jsonldb.ID(0),
				OrganizationID: jsonldb.ID(200),
				Role:           UserRoleAdmin,
			}
			if err := zeroUser.Validate(); err == nil {
				t.Error("Expected error for zero UserID")
			}
		})

		t.Run("zero OrganizationID", func(t *testing.T) {
			zeroOrg := &Membership{
				ID:             jsonldb.ID(1),
				UserID:         jsonldb.ID(100),
				OrganizationID: jsonldb.ID(0),
				Role:           UserRoleAdmin,
			}
			if err := zeroOrg.Validate(); err == nil {
				t.Error("Expected error for zero OrganizationID")
			}
		})

		t.Run("empty Role", func(t *testing.T) {
			emptyRole := &Membership{
				ID:             jsonldb.ID(1),
				UserID:         jsonldb.ID(100),
				OrganizationID: jsonldb.ID(200),
				Role:           "",
			}
			if err := emptyRole.Validate(); err == nil {
				t.Error("Expected error for empty Role")
			}
		})
	})

	t.Run("Clone", func(t *testing.T) {
		original := &Membership{
			ID:             jsonldb.ID(1),
			UserID:         jsonldb.ID(100),
			OrganizationID: jsonldb.ID(200),
			Role:           UserRoleAdmin,
			Settings:       MembershipSettings{Notifications: true},
		}

		clone := original.Clone()

		if clone.ID != original.ID {
			t.Error("Clone ID should match")
		}

		clone.Role = UserRoleViewer
		if original.Role == UserRoleViewer {
			t.Error("Clone should be independent of original")
		}
	})

	t.Run("GetID", func(t *testing.T) {
		m := &Membership{ID: jsonldb.ID(42)}
		if m.GetID() != jsonldb.ID(42) {
			t.Errorf("GetID() = %v, want %v", m.GetID(), jsonldb.ID(42))
		}
	})
}

func TestMembershipService(t *testing.T) {
	service, err := NewMembershipService(filepath.Join(t.TempDir(), "memberships.jsonl"))
	if err != nil {
		t.Fatalf("NewMembershipService failed: %v", err)
	}

	userID := jsonldb.ID(100)
	orgID := jsonldb.ID(200)
	var membership *Membership

	t.Run("Create", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			var createErr error
			membership, createErr = service.Create(userID, orgID, UserRoleAdmin)
			if createErr != nil {
				t.Fatalf("Create failed: %v", createErr)
			}

			if membership.UserID != userID {
				t.Errorf("UserID = %v, want %v", membership.UserID, userID)
			}
			if membership.OrganizationID != orgID {
				t.Errorf("OrganizationID = %v, want %v", membership.OrganizationID, orgID)
			}
			if membership.Role != UserRoleAdmin {
				t.Errorf("Role = %v, want %v", membership.Role, UserRoleAdmin)
			}
			if membership.ID.IsZero() {
				t.Error("Expected non-zero membership ID")
			}
		})

		t.Run("duplicate", func(t *testing.T) {
			_, createErr := service.Create(userID, orgID, UserRoleEditor)
			if createErr == nil {
				t.Error("Expected error when creating duplicate membership")
			}
		})

		t.Run("zero user ID", func(t *testing.T) {
			_, createErr := service.Create(jsonldb.ID(0), orgID, UserRoleAdmin)
			if createErr == nil {
				t.Error("Expected error for invalid user ID")
			}
		})

		t.Run("zero org ID", func(t *testing.T) {
			_, createErr := service.Create(userID, jsonldb.ID(0), UserRoleAdmin)
			if createErr == nil {
				t.Error("Expected error for invalid org ID")
			}
		})
	})

	t.Run("Get", func(t *testing.T) {
		t.Run("existing", func(t *testing.T) {
			retrieved, getErr := service.Get(userID, orgID)
			if getErr != nil {
				t.Fatalf("Get failed: %v", getErr)
			}
			if retrieved.Role != UserRoleAdmin {
				t.Errorf("Role = %v, want %v", retrieved.Role, UserRoleAdmin)
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			_, getErr := service.Get(jsonldb.ID(999), orgID)
			if getErr == nil {
				t.Error("Expected error for non-existent membership")
			}
		})
	})

	t.Run("Iter", func(t *testing.T) {
		// Create second user in same org
		userID2 := jsonldb.ID(101)
		if _, err := service.Create(userID2, orgID, UserRoleViewer); err != nil {
			t.Fatalf("Create membership for userID2 failed: %v", err)
		}

		// Create same user in different org
		orgID2 := jsonldb.ID(201)
		if _, err := service.Create(userID, orgID2, UserRoleEditor); err != nil {
			t.Fatalf("Create membership for orgID2 failed: %v", err)
		}

		t.Run("by user", func(t *testing.T) {
			iter, iterErr := service.Iter(userID)
			if iterErr != nil {
				t.Fatalf("Iter failed: %v", iterErr)
			}
			count := 0
			for range iter {
				count++
			}
			if count != 2 {
				t.Errorf("Expected 2 memberships for user, got %d", count)
			}
		})

		t.Run("zero user ID", func(t *testing.T) {
			_, iterErr := service.Iter(jsonldb.ID(0))
			if iterErr == nil {
				t.Error("Expected error for invalid user ID in Iter")
			}
		})
	})

	t.Run("Modify", func(t *testing.T) {
		t.Run("role change", func(t *testing.T) {
			toUpdate, _ := service.Get(userID, orgID)
			_, modErr := service.Modify(toUpdate.ID, func(m *Membership) error {
				m.Role = UserRoleEditor
				return nil
			})
			if modErr != nil {
				t.Fatalf("Modify failed: %v", modErr)
			}

			updated, _ := service.Get(userID, orgID)
			if updated.Role != UserRoleEditor {
				t.Errorf("Role after update = %v, want %v", updated.Role, UserRoleEditor)
			}
		})

		t.Run("settings change", func(t *testing.T) {
			toUpdate, _ := service.Get(userID, orgID)
			_, modErr := service.Modify(toUpdate.ID, func(m *Membership) error {
				m.Settings = MembershipSettings{Notifications: true}
				return nil
			})
			if modErr != nil {
				t.Fatalf("Modify failed: %v", modErr)
			}

			updatedSettings, _ := service.Get(userID, orgID)
			if !updatedSettings.Settings.Notifications {
				t.Error("Settings.Notifications = false, want true")
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			_, modErr := service.Modify(jsonldb.ID(999), func(m *Membership) error {
				return nil
			})
			if modErr == nil {
				t.Error("Expected error when modifying non-existent membership")
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			_, modErr := service.Modify(jsonldb.ID(0), func(m *Membership) error {
				return nil
			})
			if modErr == nil {
				t.Error("Expected error when modifying with zero ID")
			}
		})
	})

	t.Run("Persistence", func(t *testing.T) {
		tablePath := filepath.Join(t.TempDir(), "memberships.jsonl")
		persistUserID := jsonldb.ID(100)
		persistOrgID := jsonldb.ID(200)

		service1, svcErr := NewMembershipService(tablePath)
		if svcErr != nil {
			t.Fatal(svcErr)
		}

		_, createErr := service1.Create(persistUserID, persistOrgID, UserRoleAdmin)
		if createErr != nil {
			t.Fatal(createErr)
		}

		// Create new service instance (simulating restart)
		service2, svcErr := NewMembershipService(tablePath)
		if svcErr != nil {
			t.Fatal(svcErr)
		}

		retrieved, getErr := service2.Get(persistUserID, persistOrgID)
		if getErr != nil {
			t.Fatalf("Failed to retrieve persisted membership: %v", getErr)
		}
		if retrieved.Role != UserRoleAdmin {
			t.Errorf("Persisted Role = %v, want %v", retrieved.Role, UserRoleAdmin)
		}
	})
}
