package identity

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

func TestOrganizationService(t *testing.T) {
	tempDir := t.TempDir()

	fileStore, err := infra.NewFileStore(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	service, err := NewOrganizationService(tempDir, fileStore, nil)
	if err != nil {
		t.Fatalf("NewOrganizationService failed: %v", err)
	}

	var org, org2 *Organization

	t.Run("CreateOrganization", func(t *testing.T) {
		t.Run("valid name", func(t *testing.T) {
			var createErr error
			org, createErr = service.Create(t.Context(), "Test Organization")
			if createErr != nil {
				t.Fatalf("CreateOrganization failed: %v", createErr)
			}

			if org.Name != "Test Organization" {
				t.Errorf("Name = %q, want %q", org.Name, "Test Organization")
			}
			if org.ID.IsZero() {
				t.Error("Expected non-zero ID")
			}
			if org.Created.IsZero() {
				t.Error("Expected non-zero Created time")
			}
			if org.Onboarding.Completed {
				t.Error("Expected Onboarding.Completed = false for new org")
			}
			if org.Onboarding.Step != "name" {
				t.Errorf("Onboarding.Step = %q, want %q", org.Onboarding.Step, "name")
			}
		})

		t.Run("empty name", func(t *testing.T) {
			_, createErr := service.Create(t.Context(), "")
			if createErr == nil {
				t.Error("Expected error when creating organization with empty name")
			}
		})

		t.Run("second organization", func(t *testing.T) {
			var createErr error
			org2, createErr = service.Create(t.Context(), "Second Org")
			if createErr != nil {
				t.Fatalf("CreateOrganization (second) failed: %v", createErr)
			}
		})
	})

	t.Run("GetOrganization", func(t *testing.T) {
		t.Run("existing ID", func(t *testing.T) {
			retrieved, getErr := service.Get(org.ID)
			if getErr != nil {
				t.Fatalf("GetOrganization failed: %v", getErr)
			}
			if retrieved.Name != "Test Organization" {
				t.Errorf("Retrieved Name = %q, want %q", retrieved.Name, "Test Organization")
			}
		})

		t.Run("non-existent ID", func(t *testing.T) {
			_, getErr := service.Get(jsonldb.ID(99999))
			if getErr == nil {
				t.Error("Expected error for non-existent organization")
			}
		})
	})

	t.Run("ModifySettings", func(t *testing.T) {
		newSettings := OrganizationSettings{
			AllowedDomains: []string{"example.com"},
			PublicAccess:   true,
		}

		t.Run("existing organization", func(t *testing.T) {
			_, modifyErr := service.Modify(org.ID, func(o *Organization) error {
				o.Settings = newSettings
				return nil
			})
			if modifyErr != nil {
				t.Fatalf("Modify failed: %v", modifyErr)
			}

			updatedOrg, _ := service.Get(org.ID)
			if !updatedOrg.Settings.PublicAccess {
				t.Error("Expected Settings.PublicAccess = true after update")
			}
			if len(updatedOrg.Settings.AllowedDomains) != 1 {
				t.Errorf("AllowedDomains length = %d, want 1", len(updatedOrg.Settings.AllowedDomains))
			}
		})

		t.Run("non-existent organization", func(t *testing.T) {
			_, modifyErr := service.Modify(jsonldb.ID(99999), func(o *Organization) error {
				o.Settings = newSettings
				return nil
			})
			if modifyErr == nil {
				t.Error("Expected error when modifying non-existent org")
			}
		})
	})

	t.Run("ModifyOnboarding", func(t *testing.T) {
		newState := OnboardingState{
			Completed: true,
			Step:      "done",
		}

		t.Run("existing organization", func(t *testing.T) {
			_, modifyErr := service.Modify(org2.ID, func(o *Organization) error {
				o.Onboarding = newState
				return nil
			})
			if modifyErr != nil {
				t.Fatalf("Modify failed: %v", modifyErr)
			}

			updatedOrg2, _ := service.Get(org2.ID)
			if !updatedOrg2.Onboarding.Completed {
				t.Error("Expected Onboarding.Completed = true after update")
			}
			if updatedOrg2.Onboarding.Step != "done" {
				t.Errorf("Onboarding.Step = %q, want %q", updatedOrg2.Onboarding.Step, "done")
			}
		})

		t.Run("non-existent organization", func(t *testing.T) {
			_, modifyErr := service.Modify(jsonldb.ID(99999), func(o *Organization) error {
				o.Onboarding = newState
				return nil
			})
			if modifyErr == nil {
				t.Error("Expected error when modifying non-existent org")
			}
		})
	})

	t.Run("Persistence", func(t *testing.T) {
		persistDir := t.TempDir()

		// Create service and add organization
		fs1, _ := infra.NewFileStore(persistDir)
		svc1, svcErr := NewOrganizationService(persistDir, fs1, nil)
		if svcErr != nil {
			t.Fatal(svcErr)
		}

		persistOrg, createErr := svc1.Create(t.Context(), "Persistent Org")
		if createErr != nil {
			t.Fatal(createErr)
		}

		orgID := persistOrg.ID

		// Create new service instance (simulating restart)
		fs2, _ := infra.NewFileStore(persistDir)
		svc2, svc2Err := NewOrganizationService(persistDir, fs2, nil)
		if svc2Err != nil {
			t.Fatal(svc2Err)
		}

		// Verify organization persisted
		retrieved, getErr := svc2.Get(orgID)
		if getErr != nil {
			t.Fatalf("Failed to retrieve persisted organization: %v", getErr)
		}
		if retrieved.Name != "Persistent Org" {
			t.Errorf("Persisted Name = %q, want %q", retrieved.Name, "Persistent Org")
		}
	})
}
