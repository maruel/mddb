// Tests and benchmarks workspace discovery, canonical snapshots, complete reads, and competing writes.

package content

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestSnapshotRevisionFreshness(t *testing.T) {
	for _, backend := range []git.Backend{git.BackendExec, git.BackendGoGit} {
		t.Run(strconv.Itoa(int(backend)), func(t *testing.T) {
			dir := t.TempDir()
			repo, err := git.NewManagerWithBackend(dir, "Test", "test@example.com", backend).WorkspaceRepo(t.Context(), "workspace", true)
			if err != nil {
				t.Fatal(err)
			}
			quotas := storage.DefaultResourceQuotas()
			ws := newWorkspaceFileStore(filepath.Join(dir, "workspace"), repo, &quotas)
			a := git.Author{Name: "Test", Email: "test@example.com"}
			n, err := ws.CreatePageUnderParent(t.Context(), 0, "Title", "original body", a)
			if err != nil {
				t.Fatal(err)
			}
			read := func() PageSnapshot {
				snap, err := ws.ReadPageSnapshot(t.Context(), n.ID, 0, 4096)
				if err != nil {
					t.Fatal(err)
				}
				return snap
			}
			old := read()
			path := ws.pageIndexFile(n.ID, 0)
			st, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			// A failed in-place mutation can preserve file identity, size and mtime.
			// The transaction generation must still retire its previously verified digest.
			original, err := os.ReadFile(path) //nolint:gosec // Test-owned page path is constructed from generated node IDs.
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("partially saved")
			if err := repo.CommitTx(t.Context(), a, func() (string, []string, error) {
				if err := os.WriteFile(path, bytes.Replace(original, []byte("original body"), []byte("replaced body"), 1), 0o600); err != nil {
					return "", nil, err
				}
				if err := os.Chtimes(path, st.ModTime(), st.ModTime()); err != nil {
					return "", nil, err
				}
				return "", nil, failure
			}); !errors.Is(err, failure) {
				t.Fatal(err)
			}
			replaced := read()
			if replaced.Node.Content != "replaced body" || replaced.Revision == old.Revision {
				t.Fatal("partial save retained stale revision")
			}
			// An external writer atomically replaces a file without a storage transaction.
			importData := bytes.Replace(original, []byte("original body"), []byte("imported body"), 1)
			importData = bytes.Replace(importData, []byte("title: Title"), []byte("title: Other"), 1)
			if err := storage.WriteFile(path, importData); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(path, st.ModTime(), st.ModTime()); err != nil {
				t.Fatal(err)
			}
			imported := read()
			if imported.Node.Title != "Other" || imported.Node.Content != "imported body" || imported.Revision == replaced.Revision {
				t.Fatal("atomic import retained stale snapshot")
			}
			if _, err := ws.UpdatePageFrontmatter(t.Context(), n.ID, "icon", "cover", a); err != nil {
				t.Fatal(err)
			}
			meta := read()
			if meta.Node.Icon != "icon" || meta.Node.Cover != "cover" || meta.Revision != imported.Revision {
				t.Fatal("metadata and body revision mixed")
			}
			// Discover and read enough real pages to evict retained digest identities.
			var ids []ksid.ID
			if err := repo.CommitTx(t.Context(), a, func() (string, []string, error) {
				var files []string
				for range 129 {
					id := ksid.NewID()
					ids = append(ids, id)
					if err := ws.writePageFile(id, 0, &page{title: "Eviction", content: "bounded body"}); err != nil {
						return "", nil, err
					}
					ws.setParent(id, 0)
					files = append(files, ws.gitPath(0, id, "index.md"))
				}
				return "eviction pages", files, nil
			}); err != nil {
				t.Fatal(err)
			}
			for _, id := range ids {
				if _, err := ws.ReadPageSnapshot(t.Context(), id, 0, 4096); err != nil {
					t.Fatal(err)
				}
			}
			final := read()
			if final.Revision != meta.Revision || final.Node.Content != meta.Node.Content {
				t.Fatal("cache eviction changed snapshot")
			}
			if len(ws.snapshotRevisions) > 128 {
				t.Fatalf("retained %d snapshot digests", len(ws.snapshotRevisions))
			}
		})
	}
}

func TestSnapshotBodyLayout(t *testing.T) {
	for _, backend := range []git.Backend{git.BackendExec, git.BackendGoGit} {
		t.Run(strconv.Itoa(int(backend)), func(t *testing.T) {
			dir := t.TempDir()
			repo, err := git.NewManagerWithBackend(dir, "Test", "test@example.com", backend).WorkspaceRepo(t.Context(), "workspace", true)
			if err != nil {
				t.Fatal(err)
			}
			quotas := storage.DefaultResourceQuotas()
			ws := newWorkspaceFileStore(filepath.Join(dir, "workspace"), repo, &quotas)
			a := git.Author{Name: "Test", Email: "test@example.com"}
			n, err := ws.CreatePageUnderParent(t.Context(), 0, "Blank prefix", strings.Repeat("\n", 1<<20)+strings.Repeat("body\n", 2000), a)
			if err != nil {
				t.Fatal(err)
			}
			first, err := ws.ReadPageSnapshot(t.Context(), n.ID, 0, 4096)
			if err != nil {
				t.Fatal(err)
			}
			if first.ContentBytes != 10000 || first.Node.Title != "Blank prefix" {
				t.Fatalf("initial snapshot=%+v", first)
			}
			var all strings.Builder
			all.WriteString(first.Node.Content)
			for offset := first.NextOffset; offset < first.ContentBytes; {
				snap, err := ws.ReadPageSnapshot(t.Context(), n.ID, offset, 4096)
				if err != nil {
					t.Fatal(err)
				}
				if snap.Revision != first.Revision {
					t.Fatal("body layout changed revision")
				}
				all.WriteString(snap.Node.Content)
				offset = snap.NextOffset
			}
			if all.String() != strings.Repeat("body\n", 2000) {
				t.Fatal("blank prefix changed canonical content")
			}
			// An independent hybrid table schema must stay current while body layout is cached.
			if err := repo.CommitTx(t.Context(), a, func() (string, []string, error) {
				path := ws.tableMetadataFile(n.ID, 0)
				return "hybrid metadata", []string{ws.gitPath(0, n.ID, "metadata.json")}, storage.WriteFile(path, []byte(`{"title":"Table title","properties":[],"views":[]}`))
			}); err != nil {
				t.Fatal(err)
			}
			hybrid, err := ws.ReadPageSnapshot(t.Context(), n.ID, first.NextOffset, 4096)
			if err != nil {
				t.Fatal(err)
			}
			if hybrid.Node.Type != NodeTypeHybrid || hybrid.Node.Title != "Table title" || hybrid.Revision != first.Revision {
				t.Fatal("cached body layout hid independent metadata")
			}
			for _, tc := range []struct{ name, file, body string }{
				{"plain leading blanks", "\n\nplain", "\n\nplain"},
				{"empty normalized body", "---\ntitle: Empty\n---\n\n\n", ""},
				{"delimiter suffix", "---\ntitle: Tail\n---tail", "tail"},
				{"CRLF body", "---\ntitle: CRLF\n---\n\r\nbody", "\r\nbody"},
				{"unterminated front matter", "---\ntitle: Incomplete\nbody", "---\ntitle: Incomplete\nbody"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if err := storage.WriteFile(ws.pageIndexFile(n.ID, 0), []byte(tc.file)); err != nil {
						t.Fatal(err)
					}
					for range 2 { // Verify both cold layout discovery and cached reuse.
						snap, err := ws.ReadPageSnapshot(t.Context(), n.ID, 0, 4096)
						if err != nil {
							t.Fatal(err)
						}
						if snap.Node.Content != tc.body || snap.ContentBytes != int64(len(tc.body)) {
							t.Fatalf("content=%q bytes=%d", snap.Node.Content, snap.ContentBytes)
						}
					}
				})
			}
		})
	}
}

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
			if err := ws.repo.ReadTx(b.Context(), func(*git.SavedFiles) error { return ws.scanIndex(b.Context().Err) }); err != nil {
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
	for _, mode := range []string{"Cold", "Warm"} {
		b.Run(mode, func(b *testing.B) {
			ws, id := benchmarkSnapshotWorkspace(b, 4<<20)
			b.ReportAllocs()
			b.SetBytes(32 << 10)
			for b.Loop() {
				if mode == "Cold" {
					clear(ws.snapshotRevisions)
				}
				if _, err := ws.ReadPageSnapshot(b.Context(), id, 0, 32<<10); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSnapshotCacheRetention measures retained digest/identity memory for a
// full cache, with one large document and otherwise 32 KiB pages. Body size must
// not become retained cache memory. The post-loop GC comparison removes only
// digest entries, keeping repository version tokens and discovery state alive.
func BenchmarkSnapshotCacheRetention(b *testing.B) {
	for _, size := range []int{1 << 20, 4 << 20} {
		b.Run(strconv.Itoa(size>>20)+"MiB", func(b *testing.B) {
			ws, first := benchmarkSnapshotWorkspace(b, size)
			ids := make([]ksid.ID, 1, 128)
			ids[0] = first
			for range 127 {
				id := ksid.NewID()
				if err := ws.writePageFile(id, 0, &page{title: "Small", content: strings.Repeat("a", 32<<10)}); err != nil {
					b.Fatal(err)
				}
				ws.setParent(id, 0)
				ids = append(ids, id)
			}
			for b.Loop() {
				for _, id := range ids {
					if _, err := ws.ReadPageSnapshot(b.Context(), id, 0, 32<<10); err != nil {
						b.Fatal(err)
					}
				}
			}
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			ws.snapshotRevisions = nil
			runtime.GC()
			runtime.ReadMemStats(&after)
			runtime.KeepAlive(ws)
			b.ReportMetric(float64(before.HeapAlloc)-float64(after.HeapAlloc), "retained-digest-B")
		})
	}
}

// BenchmarkPageTraversal measures the work a client performs to read every
// 32 KiB chunk, not only the first response. Run before/after changes with:
// go test ./backend/internal/storage/content -run '^$' -bench 'Benchmark(PageTraversal|LargePageSnapshot)' -benchmem -count=3
//
// Plain uses an ordinary body; BlankPrefix adds equally many leading blank bytes.
// Header discovery must not scan that prefix for every continuation window.
// CompleteCold verifies the revision afresh each traversal; Complete measures
// warmed document reads. Both include every bounded content window.
// Complete traversal should scale with delivered bytes rather than chunks times
// document size. The competing writer reports its observed latency separately;
// compare it to WriterAlone rather than imposing machine-dependent time limits.
// On one machine, the optimization budget is at most 6x traversal time for 4x
// payload (linear work plus 50% headroom) and at most 2x WriterAlone latency.
// Compare repeated measurements, not timing assertions in ordinary tests.
func BenchmarkPageTraversal(b *testing.B) {
	for _, shape := range []string{"Plain", "BlankPrefix"} {
		b.Run(shape, func(b *testing.B) {
			for _, size := range []int{1 << 20, 4 << 20} {
				b.Run(strconv.Itoa(size>>20)+"MiB", func(b *testing.B) {
					ws, id := benchmarkSnapshotWorkspace(b, size)
					if shape == "BlankPrefix" {
						if err := storage.WriteFile(ws.pageIndexFile(id, 0), []byte("---\ntitle: Large\n---\n\n"+strings.Repeat("\n", size)+strings.Repeat("a", size))); err != nil {
							b.Fatal(err)
						}
					}
					author := git.Author{Name: "Benchmark", Email: "bench@example.com"}
					writer, err := ws.CreatePageUnderParent(b.Context(), 0, "Writer", "Initial", author)
					if err != nil {
						b.Fatal(err)
					}
					writer, err = ws.ReadNode(b.Context(), writer.ID)
					if err != nil {
						b.Fatal(err)
					}
					for _, mode := range []string{"Complete", "CompleteCold", "CompleteWithWriter", "WriterAlone"} {
						b.Run(mode, func(b *testing.B) {
							b.ReportAllocs()
							if mode != "WriterAlone" {
								b.SetBytes(int64(size))
							}
							var chunks int64
							var writerLatency time.Duration
							for b.Loop() {
								if mode == "CompleteCold" {
									clear(ws.snapshotRevisions)
								}
								var wg sync.WaitGroup
								var writeErr error
								var readErr error
								write := func() {
									start := time.Now()
									var saved *Node
									saved, writeErr = ws.UpdatePage(b.Context(), writer.ID, "Writer", writer.Content+"x", writer.Revision, author)
									if writeErr == nil {
										writer = saved
									}
									writerLatency += time.Since(start)
								}
								if mode == "WriterAlone" {
									write()
								} else {
									var offset, received int64
									var revision string
									for {
										snap, err := ws.ReadPageSnapshot(b.Context(), id, offset, 32<<10)
										if err != nil {
											readErr = err
											break
										}
										chunks++
										if offset == 0 {
											revision = snap.Revision
											if mode == "CompleteWithWriter" {
												wg.Go(write)
											}
										} else if snap.Revision != revision {
											readErr = errors.New("unrelated write changed the document revision")
											break
										}
										received += int64(len(snap.Node.Content))
										if snap.NextOffset >= snap.ContentBytes {
											break
										}
										if snap.NextOffset <= offset {
											readErr = errors.New("content continuation made no progress")
											break
										}
										offset = snap.NextOffset
									}
									if readErr == nil && received != int64(size) {
										readErr = fmt.Errorf("received %d bytes, want %d", received, size)
									}
								}
								wg.Wait()
								if readErr != nil {
									b.Fatal(readErr)
								}
								if writeErr != nil {
									b.Fatal(writeErr)
								}
							}
							if mode != "WriterAlone" {
								b.ReportMetric(float64(chunks)/float64(b.N), "chunks/op")
							}
							if mode == "CompleteWithWriter" || mode == "WriterAlone" {
								b.ReportMetric(float64(writerLatency.Nanoseconds())/float64(b.N), "writer-ns/op")
							}
						})
					}
				})
			}
		})
	}
}

func benchmarkSnapshotWorkspace(b *testing.B, size int) (*WorkspaceFileStore, ksid.ID) {
	dir := b.TempDir()
	repo, err := git.NewManager(dir, "bench", "bench@example.com").WorkspaceRepo(b.Context(), "workspace", true)
	if err != nil {
		b.Fatal(err)
	}
	quotas := storage.DefaultResourceQuotas()
	ws := newWorkspaceFileStore(filepath.Join(dir, "workspace"), repo, &quotas)
	id := ksid.NewID()
	path := filepath.Join(ws.wsDir, id.String())
	if err := os.MkdirAll(path, 0o700); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "index.md"), []byte("---\ntitle: Large\n---\n\n"+strings.Repeat("a", size)), 0o600); err != nil {
		b.Fatal(err)
	}
	if err := ws.repo.ReadTx(b.Context(), func(*git.SavedFiles) error { return ws.scanIndex(b.Context().Err) }); err != nil {
		b.Fatal(err)
	}
	return ws, id
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
