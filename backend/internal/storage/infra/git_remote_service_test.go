package infra

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func TestGitRemoteService(t *testing.T) {
	tmpDir := t.TempDir()

	dbDir := filepath.Join(tmpDir, "db")
	if err := os.MkdirAll(dbDir, 0o750); err != nil {
		t.Fatal(err)
	}

	s, err := NewGitRemoteService(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	orgID := jsonldb.ID(100)
	name := "origin"
	url := "https://github.com/user/repo.git"
	token := "ghp_test_token" //nolint:gosec // G101: test token, not a real credential

	// Create remote
	remote, err := s.Create(orgID, name, url, "github", "token", token)
	if err != nil {
		t.Fatalf("Failed to create remote: %v", err)
	}

	if remote.Name != name || remote.URL != url {
		t.Errorf("Unexpected remote data: %+v", remote)
	}

	// List remotes
	remotes, err := s.List(orgID)
	if err != nil || len(remotes) != 1 {
		t.Fatalf("Failed to list remotes: %v, len=%d", err, len(remotes))
	}

	// Verify token stored on remote
	if remote.Token != token {
		t.Errorf("Token mismatch: got=%s, want=%s", remote.Token, token)
	}

	// Update sync
	if err := s.UpdateLastSync(remote.ID); err != nil {
		t.Errorf("Failed to update sync: %v", err)
	}

	// Delete
	if err := s.Delete(orgID, remote.ID); err != nil {
		t.Fatalf("Failed to delete remote: %v", err)
	}

	remotes, _ = s.List(orgID)
	if len(remotes) != 0 {
		t.Errorf("Remote still exists after deletion")
	}
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
