// Tests and measures bounded workspace discovery and canonical page snapshots.

package content

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func BenchmarkWorkspaceProjection(b *testing.B) {
	for _, count := range []int{100, 2000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			dir := b.TempDir()
			repo, err := git.NewManager(dir, "bench", "bench@example.com").WorkspaceRepo(b.Context(), "workspace", true)
			if err != nil {
				b.Fatal(err)
			}
			ws := newWorkspaceFileStore(filepath.Join(dir, "workspace"), repo, nil)
			body := strings.Repeat("abcdefgh", 1024)
			var first ksid.ID
			for range count {
				id := ksid.NewID()
				if first == 0 {
					first = id
				}
				path := filepath.Join(ws.wsDir, id.String())
				if err := os.MkdirAll(path, 0o700); err != nil {
					b.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "index.md"), []byte("---\ntitle: Benchmark page\n---\n\n"+body), 0o600); err != nil {
					b.Fatal(err)
				}
			}
			if err := ws.repo.ReadTx(b.Context(), func() error { return ws.scanIndex(b.Context().Err) }); err != nil {
				b.Fatal(err)
			}
			b.Run("Children", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := ws.ListNodeSummaries(b.Context(), new(ksid.ID), 0, 50); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("IndexCold", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if err := ws.scanIndex(b.Context().Err); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("Read", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := ws.ReadPageSnapshot(b.Context(), first, 0, 32<<10); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

func BenchmarkLargePageSnapshot(b *testing.B) {
	dir := b.TempDir()
	repo, err := git.NewManager(dir, "bench", "bench@example.com").WorkspaceRepo(b.Context(), "workspace", true)
	if err != nil {
		b.Fatal(err)
	}
	ws := newWorkspaceFileStore(filepath.Join(dir, "workspace"), repo, nil)
	id := ksid.NewID()
	path := filepath.Join(ws.wsDir, id.String())
	if err := os.MkdirAll(path, 0o700); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "index.md"), []byte("---\ntitle: Large\n---\n\n"+strings.Repeat("abcdefgh", 512<<10)), 0o600); err != nil {
		b.Fatal(err)
	}
	if err := ws.repo.ReadTx(b.Context(), func() error { return ws.scanIndex(b.Context().Err) }); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(4 << 20)
	for b.Loop() {
		if _, err := ws.ReadPageSnapshot(b.Context(), id, 0, 32<<10); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkColdWorkspaceDiscovery measures the actual service initialization
// path plus first MCP summary page, independently of warm store/index reads.
func BenchmarkColdWorkspaceDiscovery(b *testing.B) {
	dir := b.TempDir()
	orgs, err := identity.NewOrganizationService(filepath.Join(dir, "orgs.jsonl"))
	if err != nil {
		b.Fatal(err)
	}
	workspaces, err := identity.NewWorkspaceService(filepath.Join(dir, "workspaces.jsonl"))
	if err != nil {
		b.Fatal(err)
	}
	org, err := orgs.Create(b.Context(), "Benchmark", "bench@example.com")
	if err != nil {
		b.Fatal(err)
	}
	workspace, err := workspaces.Create(b.Context(), org.ID, "Benchmark")
	if err != nil {
		b.Fatal(err)
	}
	quotas := storage.DefaultResourceQuotas()
	fs, err := NewFileStoreService(dir, git.NewManager(dir, "bench", "bench@example.com"), workspaces, orgs, &quotas)
	if err != nil {
		b.Fatal(err)
	}
	if err := fs.InitWorkspace(b.Context(), workspace.ID); err != nil {
		b.Fatal(err)
	}
	body := []byte("---\ntitle: Benchmark\n---\n\n" + strings.Repeat("abcdefgh", 1024))
	for range 2000 {
		id := ksid.NewID()
		path := filepath.Join(dir, workspace.ID.String(), id.String())
		if err := os.Mkdir(path, 0o700); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "index.md"), body, 0o600); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		fs.InvalidateWorkspaceStore(workspace.ID)
		ws, err := fs.GetWorkspaceStore(b.Context(), workspace.ID)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := ws.ListNodeSummaries(b.Context(), new(ksid.ID), 0, 50); err != nil {
			b.Fatal(err)
		}
	}
}

func TestRevision(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		got, err := Revision(t.Context(), "Title", strings.NewReader("Body"), 4)
		if err != nil {
			t.Fatal(err)
		}
		if got != "081b5a6e3e41095d33d0045829488527f7e9ea6c2d0c15133189b6bf9d61c3ce" {
			t.Fatalf("canonical digest = %s", got)
		}
	})
	t.Run("error", func(t *testing.T) {
		for _, n := range []int64{-1, 3, 5, math.MaxInt64} {
			if _, err := Revision(t.Context(), "Title", strings.NewReader("Body"), n); err == nil {
				t.Fatalf("accepted length %d", n)
			}
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := Revision(ctx, "Title", strings.NewReader("Body"), 4); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation = %v", err)
		}
	})
}
