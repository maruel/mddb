// Tests for sync service: acquire/release locking, getAuthURL, and push/pull error paths.

package syncsvc

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/ksid"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func newTestService(t *testing.T) (*Service, *identity.WorkspaceService) {
	t.Helper()
	tmpDir := t.TempDir()
	wsSvc, err := identity.NewWorkspaceService(filepath.Join(tmpDir, "workspaces.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	svc := New(wsSvc, nil, nil, nil)
	return svc, wsSvc
}

func TestTryAcquireRelease(t *testing.T) {
	svc, _ := newTestService(t)
	id := ksid.NewID()

	if !svc.tryAcquire(id) {
		t.Fatal("expected to acquire")
	}
	if svc.tryAcquire(id) {
		t.Fatal("expected second acquire to fail")
	}

	svc.release(id)

	if !svc.tryAcquire(id) {
		t.Fatal("expected re-acquire after release")
	}
	svc.release(id)
}

func TestPush_NoRemote(t *testing.T) {
	svc, wsSvc := newTestService(t)
	ctx := context.Background()

	orgID := ksid.NewID()
	ws, err := wsSvc.Create(ctx, orgID, "test-ws")
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Push(ctx, ws.ID)
	if err == nil || err.Error() != "no remote configured" {
		t.Fatalf("expected 'no remote configured', got %v", err)
	}
}

func TestPull_NoRemote(t *testing.T) {
	svc, wsSvc := newTestService(t)
	ctx := context.Background()

	orgID := ksid.NewID()
	ws, err := wsSvc.Create(ctx, orgID, "test-ws")
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Pull(ctx, ws.ID)
	if err == nil || err.Error() != "no remote configured" {
		t.Fatalf("expected 'no remote configured', got %v", err)
	}
}

func TestPush_NotFound(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Push(ctx, ksid.NewID())
	if err == nil {
		t.Fatal("expected error for non-existent workspace")
	}
}

func TestGetAuthURL_Token(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	ws := &identity.Workspace{
		GitRemote: identity.GitRemote{
			URL:      "https://github.com/org/repo.git",
			Type:     "github",
			AuthType: "token",
			Token:    "ghp_testtoken",
		},
	}

	url, err := svc.getAuthURL(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://x-access-token:ghp_testtoken@github.com/org/repo.git" {
		t.Fatalf("unexpected URL: %s", url)
	}
}

func TestGetAuthURL_NoAuth(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	ws := &identity.Workspace{
		GitRemote: identity.GitRemote{
			URL:  "https://github.com/org/repo.git",
			Type: "github",
		},
	}

	url, err := svc.getAuthURL(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/org/repo.git" {
		t.Fatalf("expected raw URL, got: %s", url)
	}
}

func TestGetAuthURL_GitHubApp_NilClient(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	ws := &identity.Workspace{
		GitRemote: identity.GitRemote{
			URL:            "https://github.com/org/repo.git",
			AuthType:       "github_app",
			InstallationID: 42,
			RepoOwner:      "org",
			RepoName:       "repo",
		},
	}

	// With nil githubApp client, should fall through to raw URL.
	url, err := svc.getAuthURL(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/org/repo.git" {
		t.Fatalf("expected raw URL fallback, got: %s", url)
	}
}

func TestSetSyncStatus(t *testing.T) {
	svc, wsSvc := newTestService(t)
	ctx := context.Background()

	orgID := ksid.NewID()
	ws, err := wsSvc.Create(ctx, orgID, "test-ws")
	if err != nil {
		t.Fatal(err)
	}

	svc.setSyncStatus(ws.ID, "syncing", "")

	ws, err = wsSvc.Get(ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ws.GitRemote.SyncStatus != "syncing" {
		t.Fatalf("expected syncing, got %s", ws.GitRemote.SyncStatus)
	}

	svc.setSyncStatus(ws.ID, "error", "something broke")

	ws, err = wsSvc.Get(ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ws.GitRemote.SyncStatus != "error" {
		t.Fatalf("expected error, got %s", ws.GitRemote.SyncStatus)
	}
	if ws.GitRemote.LastSyncError != "something broke" {
		t.Fatalf("expected 'something broke', got %s", ws.GitRemote.LastSyncError)
	}
}

func TestUpdateLastSync(t *testing.T) {
	svc, wsSvc := newTestService(t)
	ctx := context.Background()

	orgID := ksid.NewID()
	ws, err := wsSvc.Create(ctx, orgID, "test-ws")
	if err != nil {
		t.Fatal(err)
	}

	// Set error status first.
	svc.setSyncStatus(ws.ID, "error", "old error")

	// Update should clear error and set idle.
	svc.updateLastSync(ws.ID)

	ws, err = wsSvc.Get(ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ws.GitRemote.SyncStatus != "idle" {
		t.Fatalf("expected idle, got %s", ws.GitRemote.SyncStatus)
	}
	if ws.GitRemote.LastSyncError != "" {
		t.Fatalf("expected empty error, got %s", ws.GitRemote.LastSyncError)
	}
	if ws.GitRemote.LastSync.IsZero() {
		t.Fatal("expected LastSync to be set")
	}
}
