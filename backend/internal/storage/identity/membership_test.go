package identity

import (
	"os"
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
	tempDir := t.TempDir()
	userService, err := NewUserService(filepath.Join(tempDir, "users.jsonl"))
	if err != nil {
		t.Fatalf("NewUserService failed: %v", err)
	}

	orgService, err := NewOrganizationService(filepath.Join(tempDir, "organizations.jsonl"))
	if err != nil {
		t.Fatalf("NewOrganizationService failed: %v", err)
	}

	service, err := NewMembershipService(filepath.Join(tempDir, "memberships.jsonl"), userService, orgService)
	if err != nil {
		t.Fatalf("NewMembershipService failed: %v", err)
	}

	// Create a user for testing
	user, err := userService.Create("test@example.com", "password", "Test User")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	userID := user.ID

	// Create an organization for testing
	org, err := orgService.Create(t.Context(), "Test Org")
	if err != nil {
		t.Fatalf("Failed to create org: %v", err)
	}
	orgID := org.ID

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

	t.Run("UserOrgQuota", func(t *testing.T) {
		tempDir := t.TempDir()
		quotaUserService, err := NewUserService(filepath.Join(tempDir, "users.jsonl"))
		if err != nil {
			t.Fatalf("NewUserService failed: %v", err)
		}

		quotaOrgService, err := NewOrganizationService(filepath.Join(tempDir, "organizations.jsonl"))
		if err != nil {
			t.Fatalf("NewOrganizationService failed: %v", err)
		}

		quotaService, err := NewMembershipService(filepath.Join(tempDir, "quota_memberships.jsonl"), quotaUserService, quotaOrgService)
		if err != nil {
			t.Fatalf("NewMembershipService failed: %v", err)
		}

		quotaUser, err := quotaUserService.Create("quota@example.com", "password", "Quota User")
		if err != nil {
			t.Fatalf("Failed to create quota user: %v", err)
		}
		quotaUserID := quotaUser.ID
		maxOrgs := quotaUser.Quotas.MaxOrgs

		// Create memberships up to the limit
		for i := range maxOrgs {
			// Create dummy orgs for testing
			quotaOrg, err := quotaOrgService.Create(t.Context(), "Quota Org")
			if err != nil {
				t.Fatalf("Failed to create quota org: %v", err)
			}
			quotaOrgID := quotaOrg.ID
			_, createErr := quotaService.Create(quotaUserID, quotaOrgID, UserRoleViewer)
			if createErr != nil {
				t.Fatalf("Create membership %d failed: %v", i+1, createErr)
			}
		}

		// Verify count
		count := quotaService.CountUserMemberships(quotaUserID)
		if count != maxOrgs {
			t.Errorf("CountUserMemberships = %d, want %d", count, maxOrgs)
		}

		// Try to create one more - should fail
		failOrg, err := quotaOrgService.Create(t.Context(), "Fail Org")
		if err != nil {
			t.Fatalf("Failed to create fail org: %v", err)
		}
		_, createErr := quotaService.Create(quotaUserID, failOrg.ID, UserRoleViewer)
		if createErr == nil {
			t.Error("Expected error when exceeding user org quota")
		}

		// Different user should still be able to create memberships
		otherUser, err := quotaUserService.Create("other@example.com", "password", "Other User")
		if err != nil {
			t.Fatalf("Failed to create other user: %v", err)
		}
		otherUserID := otherUser.ID
		// Create a new org for other user to avoid org user limit (which is 3 by default)
		otherOrg, err := quotaOrgService.Create(t.Context(), "Other Org")
		if err != nil {
			t.Fatalf("Failed to create other org: %v", err)
		}
		_, createErr = quotaService.Create(otherUserID, otherOrg.ID, UserRoleViewer)
		if createErr != nil {
			t.Errorf("Different user should be able to create membership: %v", createErr)
		}
	})

	t.Run("OrgUserQuota", func(t *testing.T) {
		tempDir := t.TempDir()
		userService, err := NewUserService(filepath.Join(tempDir, "users.jsonl"))
		if err != nil {
			t.Fatalf("NewUserService failed: %v", err)
		}
		orgService, err := NewOrganizationService(filepath.Join(tempDir, "organizations.jsonl"))
		if err != nil {
			t.Fatalf("NewOrganizationService failed: %v", err)
		}
		service, err := NewMembershipService(filepath.Join(tempDir, "memberships.jsonl"), userService, orgService)
		if err != nil {
			t.Fatalf("NewMembershipService failed: %v", err)
		}

		// Create org with small quota
		org, err := orgService.Create(t.Context(), "Small Org")
		if err != nil {
			t.Fatalf("Failed to create org: %v", err)
		}
		// Manually update quota to 1 for testing
		_, err = orgService.Modify(org.ID, func(o *Organization) error {
			o.Quotas.MaxUsers = 1
			return nil
		})
		if err != nil {
			t.Fatalf("Failed to modify org quota: %v", err)
		}

		// Create first user
		user1, err := userService.Create("user1@example.com", "password", "User 1")
		if err != nil {
			t.Fatalf("Failed to create user 1: %v", err)
		}
		// Add first user to org
		_, err = service.Create(user1.ID, org.ID, UserRoleAdmin)
		if err != nil {
			t.Fatalf("Failed to add user 1: %v", err)
		}

		// Create second user
		user2, err := userService.Create("user2@example.com", "password", "User 2")
		if err != nil {
			t.Fatalf("Failed to create user 2: %v", err)
		}
		// Try to add second user to org - should fail
		_, err = service.Create(user2.ID, org.ID, UserRoleViewer)
		if err == nil {
			t.Error("Expected error when exceeding org user quota")
		}
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
		user2, err := userService.Create("test2@example.com", "password", "Test User 2")
		if err != nil {
			t.Fatalf("Failed to create user 2: %v", err)
		}
		userID2 := user2.ID
		if _, err := service.Create(userID2, orgID, UserRoleViewer); err != nil {
			t.Fatalf("Create membership for userID2 failed: %v", err)
		}

		// Create same user in different org
		org2, err := orgService.Create(t.Context(), "Test Org 2")
		if err != nil {
			t.Fatalf("Failed to create org 2: %v", err)
		}
		orgID2 := org2.ID
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
		persistDir := t.TempDir()
		tablePath := filepath.Join(persistDir, "memberships.jsonl")
		userPath := filepath.Join(persistDir, "users.jsonl")

		userService1, err := NewUserService(userPath)
		if err != nil {
			t.Fatal(err)
		}
		orgService1, err := NewOrganizationService(filepath.Join(persistDir, "organizations.jsonl"))
		if err != nil {
			t.Fatal(err)
		}

		user, err := userService1.Create("persist@example.com", "password", "Persist User")
		if err != nil {
			t.Fatal(err)
		}
		persistUserID := user.ID

		org, err := orgService1.Create(t.Context(), "Persist Org")
		if err != nil {
			t.Fatal(err)
		}
		persistOrgID := org.ID

		service1, svcErr := NewMembershipService(tablePath, userService1, orgService1)
		if svcErr != nil {
			t.Fatal(svcErr)
		}

		_, createErr := service1.Create(persistUserID, persistOrgID, UserRoleAdmin)
		if createErr != nil {
			t.Fatal(createErr)
		}

		// Create new service instance (simulating restart)
		userService2, err := NewUserService(userPath)
		if err != nil {
			t.Fatal(err)
		}
		orgService2, err := NewOrganizationService(filepath.Join(persistDir, "organizations.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		service2, svcErr := NewMembershipService(tablePath, userService2, orgService2)
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

	t.Run("InvalidJSONL", func(t *testing.T) {
		t.Run("malformed JSON", func(t *testing.T) {
			tempDir := t.TempDir()
			jsonlPath := filepath.Join(tempDir, "invalid_memberships.jsonl")
			userService, _ := NewUserService(filepath.Join(tempDir, "users.jsonl"))
			orgService, _ := NewOrganizationService(filepath.Join(tempDir, "organizations.jsonl"))

			// Write invalid JSON to the file (malformed JSON)
			err := os.WriteFile(jsonlPath, []byte(`{"version":"1.0","columns":[]}
{"id":1,"user_id":100,"org_id":200,"role":"admin"}
{"id":2,"user_id":101,"org_id":201,"role":"viewer"
`), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			_, err = NewMembershipService(jsonlPath, userService, orgService)
			if err == nil {
				t.Error("Expected error when loading invalid JSONL file")
			}
		})

		t.Run("malformed row with empty role", func(t *testing.T) {
			tempDir := t.TempDir()
			jsonlPath := filepath.Join(tempDir, "malformed_memberships.jsonl")
			userService, _ := NewUserService(filepath.Join(tempDir, "users.jsonl"))
			orgService, _ := NewOrganizationService(filepath.Join(tempDir, "organizations.jsonl"))

			// Write JSON with malformed row (missing required fields)
			err := os.WriteFile(jsonlPath, []byte(`{"version":"1.0","columns":[]}
{"id":1,"user_id":100,"org_id":200,"role":""}
`), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			_, err = NewMembershipService(jsonlPath, userService, orgService)
			if err == nil {
				t.Error("Expected error when loading JSONL with invalid row (empty role)")
			}
		})

		t.Run("row with zero ID", func(t *testing.T) {
			tempDir := t.TempDir()
			jsonlPath := filepath.Join(tempDir, "zero_id_memberships.jsonl")
			userService, _ := NewUserService(filepath.Join(tempDir, "users.jsonl"))
			orgService, _ := NewOrganizationService(filepath.Join(tempDir, "organizations.jsonl"))

			// Write JSON with zero ID
			err := os.WriteFile(jsonlPath, []byte(`{"version":"1.0","columns":[]}
{"id":0,"user_id":100,"org_id":200,"role":"admin"}
`), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			_, err = NewMembershipService(jsonlPath, userService, orgService)
			if err == nil {
				t.Error("Expected error when loading JSONL with zero ID")
			}
		})
	})
}
