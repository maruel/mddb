package identity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func TestOrganization(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			valid := &Organization{
				ID:   jsonldb.ID(1),
				Name: "Test Org",
				Quotas: OrganizationQuota{
					MaxPages:           100,
					MaxStorage:         1024,
					MaxUsers:           10,
					MaxRecordsPerTable: 1000,
					MaxAssetSize:       1024,
				},
			}
			if err := valid.Validate(); err != nil {
				t.Errorf("Expected valid organization, got error: %v", err)
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			zeroID := &Organization{
				ID:   jsonldb.ID(0),
				Name: "Test Org",
				Quotas: OrganizationQuota{
					MaxPages:           100,
					MaxStorage:         1024,
					MaxUsers:           10,
					MaxRecordsPerTable: 1000,
					MaxAssetSize:       1024,
				},
			}
			if err := zeroID.Validate(); err == nil {
				t.Error("Expected error for zero ID")
			}
		})

		t.Run("empty name", func(t *testing.T) {
			emptyName := &Organization{
				ID:   jsonldb.ID(1),
				Name: "",
				Quotas: OrganizationQuota{
					MaxPages:           100,
					MaxStorage:         1024,
					MaxUsers:           10,
					MaxRecordsPerTable: 1000,
					MaxAssetSize:       1024,
				},
			}
			if err := emptyName.Validate(); err == nil {
				t.Error("Expected error for empty name")
			}
		})
		t.Run("invalid quota", func(t *testing.T) {
			invalidQuota := &Organization{
				ID:   jsonldb.ID(1),
				Name: "Test Org",
				Quotas: OrganizationQuota{
					MaxPages:           0,
					MaxStorage:         1024,
					MaxUsers:           10,
					MaxRecordsPerTable: 1000,
					MaxAssetSize:       1024,
				},
			}
			if err := invalidQuota.Validate(); err == nil {
				t.Error("Expected error for invalid quota")
			}
		})
	})

	t.Run("Clone", func(t *testing.T) {
		t.Run("with AllowedDomains", func(t *testing.T) {
			original := &Organization{
				ID:   jsonldb.ID(1),
				Name: "Test Org",
				Settings: OrganizationSettings{
					AllowedDomains: []string{"example.com", "test.com"},
					PublicAccess:   true,
				},
			}

			clone := original.Clone()

			if clone.ID != original.ID {
				t.Error("Clone ID should match")
			}
			if clone.Name != original.Name {
				t.Error("Clone Name should match")
			}

			clone.Settings.AllowedDomains[0] = "modified.com"
			if original.Settings.AllowedDomains[0] == "modified.com" {
				t.Error("Clone AllowedDomains should be independent of original")
			}
		})

		t.Run("nil AllowedDomains", func(t *testing.T) {
			noAllowed := &Organization{
				ID:   jsonldb.ID(1),
				Name: "No Domains",
			}
			cloneNoAllowed := noAllowed.Clone()
			if cloneNoAllowed.Settings.AllowedDomains != nil {
				t.Error("Clone of nil AllowedDomains should be nil")
			}
		})
	})

	t.Run("GetID", func(t *testing.T) {
		org := &Organization{ID: jsonldb.ID(42)}
		if org.GetID() != jsonldb.ID(42) {
			t.Errorf("GetID() = %v, want %v", org.GetID(), jsonldb.ID(42))
		}
	})
}

func TestGitRemote(t *testing.T) {
	t.Run("IsZero", func(t *testing.T) {
		t.Run("empty URL", func(t *testing.T) {
			empty := &GitRemote{}
			if !empty.IsZero() {
				t.Error("Expected IsZero() = true for empty GitRemote")
			}
		})

		t.Run("configured URL", func(t *testing.T) {
			configured := &GitRemote{URL: "https://github.com/example/repo.git"}
			if configured.IsZero() {
				t.Error("Expected IsZero() = false for configured GitRemote")
			}
		})
	})
}

func TestOrganizationService(t *testing.T) {
	tempDir := t.TempDir()

	service, err := NewOrganizationService(filepath.Join(tempDir, "organizations.jsonl"))
	if err != nil {
		t.Fatalf("NewOrganizationService failed: %v", err)
	}

	var org, org2 *Organization

	t.Run("Create", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			var createErr error
			org, createErr = service.Create(t.Context(), "Test Organization")
			if createErr != nil {
				t.Fatalf("Create failed: %v", createErr)
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
				t.Fatalf("Create (second) failed: %v", createErr)
			}
		})
	})

	t.Run("Get", func(t *testing.T) {
		t.Run("existing", func(t *testing.T) {
			retrieved, getErr := service.Get(org.ID)
			if getErr != nil {
				t.Fatalf("Get failed: %v", getErr)
			}
			if retrieved.Name != "Test Organization" {
				t.Errorf("Retrieved Name = %q, want %q", retrieved.Name, "Test Organization")
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			_, getErr := service.Get(jsonldb.ID(99999))
			if getErr == nil {
				t.Error("Expected error for non-existent organization")
			}
		})
	})

	t.Run("Modify", func(t *testing.T) {
		t.Run("settings", func(t *testing.T) {
			newSettings := OrganizationSettings{
				AllowedDomains: []string{"example.com"},
				PublicAccess:   true,
			}
			_, modErr := service.Modify(org.ID, func(o *Organization) error {
				o.Settings = newSettings
				return nil
			})
			if modErr != nil {
				t.Fatalf("Modify failed: %v", modErr)
			}

			updatedOrg, _ := service.Get(org.ID)
			if !updatedOrg.Settings.PublicAccess {
				t.Error("Expected Settings.PublicAccess = true after update")
			}
			if len(updatedOrg.Settings.AllowedDomains) != 1 {
				t.Errorf("AllowedDomains length = %d, want 1", len(updatedOrg.Settings.AllowedDomains))
			}
		})

		t.Run("onboarding", func(t *testing.T) {
			newState := OnboardingState{
				Completed: true,
				Step:      "done",
			}
			_, modErr := service.Modify(org2.ID, func(o *Organization) error {
				o.Onboarding = newState
				return nil
			})
			if modErr != nil {
				t.Fatalf("Modify failed: %v", modErr)
			}

			updatedOrg2, _ := service.Get(org2.ID)
			if !updatedOrg2.Onboarding.Completed {
				t.Error("Expected Onboarding.Completed = true after update")
			}
			if updatedOrg2.Onboarding.Step != "done" {
				t.Errorf("Onboarding.Step = %q, want %q", updatedOrg2.Onboarding.Step, "done")
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			_, modErr := service.Modify(jsonldb.ID(99999), func(o *Organization) error {
				return nil
			})
			if modErr == nil {
				t.Error("Expected error when modifying non-existent org")
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			_, modErr := service.Modify(jsonldb.ID(0), func(o *Organization) error {
				return nil
			})
			if modErr == nil {
				t.Error("Expected error when modifying with zero ID")
			}
		})
	})

	t.Run("Persistence", func(t *testing.T) {
		persistDir := t.TempDir()
		tablePath := filepath.Join(persistDir, "organizations.jsonl")

		svc1, svcErr := NewOrganizationService(tablePath)
		if svcErr != nil {
			t.Fatal(svcErr)
		}

		persistOrg, createErr := svc1.Create(t.Context(), "Persistent Org")
		if createErr != nil {
			t.Fatal(createErr)
		}

		orgID := persistOrg.ID

		// Create new service instance (simulating restart)
		svc2, svc2Err := NewOrganizationService(tablePath)
		if svc2Err != nil {
			t.Fatal(svc2Err)
		}

		retrieved, getErr := svc2.Get(orgID)
		if getErr != nil {
			t.Fatalf("Failed to retrieve persisted organization: %v", getErr)
		}
		if retrieved.Name != "Persistent Org" {
			t.Errorf("Persisted Name = %q, want %q", retrieved.Name, "Persistent Org")
		}
	})

	t.Run("InvalidJSONL", func(t *testing.T) {
		t.Run("malformed JSON", func(t *testing.T) {
			tempDir := t.TempDir()
			jsonlPath := filepath.Join(tempDir, "invalid_organizations.jsonl")

			// Write invalid JSON to the file (malformed JSON)
			err := os.WriteFile(jsonlPath, []byte(`{"version":"1.0","columns":[]}
{"id":1,"name":"Valid Org","created":"2023-01-01T00:00:00Z"}
{"id":2,"name":"Invalid JSON Org","created":"2023-01-01T00:00:00Z"
`), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			_, err = NewOrganizationService(jsonlPath)
			if err == nil {
				t.Error("Expected error when loading invalid JSONL file")
			}
		})

		t.Run("malformed row with empty name", func(t *testing.T) {
			tempDir := t.TempDir()
			jsonlPath := filepath.Join(tempDir, "malformed_organizations.jsonl")

			// Write JSON with malformed row (missing required fields)
			err := os.WriteFile(jsonlPath, []byte(`{"version":"1.0","columns":[]}
{"id":1,"name":"","created":"2023-01-01T00:00:00Z"}
`), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			_, err = NewOrganizationService(jsonlPath)
			if err == nil {
				t.Error("Expected error when loading JSONL with invalid row (empty name)")
			}
		})

		t.Run("row with zero ID", func(t *testing.T) {
			tempDir := t.TempDir()
			jsonlPath := filepath.Join(tempDir, "zero_id_organizations.jsonl")

			// Write JSON with zero ID
			err := os.WriteFile(jsonlPath, []byte(`{"version":"1.0","columns":[]}
{"id":0,"name":"Zero ID Org","created":"2023-01-01T00:00:00Z"}
`), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			_, err = NewOrganizationService(jsonlPath)
			if err == nil {
				t.Error("Expected error when loading JSONL with zero ID")
			}
		})
	})
}
