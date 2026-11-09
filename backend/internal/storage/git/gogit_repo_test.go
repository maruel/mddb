// Tests error preservation and cancellation through go-git's filesystem boundaries.

package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// cancelObjectStorage observes real commit reads at the foreign storage boundary.
type cancelObjectStorage struct {
	*filesystem.Storage

	cancel context.CancelFunc
	reads  int
}

func (s *cancelObjectStorage) EncodedObject(kind plumbing.ObjectType, hash plumbing.Hash) (plumbing.EncodedObject, error) {
	if kind == plumbing.CommitObject {
		s.reads++
		if s.reads == 2 {
			s.cancel()
		}
	}
	return s.Storage.EncodedObject(kind, hash)
}

// cancelLoadedObjectStorage cancels after a real object lookup completes.
type cancelLoadedObjectStorage struct {
	*filesystem.Storage

	cancel           context.CancelFunc
	target           plumbing.Hash
	readsAfterCancel int
	triggered        bool
}

func (s *cancelLoadedObjectStorage) EncodedObject(kind plumbing.ObjectType, hash plumbing.Hash) (plumbing.EncodedObject, error) {
	if s.triggered {
		s.readsAfterCancel++
	}
	obj, err := s.Storage.EncodedObject(kind, hash)
	if hash == s.target {
		s.triggered = true
		s.cancel()
	}
	return obj, err
}

// cancelReferenceFilesystem observes real loose-reference opens at the foreign boundary.
type cancelReferenceFilesystem struct {
	billy.Filesystem

	cancel context.CancelFunc
	target string
}

func (f *cancelReferenceFilesystem) Open(name string) (billy.File, error) {
	file, err := f.Filesystem.Open(name)
	if name == f.target {
		f.cancel()
	}
	return file, err
}

// cancelReferenceStorage exposes the wrapped reference filesystem to the owner.
type cancelReferenceStorage struct {
	*filesystem.Storage

	disk billy.Filesystem
}

func (s *cancelReferenceStorage) Filesystem() billy.Filesystem { return s.disk }

func TestGoGitRepo(t *testing.T) {
	t.Run("ReadReferences", func(t *testing.T) {
		t.Run("packed refs", func(t *testing.T) {
			dir := t.TempDir()
			repo, err := newGoGitRepo(t.Context(), dir, "Test", "test@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
				return "saved", []string{"row"}, os.WriteFile(filepath.Join(dir, "row"), []byte("saved"), 0o600)
			}); err != nil {
				t.Fatal(err)
			}
			if out, err := gitOutput(dir, "pack-refs", "--all", "--prune"); err != nil {
				t.Fatalf("pack refs: %v %s", err, out)
			}
			if n, err := repo.CommitCount(t.Context()); err != nil || n != 1 {
				t.Fatalf("packed count=%d err=%v", n, err)
			}
			if h, err := repo.GetHistory(t.Context(), "row", 10); err != nil || len(h) != 1 {
				t.Fatalf("packed history=%v err=%v", h, err)
			}
			if data, err := repo.GetFileAtCommit(t.Context(), "HEAD", "row"); err != nil || string(data) != "saved" {
				t.Fatalf("packed HEAD version=%q err=%v", data, err)
			}
			file, err := repo.FSAtCommit(t.Context(), "HEAD").Open("row")
			if err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, 5)
			n, readErr := file.Read(buf)
			if err := errors.Join(readErr, file.Close()); err != nil || n != 5 || string(buf) != "saved" {
				t.Fatalf("packed HEAD filesystem=%q err=%v", buf, err)
			}
		})
		t.Run("malformed packed hash", func(t *testing.T) {
			dir := t.TempDir()
			repo, err := newGoGitRepo(t.Context(), dir, "Test", "test@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
				return "saved", []string{"row"}, os.WriteFile(filepath.Join(dir, "row"), []byte("saved"), 0o600)
			}); err != nil {
				t.Fatal(err)
			}
			ref, err := repo.repo.Head()
			if err != nil {
				t.Fatal(err)
			}
			if out, err := gitOutput(dir, "pack-refs", "--all", "--prune"); err != nil {
				t.Fatalf("pack refs: %v %s", err, out)
			}
			data := []byte(ref.Hash().String() + "garbage " + ref.Name().String() + "\n")
			if err := os.WriteFile(filepath.Join(dir, ".git", "packed-refs"), data, 0o600); err != nil {
				t.Fatal(err)
			}
			if n, err := repo.CommitCount(t.Context()); err == nil {
				t.Errorf("malformed packed hash returned count=%d", n)
			}
			if h, err := repo.GetHistory(t.Context(), "row", 10); err == nil {
				t.Errorf("malformed packed hash returned history=%v", h)
			}
			if data, err := repo.GetFileAtCommit(t.Context(), "HEAD", "row"); err == nil {
				t.Errorf("malformed packed hash returned saved version=%q", data)
			}
		})

		t.Run("error", func(t *testing.T) {
			for _, mode := range []string{"empty loose ref", "malformed loose ref", "empty loose ref over packed ref", "unreadable loose ref", "unreadable loose ref over packed ref"} {
				t.Run(mode, func(t *testing.T) {
					dir := t.TempDir()
					repo, err := newGoGitRepo(t.Context(), dir, "Test", "test@example.com")
					if err != nil {
						t.Fatal(err)
					}
					if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
						return "saved", []string{"row"}, os.WriteFile(filepath.Join(dir, "row"), []byte("saved"), 0o600)
					}); err != nil {
						t.Fatal(err)
					}
					head, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD")) //nolint:gosec // G304: test-owned repository metadata.
					if err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(dir, ".git", strings.TrimSpace(strings.TrimPrefix(string(head), "ref: ")))
					if strings.Contains(mode, "packed") {
						if out, err := gitOutput(dir, "pack-refs", "--all", "--prune"); err != nil {
							t.Fatalf("pack refs: %v %s", err, out)
						}
					}
					var data []byte
					if mode == "malformed loose ref" {
						data = []byte("not-a-valid-hash\n")
					}
					if err := os.WriteFile(path, data, 0o600); err != nil {
						t.Fatal(err)
					}
					if strings.HasPrefix(mode, "unreadable loose ref") {
						if err := os.Chmod(path, 0); err != nil {
							t.Fatal(err)
						}
					}
					if n, err := repo.CommitCount(t.Context()); err == nil {
						t.Errorf("corrupt loose ref count=%d, no error", n)
					}
					if h, err := repo.GetHistory(t.Context(), "row", 10); err == nil {
						t.Errorf("corrupt loose ref history=%v, no error", h)
					}
					if data, err := repo.GetFileAtCommit(t.Context(), "HEAD", "row"); err == nil {
						t.Errorf("corrupt HEAD file version returned stale success: %q", data)
					}
					called := false
					if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) { called = true; return "", nil, nil }); err == nil || called {
						t.Errorf("corrupt HEAD mutation preflight: callback=%v err=%v", called, err)
					}

					if file, err := repo.FSAtCommit(t.Context(), "HEAD").Open("row"); err == nil {
						_ = file.Close()
						t.Error("corrupt HEAD filesystem returned success")
					}
					for _, branch := range []string{"", strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(string(head), "ref: ")), "refs/heads/")} {
						if err := repo.Push(t.Context(), "missing", branch); err == nil || !strings.Contains(err.Error(), "resolve") {
							t.Errorf("push bypassed corrupt branch resolution: %v", err)
						}
						if _, err := repo.Pull(t.Context(), "missing", branch); err == nil || !strings.Contains(err.Error(), "resolve") {
							t.Errorf("pull bypassed corrupt HEAD resolution: %v", err)
						}
					}
				})
			}
		})
	})
	t.Run("cancel during unborn reference resolution", func(t *testing.T) {
		for _, step := range []string{"HEAD open", "branch open", "packed fallback"} {
			for _, operation := range []string{"count", "history"} {
				t.Run(step+"/"+operation, func(t *testing.T) {
					repo, err := newGoGitRepo(t.Context(), t.TempDir(), "Test", "test@example.com")
					if err != nil {
						t.Fatal(err)
					}
					base := repo.repo.Storer.(*filesystem.Storage)
					head, err := base.Reference(plumbing.HEAD)
					if err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithCancel(t.Context())
					t.Cleanup(cancel)
					path := "HEAD"
					if step == "branch open" {
						path = head.Target().String()
					}
					if step == "packed fallback" {
						path = "packed-refs"
					}
					disk := &cancelReferenceFilesystem{Filesystem: base.Filesystem(), cancel: cancel, target: path}
					repo.repo.Storer = &cancelReferenceStorage{Storage: base, disk: disk}

					if operation == "count" {
						_, err = repo.CommitCount(ctx)
					} else {
						_, err = repo.GetHistory(ctx, "row", 10)
					}
					if !errors.Is(ctx.Err(), context.Canceled) {
						t.Fatal("reference boundary did not trigger active cancellation")
					}
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("unborn resolution lost active owner cancellation: %v", err)
					}
				})
			}
		}
	})

	t.Run("CommitFS", func(t *testing.T) {
		t.Run("cancel during object loads", func(t *testing.T) {
			for _, tc := range []struct{ name, path, target string }{
				{"root after commit", ".", "HEAD"},
				{"root after tree", ".", "HEAD^{tree}"},
				{"nested directory", "nested", "HEAD:nested"},
				{"deeper directory", "nested/deeper", "HEAD:nested/deeper"},
				{"file after commit", "nested/deeper/row", "HEAD"},
				{"file after root tree", "nested/deeper/row", "HEAD^{tree}"},
				{"file after nested tree", "nested/deeper/row", "HEAD:nested"},
				{"file after blob", "nested/deeper/row", "HEAD:nested/deeper/row"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					for _, api := range []string{"filesystem", "file content"} {
						if api == "file content" && !strings.HasPrefix(tc.name, "file ") {
							continue
						}
						t.Run(api, func(t *testing.T) {
							dir := t.TempDir()
							repo, err := newGoGitRepo(t.Context(), dir, "Test", "test@example.com")
							if err != nil {
								t.Fatal(err)
							}
							if err := os.MkdirAll(filepath.Join(dir, "nested", "deeper"), 0o700); err != nil {
								t.Fatal(err)
							}
							if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
								return "saved", []string{"nested/deeper/row"}, os.WriteFile(filepath.Join(dir, "nested", "deeper", "row"), []byte("saved"), 0o600)
							}); err != nil {
								t.Fatal(err)
							}
							out, err := gitOutput(dir, "rev-parse", tc.target)
							if err != nil {
								t.Fatal(err)
							}
							ctx, cancel := context.WithCancel(t.Context())
							t.Cleanup(cancel)
							s := &cancelLoadedObjectStorage{Storage: repo.repo.Storer.(*filesystem.Storage), cancel: cancel, target: plumbing.NewHash(strings.TrimSpace(string(out)))}
							repo.repo.Storer = s
							if api == "filesystem" {
								f, openErr := repo.FSAtCommit(ctx, "HEAD").Open(tc.path)
								if f != nil {
									_ = f.Close()
								}
								err = openErr
							} else {
								_, err = repo.GetFileAtCommit(ctx, "HEAD", tc.path)
							}
							if !s.triggered {
								t.Fatal("object boundary did not trigger cancellation")
							}
							if !errors.Is(err, context.Canceled) {
								t.Fatalf("lost cancellation: %v", err)
							}
							if s.readsAfterCancel != 0 {
								t.Fatalf("loaded %d more objects after cancellation", s.readsAfterCancel)
							}
						})
					}
				})
			}
		})
		t.Run("valid files directories and missing paths", func(t *testing.T) {
			dir := t.TempDir()
			repo, err := newGoGitRepo(t.Context(), dir, "Test", "test@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(dir, "nested"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
				return "saved", []string{"nested/row"}, os.WriteFile(filepath.Join(dir, "nested", "row"), []byte("saved"), 0o600)
			}); err != nil {
				t.Fatal(err)
			}
			head, err := repo.repo.Head()
			if err != nil {
				t.Fatal(err)
			}
			fsys := repo.FSAtCommit(t.Context(), head.Hash().String())
			for _, name := range []string{".", "nested"} {
				f, err := fsys.Open(name)
				if err != nil {
					t.Fatal(err)
				}
				info, statErr := f.Stat()
				closeErr := f.Close()
				if statErr != nil || closeErr != nil || !info.IsDir() {
					t.Fatalf("directory %s: info=%v errors=%v %v", name, info, statErr, closeErr)
				}
			}
			f, err := fsys.Open("nested/row")
			if err != nil {
				t.Fatal(err)
			}
			data := make([]byte, 5)
			n, readErr := f.Read(data)
			if err := errors.Join(readErr, f.Close()); err != nil || n != 5 || string(data) != "saved" {
				t.Fatalf("read nested file=%q err=%v", data, err)
			}
			for _, name := range []string{"missing", "nested/missing", "nested/row/child"} {
				if _, err := fsys.Open(name); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("missing path %s: %v", name, err)
				}
			}
		})

		for _, mode := range []string{"missing nested tree", "corrupt nested tree", "missing top-level blob"} {
			t.Run(mode, func(t *testing.T) {
				dir := t.TempDir()
				repo, err := newGoGitRepo(t.Context(), dir, "Test", "test@example.com")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(dir, "nested"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
					if err := os.WriteFile(filepath.Join(dir, "nested", "row"), []byte("nested"), 0o600); err != nil {
						return "", nil, err
					}
					return "saved", []string{"nested/row", "row"}, os.WriteFile(filepath.Join(dir, "row"), []byte("top"), 0o600)
				}); err != nil {
					t.Fatal(err)
				}
				head, err := repo.repo.Head()
				if err != nil {
					t.Fatal(err)
				}
				target := "HEAD:nested"
				name := "nested/row"
				if mode == "missing top-level blob" {
					target = "HEAD:row"
					name = "row"
				}
				out, err := gitOutput(dir, "rev-parse", target)
				if err != nil {
					t.Fatal(err)
				}
				hash := strings.TrimSpace(string(out))
				path := filepath.Join(dir, ".git", "objects", hash[:2], hash[2:])
				if mode == "corrupt nested tree" {
					if err := os.Chmod(path, 0o600); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte("invalid compressed tree"), 0o600); err != nil {
						t.Fatal(err)
					}
				} else if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				repo, err = newGoGitRepo(t.Context(), dir, "Test", "test@example.com")
				if err != nil {
					t.Fatal(err)
				}
				file, err := repo.FSAtCommit(t.Context(), head.Hash().String()).Open(name)
				if err == nil {
					_ = file.Close()
					t.Fatal("unreadable object was returned as a successful file or directory")
				}
				if errors.Is(err, os.ErrNotExist) {
					t.Fatalf("object corruption was hidden as missing path: %v", err)
				}
				if mode != "missing top-level blob" {
					if _, err := repo.FSAtCommit(t.Context(), head.Hash().String()).Open("nested"); err == nil || errors.Is(err, os.ErrNotExist) {
						t.Fatalf("unreadable directory returned success or missing path: %v", err)
					}
				}
				if _, err := repo.GetFileAtCommit(t.Context(), head.Hash().String(), name); err == nil || errors.Is(err, object.ErrFileNotFound) {
					t.Fatalf("file lookup hid unreadable object: %v", err)
				}
			})
		}
	})
	t.Run("GetHistory", func(t *testing.T) {
		t.Run("cancel while scanning unmatched commits", func(t *testing.T) {
			dir := t.TempDir()
			repo, err := newGoGitRepo(t.Context(), dir, "Test", "test@example.com")
			if err != nil {
				t.Fatal(err)
			}
			for _, body := range []string{"first", "second", "third", "fourth"} {
				if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
					return body, []string{"row"}, os.WriteFile(filepath.Join(dir, "row"), []byte(body), 0o600)
				}); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			storage := &cancelObjectStorage{Storage: repo.repo.Storer.(*filesystem.Storage), cancel: cancel}
			repo.repo.Storer = storage
			if h, err := repo.GetHistory(ctx, "never-existed", 10); !errors.Is(err, context.Canceled) {
				t.Fatalf("active filtered cancellation lost: history=%v err=%v", h, err)
			}
			if storage.reads > 2 {
				t.Fatalf("continued scanning %d commits after owner cancellation", storage.reads)
			}
		})
	})
}
