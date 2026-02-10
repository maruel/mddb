package identity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/rid"
)

func TestOrganization(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			valid := &Organization{
				ID:   rid.ID(1),
				Name: "Test Org",
				Quotas: OrganizationQuotas{
					MaxWorkspacesPerOrg:    3,
					MaxMembersPerOrg:       10,
					MaxMembersPerWorkspace: 10,
					MaxTotalStorageBytes:   5 * 1024 * 1024 * 1024,
				},
			}
			if err := valid.Validate(); err != nil {
				t.Errorf("Expected valid organization, got error: %v", err)
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			zeroID := &Organization{
				ID:   rid.ID(0),
				Name: "Test Org",
				Quotas: OrganizationQuotas{
					MaxWorkspacesPerOrg:    3,
					MaxMembersPerOrg:       10,
					MaxMembersPerWorkspace: 10,
					MaxTotalStorageBytes:   5 * 1024 * 1024 * 1024,
				},
			}
			if err := zeroID.Validate(); err == nil {
				t.Error("Expected error for zero ID")
			}
		})

		t.Run("empty name", func(t *testing.T) {
			emptyName := &Organization{
				ID:   rid.ID(1),
				Name: "",
				Quotas: OrganizationQuotas{
					MaxWorkspacesPerOrg:    3,
					MaxMembersPerOrg:       10,
					MaxMembersPerWorkspace: 10,
					MaxTotalStorageBytes:   5 * 1024 * 1024 * 1024,
				},
			}
			if err := emptyName.Validate(); err == nil {
				t.Error("Expected error for empty name")
			}
		})
		t.Run("invalid quota", func(t *testing.T) {
			invalidQuota := &Organization{
				ID:   rid.ID(1),
				Name: "Test Org",
				Quotas: OrganizationQuotas{
					MaxWorkspacesPerOrg:    0,
					MaxMembersPerOrg:       10,
					MaxMembersPerWorkspace: 10,
					MaxTotalStorageBytes:   5 * 1024 * 1024 * 1024,
				},
			}
			if err := invalidQuota.Validate(); err == nil {
				t.Error("Expected error for invalid quota")
			}
		})
	})

	t.Run("Clone", func(t *testing.T) {
		original := &Organization{
			ID:   rid.ID(1),
			Name: "Test Org",
		}

		clone := original.Clone()

		if clone.ID != original.ID {
			t.Error("Clone ID should match")
		}
		if clone.Name != original.Name {
			t.Error("Clone Name should match")
		}
	})

	t.Run("GetID", func(t *testing.T) {
		org := &Organization{ID: rid.ID(42)}
		if org.GetID() != rid.ID(42) {
			t.Errorf("GetID() = %v, want %v", org.GetID(), rid.ID(42))
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

	var org *Organization

	t.Run("Create", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			var createErr error
			org, createErr = service.Create(t.Context(), "Test Organization", "billing@example.com")
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
		})

		t.Run("empty name", func(t *testing.T) {
			_, createErr := service.Create(t.Context(), "", "billing@example.com")
			if createErr == nil {
				t.Error("Expected error when creating organization with empty name")
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
			_, getErr := service.Get(rid.ID(99999))
			if getErr == nil {
				t.Error("Expected error for non-existent organization")
			}
		})
	})

	t.Run("Modify", func(t *testing.T) {
		t.Run("name", func(t *testing.T) {
			_, modErr := service.Modify(org.ID, func(o *Organization) error {
				o.Name = "Updated Org Name"
				return nil
			})
			if modErr != nil {
				t.Fatalf("Modify failed: %v", modErr)
			}

			updatedOrg, _ := service.Get(org.ID)
			if updatedOrg.Name != "Updated Org Name" {
				t.Errorf("Expected Name = 'Updated Org Name', got '%s'", updatedOrg.Name)
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			_, modErr := service.Modify(rid.ID(99999), func(o *Organization) error {
				return nil
			})
			if modErr == nil {
				t.Error("Expected error when modifying non-existent org")
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			_, modErr := service.Modify(rid.ID(0), func(o *Organization) error {
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

		persistOrg, createErr := svc1.Create(t.Context(), "Persistent Org", "billing@example.com")
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
