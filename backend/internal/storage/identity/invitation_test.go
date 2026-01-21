package identity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func TestInvitation(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			valid := &Invitation{
				ID:             jsonldb.ID(1),
				Email:          "test@example.com",
				OrganizationID: jsonldb.ID(100),
				Token:          "token123",
			}
			if err := valid.Validate(); err != nil {
				t.Errorf("Expected valid invitation, got error: %v", err)
			}
		})

		t.Run("zero ID", func(t *testing.T) {
			zeroID := &Invitation{
				ID:             jsonldb.ID(0),
				Email:          "test@example.com",
				OrganizationID: jsonldb.ID(100),
				Token:          "token123",
			}
			if err := zeroID.Validate(); err == nil {
				t.Error("Expected error for zero ID")
			}
		})

		t.Run("empty email", func(t *testing.T) {
			emptyEmail := &Invitation{
				ID:             jsonldb.ID(1),
				Email:          "",
				OrganizationID: jsonldb.ID(100),
				Token:          "token123",
			}
			if err := emptyEmail.Validate(); err == nil {
				t.Error("Expected error for empty email")
			}
		})

		t.Run("zero org ID", func(t *testing.T) {
			zeroOrg := &Invitation{
				ID:             jsonldb.ID(1),
				Email:          "test@example.com",
				OrganizationID: jsonldb.ID(0),
				Token:          "token123",
			}
			if err := zeroOrg.Validate(); err == nil {
				t.Error("Expected error for zero org ID")
			}
		})

		t.Run("empty token", func(t *testing.T) {
			emptyToken := &Invitation{
				ID:             jsonldb.ID(1),
				Email:          "test@example.com",
				OrganizationID: jsonldb.ID(100),
				Token:          "",
			}
			if err := emptyToken.Validate(); err == nil {
				t.Error("Expected error for empty token")
			}
		})
	})

	t.Run("Clone", func(t *testing.T) {
		original := &Invitation{
			ID:             jsonldb.ID(1),
			Email:          "test@example.com",
			OrganizationID: jsonldb.ID(100),
			Token:          "token123",
			Role:           UserRoleEditor,
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
		inv := &Invitation{ID: jsonldb.ID(42)}
		if inv.GetID() != jsonldb.ID(42) {
			t.Errorf("GetID() = %v, want %v", inv.GetID(), jsonldb.ID(42))
		}
	})
}

func TestInvitationService(t *testing.T) {
	service, err := NewInvitationService(filepath.Join(t.TempDir(), "invitations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	orgID := jsonldb.ID(100)
	email := "invitee@example.com"
	role := UserRoleEditor
	var inv, inv2 *Invitation

	t.Run("Create", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			var createErr error
			inv, createErr = service.Create(email, orgID, role)
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
			_, createErr := service.Create("", orgID, role)
			if createErr == nil {
				t.Error("Expected error for empty email")
			}
		})

		t.Run("zero orgID", func(t *testing.T) {
			_, createErr := service.Create(email, jsonldb.ID(0), role)
			if createErr == nil {
				t.Error("Expected error for zero orgID")
			}
		})

		t.Run("second invitation", func(t *testing.T) {
			var createErr error
			inv2, createErr = service.Create("second@example.com", orgID, UserRoleViewer)
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

	t.Run("Iter", func(t *testing.T) {
		t.Run("by org", func(t *testing.T) {
			iter, iterErr := service.Iter(orgID)
			if iterErr != nil {
				t.Fatalf("Failed to iterate invitations: %v", iterErr)
			}
			count := 0
			for range iter {
				count++
			}
			if count != 2 {
				t.Errorf("Expected 2 invitations, got %d", count)
			}
		})

		t.Run("zero orgID", func(t *testing.T) {
			_, iterErr := service.Iter(jsonldb.ID(0))
			if iterErr == nil {
				t.Error("Expected error for zero orgID in Iter")
			}
		})

		t.Run("different org", func(t *testing.T) {
			iter, _ := service.Iter(jsonldb.ID(999))
			count := 0
			for range iter {
				count++
			}
			if count != 0 {
				t.Errorf("Expected 0 invitations for different org, got %d", count)
			}
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("zero ID", func(t *testing.T) {
			delErr := service.Delete(jsonldb.ID(0))
			if delErr == nil {
				t.Error("Expected error for Delete with zero ID")
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			delErr := service.Delete(jsonldb.ID(99999))
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

func TestInvalidJSONLInvitationFiles(t *testing.T) {
	// Test with invalid JSONL content for invitations
	t.Run("InvalidJSONLInvitation", func(t *testing.T) {
		tempDir := t.TempDir()
		jsonlPath := filepath.Join(tempDir, "invalid_invitations.jsonl")

		// Write invalid JSON to the file (malformed JSON)
		err := os.WriteFile(jsonlPath, []byte(`{"version":"1.0","columns":[]}
{"id":1,"email":"test@example.com","org_id":100,"token":"token123"}
{"id":2,"email":"test2@example.com","org_id":101,"token":"token456"
`), 0o644)
		if err != nil {
			t.Fatal(err)
		}

		_, err = NewInvitationService(jsonlPath)
		if err == nil {
			t.Error("Expected error when loading invalid JSONL file")
		}
	})

	t.Run("InvalidJSONLInvitationWithMalformedRow", func(t *testing.T) {
		tempDir := t.TempDir()
		jsonlPath := filepath.Join(tempDir, "malformed_invitations.jsonl")

		// Write JSON with malformed row (missing required fields)
		err := os.WriteFile(jsonlPath, []byte(`{"version":"1.0","columns":[]}
{"id":1,"email":"","org_id":100,"token":"token123"}
`), 0o644)
		if err != nil {
			t.Fatal(err)
		}

		_, err = NewInvitationService(jsonlPath)
		if err == nil {
			t.Error("Expected error when loading JSONL with invalid row (empty email)")
		}
	})

	t.Run("InvalidJSONLInvitationWithZeroID", func(t *testing.T) {
		tempDir := t.TempDir()
		jsonlPath := filepath.Join(tempDir, "zero_id_invitations.jsonl")

		// Write JSON with zero ID
		err := os.WriteFile(jsonlPath, []byte(`{"version":"1.0","columns":[]}
{"id":0,"email":"test@example.com","org_id":100,"token":"token123"}
`), 0o644)
		if err != nil {
			t.Fatal(err)
		}

		_, err = NewInvitationService(jsonlPath)
		if err == nil {
			t.Error("Expected error when loading JSONL with zero ID")
		}
	})
}
