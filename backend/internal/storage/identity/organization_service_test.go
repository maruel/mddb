package identity

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
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

	var org, org2 *entity.Organization

	t.Run("CreateOrganization", func(t *testing.T) {
		t.Run("valid name", func(t *testing.T) {
			var createErr error
			org, createErr = service.CreateOrganization(t.Context(), "Test Organization")
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
			_, createErr := service.CreateOrganization(t.Context(), "")
			if createErr == nil {
				t.Error("Expected error when creating organization with empty name")
			}
		})

		t.Run("second organization", func(t *testing.T) {
			var createErr error
			org2, createErr = service.CreateOrganization(t.Context(), "Second Org")
			if createErr != nil {
				t.Fatalf("CreateOrganization (second) failed: %v", createErr)
			}
		})
	})

	t.Run("GetOrganization", func(t *testing.T) {
		t.Run("existing ID", func(t *testing.T) {
			retrieved, getErr := service.GetOrganization(org.ID)
			if getErr != nil {
				t.Fatalf("GetOrganization failed: %v", getErr)
			}
			if retrieved.Name != "Test Organization" {
				t.Errorf("Retrieved Name = %q, want %q", retrieved.Name, "Test Organization")
			}
		})

		t.Run("non-existent ID", func(t *testing.T) {
			_, getErr := service.GetOrganization(jsonldb.ID(99999))
			if getErr == nil {
				t.Error("Expected error for non-existent organization")
			}
		})
	})

	t.Run("UpdateSettings", func(t *testing.T) {
		newSettings := entity.OrganizationSettings{
			AllowedDomains: []string{"example.com"},
			PublicAccess:   true,
		}

		t.Run("existing organization", func(t *testing.T) {
			updateErr := service.UpdateSettings(org.ID, newSettings)
			if updateErr != nil {
				t.Fatalf("UpdateSettings failed: %v", updateErr)
			}

			updatedOrg, _ := service.GetOrganization(org.ID)
			if !updatedOrg.Settings.PublicAccess {
				t.Error("Expected Settings.PublicAccess = true after update")
			}
			if len(updatedOrg.Settings.AllowedDomains) != 1 {
				t.Errorf("AllowedDomains length = %d, want 1", len(updatedOrg.Settings.AllowedDomains))
			}
		})

		t.Run("non-existent organization", func(t *testing.T) {
			updateErr := service.UpdateSettings(jsonldb.ID(99999), newSettings)
			if updateErr == nil {
				t.Error("Expected error when updating settings for non-existent org")
			}
		})
	})

	t.Run("UpdateOnboarding", func(t *testing.T) {
		newState := entity.OnboardingState{
			Completed: true,
			Step:      "done",
		}

		t.Run("existing organization", func(t *testing.T) {
			updateErr := service.UpdateOnboarding(org2.ID, newState)
			if updateErr != nil {
				t.Fatalf("UpdateOnboarding failed: %v", updateErr)
			}

			updatedOrg2, _ := service.GetOrganization(org2.ID)
			if !updatedOrg2.Onboarding.Completed {
				t.Error("Expected Onboarding.Completed = true after update")
			}
			if updatedOrg2.Onboarding.Step != "done" {
				t.Errorf("Onboarding.Step = %q, want %q", updatedOrg2.Onboarding.Step, "done")
			}
			if updatedOrg2.Onboarding.UpdatedAt.IsZero() {
				t.Error("Expected Onboarding.UpdatedAt to be set")
			}
		})

		t.Run("non-existent organization", func(t *testing.T) {
			updateErr := service.UpdateOnboarding(jsonldb.ID(99999), newState)
			if updateErr == nil {
				t.Error("Expected error when updating onboarding for non-existent org")
			}
		})
	})

	t.Run("RootDir", func(t *testing.T) {
		if service.RootDir() != tempDir {
			t.Errorf("RootDir() = %q, want %q", service.RootDir(), tempDir)
		}
	})

	t.Run("Persistence", func(t *testing.T) {
		persistDir := t.TempDir()

		// Create service and add organization
		fs1, _ := infra.NewFileStore(persistDir)
		svc1, svcErr := NewOrganizationService(persistDir, fs1, nil)
		if svcErr != nil {
			t.Fatal(svcErr)
		}

		persistOrg, createErr := svc1.CreateOrganization(t.Context(), "Persistent Org")
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
		retrieved, getErr := svc2.GetOrganization(orgID)
		if getErr != nil {
			t.Fatalf("Failed to retrieve persisted organization: %v", getErr)
		}
		if retrieved.Name != "Persistent Org" {
			t.Errorf("Persisted Name = %q, want %q", retrieved.Name, "Persistent Org")
		}
	})
}
