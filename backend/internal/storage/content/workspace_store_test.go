// Tests and benchmarks workspace storage, canonical snapshots, and concurrent conditional writes.

package content

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
	"unicode/utf8"

	contentv1 "github.com/maruel/mddb/backend/internal/storage/content/data/v1"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestMutateTableConcurrentViews(t *testing.T) {
	t.Parallel()
	_, ws, _ := initWS(t)
	author := git.Author{Name: "Test", Email: "test@example.com"}
	table, err := ws.CreateTableUnderParent(t.Context(), 0, "Table", nil, author)
	if err != nil {
		t.Fatal(err)
	}
	ids := []ksid.ID{ksid.NewID(), ksid.NewID()}
	errCh := make(chan error, len(ids))
	var workers sync.WaitGroup
	for _, id := range ids {
		workers.Go(func() {
			_, err := ws.MutateTable(t.Context(), table.ID, author, func(node *Node) error {
				node.Views = append(node.Views, contentv1.View{ID: id, Name: id.String()})
				return nil
			})
			errCh <- err
		})
	}
	workers.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := ws.ReadTable(table.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if !slices.ContainsFunc(got.Views, func(view contentv1.View) bool { return view.ID == id }) {
			t.Fatalf("view %s missing from %v", id, got.Views)
		}
	}
}

func replaceTestRecord(t *testing.T, ctx context.Context, ws *WorkspaceFileStore, tableID ksid.ID, original *contentv1.DataRecord, data map[string]any, author git.Author) error {
	t.Helper()
	table, err := ws.ReadTable(tableID)
	if err != nil {
		return err
	}
	revision, err := RecordRevision(original)
	if err != nil {
		return err
	}
	_, err = ws.ReplaceRecord(ctx, tableID, original.ID, data, revision, table.SchemaRevision, author)
	return err
}

// testFileStore creates a FileStoreService for testing with unlimited quota.
// It also creates a workspace in the service for quota testing.
func testFileStore(t *testing.T) (*FileStoreService, ksid.ID) {
	tmpDir := t.TempDir()

	gitMgr := git.NewManager(tmpDir, "test", "test@test.com")

	orgService, err := identity.NewOrganizationService(filepath.Join(tmpDir, "organizations.jsonl"))
	if err != nil {
		t.Fatalf("failed to create OrganizationService: %v", err)
	}

	wsService, err := identity.NewWorkspaceService(filepath.Join(tmpDir, "workspaces.jsonl"))
	if err != nil {
		t.Fatalf("failed to create WorkspaceService: %v", err)
	}

	// Create a test organization with very high quotas (practically unlimited)
	org, err := orgService.Create(t.Context(), "Test Organization", "test@test.com")
	if err != nil {
		t.Fatalf("failed to create test organization: %v", err)
	}
	_, err = orgService.Modify(org.ID, func(o *identityv1.Organization) error {
		o.Quotas.MaxWorkspacesPerOrg = 1_000
		o.Quotas.MaxMembersPerOrg = 10_000
		o.Quotas.MaxMembersPerWorkspace = 10_000
		o.Quotas.MaxTotalStorageBytes = 1_000_000_000_000_000_000 // 1EB
		return nil
	})
	if err != nil {
		t.Fatalf("failed to set unlimited org quotas: %v", err)
	}

	// Create a test workspace with very high quotas (practically unlimited)
	ws, err := wsService.Create(t.Context(), org.ID, "Test Workspace")
	if err != nil {
		t.Fatalf("failed to create test workspace: %v", err)
	}
	_, err = wsService.Modify(ws.ID, func(w *identityv1.Workspace) error {
		w.Quotas.MaxPages = 1_000_000
		w.Quotas.MaxStorageBytes = 1_000_000_000_000 // 1TB
		w.Quotas.MaxRecordsPerTable = 1_000_000
		w.Quotas.MaxAssetSizeBytes = 1024 * 1024 * 1024 // 1GB
		return nil
	})
	if err != nil {
		t.Fatalf("failed to set unlimited workspace quotas: %v", err)
	}

	serverQuotas := storagev1.ResourceQuotas{
		MaxPages:              1_000_000,
		MaxStorageBytes:       1_000_000_000_000, // 1TB
		MaxRecordsPerTable:    1_000_000,
		MaxAssetSizeBytes:     1024 * 1024 * 1024, // 1GB
		MaxTablesPerWorkspace: 10_000,
		MaxColumnsPerTable:    1_000,
	}
	cfg := &storagev1.ServerConfig{JWTSecret: bytes.Repeat([]byte{1}, 32), Quotas: storage.DefaultServerQuotas(false), RateLimits: storage.DefaultRateLimits()}
	cfg.Quotas.ResourceQuotas = serverQuotas
	fs, err := NewFileStoreService(tmpDir, gitMgr, wsService, orgService, storage.NewServerSettings(cfg))
	if err != nil {
		t.Fatalf("failed to create FileStoreService: %v", err)
	}

	return fs, ws.ID
}

// initWS is a test helper that creates a FileStoreService, inits the workspace, and returns the store.
func initWS(t *testing.T) (*FileStoreService, *WorkspaceFileStore, ksid.ID) {
	fs, wsID := testFileStore(t)
	ctx := t.Context()
	if err := fs.InitWorkspace(ctx, wsID); err != nil {
		t.Fatalf("failed to init workspace: %v", err)
	}
	ws, err := fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		t.Fatalf("failed to get workspace store: %v", err)
	}
	return fs, ws, wsID
}

func TestFileStoreService(t *testing.T) {
	t.Run("GetWorkspaceStore/lowered published quota", func(t *testing.T) {
		t.Parallel()
		svc, ws, id := initWS(t)
		first, err := ws.CreatePageUnderParent(t.Context(), 0, "First", "original", git.Author{})
		if err != nil {
			t.Fatal(err)
		}
		second, err := ws.CreatePageUnderParent(t.Context(), 0, "Second", "retained", git.Author{})
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		if err := svc.settings.Update(dir, func(next *storagev1.ServerConfig) error { next.Quotas.MaxPages = 1; return nil }, nil); err != nil {
			t.Fatal(err)
		}
		current, err := svc.GetWorkspaceStore(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if current == ws || current.EffectiveQuotas().MaxPages != 1 {
			t.Fatal("published server limit reused a stale workspace store")
		}
		if _, err := current.CreatePageUnderParent(t.Context(), 0, "Rejected", "body", git.Author{}); err == nil {
			t.Fatal("new page bypassed lowered limit")
		}
		if _, err := current.ReadNode(t.Context(), first.ID); err != nil {
			t.Fatalf("over-quota read: %v", err)
		}
		if _, err := current.DeletePage(t.Context(), second.ID, git.Author{}); err != nil {
			t.Fatalf("over-quota delete: %v", err)
		}
		if _, err := current.ReadNode(t.Context(), first.ID); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("ColdDiscovery", func(t *testing.T) {
		fs, ws, id := initWS(t)
		node := ksid.NewID()
		path := filepath.Join(ws.wsDir, node.String())
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "index.md"), []byte("---\ntitle: Large\n---\n\n"+strings.Repeat("x", 4<<20)), 0o600); err != nil {
			t.Fatal(err)
		}
		fs.InvalidateWorkspaceStore(id)
		fresh, err := fs.GetWorkspaceStore(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if fresh.links.built {
			t.Fatal("opening a store eagerly scanned page bodies")
		}
		page, err := fresh.ListNodeSummaries(t.Context(), new(ksid.ID), 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Nodes) != 1 || page.Nodes[0].ID != node || page.Nodes[0].Content != "" || fresh.links.built {
			t.Fatalf("cold discovery initialized content: %#v", page)
		}
		if _, err := fresh.GetBacklinks(t.Context(), node); err != nil {
			t.Fatal(err)
		}
		if !fresh.links.built {
			t.Fatal("backlink query did not initialize its index")
		}
	})

	t.Run("SavedInitializationAndRestartRecovery", func(t *testing.T) {
		for _, backend := range []git.Backend{git.BackendExec, git.BackendGoGit} {
			t.Run(strconv.Itoa(int(backend)), func(t *testing.T) {
				svc, id := testFileStore(t)
				synctest.Test(t, func(t *testing.T) {
					svc.git = git.NewManagerWithBackend(svc.rootDir, "Test", "test@example.com", backend)
					if _, err := svc.git.Repo(t.Context(), id.String()); err != nil {
						t.Fatal(err)
					}
					index := filepath.Join(svc.rootDir, id.String(), ".git", "index")
					if err := os.Mkdir(index, 0o700); err != nil {
						t.Fatal(err)
					}
					if err := svc.InitWorkspace(t.Context(), id); err != nil {
						t.Fatalf("saved workspace initialization: %v", err)
					}
					if _, err := os.Stat(filepath.Join(svc.rootDir, id.String(), "AGENTS.md")); err != nil {
						t.Fatal(err)
					}
					if !svc.HasPendingHistory() {
						t.Fatal("initial history failure hidden")
					}
					restarted, err := NewFileStoreService(svc.rootDir, git.NewManagerWithBackend(svc.rootDir, "Test", "test@example.com", backend), svc.wsSvc, svc.orgSvc, svc.settings)
					if err != nil {
						t.Fatal(err)
					}
					if err := restarted.ReconcileHistory(t.Context()); err != nil {
						t.Fatal(err)
					}
					if !restarted.HasPendingHistory() {
						t.Fatal("startup missed an unrequested workspace")
					}
					if err := os.Remove(index); err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithCancel(t.Context())
					var wg sync.WaitGroup
					wg.Go(func() { restarted.RunCommitRecovery(ctx) })
					synctest.Wait()
					if restarted.HasPendingHistory() {
						t.Fatal("workspace initialization did not recover")
					}
					store, err := restarted.GetWorkspaceStore(t.Context(), id)
					if err != nil {
						t.Fatal(err)
					}
					count, err := store.Repo().CommitCount(t.Context())
					if err != nil || count != 1 {
						t.Fatalf("initial history count=%d err=%v", count, err)
					}
					cancel()
					wg.Wait()
				})
			})
		}
	})

	t.Run("InitWorkspace", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		// Verify AGENTS.md exists on disk.
		agentsPath := filepath.Join(fs.rootDir, wsID.String(), "AGENTS.md")
		if _, err := os.Stat(agentsPath); err != nil {
			t.Fatalf("AGENTS.md not found: %v", err)
		}
	})

	t.Run("DiscardWorkspacePreventsMigrationAfterRestart", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		if _, err := git.NewRootRepo(t.Context(), fs.rootDir, "test", "test@test.com"); err != nil {
			t.Fatal(err)
		}
		if err := fs.InitWorkspace(t.Context(), wsID); err != nil {
			t.Fatal(err)
		}
		if err := fs.DiscardWorkspace(wsID); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(fs.rootDir, wsID.String())); !os.IsNotExist(err) {
			t.Fatalf("discarded workspace directory still exists: %v", err)
		}
		if _, err := git.NewRootRepo(t.Context(), fs.rootDir, "test", "test@test.com"); err != nil {
			t.Fatal(err)
		}
		modules, err := os.ReadFile(filepath.Join(fs.rootDir, ".gitmodules"))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if strings.Contains(string(modules), wsID.String()) {
			t.Fatal("restart migrated the discarded workspace as a submodule")
		}
	})
}

func TestWorkspaceFileStore(t *testing.T) {
	t.Run("ConditionalPageWrites", func(t *testing.T) {
		for _, appendOnly := range []bool{false, true} {
			name := "Replace"
			if appendOnly {
				name = "Append"
			}
			t.Run(name, func(t *testing.T) {
				_, ws, _ := initWS(t)
				a := git.Author{Name: "Writer", Email: "writer@example.com"}
				created, err := ws.CreatePageUnderParent(t.Context(), 0, " Original ", "\n\nBase", a)
				if err != nil {
					t.Fatal(err)
				}
				baseline, err := ws.ReadNode(t.Context(), created.ID)
				if err != nil {
					t.Fatal(err)
				}
				before, err := ws.repo.CommitCount(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				start := make(chan struct{})
				results := make(chan error, 2)
				var wg sync.WaitGroup
				for i := range 2 {
					wg.Go(func() {
						<-start
						var err error
						if appendOnly {
							_, err = ws.AppendPage(t.Context(), created.ID, fmt.Sprintf("body-%d", i), baseline.Revision, a)
						} else {
							_, err = ws.UpdatePage(t.Context(), created.ID, "Changed", fmt.Sprintf("body-%d", i), baseline.Revision, a)
						}
						results <- err
					})
				}
				close(start)
				wg.Wait()
				close(results)
				successes, conflicts := 0, 0
				for err := range results {
					switch {
					case err == nil:
						successes++
					case errors.Is(err, ErrRevisionConflict):
						conflicts++
					default:
						t.Fatal(err)
					}
				}
				if successes != 1 || conflicts != 1 {
					t.Fatalf("success=%d conflict=%d", successes, conflicts)
				}
				after, err := ws.repo.CommitCount(t.Context())
				if err != nil || after != before+1 {
					t.Fatalf("commits %d -> %d: %v", before, after, err)
				}
				latest, err := ws.ReadNode(t.Context(), created.ID)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := os.ReadFile(ws.pageIndexFile(created.ID, 0))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := ws.AppendPage(t.Context(), created.ID, "stale", baseline.Revision, a); !errors.Is(err, ErrRevisionConflict) {
					t.Fatalf("stale append: %v", err)
				}
				unchanged, err := os.ReadFile(ws.pageIndexFile(created.ID, 0))
				if err != nil || !bytes.Equal(raw, unchanged) {
					t.Fatal("rejected append changed file", err)
				}
				reread, err := ws.ReadNode(t.Context(), created.ID)
				if err != nil || reread.Revision != latest.Revision {
					t.Fatal("rejected write changed revision", err)
				}
			})
		}
		t.Run("CanonicalSnapshotAndMetadata", func(t *testing.T) {
			_, ws, _ := initWS(t)
			a := git.Author{Name: "Writer", Email: "writer@example.com"}
			page, err := ws.CreatePageUnderParent(t.Context(), 0, "Before", "body", a)
			if err != nil {
				t.Fatal(err)
			}
			revision := pageRevision(t, t.Context(), ws, page.ID)
			if _, err := ws.UpdatePageFrontmatter(t.Context(), page.ID, "🌳", "cover.png", a); err != nil {
				t.Fatal(err)
			}
			saved, err := ws.UpdatePage(t.Context(), page.ID, "  Canonical  ", "\n\nnormalized", revision, a)
			if err != nil {
				t.Fatal(err)
			}
			read, err := ws.ReadNode(t.Context(), page.ID)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Title != "Canonical" || saved.Content != "normalized" || saved.Icon != "🌳" || saved.Cover != "cover.png" || saved.Revision != read.Revision || saved.Revision == revision {
				t.Fatalf("saved=%+v read=%+v", saved, read)
			}
			snap, err := ws.ReadPageSnapshot(t.Context(), page.ID, 0, 4096)
			if err != nil || snap.Revision != saved.Revision || snap.Node.Content != saved.Content {
				t.Fatalf("snapshot mismatch %+v %v", snap, err)
			}
			next, err := ws.AppendPage(t.Context(), page.ID, "next", saved.Revision, a)
			if err != nil || next.Revision == saved.Revision || next.Content != "normalized\n\nnext" {
				t.Fatalf("append=%+v %v", next, err)
			}
		})
	})

	t.Run("ConcurrentLegacyRefresh", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx, cancel := context.WithCancel(t.Context())
			var wg sync.WaitGroup
			t.Cleanup(func() { cancel(); wg.Wait() })
			a := git.Author{Name: "Test", Email: "test@example.com"}
			root, err := ws.CreatePageUnderParent(ctx, 0, "Parent", "body", a)
			if err != nil {
				t.Fatal(err)
			}
			child, err := ws.CreatePageUnderParent(ctx, root.ID, "Child", "child", a)
			if err != nil {
				t.Fatal(err)
			}
			other, err := ws.CreatePageUnderParent(ctx, 0, "Other", "other", a)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ws.ListNodeSummaries(ctx, nil, 0, 50); err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			for range 4 {
				wg.Go(func() {
					<-start
					for ctx.Err() == nil {
						_, _ = ws.ReadNode(ctx, ksid.NewID())
					}
				})
			}
			close(start)
			for range 500 {
				page, err := ws.ListNodeSummaries(ctx, nil, 0, 50)
				if err != nil {
					t.Fatal(err)
				}
				if page.Total != 3 || len(page.Nodes) != 3 || page.Next != 0 {
					t.Fatalf("legacy refresh changed inventory: total=%d nodes=%d next=%s", page.Total, len(page.Nodes), page.Next)
				}
				summary, err := ws.ReadNodeSummary(ctx, root.ID)
				if err != nil || !summary.HasChildren {
					t.Fatalf("legacy refresh changed summary: %#v %v", summary, err)
				}
				snap, err := ws.ReadPageSnapshot(ctx, root.ID, 0, 4096)
				if err != nil || snap.Node.Content != "body" || !snap.Node.HasChildren {
					t.Fatalf("legacy refresh changed snapshot: %#v %v", snap, err)
				}
			}
			// Mutation hooks publish ancestry and child flags together while
			// legacy cache misses continue independently.
			moved := make(chan error, 1)
			wg.Go(func() {
				for i := range 20 {
					parent := root.ID
					if i%2 == 1 {
						parent = other.ID
					}
					if _, err := ws.MoveNode(ctx, child.ID, parent, a); err != nil {
						moved <- err
						return
					}
				}
				moved <- nil
			})
			for range 100 {
				snap, err := ws.ReadPageSnapshot(ctx, child.ID, 0, 4096)
				if err != nil || snap.Node.Content != "child" || (snap.Node.ParentID != root.ID && snap.Node.ParentID != other.ID) {
					t.Fatalf("concurrent move lost published ancestry: %#v %v", snap, err)
				}
			}
			if err := <-moved; err != nil {
				t.Fatal(err)
			}
		})
	})

	t.Run("IterAssets", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			_, ws, _ := initWS(t)
			a := git.Author{Name: "Test", Email: "test@example.com"}
			n, err := ws.CreatePageUnderParent(t.Context(), 0, "Assets", "", a)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ws.SaveAsset(t.Context(), n.ID, ".user-note", bytes.NewReader([]byte("user")), int64(len([]byte("user"))), a); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(ws.pageDir(n.ID, 0), storage.AtomicFilePrefix+"interrupted"), []byte("temporary"), 0o600); err != nil {
				t.Fatal(err)
			}
			assets, err := ws.IterAssets(n.ID)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for asset := range assets {
				names = append(names, asset.Name)
			}
			if len(names) != 1 || names[0] != ".user-note" {
				t.Fatalf("asset listing = %v", names)
			}
		})
	})

	t.Run("ReadPageSnapshot", func(t *testing.T) {
		t.Run("PulledRevision", func(t *testing.T) {
			for _, backend := range []git.Backend{git.BackendExec, git.BackendGoGit} {
				t.Run(strconv.Itoa(int(backend)), func(t *testing.T) {
					root := t.TempDir()
					remoteDir, localDir := filepath.Join(root, "remote"), filepath.Join(root, "local")
					remoteRepo, err := git.NewManagerWithBackend(remoteDir, "test", "test@example.com", backend).Repo(t.Context(), "")
					if err != nil {
						t.Fatal(err)
					}
					quotas := storage.DefaultResourceQuotas()
					remote := newWorkspaceFileStore(remoteDir, remoteRepo, &quotas)
					node, err := remote.CreatePageUnderParent(t.Context(), 0, "Before pull", "original body", git.Author{})
					if err != nil {
						t.Fatal(err)
					}
					if out, err := exec.CommandContext(t.Context(), "git", "clone", "--quiet", remoteDir, localDir).CombinedOutput(); err != nil { //nolint:gosec // Both paths belong to this test's temporary directory.
						t.Fatalf("clone fixture: %v: %s", err, out)
					}
					localRepo, err := git.NewManagerWithBackend(localDir, "test", "test@example.com", backend).Repo(t.Context(), "")
					if err != nil {
						t.Fatal(err)
					}
					local := newWorkspaceFileStore(localDir, localRepo, &quotas)
					before, err := local.ReadPageSnapshot(t.Context(), node.ID, 0, 4096)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := remote.UpdatePage(t.Context(), node.ID, "After pull", "updated body", pageRevision(t, t.Context(), remote, node.ID), git.Author{}); err != nil {
						t.Fatal(err)
					}
					if changed, err := localRepo.Pull(t.Context(), "origin", ""); err != nil || !changed {
						t.Fatalf("pull: changed=%v, err=%v", changed, err)
					}
					after, err := local.ReadPageSnapshot(t.Context(), node.ID, 0, 4096)
					if err != nil || after.Revision == before.Revision || after.Node.Title != "After pull" || after.Node.Content != "updated body" {
						t.Fatalf("pulled snapshot reused previous revision: %+v, %v", after, err)
					}
				})
			}
		})

		dir := t.TempDir()
		repo, err := git.NewManagerWithBackend(dir, "test", "test@example.com", git.BackendGoGit).Repo(t.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		quotas := storage.DefaultResourceQuotas()
		ws := newWorkspaceFileStore(dir, repo, &quotas)
		author := git.Author{Name: "Test", Email: "test@example.com"}
		body := strings.Repeat("🌳 alpha\n", 13000)
		node, err := ws.CreatePageUnderParent(t.Context(), 0, "  Canonical title  ", "\n\n"+body, author)
		if err != nil {
			t.Fatal(err)
		}
		t.Run("valid", func(t *testing.T) {
			var all strings.Builder
			var offset int64
			revision := ""
			for {
				snap, err := ws.ReadPageSnapshot(t.Context(), node.ID, offset, 4096)
				if err != nil {
					t.Fatal(err)
				}
				if len(snap.Node.Content) > 4096 || !utf8.ValidString(snap.Node.Content) {
					t.Fatal("invalid bounded chunk")
				}
				if snap.Node.Title != "Canonical title" {
					t.Fatalf("title = %q", snap.Node.Title)
				}
				if revision != "" && snap.Revision != revision {
					t.Fatal("revision changed across chunks")
				}
				revision = snap.Revision
				all.WriteString(snap.Node.Content)
				if snap.NextOffset == snap.ContentBytes {
					break
				}
				if snap.NextOffset <= offset {
					t.Fatal("continuation made no progress")
				}
				offset = snap.NextOffset
			}
			if all.String() != body || revision != "885611606f0f6841069f8252b5057aa6fa53486ebcd80e9bed4de02e3a5af935" {
				t.Fatal("canonical content/revision mismatch")
			}
			if _, err := ws.UpdatePageFrontmatter(t.Context(), node.ID, "🙂", "cover", author); err != nil {
				t.Fatal(err)
			}
			snap, err := ws.ReadPageSnapshot(t.Context(), node.ID, 0, 4096)
			if err != nil || snap.Revision != revision {
				t.Fatalf("metadata changed content revision: %v", err)
			}
			if _, err := ws.UpdatePage(t.Context(), node.ID, "Canonical title", body+"updated", pageRevision(t, t.Context(), ws, node.ID), author); err != nil {
				t.Fatal(err)
			}
			snap, err = ws.ReadPageSnapshot(t.Context(), node.ID, 0, 4096)
			if err != nil || snap.Revision == revision {
				t.Fatalf("content change missing: %v", err)
			}
			// A read remains available after Git metadata disappears.
			if err := os.Rename(filepath.Join(dir, ".git"), filepath.Join(dir, "history-unavailable")); err != nil {
				t.Fatal(err)
			}
			if _, err := ws.ReadPageSnapshot(t.Context(), node.ID, 0, 4096); err != nil {
				t.Fatal(err)
			}
		})
		t.Run("error", func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if _, err := ws.ReadPageSnapshot(ctx, node.ID, 0, 4096); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation: %v", err)
			}
			if _, err := ws.ReadPageSnapshot(t.Context(), node.ID, 1, 4096); err == nil {
				t.Fatal("accepted split UTF-8 offset")
			}
		})
	})
	t.Run("ListNodeSummaries", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			dir := t.TempDir()
			repo, err := git.NewManager(dir, "test", "test@example.com").Repo(t.Context(), "")
			if err != nil {
				t.Fatal(err)
			}
			quotas := storage.DefaultResourceQuotas()
			ws := newWorkspaceFileStore(dir, repo, &quotas)
			a := git.Author{Name: "Test", Email: "test@example.com"}
			root, err := ws.CreatePageUnderParent(t.Context(), 0, "Root", "", a)
			if err != nil {
				t.Fatal(err)
			}
			child, err := ws.CreatePageUnderParent(t.Context(), root.ID, "Child", "", a)
			if err != nil {
				t.Fatal(err)
			}
			leaf, err := ws.CreatePageUnderParent(t.Context(), child.ID, "Leaf", "", a)
			if err != nil {
				t.Fatal(err)
			}
			var ids []ksid.ID
			var after ksid.ID
			for {
				p, err := ws.ListNodeSummaries(t.Context(), nil, after, 1)
				if err != nil {
					t.Fatal(err)
				}
				if len(p.Nodes) != 1 || p.Total != 3 {
					t.Fatalf("page = %#v", p)
				}
				ids = append(ids, p.Nodes[0].ID)
				if p.Next == 0 {
					break
				}
				after = p.Next
			}
			if len(ids) != 3 || !slices.IsSorted(ids) {
				t.Fatalf("traversal = %v", ids)
			}
			if _, err := ws.MoveNode(t.Context(), leaf.ID, 0, a); err != nil {
				t.Fatal(err)
			}
			p, err := ws.ListNodeSummaries(t.Context(), &child.ID, 0, 2)
			if err != nil || p.Total != 0 {
				t.Fatalf("move index: %#v %v", p, err)
			}
			if _, err := ws.DeletePage(t.Context(), root.ID, a); err != nil {
				t.Fatal(err)
			}
			p, err = ws.ListNodeSummaries(t.Context(), nil, 0, 2)
			if err != nil || p.Total != 1 || p.Nodes[0].ID != leaf.ID {
				t.Fatalf("subtree retirement: %#v %v", p, err)
			}
			// Huge body lines do not enter summary parsing. Huge metadata fails explicitly.
			if err := os.WriteFile(ws.pageIndexFile(leaf.ID, 0), []byte("---\ntitle: Leaf\n---\n\n"+strings.Repeat("x", 4<<20)), 0o600); err != nil {
				t.Fatal(err)
			}
			p, err = ws.ListNodeSummaries(t.Context(), new(ksid.ID), 0, 1)
			if err != nil || p.Nodes[0].Content != "" {
				t.Fatalf("summary: %#v %v", p, err)
			}
		})
		t.Run("error", func(t *testing.T) {
			_, ws, _ := initWS(t)
			a := git.Author{Name: "Test", Email: "test@example.com"}
			leaf, err := ws.CreatePageUnderParent(t.Context(), 0, "Leaf", "", a)
			if err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(ws.tableMetadataFile(leaf.ID, 0), []byte(strings.Repeat("x", MaxSnapshotMetadataBytes+1)), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ws.ReadPageSnapshot(t.Context(), leaf.ID, 0, 4096); err == nil {
				t.Fatal("accepted oversized metadata")
			}
		})
	})

	t.Run("TableMetadataVersion", func(t *testing.T) {
		_, ws, _ := initWS(t)
		leaf, err := ws.CreatePageUnderParent(t.Context(), 0, "Leaf", "", git.Author{Name: "Test", Email: "test@example.com"})
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			metadata string
			ok       bool
		}{
			{`{"title":"Imported"}`, true},
			{`{"title":"Current","version":"1.0"}`, true},
			{`{"title":"Future","version":"2.0"}`, false},
			{`{"title":"Numeric","version":1}`, false},
		} {
			if err := os.WriteFile(ws.tableMetadataFile(leaf.ID, 0), []byte(tc.metadata), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := ws.ReadTable(leaf.ID)
			_, snapshotErr := ws.ReadPageSnapshot(t.Context(), leaf.ID, 0, 4096)
			if (err == nil) != tc.ok || (snapshotErr == nil) != tc.ok {
				t.Errorf("%s: ReadTable error = %v, snapshot error = %v, want ok=%v", tc.metadata, err, snapshotErr, tc.ok)
			}
		}
	})

	t.Run("RetainedContainerDiscovery", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			fs, ws, wsID := initWS(t)
			a := git.Author{Name: "Test", Email: "test@example.com"}
			ctx := t.Context()
			parent, err := ws.CreatePageUnderParent(ctx, 0, "Parent", "body", a)
			if err != nil {
				t.Fatal(err)
			}
			child, err := ws.CreatePageUnderParent(ctx, parent.ID, "Child", "body", a)
			if err != nil {
				t.Fatal(err)
			}
			table, err := ws.CreateTableUnderParent(ctx, 0, "Table", nil, a)
			if err != nil {
				t.Fatal(err)
			}
			tableChild, err := ws.CreatePageUnderParent(ctx, table.ID, "Table child", "body", a)
			if err != nil {
				t.Fatal(err)
			}
			assetNode, err := ws.CreatePageUnderParent(ctx, 0, "Asset only", "body", a)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ws.SaveAsset(ctx, assetNode.ID, "retained.txt", bytes.NewReader([]byte("asset")), int64(len([]byte("asset"))), a); err != nil {
				t.Fatal(err)
			}
			if _, err := ws.ListNodeSummaries(ctx, nil, 0, 50); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ws.DeletePageFromNode(ctx, parent.ID, a); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ws.DeleteTableFromNode(ctx, table.ID, a); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ws.DeletePageFromNode(ctx, assetNode.ID, a); err != nil {
				t.Fatal(err)
			}
			page, err := ws.ListNodeSummaries(ctx, nil, 0, 50)
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != 2 {
				t.Fatalf("warm retained containers advertised: %#v", page)
			}
			fs.InvalidateWorkspaceStore(wsID)
			cold, err := fs.GetWorkspaceStore(ctx, wsID)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []ksid.ID{child.ID, tableChild.ID} {
				if _, err := cold.ReadNode(t.Context(), id); err != nil {
					t.Fatalf("cold child %s hidden: %v", id, err)
				}
			}
			// Context-free legacy ancestry refresh cannot publish ordered discovery.
			cold.mu.RLock()
			published := cold.indexReady
			cold.mu.RUnlock()
			if published {
				t.Fatal("legacy parent lookup published an ungated discovery index")
			}
			page, err = cold.ListNodeSummaries(ctx, nil, 0, 50)
			if err != nil || page.Total != 2 {
				t.Fatalf("cold inventory: %#v, %v", page, err)
			}
		})
	})

	author := git.Author{Name: "Test", Email: "test@test.com"}

	t.Run("HeadVersionRejectsCorruptLooseReference", func(t *testing.T) {
		dir := t.TempDir()
		repo, err := git.NewManagerWithBackend(dir, "Test", "test@example.com", git.BackendGoGit).Repo(t.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		quotas := storage.DefaultResourceQuotas()
		ws := newWorkspaceFileStore(dir, repo, &quotas)
		id := ksid.NewID()
		if _, err := writePageFixture(ws, t.Context(), id, "Title", "saved body", author); err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(t.Context(), "git", "-C", dir, "pack-refs", "--all", "--prune") //nolint:gosec // G204: fixed Git command in a test-owned repository.
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("pack refs: %v %s", err, out)
		}
		if body, err := ws.GetPageContentAtCommit(t.Context(), "HEAD", id); err != nil || strings.TrimSpace(body) != "saved body" {
			t.Fatalf("valid HEAD version=%q err=%v", body, err)
		}
		head, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD")) //nolint:gosec // G304: test-owned repository metadata.
		if err != nil {
			t.Fatal(err)
		}
		ref := strings.TrimSpace(strings.TrimPrefix(string(head), "ref: "))
		if err := os.WriteFile(filepath.Join(dir, ".git", ref), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if body, err := ws.GetPageContentAtCommit(t.Context(), "HEAD", id); err == nil || body != "" {
			t.Fatalf("corrupt HEAD version leaked stale success: body=%q err=%v", body, err)
		}
	})

	t.Run("PageOperations", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()
		nodeID := ksid.NewID()

		t.Run("PageFixture", func(t *testing.T) {
			page, err := writePageFixture(ws, ctx, nodeID, "Test Title", "# Test Content", author)
			if err != nil {
				t.Fatalf("failed to write page: %v", err)
			}
			if page.ID != nodeID {
				t.Errorf("expected ID %v, got %v", nodeID, page.ID)
			}
			if page.Title != "Test Title" {
				t.Errorf("expected title 'Test Title', got %q", page.Title)
			}
		})

		t.Run("ReadPage", func(t *testing.T) {
			readPage, err := ws.ReadPage(nodeID)
			if err != nil {
				t.Fatalf("failed to read page: %v", err)
			}
			if readPage.Title != "Test Title" {
				t.Errorf("expected title 'Test Title', got %q", readPage.Title)
			}
			if readPage.Content != "# Test Content" {
				t.Errorf("expected content '# Test Content', got %q", readPage.Content)
			}
		})

		t.Run("UpdatePage", func(t *testing.T) {
			updated, err := ws.UpdatePage(ctx, nodeID, "Updated Title", "# Updated Content", pageRevision(t, ctx, ws, nodeID), author)
			if err != nil {
				t.Fatalf("failed to update page: %v", err)
			}
			if updated.Title != "Updated Title" {
				t.Errorf("expected title 'Updated Title', got %q", updated.Title)
			}

			readUpdated, err := ws.ReadPage(nodeID)
			if err != nil {
				t.Fatalf("failed to read updated page: %v", err)
			}
			if readUpdated.Title != "Updated Title" {
				t.Errorf("expected title 'Updated Title', got %q", readUpdated.Title)
			}
		})

		t.Run("DeletePage", func(t *testing.T) {
			if _, err := ws.DeletePage(ctx, nodeID, author); err != nil {
				t.Fatalf("failed to delete page: %v", err)
			}
			if _, err := ws.ReadPage(nodeID); err == nil {
				t.Error("expected error reading deleted page")
			}
		})

		t.Run("ReadNonExistent", func(t *testing.T) {
			if _, err := ws.ReadPage(ksid.NewID()); err == nil {
				t.Error("expected error reading non-existent page")
			}
		})
	})

	t.Run("ListPages", func(t *testing.T) {
		fs, ws, wsID := initWS(t)
		ctx := t.Context()

		nodeIDs := make([]ksid.ID, 0, 3)
		for i := range 3 {
			id := ksid.NewID()
			nodeIDs = append(nodeIDs, id)
			if _, err := writePageFixture(ws, ctx, id, "Page "+string(rune(48+i)), "content", author); err != nil {
				t.Fatalf("failed to write page %d: %v", i, err)
			}
		}

		it, err := ws.IterPages()
		if err != nil {
			t.Fatalf("failed to list pages: %v", err)
		}
		count := 0
		for p := range it {
			count++
			if p == nil {
				t.Error("iterator yielded nil page")
			}
		}
		if count != 3 {
			t.Errorf("expected 3 pages, got %d", count)
		}

		t.Run("DirectoryStructure", func(t *testing.T) {
			for _, nodeID := range nodeIDs {
				expectedDir := filepath.Join(fs.rootDir, wsID.String(), nodeID.String())
				if _, err := os.Stat(expectedDir); err != nil {
					t.Errorf("expected page directory %s to exist: %v", expectedDir, err)
				}
				expectedFile := filepath.Join(expectedDir, "index.md")
				if _, err := os.Stat(expectedFile); err != nil {
					t.Errorf("expected file %s to exist: %v", expectedFile, err)
				}
			}
		})
	})

	t.Run("TableOperations", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		tableID := ksid.NewID()
		node := &Node{
			ID:       tableID,
			Title:    "Test Table",
			Type:     NodeTypeTable,
			Created:  storagev1.Now(),
			Modified: storagev1.Now(),
			Properties: []contentv1.Property{
				{Name: "name", Type: contentv1.PropertyTypeText, Required: true},
				{Name: "value", Type: contentv1.PropertyTypeText},
			},
		}

		t.Run("WriteTable", func(t *testing.T) {
			if err := ws.WriteTable(ctx, node, true, author); err != nil {
				t.Fatalf("failed to write table: %v", err)
			}
		})

		t.Run("ReadTable", func(t *testing.T) {
			read, err := ws.ReadTable(tableID)
			if err != nil {
				t.Fatalf("failed to read table: %v", err)
			}
			if read.Title != "Test Table" {
				t.Errorf("expected title 'Test Table', got %q", read.Title)
			}
		})

		t.Run("SelectOptionsRoundTrip", func(t *testing.T) {
			// Write a table with a select property that has options, then read
			// it back and verify options are preserved (regression test).
			selectID := ksid.NewID()
			selectNode := &Node{
				ID:    selectID,
				Title: "Select Table",
				Type:  NodeTypeTable,
				Properties: []contentv1.Property{
					{
						Name: "Status",
						Type: contentv1.PropertyTypeSelect,
						Options: []contentv1.SelectOption{
							{ID: "opt1", Name: "Alpha", Color: "#e03e3e"},
							{ID: "opt2", Name: "Beta"},
						},
					},
				},
			}
			if err := ws.WriteTable(ctx, selectNode, true, author); err != nil {
				t.Fatalf("failed to write table: %v", err)
			}
			read, err := ws.ReadNode(ctx, selectID)
			if err != nil {
				t.Fatalf("failed to read node: %v", err)
			}
			if len(read.Properties) != 1 {
				t.Fatalf("expected 1 property, got %d", len(read.Properties))
			}
			prop := read.Properties[0]
			if prop.Name != "Status" {
				t.Errorf("expected property name 'Status', got %q", prop.Name)
			}
			if prop.Type != contentv1.PropertyTypeSelect {
				t.Errorf("expected property type 'select', got %q", prop.Type)
			}
			if len(prop.Options) != 2 {
				t.Fatalf("expected 2 options, got %d", len(prop.Options))
			}
			if prop.Options[0].ID != "opt1" || prop.Options[0].Name != "Alpha" || prop.Options[0].Color != "#e03e3e" {
				t.Errorf("unexpected option[0]: %+v", prop.Options[0])
			}
			if prop.Options[1].ID != "opt2" || prop.Options[1].Name != "Beta" {
				t.Errorf("unexpected option[1]: %+v", prop.Options[1])
			}
		})

		t.Run("RecordOperations", func(t *testing.T) {
			record := &contentv1.DataRecord{
				ID:       ksid.NewID(),
				Data:     map[string]any{"name": "test", "value": "data"},
				Created:  storagev1.Now(),
				Modified: storagev1.Now(),
			}

			t.Run("AppendRecord", func(t *testing.T) {
				if err := ws.AppendRecord(ctx, tableID, record, author); err != nil {
					t.Fatalf("failed to append record: %v", err)
				}
			})

			t.Run("IterRecords", func(t *testing.T) {
				it, err := ws.IterRecords(tableID)
				if err != nil {
					t.Fatalf("failed to iter records: %v", err)
				}
				count := 0
				for r := range it {
					count++
					if r.ID != record.ID {
						t.Errorf("expected record ID %v, got %v", record.ID, r.ID)
					}
				}
				if count != 1 {
					t.Errorf("expected 1 record, got %d", count)
				}
			})

			t.Run("UpdateRecord", func(t *testing.T) {
				updated := &contentv1.DataRecord{
					ID:       record.ID,
					Data:     map[string]any{"name": "updated", "value": "new data"},
					Created:  record.Created,
					Modified: storagev1.Now(),
				}
				if err := replaceTestRecord(t, ctx, ws, tableID, record, updated.Data, author); err != nil {
					t.Fatalf("failed to update record: %v", err)
				}

				it, err := ws.IterRecords(tableID)
				if err != nil {
					t.Fatalf("failed to iter records: %v", err)
				}
				for r := range it {
					if r.ID == record.ID {
						if r.Data["name"] != "updated" {
							t.Errorf("expected name 'updated', got %v", r.Data["name"])
						}
						break
					}
				}
			})

			t.Run("DeleteRecord", func(t *testing.T) {
				it, err := ws.IterRecords(tableID)
				if err != nil {
					t.Fatal(err)
				}
				var revision string
				for current := range it {
					if current.ID == record.ID {
						revision, err = RecordRevision(current)
						if err != nil {
							t.Fatal(err)
						}
						break
					}
				}
				if err := ws.DeleteRecord(ctx, tableID, record.ID, revision, author); err != nil {
					t.Fatalf("failed to delete record: %v", err)
				}

				it, err = ws.IterRecords(tableID)
				if err != nil {
					t.Fatalf("failed to iter records: %v", err)
				}
				for r := range it {
					if r.ID == record.ID {
						t.Error("expected record to be deleted")
					}
				}
			})
		})

		t.Run("DeleteTable", func(t *testing.T) {
			if err := ws.DeleteTable(ctx, tableID, author); err != nil {
				t.Fatalf("failed to delete table: %v", err)
			}
			if _, err := ws.ReadTable(tableID); err == nil {
				t.Error("expected error reading deleted table")
			}
		})

		t.Run("UpdateRecord_DataPersistence", func(t *testing.T) {
			testTableID := ksid.NewID()
			testTable := &Node{
				ID:       testTableID,
				Title:    "Test Table",
				Type:     NodeTypeTable,
				Created:  storagev1.Now(),
				Modified: storagev1.Now(),
				Properties: []contentv1.Property{
					{Name: "name", Type: contentv1.PropertyTypeText},
					{Name: "value", Type: contentv1.PropertyTypeText},
				},
			}
			if err := ws.WriteTable(ctx, testTable, true, author); err != nil {
				t.Fatalf("failed to write table: %v", err)
			}

			initialRecord := &contentv1.DataRecord{
				ID:       ksid.NewID(),
				Data:     map[string]any{"name": "original", "value": "data"},
				Created:  storagev1.Now(),
				Modified: storagev1.Now(),
			}
			if err := ws.AppendRecord(ctx, testTableID, initialRecord, author); err != nil {
				t.Fatalf("failed to append record: %v", err)
			}

			updatedRecord := &contentv1.DataRecord{
				ID:       initialRecord.ID,
				Data:     map[string]any{"name": "updated", "value": "new data"},
				Created:  initialRecord.Created,
				Modified: storagev1.Now(),
			}
			if err := replaceTestRecord(t, ctx, ws, testTableID, initialRecord, updatedRecord.Data, author); err != nil {
				t.Fatalf("failed to update record: %v", err)
			}

			it, err := ws.IterRecords(testTableID)
			if err != nil {
				t.Fatalf("failed to iter records: %v", err)
			}
			found := false
			for r := range it {
				if r.ID != initialRecord.ID {
					continue
				}
				found = true
				if r.Data == nil {
					t.Error("record data is nil - data was not persisted")
				}
				if name, ok := r.Data["name"]; !ok || name != "updated" {
					t.Errorf("expected name='updated', got %v", name)
				}
				if value, ok := r.Data["value"]; !ok || value != "new data" {
					t.Errorf("expected value='new data', got %v", value)
				}
				break
			}
			if !found {
				t.Error("record not found after update")
			}
		})

		t.Run("UpdateRecord_NilDataBecomesEmptyMap", func(t *testing.T) {
			testTableID := ksid.NewID()
			testTable := &Node{
				ID:       testTableID,
				Title:    "Test Table",
				Type:     NodeTypeTable,
				Created:  storagev1.Now(),
				Modified: storagev1.Now(),
				Properties: []contentv1.Property{
					{Name: "name", Type: contentv1.PropertyTypeText},
				},
			}
			if err := ws.WriteTable(ctx, testTable, true, author); err != nil {
				t.Fatalf("failed to write table: %v", err)
			}

			initialRecord := &contentv1.DataRecord{
				ID:       ksid.NewID(),
				Data:     map[string]any{"name": "test"},
				Created:  storagev1.Now(),
				Modified: storagev1.Now(),
			}
			if err := ws.AppendRecord(ctx, testTableID, initialRecord, author); err != nil {
				t.Fatalf("failed to append record: %v", err)
			}

			updatedRecord := &contentv1.DataRecord{
				ID:       initialRecord.ID,
				Data:     nil, // Explicitly nil
				Created:  initialRecord.Created,
				Modified: storagev1.Now(),
			}

			if err := replaceTestRecord(t, ctx, ws, testTableID, initialRecord, updatedRecord.Data, author); err != nil {
				t.Fatalf("failed to update record: %v", err)
			}

			it, err := ws.IterRecords(testTableID)
			if err != nil {
				t.Fatalf("failed to iter records: %v", err)
			}
			found := false
			for r := range it {
				if r.ID == initialRecord.ID {
					if r.Data == nil {
						t.Fatal("nil data was persisted")
					}
					found = true
					break
				}
			}
			if !found {
				t.Error("record not found after update with nil data")
			}
		})
	})

	t.Run("AssetOperations", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		nodeID := ksid.NewID()
		if _, err := writePageFixture(ws, ctx, nodeID, "With Assets", "content", author); err != nil {
			t.Fatalf("failed to write page: %v", err)
		}

		assetName := "test.txt"
		assetData := []byte("test asset content")

		t.Run("SaveAsset", func(t *testing.T) {
			asset, err := ws.SaveAsset(ctx, nodeID, assetName, bytes.NewReader(assetData), int64(len(assetData)), author)
			if err != nil {
				t.Fatalf("failed to save asset: %v", err)
			}
			if asset.Name != assetName {
				t.Errorf("expected name %q, got %q", assetName, asset.Name)
			}
		})

		t.Run("ReadAsset", func(t *testing.T) {
			data, err := ws.ReadAsset(nodeID, assetName)
			if err != nil {
				t.Fatalf("failed to read asset: %v", err)
			}
			if !bytes.Equal(data, assetData) {
				t.Errorf("expected data %q, got %q", string(assetData), string(data))
			}
		})

		t.Run("IterAssets", func(t *testing.T) {
			it, err := ws.IterAssets(nodeID)
			if err != nil {
				t.Fatalf("failed to iter assets: %v", err)
			}
			count := 0
			for asset := range it {
				count++
				if asset.Name != assetName {
					t.Errorf("expected name %q, got %q", assetName, asset.Name)
				}
			}
			if count != 1 {
				t.Errorf("expected 1 asset, got %d", count)
			}
		})

		t.Run("DeleteAsset", func(t *testing.T) {
			if err := ws.DeleteAsset(ctx, nodeID, assetName, author); err != nil {
				t.Fatalf("failed to delete asset: %v", err)
			}
			if _, err := ws.ReadAsset(nodeID, assetName); err == nil {
				t.Error("expected error reading deleted asset")
			}
		})
	})

	t.Run("UpdatePageFrontmatter", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		t.Run("SetIconAndCover", func(t *testing.T) {
			// Simulate: create page via CreatePageUnderParent (normal production flow)
			node, err := ws.CreatePageUnderParent(ctx, 0, "My Page", "content", author)
			if err != nil {
				t.Fatalf("failed to create page: %v", err)
			}

			// Upload an asset (as would happen during cover image upload)
			imageData := []byte("fake image data")
			asset, err := ws.SaveAsset(ctx, node.ID, "cover.jpg", bytes.NewReader(imageData), int64(len(imageData)), author)
			if err != nil {
				t.Fatalf("failed to save asset: %v", err)
			}

			// Now update frontmatter with the asset as cover
			updated, err := ws.UpdatePageFrontmatter(ctx, node.ID, "🎨", asset.Name, author)
			if err != nil {
				t.Fatalf("UpdatePageFrontmatter failed: %v", err)
			}
			if updated.Icon != "🎨" {
				t.Errorf("expected icon '🎨', got %q", updated.Icon)
			}
			if updated.Cover != "cover.jpg" {
				t.Errorf("expected cover 'cover.jpg', got %q", updated.Cover)
			}

			// Verify persisted on disk
			readBack, err := ws.ReadPage(node.ID)
			if err != nil {
				t.Fatalf("failed to read page after frontmatter update: %v", err)
			}
			if readBack.Icon != "🎨" {
				t.Errorf("persisted icon: expected '🎨', got %q", readBack.Icon)
			}
			if readBack.Cover != "cover.jpg" {
				t.Errorf("persisted cover: expected 'cover.jpg', got %q", readBack.Cover)
			}
		})

		t.Run("ClearCover", func(t *testing.T) {
			node, err := ws.CreatePageUnderParent(ctx, 0, "Page With Cover", "content", author)
			if err != nil {
				t.Fatalf("failed to create page: %v", err)
			}
			// Set icon and cover
			if _, err := ws.UpdatePageFrontmatter(ctx, node.ID, "🎨", "cover.jpg", author); err != nil {
				t.Fatalf("failed to set frontmatter: %v", err)
			}
			// Clear cover
			updated, err := ws.UpdatePageFrontmatter(ctx, node.ID, "🎨", "", author)
			if err != nil {
				t.Fatalf("failed to clear cover: %v", err)
			}
			if updated.Cover != "" {
				t.Errorf("expected empty cover, got %q", updated.Cover)
			}
		})

		t.Run("NonExistentPage", func(t *testing.T) {
			_, err := ws.UpdatePageFrontmatter(ctx, ksid.NewID(), "🎨", "cover.jpg", author)
			if err == nil {
				t.Error("expected error for non-existent page")
			}
		})
	})

	t.Run("PageVersionHistory", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()
		nodeID := ksid.NewID()

		if _, err := writePageFixture(ws, ctx, nodeID, "Initial", "initial content", author); err != nil {
			t.Fatalf("failed to write initial page: %v", err)
		}
		if _, err := ws.UpdatePage(ctx, nodeID, "Updated", "updated content", pageRevision(t, ctx, ws, nodeID), author); err != nil {
			t.Fatalf("failed to update page: %v", err)
		}

		history, err := ws.GetHistory(ctx, nodeID, 10)
		if err != nil {
			t.Fatalf("failed to get history: %v", err)
		}
		if len(history) < 2 {
			t.Errorf("expected at least 2 commits, got %d", len(history))
		}
	})

	t.Run("NestedPageVersionHistory", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		parent, err := ws.CreateNode(ctx, "Parent", NodeTypeDocument, 0, author)
		if err != nil {
			t.Fatalf("failed to create parent: %v", err)
		}
		child, err := ws.CreatePageUnderParent(ctx, parent.ID, "Child", "initial content", author)
		if err != nil {
			t.Fatalf("failed to create child: %v", err)
		}

		for i := 1; i <= 3; i++ {
			if _, err := ws.UpdatePage(ctx, child.ID, "Child", "content v"+string(rune(48+i)), pageRevision(t, ctx, ws, child.ID), author); err != nil {
				t.Fatalf("failed to update child page (v%d): %v", i, err)
			}
		}

		history, err := ws.GetHistory(ctx, child.ID, 10)
		if err != nil {
			t.Fatalf("failed to get history for nested page: %v", err)
		}
		if len(history) < 4 {
			t.Errorf("expected at least 4 commits for nested page, got %d", len(history))
		}

		foundChildCommit := false
		for _, commit := range history {
			if strings.Contains(commit.Message, child.ID.String()) {
				foundChildCommit = true
				break
			}
		}
		if !foundChildCommit {
			t.Errorf("history should contain commits for child page %s", child.ID)
		}
	})

	t.Run("NodeTree", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		var zeroID ksid.ID
		node1, err := ws.CreateNode(ctx, "Node 1", NodeTypeDocument, zeroID, author)
		if err != nil {
			t.Fatalf("failed to create node 1: %v", err)
		}
		if node1.ID.IsZero() {
			t.Errorf("expected node1 to have non-zero ID")
		}

		node2, err := ws.CreateNode(ctx, "Node 2", NodeTypeTable, node1.ID, author)
		if err != nil {
			t.Fatalf("failed to create node 2: %v", err)
		}

		if _, err = ws.ReadNode(t.Context(), 0); err == nil {
			t.Fatalf("expected error reading node 0, got nil")
		}

		readNode1, err := ws.ReadNode(t.Context(), node1.ID)
		if err != nil {
			t.Fatalf("failed to read node 1: %v", err)
		}
		if readNode1.Title != "Node 1" {
			t.Errorf("expected title 'Node 1', got %q", readNode1.Title)
		}

		topLevel, err := ws.ListChildren(0)
		if err != nil {
			t.Fatalf("failed to list children of 0: %v", err)
		}
		if len(topLevel) != 1 || topLevel[0].ID != node1.ID {
			t.Errorf("expected [node1] in ListChildren(0), got %v", topLevel)
		}

		children, err := ws.ListChildren(node1.ID)
		if err != nil {
			t.Fatalf("failed to list children: %v", err)
		}
		if len(children) != 1 || children[0].ID != node2.ID {
			t.Errorf("expected [node2], got %v", children)
		}
	})

	t.Run("ListChildren_ReturnsOnlyTopLevelNodes", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		var zeroID ksid.ID
		topLevel, err := ws.CreateNode(ctx, "Top Level", NodeTypeDocument, zeroID, author)
		if err != nil {
			t.Fatalf("failed to create top-level: %v", err)
		}
		child, err := ws.CreateNode(ctx, "Child", NodeTypeDocument, topLevel.ID, author)
		if err != nil {
			t.Fatalf("failed to create child: %v", err)
		}
		grandchild, err := ws.CreateNode(ctx, "Grandchild", NodeTypeDocument, child.ID, author)
		if err != nil {
			t.Fatalf("failed to create grandchild: %v", err)
		}

		topLevelNodes, err := ws.ListChildren(0)
		if err != nil {
			t.Fatalf("failed to list children of 0: %v", err)
		}
		if len(topLevelNodes) != 1 {
			t.Errorf("expected 1 top-level node, got %d", len(topLevelNodes))
		}
		if len(topLevelNodes) > 0 && topLevelNodes[0].ID != topLevel.ID {
			t.Errorf("expected topLevel node, got %v", topLevelNodes[0].ID)
		}
		if !topLevelNodes[0].ParentID.IsZero() {
			t.Errorf("expected top-level node to have zero ParentID, got %v", topLevelNodes[0].ParentID)
		}
		if !topLevelNodes[0].HasChildren {
			t.Error("expected HasChildren to be true (node has children)")
		}
		if len(topLevelNodes[0].Children) != 0 {
			t.Errorf("expected Children to be empty (lazy load), got %d children", len(topLevelNodes[0].Children))
		}

		children, err := ws.ListChildren(topLevel.ID)
		if err != nil {
			t.Fatalf("failed to list children: %v", err)
		}
		if len(children) != 1 || children[0].ID != child.ID {
			t.Errorf("expected [child], got %v", children)
		}

		grandchildren, err := ws.ListChildren(child.ID)
		if err != nil {
			t.Fatalf("failed to list grandchildren: %v", err)
		}
		if len(grandchildren) != 1 || grandchildren[0].ID != grandchild.ID {
			t.Errorf("expected [grandchild], got %v", grandchildren)
		}
	})

	t.Run("TopLevelPages", func(t *testing.T) {
		fs, ws, wsID := initWS(t)
		ctx := t.Context()

		var zeroID ksid.ID
		page1, err := ws.CreateNode(ctx, "Welcome", NodeTypeDocument, zeroID, author)
		if err != nil {
			t.Fatalf("failed to create page 1: %v", err)
		}
		if page1.ID.IsZero() {
			t.Errorf("expected page1 to have non-zero ID")
		}

		page1Path := filepath.Join(fs.rootDir, wsID.String(), page1.ID.String(), "index.md")
		if _, err := os.Stat(page1Path); err != nil {
			t.Errorf("expected page at %s, got error: %v", page1Path, err)
		}

		page2, err := ws.CreateNode(ctx, "Page 2", NodeTypeDocument, zeroID, author)
		if err != nil {
			t.Fatalf("failed to create page 2: %v", err)
		}
		if page2.ID.IsZero() {
			t.Errorf("expected page2 to have non-zero ID")
		}

		page2Path := filepath.Join(fs.rootDir, wsID.String(), page2.ID.String(), "index.md")
		if _, err := os.Stat(page2Path); err != nil {
			t.Errorf("expected page at %s, got error: %v", page2Path, err)
		}

		if _, err = ws.ReadNode(t.Context(), 0); err == nil {
			t.Fatalf("expected error reading node 0")
		}

		readPage1, err := ws.ReadNode(t.Context(), page1.ID)
		if err != nil {
			t.Fatalf("failed to read page1: %v", err)
		}
		if readPage1.Title != "Welcome" {
			t.Errorf("expected title 'Welcome', got %q", readPage1.Title)
		}

		topLevel, err := ws.ListChildren(zeroID)
		if err != nil {
			t.Fatalf("failed to list children: %v", err)
		}
		if len(topLevel) != 2 {
			t.Errorf("expected 2 top-level pages, got %d", len(topLevel))
		}
	})

	t.Run("CreatePageUnderParent", func(t *testing.T) {
		fs, ws, wsID := initWS(t)
		ctx := t.Context()

		var zeroID ksid.ID
		page, err := ws.CreatePageUnderParent(ctx, zeroID, "My First Page", "", author)
		if err != nil {
			t.Fatalf("failed to create page: %v", err)
		}
		if page.ID.IsZero() {
			t.Errorf("expected page to have non-zero ID")
		}

		pagePath := filepath.Join(fs.rootDir, wsID.String(), page.ID.String(), "index.md")
		if _, err := os.Stat(pagePath); err != nil {
			t.Errorf("expected page at %s, got error: %v", pagePath, err)
		}

		readPage, err := ws.ReadNode(t.Context(), page.ID)
		if err != nil {
			t.Fatalf("failed to read page: %v", err)
		}
		if readPage.Title != "My First Page" {
			t.Errorf("expected title 'My First Page', got %q", readPage.Title)
		}

		if _, err := ws.DeletePage(ctx, page.ID, author); err != nil {
			t.Fatalf("failed to delete page: %v", err)
		}
		if _, err := os.Stat(pagePath); !os.IsNotExist(err) {
			t.Errorf("expected page to be deleted, but file still exists")
		}
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		t.Run("ReadNonExistentPage", func(t *testing.T) {
			if _, err := ws.ReadPage(ksid.NewID()); err == nil {
				t.Error("expected error reading non-existent page")
			}
		})

		t.Run("ReadNonExistentTable", func(t *testing.T) {
			if _, err := ws.ReadTable(ksid.NewID()); err == nil {
				t.Error("expected error reading non-existent table")
			}
		})

		t.Run("DeleteNonExistentPage", func(t *testing.T) {
			if _, err := ws.DeletePage(ctx, ksid.NewID(), author); err == nil {
				t.Error("expected error deleting non-existent page")
			}
		})

		t.Run("IterAssets_NonExistentPage", func(t *testing.T) {
			iter, err := ws.IterAssets(ksid.NewID())
			if err != nil {
				t.Fatalf("expected nil error for non-existent page, got: %v", err)
			}
			count := 0
			for range iter {
				count++
			}
			if count != 0 {
				t.Errorf("expected 0 assets for non-existent page, got %d", count)
			}
		})
	})

	t.Run("GitPathWithNestedPages", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		topLevel, err := ws.CreateNode(ctx, "Top Level", NodeTypeDocument, 0, author)
		if err != nil {
			t.Fatalf("failed to create top-level: %v", err)
		}
		child, err := ws.CreateNode(ctx, "Child", NodeTypeDocument, topLevel.ID, author)
		if err != nil {
			t.Fatalf("failed to create child: %v", err)
		}
		grandchild, err := ws.CreateNode(ctx, "Grandchild", NodeTypeDocument, child.ID, author)
		if err != nil {
			t.Fatalf("failed to create grandchild: %v", err)
		}

		topLevelPath := ws.gitPath(topLevel.ParentID, topLevel.ID, "index.md")
		expectedTopLevelPath := filepath.Join(topLevel.ID.String(), "index.md")
		if topLevelPath != expectedTopLevelPath {
			t.Errorf("topLevel gitPath mismatch: got %s, expected %s", topLevelPath, expectedTopLevelPath)
		}

		childPath := ws.gitPath(child.ParentID, child.ID, "index.md")
		expectedChildPath := filepath.Join(topLevel.ID.String(), child.ID.String(), "index.md")
		if childPath != expectedChildPath {
			t.Errorf("child gitPath mismatch:\n  got:      %s\n  expected: %s", childPath, expectedChildPath)
		}

		grandchildPath := ws.gitPath(grandchild.ParentID, grandchild.ID, "index.md")
		expectedGrandchildPath := filepath.Join(topLevel.ID.String(), child.ID.String(), grandchild.ID.String(), "index.md")
		if grandchildPath != expectedGrandchildPath {
			t.Errorf("grandchild gitPath mismatch:\n  got:      %s\n  expected: %s", grandchildPath, expectedGrandchildPath)
		}

		if _, err := os.Stat(filepath.Join(ws.wsDir, topLevel.ID.String(), "index.md")); err != nil {
			t.Errorf("expected top-level page at workspace/<id>/index.md: %v", err)
		}
		if _, err := os.Stat(filepath.Join(ws.wsDir, topLevel.ID.String(), child.ID.String(), "index.md")); err != nil {
			t.Errorf("expected child page at workspace/<topLevel>/<child>/index.md: %v", err)
		}
		if _, err := os.Stat(filepath.Join(ws.wsDir, topLevel.ID.String(), child.ID.String(), grandchild.ID.String(), "index.md")); err != nil {
			t.Errorf("expected grandchild page at workspace/<topLevel>/<child>/<grandchild>/index.md: %v", err)
		}
	})

	t.Run("MoveNode", func(t *testing.T) {
		t.Run("MoveToDifferentParent", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			a, err := ws.CreateNode(ctx, "A", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create A: %v", err)
			}
			b, err := ws.CreateNode(ctx, "B", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create B: %v", err)
			}
			child, err := ws.CreateNode(ctx, "Child", NodeTypeDocument, a.ID, author)
			if err != nil {
				t.Fatalf("create child: %v", err)
			}

			if _, err := ws.MoveNode(ctx, child.ID, b.ID, author); err != nil {
				t.Fatalf("move: %v", err)
			}

			children, err := ws.ListChildren(b.ID)
			if err != nil {
				t.Fatalf("list children of B: %v", err)
			}
			if len(children) != 1 || children[0].ID != child.ID {
				t.Errorf("expected child under B, got %v", children)
			}

			children, err = ws.ListChildren(a.ID)
			if err != nil {
				t.Fatalf("list children of A: %v", err)
			}
			if len(children) != 0 {
				t.Errorf("expected no children under A, got %d", len(children))
			}

			node, err := ws.ReadNode(t.Context(), child.ID)
			if err != nil {
				t.Fatalf("read child after move: %v", err)
			}
			if node.Title != "Child" {
				t.Errorf("expected title 'Child', got %q", node.Title)
			}
		})

		t.Run("MoveToRoot", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			parent, err := ws.CreateNode(ctx, "Parent", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create parent: %v", err)
			}
			child, err := ws.CreateNode(ctx, "Child", NodeTypeDocument, parent.ID, author)
			if err != nil {
				t.Fatalf("create child: %v", err)
			}

			if _, err := ws.MoveNode(ctx, child.ID, 0, author); err != nil {
				t.Fatalf("move to root: %v", err)
			}

			topLevel, err := ws.ListChildren(0)
			if err != nil {
				t.Fatalf("list root: %v", err)
			}
			found := false
			for _, n := range topLevel {
				if n.ID == child.ID {
					found = true
					break
				}
			}
			if !found {
				t.Error("expected child in top-level nodes after move to root")
			}
		})

		t.Run("CycleRejected", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			parent, err := ws.CreateNode(ctx, "Parent", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create parent: %v", err)
			}
			child, err := ws.CreateNode(ctx, "Child", NodeTypeDocument, parent.ID, author)
			if err != nil {
				t.Fatalf("create child: %v", err)
			}

			_, err = ws.MoveNode(ctx, parent.ID, child.ID, author)
			if !errors.Is(err, errCycleDetected) {
				t.Errorf("expected cycle error, got %v", err)
			}
		})

		t.Run("DescendantCycle", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			a, err := ws.CreateNode(ctx, "A", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create A: %v", err)
			}
			b, err := ws.CreateNode(ctx, "B", NodeTypeDocument, a.ID, author)
			if err != nil {
				t.Fatalf("create B: %v", err)
			}
			c, err := ws.CreateNode(ctx, "C", NodeTypeDocument, b.ID, author)
			if err != nil {
				t.Fatalf("create C: %v", err)
			}

			_, err = ws.MoveNode(ctx, a.ID, c.ID, author)
			if !errors.Is(err, errCycleDetected) {
				t.Errorf("expected cycle error for descendant, got %v", err)
			}
		})

		t.Run("NoOpSameParent", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			parent, err := ws.CreateNode(ctx, "Parent", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create parent: %v", err)
			}
			child, err := ws.CreateNode(ctx, "Child", NodeTypeDocument, parent.ID, author)
			if err != nil {
				t.Fatalf("create child: %v", err)
			}

			if _, err := ws.MoveNode(ctx, child.ID, parent.ID, author); err != nil {
				t.Fatalf("no-op move: %v", err)
			}

			children, err := ws.ListChildren(parent.ID)
			if err != nil {
				t.Fatalf("list children: %v", err)
			}
			if len(children) != 1 || children[0].ID != child.ID {
				t.Errorf("expected child still under parent, got %v", children)
			}
		})

		t.Run("ChildrenAccessibleAfterMove", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			a, err := ws.CreateNode(ctx, "A", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create A: %v", err)
			}
			b, err := ws.CreateNode(ctx, "B", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create B: %v", err)
			}
			child, err := ws.CreateNode(ctx, "Child", NodeTypeDocument, a.ID, author)
			if err != nil {
				t.Fatalf("create child: %v", err)
			}
			grandchild, err := ws.CreateNode(ctx, "Grandchild", NodeTypeDocument, child.ID, author)
			if err != nil {
				t.Fatalf("create grandchild: %v", err)
			}

			if _, err := ws.MoveNode(ctx, child.ID, b.ID, author); err != nil {
				t.Fatalf("move: %v", err)
			}

			node, err := ws.ReadNode(t.Context(), grandchild.ID)
			if err != nil {
				t.Fatalf("read grandchild after move: %v", err)
			}
			if node.Title != "Grandchild" {
				t.Errorf("expected title 'Grandchild', got %q", node.Title)
			}

			gc, err := ws.ListChildren(child.ID)
			if err != nil {
				t.Fatalf("list grandchild: %v", err)
			}
			if len(gc) != 1 || gc[0].ID != grandchild.ID {
				t.Errorf("expected grandchild under child, got %v", gc)
			}
		})

		t.Run("MoveNonExistent", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			_, err := ws.MoveNode(ctx, ksid.NewID(), 0, author)
			if err == nil {
				t.Error("expected error moving non-existent node")
			}
		})

		t.Run("MoveToNonExistentParent", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			node, err := ws.CreateNode(ctx, "Node", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create node: %v", err)
			}

			_, err = ws.MoveNode(ctx, node.ID, ksid.NewID(), author)
			if err == nil {
				t.Error("expected error moving to non-existent parent")
			}
		})

		t.Run("PreservesLinks", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			pageA, err := ws.CreatePageUnderParent(ctx, 0, "A", "", author)
			if err != nil {
				t.Fatalf("create A: %v", err)
			}
			pageB, err := ws.CreatePageUnderParent(ctx, 0, "B", "", author)
			if err != nil {
				t.Fatalf("create B: %v", err)
			}
			pageC, err := ws.CreatePageUnderParent(ctx, pageA.ID, "C", "", author)
			if err != nil {
				t.Fatalf("create C: %v", err)
			}

			cLink := fmt.Sprintf("See [B](../%s/index.md)", pageB.ID)
			if _, err := ws.UpdatePage(ctx, pageC.ID, "C", cLink, pageRevision(t, ctx, ws, pageC.ID), author); err != nil {
				t.Fatalf("update C: %v", err)
			}
			bLink := fmt.Sprintf("See [C](../%s/index.md)", pageC.ID)
			if _, err := ws.UpdatePage(ctx, pageB.ID, "B", bLink, pageRevision(t, ctx, ws, pageB.ID), author); err != nil {
				t.Fatalf("update B: %v", err)
			}

			if _, err := ws.MoveNode(ctx, pageC.ID, 0, author); err != nil {
				t.Fatalf("move C: %v", err)
			}

			nodeC, err := ws.ReadPage(pageC.ID)
			if err != nil {
				t.Fatalf("read C: %v", err)
			}
			if nodeC.Content != cLink {
				t.Errorf("C link should be preserved, want %q, got %q", cLink, nodeC.Content)
			}

			nodeB, err := ws.ReadPage(pageB.ID)
			if err != nil {
				t.Fatalf("read B: %v", err)
			}
			if nodeB.Content != bLink {
				t.Errorf("B link should be preserved, want %q, got %q", bLink, nodeB.Content)
			}
		})
	})

	t.Run("WorkspaceAdmission", func(t *testing.T) {
		t.Run("returned large integer revision survives restart", func(t *testing.T) {
			svc, ws, id := initWS(t)
			node, err := ws.CreateTableUnderParent(t.Context(), 0, "Numbers", []contentv1.Property{{Name: "number", Type: contentv1.PropertyTypeNumber}}, author)
			if err != nil {
				t.Fatal(err)
			}
			node, err = ws.ReadTable(node.ID)
			if err != nil {
				t.Fatal(err)
			}
			record := &contentv1.DataRecord{ID: ksid.NewID(), Data: map[string]any{"number": "1"}, Created: storagev1.Now(), Modified: storagev1.Now()}
			if err := ws.AppendRecord(t.Context(), node.ID, record, author); err != nil {
				t.Fatal(err)
			}
			revision, err := RecordRevision(record)
			if err != nil {
				t.Fatal(err)
			}
			saved, err := ws.ReplaceRecord(t.Context(), node.ID, record.ID, map[string]any{"number": "9007199254740993"}, revision, node.SchemaRevision, author)
			if err != nil {
				t.Fatal(err)
			}
			revision, err = RecordRevision(saved)
			if err != nil {
				t.Fatal(err)
			}
			restarted, err := NewFileStoreService(svc.rootDir, git.NewManager(svc.rootDir, "Test", "test@example.com"), svc.wsSvc, svc.orgSvc, svc.settings)
			if err != nil {
				t.Fatal(err)
			}
			store, err := restarted.GetWorkspaceStore(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := store.ReadRecordsPage(node.ID, 0, 1)
			if err != nil {
				t.Fatal(err)
			}
			if rows[0].Data["number"] != int64(9007199254740993) {
				t.Fatalf("lost persisted integer: %#v", rows[0].Data)
			}
			if _, err := store.ReplaceRecord(t.Context(), node.ID, record.ID, map[string]any{"number": "1"}, revision, node.SchemaRevision, author); err != nil {
				t.Fatalf("returned revision rejected after restart: %v", err)
			}
		})
		t.Run("serialized page replacements and recovery", func(t *testing.T) {
			_, ws, _ := initWS(t)
			n, err := ws.CreatePageUnderParent(t.Context(), 0, "Page", strings.Repeat("a", 200), author)
			if err != nil {
				t.Fatal(err)
			}
			_, used, err := ws.GetWorkspaceUsage()
			if err != nil {
				t.Fatal(err)
			}
			q := ws.EffectiveQuotas()
			q.MaxStorageBytes = used
			ws.quotas = &q
			snap, err := ws.ReadPageSnapshot(t.Context(), n.ID, 0, 4096)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ws.AppendPage(t.Context(), n.ID, "growth", snap.Revision, author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("append at ceiling: %v", err)
			}
			if _, err := ws.UpdatePageFrontmatter(t.Context(), n.ID, "large icon", "cover", author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("front matter growth: %v", err)
			}
			if _, err := ws.UpdatePage(t.Context(), n.ID, "a much longer title", strings.Repeat("a", 200), snap.Revision, author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("serialized header not charged: %v", err)
			}
			if _, err := ws.UpdatePage(t.Context(), n.ID, "Page", "stale", "wrong revision", author); !errors.Is(err, ErrRevisionConflict) {
				t.Fatalf("stale write: %v", err)
			}
			_, after, err := ws.GetWorkspaceUsage()
			if err != nil || after != used {
				t.Fatalf("rejection changed usage: %d -> %d (%v)", used, after, err)
			}
			q.MaxStorageBytes = 0
			q.MaxPages = 0
			n, err = ws.UpdatePage(t.Context(), n.ID, "Page", strings.Repeat("b", 200), snap.Revision, author)
			if err != nil {
				t.Fatalf("equal replacement over zero ceiling: %v", err)
			}
			n, err = ws.UpdatePage(t.Context(), n.ID, "Page", "short", n.Revision, author)
			if err != nil {
				t.Fatalf("shrinking recovery: %v", err)
			}
			if _, err := ws.CreatePageUnderParent(t.Context(), 0, "blocked", "", author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("new page over zero count: %v", err)
			}
			if _, err := ws.DeletePage(t.Context(), n.ID, author); err != nil {
				t.Fatal(err)
			}
		})
		t.Run("inherited zero permits existing shrinking content", func(t *testing.T) {
			fs, ws, wsID := initWS(t)
			n, err := ws.CreatePageUnderParent(t.Context(), 0, "Inherited", strings.Repeat("a", 100), author)
			if err != nil {
				t.Fatal(err)
			}
			w, err := fs.wsSvc.Get(wsID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fs.orgSvc.Modify(w.OrganizationID, func(o *identityv1.Organization) error {
				o.Quotas.MaxPages = 0
				o.Quotas.MaxStorageBytes = 0
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := fs.wsSvc.Modify(wsID, func(w *identityv1.Workspace) error { w.Quotas.MaxPages = -1; w.Quotas.MaxStorageBytes = -1; return nil }); err != nil {
				t.Fatal(err)
			}
			fs.InvalidateWorkspaceStore(wsID)
			ws, err = fs.GetWorkspaceStore(t.Context(), wsID)
			if err != nil {
				t.Fatal(err)
			}
			if ws.EffectiveQuotas().MaxPages != 0 || ws.EffectiveQuotas().MaxStorageBytes != 0 {
				t.Fatal("zero not inherited")
			}
			snap, err := ws.ReadPageSnapshot(t.Context(), n.ID, 0, 4096)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ws.UpdatePage(t.Context(), n.ID, "Inherited", "short", snap.Revision, author); err != nil {
				t.Fatal(err)
			}
			if _, err := ws.CreatePageUnderParent(t.Context(), 0, "blocked", "", author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("inherited zero creation: %v", err)
			}
		})

		t.Run("table ceilings and schema recovery", func(t *testing.T) {
			_, ws, _ := initWS(t)
			q := ws.EffectiveQuotas()
			q.MaxPages = 0
			q.MaxTablesPerWorkspace = 1
			q.MaxColumnsPerTable = 2
			ws.quotas = &q
			n, err := ws.CreateTableUnderParent(t.Context(), 0, "Table", []contentv1.Property{{Name: "a", Type: "text"}, {Name: "b", Type: "text"}}, author)
			if err != nil {
				t.Fatalf("table uses independent count: %v", err)
			}
			if _, err := ws.CreateNode(t.Context(), "extra", NodeTypeTable, 0, author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("node table bypassed count: %v", err)
			}
			q.MaxColumnsPerTable = 0
			n.Properties = n.Properties[:1]
			if err := ws.WriteTable(t.Context(), n, false, author); err != nil {
				t.Fatalf("schema shrink: %v", err)
			}
			n.Properties = append(n.Properties, contentv1.Property{Name: "new", Type: "text"})
			if err := ws.WriteTable(t.Context(), n, false, author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("schema growth over zero: %v", err)
			}
			_, used, err := ws.GetWorkspaceUsage()
			if err != nil {
				t.Fatal(err)
			}
			q.MaxStorageBytes = used
			n.Properties = n.Properties[:1]
			n.Title = strings.Repeat("large", 100)
			if err := ws.WriteTable(t.Context(), n, false, author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("metadata growth: %v", err)
			}
			if err := ws.DeleteTable(t.Context(), n.ID, author); err != nil {
				t.Fatal(err)
			}
		})
		t.Run("first direct JSONL append consumes a table slot", func(t *testing.T) {
			_, ws, _ := initWS(t)
			n, err := ws.CreatePageUnderParent(t.Context(), 0, "Page", "", author)
			if err != nil {
				t.Fatal(err)
			}
			q := ws.EffectiveQuotas()
			q.MaxTablesPerWorkspace = 0
			ws.quotas = &q
			if err := ws.AppendRecord(t.Context(), n.ID, &contentv1.DataRecord{ID: ksid.NewID()}, author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("implicit table bypass: %v", err)
			}
			if _, err := os.Stat(ws.tableRecordsFile(n.ID, 0)); !os.IsNotExist(err) {
				t.Fatalf("rejected append left data file: %v", err)
			}
		})

		t.Run("metadata shares an existing JSONL table slot", func(t *testing.T) {
			_, ws, _ := initWS(t)
			n, err := ws.CreatePageUnderParent(t.Context(), 0, "Hybrid", "", author)
			if err != nil {
				t.Fatal(err)
			}
			q := ws.EffectiveQuotas()
			q.MaxTablesPerWorkspace = 1
			ws.quotas = &q
			if err := ws.AppendRecord(t.Context(), n.ID, &contentv1.DataRecord{ID: ksid.NewID()}, author); err != nil {
				t.Fatal(err)
			}
			_, used, err := ws.GetWorkspaceUsage()
			if err != nil {
				t.Fatal(err)
			}
			limit := q.MaxStorageBytes
			q.MaxStorageBytes = used
			if err := ws.WriteTable(t.Context(), &Node{ID: n.ID, Title: "Hybrid", Type: NodeTypeTable}, true, author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("metadata bytes bypassed admission: %v", err)
			}
			if _, err := os.Stat(ws.tableMetadataFile(n.ID, 0)); !os.IsNotExist(err) {
				t.Fatalf("rejected metadata published: %v", err)
			}
			q.MaxStorageBytes = limit
			if err := ws.WriteTable(t.Context(), &Node{ID: n.ID, Title: "Hybrid", Type: NodeTypeTable}, true, author); err != nil {
				t.Fatalf("existing JSONL table charged twice: %v", err)
			}
			u, err := ws.scanUsage()
			if err != nil || u.tables != 1 {
				t.Fatalf("table count=%d error=%v", u.tables, err)
			}
		})
		t.Run("hybrid combined count and byte ceilings", func(t *testing.T) {
			fixed := time.Now().Truncate(time.Second).Add(123 * time.Millisecond)
			synctest.Test(t, func(t *testing.T) {
				// Pin serialized timestamps after the KSID epoch without wall-clock waits.
				time.Sleep(time.Until(fixed))
				_, ws, _ := initWS(t)
				_, before, err := ws.GetWorkspaceUsage()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := ws.CreateNode(t.Context(), "Hybrid", NodeTypeHybrid, 0, author); err != nil {
					t.Fatal(err)
				}
				_, used, err := ws.GetWorkspaceUsage()
				if err != nil {
					t.Fatal(err)
				}
				unit := used - before
				q := ws.EffectiveQuotas()
				ws.quotas = &q
				for _, counts := range [][2]int{{1, 2}, {2, 1}} {
					q.MaxPages = counts[0]
					q.MaxTablesPerWorkspace = counts[1]
					if _, err := ws.CreateNode(t.Context(), "Hybrid", NodeTypeHybrid, 0, author); !errors.Is(err, errQuotaExceeded) {
						t.Fatalf("hybrid count ceilings %v: %v", counts, err)
					}
				}
				q.MaxPages = 2
				q.MaxTablesPerWorkspace = 2
				q.MaxStorageBytes = used + unit - 1
				if _, err := ws.CreateNode(t.Context(), "Hybrid", NodeTypeHybrid, 0, author); !errors.Is(err, errQuotaExceeded) {
					t.Fatalf("combined hybrid byte ceiling: %v", err)
				}
				u, err := ws.scanUsage()
				if err != nil || u.pages != 1 || u.tables != 1 || u.bytes != used {
					t.Fatalf("rejected hybrid published partial data: %+v (%v)", u, err)
				}
				q.MaxStorageBytes = used + unit
				if _, err := ws.CreateNode(t.Context(), "Hybrid", NodeTypeHybrid, 0, author); err != nil {
					t.Fatalf("hybrid at exact byte ceiling: %v", err)
				}
			})
		})
		t.Run("view replacement and shrinking recovery", func(t *testing.T) {
			fixed := time.Now().Truncate(time.Second).Add(123 * time.Millisecond)
			synctest.Test(t, func(t *testing.T) {
				// Pin serialized timestamps after the KSID epoch without wall-clock waits.
				time.Sleep(time.Until(fixed))
				_, ws, _ := initWS(t)
				n, err := ws.CreateTableUnderParent(t.Context(), 0, "Views", nil, author)
				if err != nil {
					t.Fatal(err)
				}
				_, used, err := ws.GetWorkspaceUsage()
				if err != nil {
					t.Fatal(err)
				}
				q := ws.EffectiveQuotas()
				q.MaxStorageBytes = used
				ws.quotas = &q
				n.Views = []contentv1.View{{ID: ksid.NewID(), Name: "Long view name", Type: contentv1.ViewTypeTable}}
				if err := ws.WriteTable(t.Context(), n, false, author); !errors.Is(err, errQuotaExceeded) {
					t.Fatalf("view growth: %v", err)
				}
				stored, err := ws.ReadTable(n.ID)
				if err != nil || len(stored.Views) != 0 {
					t.Fatalf("rejected view persisted: %+v (%v)", stored, err)
				}
				q.MaxStorageBytes = 1 << 30
				if err := ws.WriteTable(t.Context(), n, false, author); err != nil {
					t.Fatal(err)
				}
				_, used, err = ws.GetWorkspaceUsage()
				if err != nil {
					t.Fatal(err)
				}
				q.MaxStorageBytes = used
				n.Views[0].Name = "Same size name"
				if err := ws.WriteTable(t.Context(), n, false, author); err != nil {
					t.Fatalf("equal view replacement: %v", err)
				}
				q.MaxStorageBytes = 0
				n.Views[0].Name = "Short"
				if err := ws.WriteTable(t.Context(), n, false, author); err != nil {
					t.Fatalf("view shrinking recovery: %v", err)
				}
			})
		})

		t.Run("exact JSONL bytes and record recovery", func(t *testing.T) {
			_, ws, _ := initWS(t)
			n, err := ws.CreateTableUnderParent(t.Context(), 0, "Records", nil, author)
			if err != nil {
				t.Fatal(err)
			}
			r := &contentv1.DataRecord{ID: ksid.NewID(), Data: map[string]any{"text": strings.Repeat("a", 200)}, Created: storagev1.Now(), Modified: storagev1.Time(1_000_000_000_999)}
			_, used, err := ws.GetWorkspaceUsage()
			if err != nil {
				t.Fatal(err)
			}
			row, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			q := ws.EffectiveQuotas()
			q.MaxStorageBytes = used + int64(len(row))
			ws.quotas = &q
			if err := ws.AppendRecord(t.Context(), n.ID, r, author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("header/newlines uncharged: %v", err)
			}
			if _, err := os.Stat(ws.tableRecordsFile(n.ID, 0)); !os.IsNotExist(err) {
				t.Fatalf("rejected first append left table: %v", err)
			}
			q.MaxStorageBytes = 1 << 30
			if err := ws.AppendRecord(t.Context(), n.ID, r, author); err != nil {
				t.Fatal(err)
			}
			_, used, err = ws.GetWorkspaceUsage()
			if err != nil {
				t.Fatal(err)
			}
			q.MaxStorageBytes = used
			update := func() error {
				records, err := ws.ReadRecordsPage(n.ID, 0, 1)
				if err != nil {
					return err
				}
				return replaceTestRecord(t, t.Context(), ws, n.ID, records[0], r.Data, author)
			}
			r.Data["text"] = strings.Repeat("b", 200)
			if err := update(); err != nil {
				t.Fatalf("equal record: %v", err)
			}
			published, err := os.ReadFile(ws.tableRecordsFile(n.ID, 0))
			if err != nil {
				t.Fatal(err)
			}
			r.Data["text"] = strings.Repeat("long", 200)
			if err := update(); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("record growth: %v", err)
			}
			after, err := os.ReadFile(ws.tableRecordsFile(n.ID, 0))
			if err != nil || !bytes.Equal(after, published) {
				t.Fatalf("rejected staged JSONL changed publication (%v)", err)
			}
			staging, err := filepath.Glob(filepath.Join(ws.pageDir(n.ID, 0), ".jsonldb-*"))
			if err != nil || len(staging) != 0 {
				t.Fatalf("retained JSONL staging: %v (%v)", staging, err)
			}
			q.MaxRecordsPerTable = 0
			q.MaxStorageBytes = 0
			r.Data["text"] = "short"
			if err := update(); err != nil {
				t.Fatalf("record shrinking: %v", err)
			}
			if err := ws.AppendRecord(t.Context(), n.ID, &contentv1.DataRecord{ID: ksid.NewID()}, author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("record count: %v", err)
			}
			records, err := ws.ReadRecordsPage(n.ID, 0, 1)
			if err != nil {
				t.Fatal(err)
			}
			revision, err := RecordRevision(records[0])
			if err != nil {
				t.Fatal(err)
			}
			if err := ws.DeleteRecord(t.Context(), n.ID, r.ID, revision, author); err != nil {
				t.Fatal(err)
			}
		})
		t.Run("asset net replacement and recovery", func(t *testing.T) {
			_, ws, _ := initWS(t)
			n, err := ws.CreatePageUnderParent(t.Context(), 0, "Assets", "", author)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ws.SaveAsset(t.Context(), n.ID, "file.txt", bytes.NewReader([]byte("abcd")), int64(len([]byte("abcd"))), author); err != nil {
				t.Fatal(err)
			}
			q := ws.EffectiveQuotas()
			q.MaxAssetSizeBytes = 0
			q.MaxStorageBytes = 0
			ws.quotas = &q
			for _, data := range []string{"efgh", "abc"} {
				if _, err := ws.SaveAsset(t.Context(), n.ID, "file.txt", bytes.NewReader([]byte(data)), int64(len([]byte(data))), author); err != nil {
					t.Fatalf("replacement %q: %v", data, err)
				}
			}
			if _, err := ws.SaveAsset(t.Context(), n.ID, "file.txt", bytes.NewReader([]byte("growth")), int64(len([]byte("growth"))), author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("asset growth: %v", err)
			}
			data, err := ws.ReadAsset(n.ID, "file.txt")
			if err != nil || string(data) != "abc" {
				t.Fatalf("quota rejection replaced asset: %q (%v)", data, err)
			}
			if err := ws.DeleteAsset(t.Context(), n.ID, "file.txt", author); err != nil {
				t.Fatal(err)
			}
		})
		t.Run("asset stream lengths and early admission", func(t *testing.T) {
			_, ws, _ := initWS(t)
			n, err := ws.CreatePageUnderParent(t.Context(), 0, "Streams", "", author)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ws.SaveAsset(t.Context(), n.ID, "file.txt", strings.NewReader("old"), 3, author); err != nil {
				t.Fatal(err)
			}
			q := ws.EffectiveQuotas()
			q.MaxAssetSizeBytes = 0
			ws.quotas = &q
			reader := strings.NewReader("too large")
			if _, err := ws.SaveAsset(t.Context(), n.ID, "new.txt", reader, int64(reader.Len()), author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("stream quota: %v", err)
			}
			if reader.Len() != len("too large") {
				t.Fatal("rejected stream was read before admission")
			}
			if _, err := ws.SaveAsset(t.Context(), n.ID, "file.txt", strings.NewReader("oversized"), 2, author); err == nil {
				t.Fatal("oversized stream succeeded")
			}
			if _, err := ws.SaveAsset(t.Context(), n.ID, "file.txt", strings.NewReader("x"), 2, author); err == nil {
				t.Fatal("short stream succeeded")
			}
			data, err := ws.ReadAsset(n.ID, "file.txt")
			if err != nil || string(data) != "old" {
				t.Fatalf("failed stream replaced prior publication: %q (%v)", data, err)
			}
			staging, err := filepath.Glob(filepath.Join(ws.pageDir(n.ID, 0), storage.AtomicFilePrefix+"*"))
			if err != nil || len(staging) != 0 {
				t.Fatalf("retained stream staging: %v (%v)", staging, err)
			}
		})

		t.Run("failed final asset bytes preserve the storage publication", func(t *testing.T) {
			_, ws, _ := initWS(t)
			n, err := ws.CreatePageUnderParent(t.Context(), 0, "Stream errors", "", author)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ws.SaveAsset(t.Context(), n.ID, "file.txt", strings.NewReader("old"), 3, author); err != nil {
				t.Fatal(err)
			}
			failure := errors.New("asset source final-byte failure")
			source := &assetFinalErrorReader{data: []byte("new"), err: failure}
			if _, err := ws.SaveAsset(t.Context(), n.ID, "file.txt", source, 3, author); !errors.Is(err, failure) {
				t.Fatalf("storage owner lost source error: %v", err)
			}
			data, err := ws.ReadAsset(n.ID, "file.txt")
			if err != nil || string(data) != "old" {
				t.Fatalf("failed asset stream published: %q (%v)", data, err)
			}
			staging, err := filepath.Glob(filepath.Join(ws.pageDir(n.ID, 0), storage.AtomicFilePrefix+"*"))
			if err != nil || len(staging) != 0 {
				t.Fatalf("retained failed-stream staging: %v (%v)", staging, err)
			}
		})

		t.Run("store replacements share last-byte authority", func(t *testing.T) {
			fs, initial, wsID := initWS(t)
			n, err := initial.CreatePageUnderParent(t.Context(), 0, "Concurrent", "", author)
			if err != nil {
				t.Fatal(err)
			}
			_, used, err := initial.GetWorkspaceUsage()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fs.wsSvc.Modify(wsID, func(w *identityv1.Workspace) error { w.Quotas.MaxStorageBytes = used + 1; return nil }); err != nil {
				t.Fatal(err)
			}
			fs.InvalidateWorkspaceStore(wsID)
			first, err := fs.GetWorkspaceStore(t.Context(), wsID)
			if err != nil {
				t.Fatal(err)
			}
			fs.InvalidateWorkspaceStore(wsID)
			second, err := fs.GetWorkspaceStore(t.Context(), wsID)
			if err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			for i, store := range []*WorkspaceFileStore{first, second} {
				go func() {
					<-start
					_, err := store.SaveAsset(t.Context(), n.ID, fmt.Sprintf("%d.txt", i), bytes.NewReader([]byte("x")), int64(len([]byte("x"))), author)
					results <- err
				}()
			}
			close(start)
			successes := 0
			for range 2 {
				err := <-results
				if err == nil {
					successes++
				} else if !errors.Is(err, errQuotaExceeded) {
					t.Fatal(err)
				}
			}
			if successes != 1 {
				t.Fatalf("last-byte successful writers: %d", successes)
			}
		})
	})

	t.Run("Quotas", func(t *testing.T) {
		t.Run("PageQuota", func(t *testing.T) {
			fs, _, wsID := initWS(t)
			ctx := t.Context()

			if _, err := fs.wsSvc.Modify(wsID, func(w *identityv1.Workspace) error {
				w.Quotas.MaxPages = 2
				return nil
			}); err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			// Re-get store after quota change.
			fs.InvalidateWorkspaceStore(wsID)
			ws, err := fs.GetWorkspaceStore(ctx, wsID)
			if err != nil {
				t.Fatalf("failed to get workspace store: %v", err)
			}

			for _, title := range []string{"Page 1", "Page 2"} {
				if _, err := ws.CreatePageUnderParent(ctx, 0, title, "content", author); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := ws.CreatePageUnderParent(ctx, 0, "Page 3", "content", author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("page count not enforced: %v", err)
			}
		})

		t.Run("StorageQuota", func(t *testing.T) {
			fs, _, wsID := initWS(t)
			ctx := t.Context()

			if _, err := fs.wsSvc.Modify(wsID, func(w *identityv1.Workspace) error {
				w.Quotas.MaxStorageBytes = 1024 * 1024 // 1 MB
				return nil
			}); err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			fs.InvalidateWorkspaceStore(wsID)
			ws, err := fs.GetWorkspaceStore(ctx, wsID)
			if err != nil {
				t.Fatalf("failed to get workspace store: %v", err)
			}

			largeContent := make([]byte, 2*1024*1024)
			for i := range len(largeContent) {
				largeContent[i] = byte('x')
			}

			if _, err := ws.CreatePageUnderParent(ctx, 0, "Large", string(largeContent), author); !errors.Is(err, errQuotaExceeded) {
				t.Fatalf("storage quota not enforced: %v", err)
			}
		})

		t.Run("RecordQuota", func(t *testing.T) {
			fs, _, wsID := initWS(t)
			ctx := t.Context()

			if _, err := fs.wsSvc.Modify(wsID, func(w *identityv1.Workspace) error {
				w.Quotas.MaxRecordsPerTable = 5
				return nil
			}); err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			fs.InvalidateWorkspaceStore(wsID)
			ws, err := fs.GetWorkspaceStore(ctx, wsID)
			if err != nil {
				t.Fatalf("failed to get workspace store: %v", err)
			}

			tableID := ksid.NewID()
			tableNode := &Node{
				ID:       tableID,
				Title:    "Test",
				Type:     NodeTypeTable,
				Created:  storagev1.Now(),
				Modified: storagev1.Now(),
			}
			if err := ws.WriteTable(ctx, tableNode, true, author); err != nil {
				t.Fatalf("failed to create table: %v", err)
			}

			for i := range 5 {
				rec := &contentv1.DataRecord{
					ID:       ksid.NewID(),
					Data:     map[string]any{"name": "Record"},
					Created:  storagev1.Now(),
					Modified: storagev1.Now(),
				}
				if err := ws.AppendRecord(ctx, tableID, rec, author); err != nil {
					t.Fatalf("failed to create record %d: %v", i, err)
				}
			}

			rec := &contentv1.DataRecord{
				ID:       ksid.NewID(),
				Data:     map[string]any{"name": "Extra"},
				Created:  storagev1.Now(),
				Modified: storagev1.Now(),
			}
			if err := ws.AppendRecord(ctx, tableID, rec, author); err == nil {
				t.Error("expected record quota exceeded error")
			}
		})

		t.Run("UpdateRecord_SameSizeAllowed", func(t *testing.T) {
			fs, _, wsID := initWS(t)
			ctx := t.Context()

			if _, err := fs.wsSvc.Modify(wsID, func(w *identityv1.Workspace) error {
				w.Quotas.MaxStorageBytes = 1024 * 1024 // 1MB
				return nil
			}); err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			fs.InvalidateWorkspaceStore(wsID)
			ws, err := fs.GetWorkspaceStore(ctx, wsID)
			if err != nil {
				t.Fatalf("failed to get workspace store: %v", err)
			}

			tableID := ksid.NewID()
			tableNode := &Node{
				ID:       tableID,
				Title:    "Test",
				Type:     NodeTypeTable,
				Created:  storagev1.Now(),
				Modified: storagev1.Now(),
			}
			if err := ws.WriteTable(ctx, tableNode, true, author); err != nil {
				t.Fatalf("failed to create table: %v", err)
			}

			recordID := ksid.NewID()
			record := &contentv1.DataRecord{
				ID:       recordID,
				Data:     map[string]any{"field": strings.Repeat("a", 100)},
				Created:  storagev1.Now(),
				Modified: storagev1.Now(),
			}
			if err := ws.AppendRecord(ctx, tableID, record, author); err != nil {
				t.Fatalf("failed to create record: %v", err)
			}

			_, storageUsage, _ := ws.GetWorkspaceUsage()
			if _, err := fs.wsSvc.Modify(wsID, func(w *identityv1.Workspace) error {
				w.Quotas.MaxStorageBytes = storageUsage
				return nil
			}); err != nil {
				t.Fatalf("failed to reduce quota: %v", err)
			}

			updatedRecord := &contentv1.DataRecord{
				ID:       recordID,
				Data:     map[string]any{"field": strings.Repeat("b", 100)},
				Created:  record.Created,
				Modified: record.Modified,
			}

			err = replaceTestRecord(t, ctx, ws, tableID, record, updatedRecord.Data, author)
			if err != nil {
				t.Errorf("same-size update should succeed: %v", err)
			}
		})
	})

	t.Run("Markdown", func(t *testing.T) {
		fs, ws, wsID := initWS(t)
		ctx := t.Context()

		nodeID := ksid.NewID()
		if _, err := writePageFixture(ws, ctx, nodeID, "Format Test", "# Content\n\nWith multiple lines", author); err != nil {
			t.Fatalf("failed to write page: %v", err)
		}

		filePath := filepath.Join(fs.rootDir, wsID.String(), nodeID.String(), "index.md")
		data, err := os.ReadFile(filePath) //nolint:gosec // G304: test code with controlled path
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		content := string(data)

		t.Run("FrontMatterDelimiters", func(t *testing.T) {
			if !strings.Contains(content, "---") {
				t.Error("expected front matter delimiters")
			}
		})

		t.Run("FrontMatterTitle", func(t *testing.T) {
			if !strings.Contains(content, "title: Format Test") {
				t.Error("expected title in front matter")
			}
		})

		t.Run("FrontMatterTimestamps", func(t *testing.T) {
			if !strings.Contains(content, "created:") {
				t.Error("expected created timestamp")
			}
			if !strings.Contains(content, "modified:") {
				t.Error("expected modified timestamp")
			}
		})

		t.Run("ContentSeparation", func(t *testing.T) {
			parts := strings.Split(content, "---")
			if len(parts) < 3 {
				t.Error("expected three sections separated by ---")
			}
		})
	})

	t.Run("RelativeLinks", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		pageA, err := ws.CreatePageUnderParent(ctx, 0, "A", "", author)
		if err != nil {
			t.Fatalf("create A: %v", err)
		}
		pageB, err := ws.CreatePageUnderParent(ctx, 0, "B", "", author)
		if err != nil {
			t.Fatalf("create B: %v", err)
		}

		t.Run("RoundTrip", func(t *testing.T) {
			relLink := fmt.Sprintf("[B](../%s/index.md)", pageB.ID)
			content := "See " + relLink + " for details."
			if _, err := ws.UpdatePage(ctx, pageA.ID, "A", content, pageRevision(t, ctx, ws, pageA.ID), author); err != nil {
				t.Fatalf("update: %v", err)
			}
			node, err := ws.ReadPage(pageA.ID)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !strings.Contains(node.Content, relLink) {
				t.Errorf("want %q in content, got: %s", relLink, node.Content)
			}
		})

		t.Run("NoLinksPassthrough", func(t *testing.T) {
			content := "Plain text with no links."
			if _, err := ws.UpdatePage(ctx, pageA.ID, "A", content, pageRevision(t, ctx, ws, pageA.ID), author); err != nil {
				t.Fatalf("update: %v", err)
			}
			node, err := ws.ReadPage(pageA.ID)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if node.Content != content {
				t.Errorf("want %q, got %q", content, node.Content)
			}
		})

		t.Run("CreateWithLinks", func(t *testing.T) {
			relLink := fmt.Sprintf("[A](../%s/index.md)", pageA.ID)
			page, err := ws.CreatePageUnderParent(ctx, 0, "D", "Link to "+relLink, author)
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			node, err := ws.ReadPage(page.ID)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !strings.Contains(node.Content, relLink) {
				t.Errorf("want %q in content, got: %s", relLink, node.Content)
			}
		})
	})

	t.Run("Backlinks", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		target, err := ws.CreatePageUnderParent(ctx, 0, "Target", "", author)
		if err != nil {
			t.Fatalf("failed to create target: %v", err)
		}
		linkContent := fmt.Sprintf("[Target](../%s/index.md)", target.ID)
		source, err := ws.CreatePageUnderParent(ctx, 0, "Source", linkContent, author)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}
		if _, err = ws.CreatePageUnderParent(ctx, 0, "Unrelated", "no links here", author); err != nil {
			t.Fatalf("failed to create unrelated: %v", err)
		}

		t.Run("Found", func(t *testing.T) {
			backlinks, err := ws.GetBacklinks(ctx, target.ID)
			if err != nil {
				t.Fatalf("failed to get backlinks: %v", err)
			}
			if len(backlinks) != 1 {
				t.Fatalf("expected 1 backlink, got %d", len(backlinks))
			}
			if backlinks[0].NodeID != source.ID {
				t.Errorf("expected backlink from %s, got %s", source.ID, backlinks[0].NodeID)
			}
			if backlinks[0].Title != "Source" {
				t.Errorf("expected backlink title 'Source', got %s", backlinks[0].Title)
			}
		})

		t.Run("None", func(t *testing.T) {
			backlinks, err := ws.GetBacklinks(t.Context(), source.ID)
			if err != nil {
				t.Fatalf("failed to get backlinks: %v", err)
			}
			if len(backlinks) != 0 {
				t.Errorf("expected 0 backlinks, got %d", len(backlinks))
			}
		})

		t.Run("AfterLinkRemoved", func(t *testing.T) {
			if _, err := ws.UpdatePage(ctx, source.ID, "Source", "no more links", pageRevision(t, ctx, ws, source.ID), author); err != nil {
				t.Fatalf("failed to update page: %v", err)
			}
			backlinks, err := ws.GetBacklinks(ctx, target.ID)
			if err != nil {
				t.Fatalf("failed to get backlinks: %v", err)
			}
			if len(backlinks) != 0 {
				t.Errorf("expected 0 backlinks after link removed, got %d", len(backlinks))
			}
		})
	})

	t.Run("NodeTitles", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		a, err := ws.CreatePageUnderParent(ctx, 0, "Alpha", "", author)
		if err != nil {
			t.Fatalf("failed to create page: %v", err)
		}
		b, err := ws.CreatePageUnderParent(ctx, 0, "Beta", "", author)
		if err != nil {
			t.Fatalf("failed to create page: %v", err)
		}

		titles, err := ws.GetNodeTitles(t.Context(), []ksid.ID{a.ID, b.ID})
		if err != nil {
			t.Fatalf("failed to get node titles: %v", err)
		}
		if len(titles) != 2 {
			t.Errorf("expected 2 titles, got %d", len(titles))
		}
		if titles[a.ID] != "Alpha" {
			t.Errorf("expected 'Alpha', got %s", titles[a.ID])
		}
		if titles[b.ID] != "Beta" {
			t.Errorf("expected 'Beta', got %s", titles[b.ID])
		}
	})

	t.Run("WorkspaceUsage", func(t *testing.T) {
		t.Run("CountsPages", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			for i := range 3 {
				nodeID := ksid.NewID()
				if _, err := writePageFixture(ws, ctx, nodeID, "Page "+string(rune(49+i)), "content", author); err != nil {
					t.Fatalf("failed to create page: %v", err)
				}
			}

			pageCount, _, err := ws.GetWorkspaceUsage()
			if err != nil {
				t.Fatalf("failed to get usage: %v", err)
			}
			if pageCount != 3 {
				t.Errorf("expected pageCount=3, got %d", pageCount)
			}
		})

		t.Run("ExcludesGitDir", func(t *testing.T) {
			_, ws, _ := initWS(t)

			// Create a fake .git/objects/maintenance.lock to simulate git internals.
			// GetWorkspaceUsage must skip .git and not fail if transient files vanish.
			lockDir := filepath.Join(ws.wsDir, ".git", "objects")
			if err := os.MkdirAll(lockDir, 0o750); err != nil {
				t.Fatal(err)
			}
			lockFile := filepath.Join(lockDir, "maintenance.lock")
			if err := os.WriteFile(lockFile, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}

			_, storageBytes, err := ws.GetWorkspaceUsage()
			if err != nil {
				t.Fatalf("GetWorkspaceUsage failed with .git present: %v", err)
			}

			// Storage must not include the .git contents.
			// Remove .git and compare.
			if err := os.RemoveAll(filepath.Join(ws.wsDir, ".git")); err != nil {
				t.Fatal(err)
			}
			_, withoutGit, err := ws.GetWorkspaceUsage()
			if err != nil {
				t.Fatal(err)
			}
			if storageBytes != withoutGit {
				t.Errorf("storage with .git=%d, without=%d; .git contents should be excluded", storageBytes, withoutGit)
			}
		})

		t.Run("HybridNodeCountedOnce", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			nodeID := ksid.NewID()
			if _, err := writePageFixture(ws, ctx, nodeID, "Hybrid", "content", author); err != nil {
				t.Fatalf("failed to create page: %v", err)
			}

			hybridNode := &Node{
				ID:       nodeID,
				Title:    "Hybrid",
				Type:     NodeTypeTable,
				Created:  storagev1.Now(),
				Modified: storagev1.Now(),
			}
			if err := ws.WriteTable(ctx, hybridNode, false, author); err != nil {
				t.Fatalf("failed to add table metadata: %v", err)
			}

			pageCount, _, err := ws.GetWorkspaceUsage()
			if err != nil {
				t.Fatalf("failed to get usage: %v", err)
			}
			if pageCount != 1 {
				t.Errorf("expected pageCount=1 for hybrid node, got %d", pageCount)
			}
		})
	})
}

func TestExtractLinkedNodeIDs(t *testing.T) {
	nodeA := ksid.NewID()
	nodeB := ksid.NewID()
	nodeC := ksid.NewID()

	tests := []struct {
		name     string
		content  string
		expected []ksid.ID
	}{
		{
			name:     "empty content",
			content:  "",
			expected: nil,
		},
		{
			name:     "no links",
			content:  "Some text without any links",
			expected: nil,
		},
		{
			name:     "sibling relative link",
			content:  "Check [my page](../" + nodeA.String() + "/index.md) here",
			expected: []ksid.ID{nodeA},
		},
		{
			name:     "multiple relative links",
			content:  "See [page1](../" + nodeA.String() + "/index.md) and [page2](../" + nodeB.String() + "/index.md)",
			expected: []ksid.ID{nodeA, nodeB},
		},
		{
			name:     "child link",
			content:  "[page](" + nodeA.String() + "/index.md)",
			expected: []ksid.ID{nodeA},
		},
		{
			name:     "duplicate links extracted once",
			content:  "[a](../" + nodeC.String() + "/index.md) and [b](../" + nodeC.String() + "/index.md)",
			expected: []ksid.ID{nodeC},
		},
		{
			name:     "external link ignored",
			content:  "[google](https://google.com) and [internal](../" + nodeA.String() + "/index.md)",
			expected: []ksid.ID{nodeA},
		},
		{
			name:     "deep relative link",
			content:  "# Header\n\nSome text with [a link](../../" + nodeB.String() + "/index.md) in it.\n\n![image](image.png)",
			expected: []ksid.ID{nodeB},
		},
		{
			name:     "absolute link ignored",
			content:  "[abs](/some/" + nodeA.String() + "/index.md)",
			expected: nil,
		},
		{
			name:     "invalid ID ignored",
			content:  "[bad](../not-valid-id/index.md)",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids := ExtractLinkedNodeIDs(tt.content)

			if len(ids) != len(tt.expected) {
				t.Errorf("expected %d IDs, got %d: %v", len(tt.expected), len(ids), ids)
				return
			}

			for i, exp := range tt.expected {
				if ids[i] != exp {
					t.Errorf("expected ID[%d]=%s, got %s", i, exp.String(), ids[i].String())
				}
			}
		})
	}
}

// writePageFixture creates exact-ID legacy fixtures; production creates pages through CreatePageUnderParent.
// writePage builds test fixtures without a history commit.
func writePage(ws *WorkspaceFileStore, id, parentID ksid.ID, title, content string) (*Node, error) {
	now := storagev1.Now()
	p := &page{
		title:    title,
		content:  content,
		created:  now,
		modified: now,
	}

	if err := ws.writePageFile(id, parentID, p); err != nil {
		return nil, err
	}

	return &Node{
		ID:       id,
		ParentID: parentID,
		Title:    title,
		Type:     NodeTypeDocument,
		Content:  content,
		Created:  p.created,
		Modified: p.modified,
		Icon:     p.icon,
		Cover:    p.cover,
	}, nil
}

func writePageFixture(ws *WorkspaceFileStore, ctx context.Context, id ksid.ID, title, content string, author git.Author) (*Node, error) {
	var node *Node
	err := ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		var err error
		node, err = writePage(ws, id, 0, title, content)
		if err != nil {
			return "", nil, err
		}
		files := []string{ws.gitPath(0, id, "index.md")}
		return "update: page " + id.String(), files, nil
	})
	if err == nil {
		ws.links.update(id, node.Content)
	}
	return node, err
}

func pageRevision(t *testing.T, ctx context.Context, ws *WorkspaceFileStore, id ksid.ID) string {
	node, err := ws.ReadNode(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return node.Revision
}

func BenchmarkWorkspaceFileStore(b *testing.B) {
	b.Run("ReplaceRecord256KiB", func(b *testing.B) {
		dir := b.TempDir()
		repo, err := git.NewManagerWithBackend(dir, "Bench", "bench@example.com", git.BackendGoGit).Repo(b.Context(), "")
		if err != nil {
			b.Fatal(err)
		}
		quotas := storage.DefaultResourceQuotas()
		ws := newWorkspaceFileStore(dir, repo, &quotas)
		author := git.Author{Name: "Bench", Email: "bench@example.com"}
		table, err := ws.CreateTableUnderParent(b.Context(), 0, "Table", []contentv1.Property{{Name: "body", Type: contentv1.PropertyTypeText}}, author)
		if err != nil {
			b.Fatal(err)
		}
		schema, err := ws.ReadTable(table.ID)
		if err != nil {
			b.Fatal(err)
		}
		body := strings.Repeat("x", 256<<10)
		now := storagev1.Now()
		record := &contentv1.DataRecord{ID: ksid.NewID(), Data: map[string]any{"body": body}, Created: now, Modified: now}
		if err := ws.AppendRecord(b.Context(), table.ID, record, author); err != nil {
			b.Fatal(err)
		}
		revision, err := RecordRevision(record)
		if err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.SetBytes(int64(len(body)))
		i := 0
		for b.Loop() {
			saved, err := ws.ReplaceRecord(b.Context(), table.ID, record.ID, map[string]any{"body": body, "step": strconv.Itoa(i)}, revision, schema.SchemaRevision, author)
			if err != nil {
				b.Fatal(err)
			}
			revision, err = RecordRevision(saved)
			if err != nil {
				b.Fatal(err)
			}
			i++
		}
	})

	b.Run("RecordPage1000", func(b *testing.B) {
		dir := b.TempDir()
		repo, err := git.NewManager(dir, "bench", "bench@example.com").WorkspaceRepo(b.Context(), "workspace", true)
		if err != nil {
			b.Fatal(err)
		}
		q := storage.DefaultResourceQuotas()
		ws := newWorkspaceFileStore(filepath.Join(dir, "workspace"), repo, &q)
		node, err := ws.CreateTableUnderParent(b.Context(), 0, "Records", []contentv1.Property{{Name: "number", Type: contentv1.PropertyTypeNumber}}, git.Author{Name: "Bench", Email: "bench@example.com"})
		if err != nil {
			b.Fatal(err)
		}
		table, err := jsonldb.NewTable[*contentv1.DataRecord](ws.tableRecordsFile(node.ID, 0))
		if err != nil {
			b.Fatal(err)
		}
		for range 1000 {
			r := &contentv1.DataRecord{ID: ksid.NewID(), Data: map[string]any{"name": "Representative record", "number": int64(9007199254740993)}, Created: storagev1.Now(), Modified: storagev1.Now()}
			if err := table.Append(r, nil); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			rows, err := ws.ReadRecordsPage(node.ID, 500, 50)
			if err != nil || len(rows) != 50 {
				b.Fatalf("record page: %d rows, %v", len(rows), err)
			}
		}
	})
	b.Run("LargeWorkspacePageWrite", func(b *testing.B) {
		dir := b.TempDir()
		repo, err := git.NewManager(dir, "bench", "bench@example.com").WorkspaceRepo(b.Context(), "workspace", true)
		if err != nil {
			b.Fatal(err)
		}
		quotas := storage.DefaultResourceQuotas()
		ws := newWorkspaceFileStore(filepath.Join(dir, "workspace"), repo, &quotas)
		var id ksid.ID
		files := make([]string, 0, 500)
		for range 500 {
			id = ksid.NewID()
			if _, err := writePage(ws, id, 0, "Benchmark", strings.Repeat("a", 1024)); err != nil {
				b.Fatal(err)
			}
			files = append(files, id.String()+"/index.md")
		}
		if err := repo.CommitTx(b.Context(), git.Author{}, func() (string, []string, error) { return "fixture", files, nil }); err != nil {
			b.Fatal(err)
		}
		snapshot, err := ws.ReadPageSnapshot(b.Context(), id, 0, 4096)
		if err != nil {
			b.Fatal(err)
		}
		revision := snapshot.Revision
		b.ReportAllocs()
		b.ResetTimer()
		for i := range b.N {
			node, err := ws.UpdatePage(b.Context(), id, "Benchmark", strings.Repeat(string(rune('b'+i%2)), 1025+i), revision, git.Author{})
			if err != nil {
				b.Fatal(err)
			}
			revision = node.Revision
		}
	})

	b.Run("Tables", func(b *testing.B) {
		tmpDir := b.TempDir()
		ctx := b.Context()
		author := git.Author{Name: "Benchmark", Email: "bench@test.com"}

		gitMgr := git.NewManager(tmpDir, "test", "test@test.com")

		orgService, err := identity.NewOrganizationService(filepath.Join(tmpDir, "organizations.jsonl"))
		if err != nil {
			b.Fatal(err)
		}

		wsService, err := identity.NewWorkspaceService(filepath.Join(tmpDir, "workspaces.jsonl"))
		if err != nil {
			b.Fatal(err)
		}

		// Create a test organization with very high quotas (practically unlimited)
		org, err := orgService.Create(ctx, "Benchmark Organization", "bench@test.com")
		if err != nil {
			b.Fatal(err)
		}
		_, err = orgService.Modify(org.ID, func(o *identityv1.Organization) error {
			o.Quotas.MaxWorkspacesPerOrg = 1_000
			o.Quotas.MaxMembersPerOrg = 10_000
			o.Quotas.MaxMembersPerWorkspace = 10_000
			o.Quotas.MaxTotalStorageBytes = 1_000_000_000_000_000_000 // 1EB
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}

		// Create a test workspace with very high quotas (practically unlimited)
		ws, err := wsService.Create(ctx, org.ID, "Benchmark Workspace")
		if err != nil {
			b.Fatal(err)
		}
		_, err = wsService.Modify(ws.ID, func(w *identityv1.Workspace) error {
			w.Quotas.MaxPages = 1_000_000
			w.Quotas.MaxStorageBytes = 1_000_000_000_000 // 1TB
			w.Quotas.MaxRecordsPerTable = 1_000_000
			w.Quotas.MaxAssetSizeBytes = 1024 * 1024 * 1024 // 1GB
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}

		serverQuotas := storagev1.ResourceQuotas{
			MaxPages:              1_000_000,
			MaxStorageBytes:       1_000_000_000_000, // 1TB
			MaxRecordsPerTable:    1_000_000,
			MaxAssetSizeBytes:     1024 * 1024 * 1024, // 1GB
			MaxTablesPerWorkspace: 10_000,
			MaxColumnsPerTable:    1_000,
		}
		fs, err := NewFileStoreService(tmpDir, gitMgr, wsService, orgService, storage.NewServerSettings(&storagev1.ServerConfig{Quotas: storagev1.ServerQuotas{ResourceQuotas: serverQuotas}}))
		if err != nil {
			b.Fatal(err)
		}

		wsID := ws.ID

		// Initialize git repo for workspace
		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			b.Fatalf("failed to init workspace: %v", err)
		}

		// Get workspace store
		wsStore, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			b.Fatalf("failed to get workspace store: %v", err)
		}

		dbID := ksid.NewID()
		node := &Node{
			ID:       dbID,
			Title:    "Benchmark Table",
			Type:     NodeTypeTable,
			Created:  storagev1.Now(),
			Modified: storagev1.Now(),
			Properties: []contentv1.Property{
				{Name: "Title", Type: "text"},
				{Name: "Value", Type: "number"},
			},
		}

		if err := wsStore.WriteTable(ctx, node, true, author); err != nil {
			b.Fatal(err)
		}

		// Benchmark Record Creation (Append)
		b.Run("AppendRecord", func(b *testing.B) {
			for i := range b.N {
				record := &contentv1.DataRecord{
					ID:       ksid.NewID(),
					Created:  storagev1.Now(),
					Modified: storagev1.Now(),
					Data: map[string]any{
						"c1": "Item",
						"c2": i,
					},
				}
				if err := wsStore.AppendRecord(ctx, dbID, record, author); err != nil {
					b.Fatal(err)
				}
			}
		})

		// Benchmark Reading All Records
		// We need a fresh file store or just read from the existing one populated by previous step
		// To make this isolated, we can pre-populate a DB with N records
		b.Run("IterRecords", func(b *testing.B) {
			// Prepare a table with 1000 records
			readDBID := ksid.NewID()
			readNode := &Node{ID: readDBID, Title: "Read Bench", Type: NodeTypeTable, Created: storagev1.Now(), Modified: storagev1.Now()}
			if err := wsStore.WriteTable(ctx, readNode, true, author); err != nil {
				b.Fatal(err)
			}
			for range 1000 {
				record := &contentv1.DataRecord{
					ID:       ksid.NewID(),
					Data:     map[string]any{"c1": "test"},
					Created:  storagev1.Now(),
					Modified: storagev1.Now(),
				}
				if err := wsStore.AppendRecord(ctx, readDBID, record, author); err != nil {
					b.Fatal(err)
				}
			}

			b.ResetTimer()
			for range b.N {
				it, err := wsStore.IterRecords(readDBID)
				if err != nil {
					b.Fatal(err)
				}
				records := slices.Collect(it)
				if len(records) != 1000 {
					b.Errorf("expected 1000 records, got %d", len(records))
				}
			}
		})

		// Benchmark Reading Page of Records
		b.Run("ReadRecordsPage", func(b *testing.B) {
			readDBID := ksid.ID(100)
			readNode := &Node{ID: readDBID, Title: "Read Bench Page", Type: NodeTypeTable, Created: storagev1.Now(), Modified: storagev1.Now()}
			if err := wsStore.WriteTable(ctx, readNode, true, author); err != nil {
				b.Fatal(err)
			}
			// Write 10,000 records
			for range 10000 {
				record := &contentv1.DataRecord{
					ID:       ksid.NewID(),
					Data:     map[string]any{"c1": "test"},
					Created:  storagev1.Now(),
					Modified: storagev1.Now(),
				}
				if err := wsStore.AppendRecord(ctx, readDBID, record, author); err != nil {
					b.Fatal(err)
				}
			}

			b.ResetTimer()
			for range b.N {
				// Read 50 records from middle
				records, err := wsStore.ReadRecordsPage(readDBID, 5000, 50)
				if err != nil {
					b.Fatal(err)
				}
				if len(records) != 50 {
					b.Errorf("expected 50 records, got %d", len(records))
				}
			}
		})
	})

	b.Run("Replace4MiB", func(b *testing.B) {
		dir := b.TempDir()
		repo, err := git.NewManagerWithBackend(dir, "Bench", "bench@example.com", git.BackendGoGit).Repo(b.Context(), "")
		if err != nil {
			b.Fatal(err)
		}
		quotas := storage.DefaultResourceQuotas()
		ws := newWorkspaceFileStore(dir, repo, &quotas)
		author := git.Author{Name: "Bench", Email: "bench@example.com"}
		body := strings.Repeat("x", 4<<20)
		page, err := ws.CreatePageUnderParent(b.Context(), 0, "Initial", body, author)
		if err != nil {
			b.Fatal(err)
		}
		read, err := ws.ReadNode(b.Context(), page.ID)
		if err != nil {
			b.Fatal(err)
		}
		revision := read.Revision
		b.ReportAllocs()
		b.SetBytes(int64(len(body)))
		i := 0
		for b.Loop() {
			saved, err := ws.UpdatePage(b.Context(), page.ID, strconv.Itoa(i), body, revision, author)
			if err != nil {
				b.Fatal(err)
			}
			revision = saved.Revision
			i++
		}
	})
}

// assetFinalErrorReader reports a source failure together with its final bytes.
type assetFinalErrorReader struct {
	data []byte
	err  error
	done bool
}

func (r *assetFinalErrorReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		r.done = true
		return n, r.err
	}
	return n, nil
}
