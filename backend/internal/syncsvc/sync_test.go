// Tests for sync service: acquire/release locking, getAuthURL, and push/pull error paths.

package syncsvc

import (
	"path/filepath"
	"testing"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func newTestService(t *testing.T) (*Service, *identity.WorkspaceService) {
	tmpDir := t.TempDir()
	wsSvc, err := identity.NewWorkspaceService(filepath.Join(tmpDir, "workspaces.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	svc := New(wsSvc, nil, nil, nil)
	return svc, wsSvc
}

func TestService(t *testing.T) {
	t.Run("tryAcquire", func(t *testing.T) {
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
	})

	t.Run("Push", func(t *testing.T) {
		t.Run("no remote", func(t *testing.T) {
			svc, wsSvc := newTestService(t)
			ctx := t.Context()

			orgID := ksid.NewID()
			ws, err := wsSvc.Create(ctx, orgID, "test-ws")
			if err != nil {
				t.Fatal(err)
			}

			err = svc.Push(ctx, ws.ID)
			if err == nil || err.Error() != "no remote configured" {
				t.Fatalf("expected 'no remote configured', got %v", err)
			}
		})

		t.Run("not found", func(t *testing.T) {
			svc, _ := newTestService(t)
			ctx := t.Context()

			err := svc.Push(ctx, ksid.NewID())
			if err == nil {
				t.Fatal("expected error for non-existent workspace")
			}
		})
	})

	t.Run("Pull", func(t *testing.T) {
		t.Run("no remote", func(t *testing.T) {
			svc, wsSvc := newTestService(t)
			ctx := t.Context()

			orgID := ksid.NewID()
			ws, err := wsSvc.Create(ctx, orgID, "test-ws")
			if err != nil {
				t.Fatal(err)
			}

			err = svc.Pull(ctx, ws.ID)
			if err == nil || err.Error() != "no remote configured" {
				t.Fatalf("expected 'no remote configured', got %v", err)
			}
		})
	})

	t.Run("getAuthURL", func(t *testing.T) {
		t.Run("token", func(t *testing.T) {
			svc, _ := newTestService(t)
			ctx := t.Context()

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
		})

		t.Run("no auth", func(t *testing.T) {
			svc, _ := newTestService(t)
			ctx := t.Context()

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
		})

		t.Run("github app nil client", func(t *testing.T) {
			svc, _ := newTestService(t)
			ctx := t.Context()

			ws := &identity.Workspace{
				GitRemote: identity.GitRemote{
					URL:            "https://github.com/org/repo.git",
					AuthType:       "github_app",
					InstallationID: 42,
					RepoOwner:      "org",
					RepoName:       "repo",
				},
			}

			url, err := svc.getAuthURL(ctx, ws)
			if err != nil {
				t.Fatal(err)
			}
			if url != "https://github.com/org/repo.git" {
				t.Fatalf("expected raw URL fallback, got: %s", url)
			}
		})
	})

	t.Run("setSyncStatus", func(t *testing.T) {
		svc, wsSvc := newTestService(t)
		ctx := t.Context()

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
	})

	t.Run("updateLastSync", func(t *testing.T) {
		svc, wsSvc := newTestService(t)
		ctx := t.Context()

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
	})
}
