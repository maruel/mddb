package entity

import (
	"context"
	"testing"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func TestGetOrgID(t *testing.T) {
	t.Run("context with org ID", func(t *testing.T) {
		if got := GetOrgID(context.WithValue(t.Context(), OrgKey, jsonldb.ID(123))); got != jsonldb.ID(123) {
			t.Errorf("GetOrgID() = %v, want %v", got, jsonldb.ID(123))
		}
	})
	t.Run("context without org ID", func(t *testing.T) {
		if got := GetOrgID(t.Context()); got != jsonldb.ID(0) {
			t.Errorf("GetOrgID() = %v, want %v", got, jsonldb.ID(0))
		}
	})
	t.Run("context with wrong type value", func(t *testing.T) {
		if got := GetOrgID(context.WithValue(t.Context(), OrgKey, "not an ID")); got != jsonldb.ID(0) {
			t.Errorf("GetOrgID() = %v, want %v", got, jsonldb.ID(0))
		}
	})
}

func TestDataRecord(t *testing.T) {
	t.Run("Clone", func(t *testing.T) {
		t.Run("copies values", func(t *testing.T) {
			original := &DataRecord{
				ID:       jsonldb.ID(1),
				Data:     map[string]any{"name": "test", "count": 42},
				Created:  time.Now(),
				Modified: time.Now(),
			}
			clone := original.Clone()
			if clone.ID != original.ID {
				t.Errorf("Clone ID = %v, want %v", clone.ID, original.ID)
			}
			if clone.Data["name"] != original.Data["name"] {
				t.Error("Clone Data not properly copied")
			}
			clone.Data["name"] = "modified"
			if original.Data["name"] == "modified" {
				t.Error("Clone Data should not share reference with original")
			}
		})
		t.Run("nil data", func(t *testing.T) {
			original := &DataRecord{ID: jsonldb.ID(1), Data: nil}
			if original.Clone().Data != nil {
				t.Error("Clone of nil Data should be nil")
			}
		})
	})
	t.Run("GetID", func(t *testing.T) {
		if got := (&DataRecord{ID: jsonldb.ID(42)}).GetID(); got != jsonldb.ID(42) {
			t.Errorf("GetID() = %v, want %v", got, jsonldb.ID(42))
		}
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			if err := (&DataRecord{ID: jsonldb.ID(1)}).Validate(); err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
		t.Run("zero ID", func(t *testing.T) {
			if err := (&DataRecord{ID: jsonldb.ID(0)}).Validate(); err == nil {
				t.Error("Validate() expected error for zero ID")
			}
		})
	})
}

func TestMembership(t *testing.T) {
	t.Run("Clone", func(t *testing.T) {
		original := &Membership{
			ID:             jsonldb.ID(1),
			UserID:         jsonldb.ID(2),
			OrganizationID: jsonldb.ID(3),
			Role:           UserRoleAdmin,
			Settings:       MembershipSettings{Notifications: true},
			Created:        time.Now(),
		}
		clone := original.Clone()
		if clone.ID != original.ID {
			t.Errorf("Clone ID = %v, want %v", clone.ID, original.ID)
		}
		if clone.Role != original.Role {
			t.Errorf("Clone Role = %v, want %v", clone.Role, original.Role)
		}
	})
	t.Run("GetID", func(t *testing.T) {
		if got := (&Membership{ID: jsonldb.ID(99)}).GetID(); got != jsonldb.ID(99) {
			t.Errorf("GetID() = %v, want %v", got, jsonldb.ID(99))
		}
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			m := &Membership{
				ID:             jsonldb.ID(1),
				UserID:         jsonldb.ID(2),
				OrganizationID: jsonldb.ID(3),
				Role:           UserRoleAdmin,
			}
			if err := m.Validate(); err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
		t.Run("zero ID", func(t *testing.T) {
			m := &Membership{
				ID:             jsonldb.ID(0),
				UserID:         jsonldb.ID(2),
				OrganizationID: jsonldb.ID(3),
				Role:           UserRoleAdmin,
			}
			if err := m.Validate(); err == nil {
				t.Error("Validate() expected error for zero ID")
			} else if err.Error() != "id is required" {
				t.Errorf("Validate() error = %q, want %q", err.Error(), "id is required")
			}
		})
		t.Run("zero UserID", func(t *testing.T) {
			m := &Membership{
				ID:             jsonldb.ID(1),
				UserID:         jsonldb.ID(0),
				OrganizationID: jsonldb.ID(3),
				Role:           UserRoleAdmin,
			}
			if err := m.Validate(); err == nil {
				t.Error("Validate() expected error for zero UserID")
			} else if err.Error() != "user_id is required" {
				t.Errorf("Validate() error = %q, want %q", err.Error(), "user_id is required")
			}
		})
		t.Run("zero OrganizationID", func(t *testing.T) {
			m := &Membership{
				ID:             jsonldb.ID(1),
				UserID:         jsonldb.ID(2),
				OrganizationID: jsonldb.ID(0),
				Role:           UserRoleAdmin,
			}
			if err := m.Validate(); err == nil {
				t.Error("Validate() expected error for zero OrganizationID")
			} else if err.Error() != "organization_id is required" {
				t.Errorf("Validate() error = %q, want %q", err.Error(), "organization_id is required")
			}
		})
		t.Run("empty Role", func(t *testing.T) {
			m := &Membership{
				ID:             jsonldb.ID(1),
				UserID:         jsonldb.ID(2),
				OrganizationID: jsonldb.ID(3),
				Role:           "",
			}
			if err := m.Validate(); err == nil {
				t.Error("Validate() expected error for empty Role")
			} else if err.Error() != "role is required" {
				t.Errorf("Validate() error = %q, want %q", err.Error(), "role is required")
			}
		})
	})
}

func TestOrganization(t *testing.T) {
	t.Run("Clone", func(t *testing.T) {
		t.Run("copies values", func(t *testing.T) {
			original := &Organization{
				ID:   jsonldb.ID(1),
				Name: "Test Org",
				Settings: OrganizationSettings{
					AllowedDomains: []string{"example.com", "test.com"},
					PublicAccess:   true,
				},
				Created: time.Now(),
			}
			clone := original.Clone()
			if clone.ID != original.ID {
				t.Errorf("Clone ID = %v, want %v", clone.ID, original.ID)
			}
			if clone.Name != original.Name {
				t.Errorf("Clone Name = %v, want %v", clone.Name, original.Name)
			}
			clone.Settings.AllowedDomains[0] = "modified.com"
			if original.Settings.AllowedDomains[0] == "modified.com" {
				t.Error("Clone should not share AllowedDomains reference with original")
			}
		})
		t.Run("nil AllowedDomains", func(t *testing.T) {
			original := &Organization{
				ID:       jsonldb.ID(1),
				Name:     "Test Org",
				Settings: OrganizationSettings{AllowedDomains: nil},
			}
			if original.Clone().Settings.AllowedDomains != nil {
				t.Error("Clone of nil AllowedDomains should be nil")
			}
		})
	})
	t.Run("GetID", func(t *testing.T) {
		if got := (&Organization{ID: jsonldb.ID(77)}).GetID(); got != jsonldb.ID(77) {
			t.Errorf("GetID() = %v, want %v", got, jsonldb.ID(77))
		}
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			if err := (&Organization{ID: jsonldb.ID(1), Name: "Test Org"}).Validate(); err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
		t.Run("zero ID", func(t *testing.T) {
			if err := (&Organization{ID: jsonldb.ID(0), Name: "Test Org"}).Validate(); err == nil {
				t.Error("Validate() expected error for zero ID")
			}
		})
		t.Run("empty Name", func(t *testing.T) {
			if err := (&Organization{ID: jsonldb.ID(1), Name: ""}).Validate(); err == nil {
				t.Error("Validate() expected error for empty Name")
			}
		})
	})
}

func TestGitRemote(t *testing.T) {
	t.Run("Clone", func(t *testing.T) {
		original := &GitRemote{
			ID:             jsonldb.ID(1),
			OrganizationID: jsonldb.ID(2),
			Name:           "origin",
			URL:            "https://github.com/test/repo",
			Type:           "github",
			AuthType:       "token",
			Created:        time.Now(),
		}
		clone := original.Clone()
		if clone.ID != original.ID {
			t.Errorf("Clone ID = %v, want %v", clone.ID, original.ID)
		}
		if clone.URL != original.URL {
			t.Errorf("Clone URL = %v, want %v", clone.URL, original.URL)
		}
	})
	t.Run("GetID", func(t *testing.T) {
		if got := (&GitRemote{ID: jsonldb.ID(55)}).GetID(); got != jsonldb.ID(55) {
			t.Errorf("GetID() = %v, want %v", got, jsonldb.ID(55))
		}
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			r := &GitRemote{
				ID:             jsonldb.ID(1),
				OrganizationID: jsonldb.ID(2),
				URL:            "https://github.com/test/repo",
			}
			if err := r.Validate(); err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
		t.Run("zero ID", func(t *testing.T) {
			r := &GitRemote{
				ID:             jsonldb.ID(0),
				OrganizationID: jsonldb.ID(2),
				URL:            "https://github.com/test/repo",
			}
			if err := r.Validate(); err == nil {
				t.Error("Validate() expected error for zero ID")
			}
		})
		t.Run("zero OrganizationID", func(t *testing.T) {
			r := &GitRemote{
				ID:             jsonldb.ID(1),
				OrganizationID: jsonldb.ID(0),
				URL:            "https://github.com/test/repo",
			}
			if err := r.Validate(); err == nil {
				t.Error("Validate() expected error for zero OrganizationID")
			}
		})
		t.Run("empty URL", func(t *testing.T) {
			r := &GitRemote{
				ID:             jsonldb.ID(1),
				OrganizationID: jsonldb.ID(2),
				URL:            "",
			}
			if err := r.Validate(); err == nil {
				t.Error("Validate() expected error for empty URL")
			}
		})
	})
}

func TestInvitation(t *testing.T) {
	t.Run("Clone", func(t *testing.T) {
		original := &Invitation{
			ID:             jsonldb.ID(1),
			Email:          "test@example.com",
			OrganizationID: jsonldb.ID(2),
			Role:           UserRoleEditor,
			Token:          "abc123",
			ExpiresAt:      time.Now().Add(24 * time.Hour),
			Created:        time.Now(),
		}
		clone := original.Clone()
		if clone.ID != original.ID {
			t.Errorf("Clone ID = %v, want %v", clone.ID, original.ID)
		}
		if clone.Email != original.Email {
			t.Errorf("Clone Email = %v, want %v", clone.Email, original.Email)
		}
	})
	t.Run("GetID", func(t *testing.T) {
		if got := (&Invitation{ID: jsonldb.ID(33)}).GetID(); got != jsonldb.ID(33) {
			t.Errorf("GetID() = %v, want %v", got, jsonldb.ID(33))
		}
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			inv := &Invitation{
				ID:             jsonldb.ID(1),
				Email:          "test@example.com",
				OrganizationID: jsonldb.ID(2),
				Token:          "abc123",
			}
			if err := inv.Validate(); err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
		t.Run("zero ID", func(t *testing.T) {
			inv := &Invitation{
				ID:             jsonldb.ID(0),
				Email:          "test@example.com",
				OrganizationID: jsonldb.ID(2),
				Token:          "abc123",
			}
			if err := inv.Validate(); err == nil {
				t.Error("Validate() expected error for zero ID")
			}
		})
		t.Run("empty Email", func(t *testing.T) {
			inv := &Invitation{
				ID:             jsonldb.ID(1),
				Email:          "",
				OrganizationID: jsonldb.ID(2),
				Token:          "abc123",
			}
			if err := inv.Validate(); err == nil {
				t.Error("Validate() expected error for empty Email")
			}
		})
		t.Run("zero OrganizationID", func(t *testing.T) {
			inv := &Invitation{
				ID:             jsonldb.ID(1),
				Email:          "test@example.com",
				OrganizationID: jsonldb.ID(0),
				Token:          "abc123",
			}
			if err := inv.Validate(); err == nil {
				t.Error("Validate() expected error for zero OrganizationID")
			}
		})
		t.Run("empty Token", func(t *testing.T) {
			inv := &Invitation{
				ID:             jsonldb.ID(1),
				Email:          "test@example.com",
				OrganizationID: jsonldb.ID(2),
				Token:          "",
			}
			if err := inv.Validate(); err == nil {
				t.Error("Validate() expected error for empty Token")
			}
		})
	})
}

func TestUser(t *testing.T) {
	t.Run("GetID", func(t *testing.T) {
		if got := (&User{ID: jsonldb.ID(88)}).GetID(); got != jsonldb.ID(88) {
			t.Errorf("GetID() = %v, want %v", got, jsonldb.ID(88))
		}
	})
}
