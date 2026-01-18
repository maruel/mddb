package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitRemoteService(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mddb-remote-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dbDir := filepath.Join(tmpDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}

	s, err := NewGitRemoteService(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	orgID := "org1"
	name := "origin"
	url := "https://github.com/user/repo.git"
	token := "ghp_test_token"

	// Create remote
	remote, err := s.CreateRemote(orgID, name, url, "github", "token", token)
	if err != nil {
		t.Fatalf("Failed to create remote: %v", err)
	}

	if remote.Name != name || remote.URL != url {
		t.Errorf("Unexpected remote data: %+v", remote)
	}

	// List remotes
	remotes, err := s.ListRemotes(orgID)
	if err != nil || len(remotes) != 1 {
		t.Fatalf("Failed to list remotes: %v, len=%d", err, len(remotes))
	}

	// Get token
	savedToken, err := s.GetToken(remote.ID)
	if err != nil || savedToken != token {
		t.Errorf("Failed to get token: %v, got=%s", err, savedToken)
	}

	// Update sync
	if err := s.UpdateLastSync(remote.ID); err != nil {
		t.Errorf("Failed to update sync: %v", err)
	}

	// Delete
	if err := s.DeleteRemote(orgID, remote.ID); err != nil {
		t.Fatalf("Failed to delete remote: %v", err)
	}

	remotes, _ = s.ListRemotes(orgID)
	if len(remotes) != 0 {
		t.Errorf("Remote still exists after deletion")
	}
}
