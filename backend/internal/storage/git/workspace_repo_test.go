// Tests saved workspace mutations and scoped, owner-canceled history recovery.

package git

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maruel/ksid"
)

func TestWorkspaceRepo(t *testing.T) {
	t.Run("ReadTx", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			dir := t.TempDir()
			repo, err := NewManagerWithBackend(dir, "test", "test@example.com", BackendGoGit).WorkspaceRepo(t.Context(), "ws", true)
			if err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{})
			release := make(chan struct{})
			done := make(chan error, 1)
			go func() { done <- repo.ReadTx(t.Context(), func() error { close(entered); <-release; return nil }) }()
			<-entered
			wrote := make(chan error, 1)
			go func() {
				wrote <- repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) { return "", nil, nil })
			}()
			synctest.Wait()
			select {
			case e := <-wrote:
				if e != nil {
					t.Fatal(e)
				}
				t.Fatal("write entered during snapshot")
			default:
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if err := <-wrote; err != nil {
				t.Fatal(err)
			}
		})
	})

	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			node := ksid.NewID().String()
			synctest.Test(t, func(t *testing.T) {
				dir := t.TempDir()
				mgr := NewManagerWithBackend(dir, "Test", "test@example.com", b.backend)
				repo, err := mgr.WorkspaceRepo(t.Context(), "ws", true)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, "ws", node, "index.md")
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				save := func(value string) error {
					return repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
						return "save", []string{node + "/index.md"}, os.WriteFile(path, []byte(value), 0o600)
					})
				}
				if err := save("first"); err != nil {
					t.Fatal(err)
				}
				index := filepath.Join(dir, "ws", ".git", "index")
				if err := os.Rename(index, index+".saved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(index, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := save("second"); err != nil {
					t.Fatalf("saved mutation failed: %v", err)
				}
				if !mgr.HasPendingChanges() {
					t.Fatal("missing pending history")
				}
				sentinel := errors.New("callback failed")
				if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) { return "", nil, sentinel }); !errors.Is(err, sentinel) {
					t.Fatalf("callback error=%v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "ws", "scratch"), []byte("private"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(filepath.Dir(path), ".jsonldb-unpublished"), []byte("private"), 0o600); err != nil {
					t.Fatal(err)
				}
				blobTemp := filepath.Join(filepath.Dir(path), "data.blobs", "tmp")
				if err := os.MkdirAll(blobTemp, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(blobTemp, "upload.tmp"), []byte("private"), 0o600); err != nil {
					t.Fatal(err)
				}

				// A saved request owns the repository while its callback is still running.
				// Health and owner shutdown must not wait behind that request.
				entered, release := make(chan struct{}), make(chan struct{})
				var requests sync.WaitGroup
				requests.Go(func() {
					if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) { close(entered); <-release; return "", nil, nil }); err != nil {
						t.Error(err)
					}
				})
				<-entered
				readCtx, stopRead := context.WithCancel(t.Context())
				readDone := make(chan error, 1)
				var reads sync.WaitGroup
				reads.Go(func() { _, err := repo.GetHistory(readCtx, node, 10); readDone <- err })
				synctest.Wait()
				stopRead()
				if err := <-readDone; !errors.Is(err, context.Canceled) {
					t.Fatalf("queued read cancellation=%v", err)
				}
				reads.Wait()
				if err := repo.Push(readCtx, "origin", ""); !errors.Is(err, context.Canceled) {
					t.Fatalf("queued remote cancellation=%v", err)
				}
				blockedCtx, stop := context.WithCancel(t.Context())
				var blocked sync.WaitGroup
				blocked.Go(func() { mgr.RunCommitRecovery(blockedCtx) })
				synctest.Wait()
				if !mgr.HasPendingChanges() {
					t.Fatal("active request hid pending history")
				}
				stop()
				blocked.Wait()
				close(release)
				requests.Wait()

				if err := repo.Push(t.Context(), "origin", ""); err == nil || !strings.Contains(err.Error(), "pending recovery") {
					t.Fatalf("push hid local pending changes: %v", err)
				}

				ctx, cancel := context.WithCancel(t.Context())
				var wg sync.WaitGroup
				wg.Go(func() { mgr.RunCommitRecovery(ctx) })
				synctest.Wait()
				if err := os.Remove(index); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(index+".saved", index); err != nil {
					t.Fatal(err)
				}
				if err := repo.CommitTx(t.Context(), Author{Name: "Request Author", Email: "request@example.com"}, func() (string, []string, error) {
					return "different saved mutation", []string{node + "/metadata.json"}, os.WriteFile(filepath.Join(filepath.Dir(path), "metadata.json"), []byte("{}"), 0o600)
				}); err != nil {
					t.Fatal(err)
				}
				if !mgr.HasPendingChanges() {
					t.Fatal("unrelated successful commit cleared pending history")
				}
				// An unrelated staged file must not hitchhike on recovery either.
				if out, err := gitOutput(filepath.Join(dir, "ws"), "add", "--", "scratch"); err != nil {
					t.Fatalf("stage scratch: %v %s", err, out)
				}
				time.Sleep(30 * time.Second)
				synctest.Wait()
				if !mgr.HasPendingChanges() {
					t.Fatal("unsafe staged scratch was committed")
				}
				if out, err := gitOutput(filepath.Join(dir, "ws"), "reset", "--", "scratch"); err != nil {
					t.Fatalf("unstage scratch: %v %s", err, out)
				}
				time.Sleep(30 * time.Second)
				synctest.Wait()
				if mgr.HasPendingChanges() {
					t.Fatal("history did not recover")
				}
				out, err := exec.CommandContext(t.Context(), "git", "-C", filepath.Join(dir, "ws"), "ls-tree", "-r", "--name-only", "HEAD").Output() //nolint:gosec // G204: fixed Git command over a test-owned repository.
				if err != nil {
					t.Fatal(err)
				}
				if string(out) != node+"/index.md\n"+node+"/metadata.json\n" {
					t.Fatalf("unsafe history files %q", out)
				}
				count, err := repo.CommitCount(t.Context())
				if err != nil || count != 3 {
					t.Fatalf("count=%d err=%v", count, err)
				}
				cancel()
				wg.Wait()
			})
		})
	}
	t.Run("RestartMoveDeleteAndRetire", func(t *testing.T) {
		for _, b := range backends {
			t.Run(b.name, func(t *testing.T) {
				node, parent, deleted := ksid.NewID().String(), ksid.NewID().String(), ksid.NewID().String()
				synctest.Test(t, func(t *testing.T) {
					dir := t.TempDir()
					mgr := NewManagerWithBackend(dir, "Test", "test@example.com", b.backend)
					repo, err := mgr.WorkspaceRepo(t.Context(), "ws", true)
					if err != nil {
						t.Fatal(err)
					}
					wsDir := filepath.Join(dir, "ws")
					for _, id := range []string{node, parent, deleted} {
						if err := os.MkdirAll(filepath.Join(wsDir, id), 0o700); err != nil {
							t.Fatal(err)
						}
						if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
							return "create", []string{id + "/index.md"}, os.WriteFile(filepath.Join(wsDir, id, "index.md"), []byte(id), 0o600)
						}); err != nil {
							t.Fatal(err)
						}
					}
					// These are saved snapshots left by a stopped process, including a move
					// and deletion with no request callback available to replay.
					if err := os.Rename(filepath.Join(wsDir, node), filepath.Join(wsDir, parent, node)); err != nil {
						t.Fatal(err)
					}
					if err := os.RemoveAll(filepath.Join(wsDir, deleted)); err != nil {
						t.Fatal(err)
					}
					index := filepath.Join(wsDir, ".git", "index")
					if err := os.Rename(index, index+".saved"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(index, 0o700); err != nil {
						t.Fatal(err)
					}
					restarted := NewManagerWithBackend(dir, "Test", "test@example.com", b.backend)
					reopened, err := restarted.WorkspaceRepo(t.Context(), "ws", false)
					if err != nil {
						t.Fatal(err)
					}
					if !restarted.HasPendingChanges() {
						t.Fatal("startup hid dirty history")
					}
					if err := os.Remove(index); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(index+".saved", index); err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithCancel(t.Context())
					var wg sync.WaitGroup
					wg.Go(func() { restarted.RunCommitRecovery(ctx) })
					synctest.Wait()
					if restarted.HasPendingChanges() {
						t.Fatal("startup changes did not recover")
					}
					out, err := exec.CommandContext(t.Context(), "git", "-C", wsDir, "ls-tree", "-r", "--name-only", "HEAD").Output() //nolint:gosec // G204: fixed Git command over a test-owned repository.
					if err != nil {
						t.Fatal(err)
					}
					want := parent + "/index.md\n" + parent + "/" + node + "/index.md\n"
					paths := strings.Split(strings.TrimSuffix(want, "\n"), "\n")
					slices.Sort(paths)
					if string(out) != strings.Join(paths, "\n")+"\n" {
						t.Fatalf("move/delete history=%q want=%q", out, want)
					}
					// Retired aliases cannot save or allow background work to recreate data.
					restarted.ForgetRepo("./ws")
					if err := os.RemoveAll(wsDir); err != nil {
						t.Fatal(err)
					}
					called := false
					if err := reopened.CommitTx(t.Context(), Author{}, func() (string, []string, error) { called = true; return "", nil, nil }); err == nil || called {
						t.Fatalf("retired alias err=%v called=%t", err, called)
					}
					time.Sleep(30 * time.Second)
					synctest.Wait()
					if _, err := os.Stat(wsDir); !os.IsNotExist(err) {
						t.Fatalf("retired workspace resurrected: %v", err)
					}
					cancel()
					wg.Wait()
				})
			})
		}
	})
	t.Run("PublishedMetadataUnavailable", func(t *testing.T) {
		for _, b := range backends {
			t.Run(b.name, func(t *testing.T) {
				dir := t.TempDir()
				if err := os.Mkdir(filepath.Join(dir, "ws"), 0o700); err != nil {
					t.Fatal(err)
				}
				mgr := NewManagerWithBackend(dir, "Test", "test@example.com", b.backend)
				if _, err := mgr.WorkspaceRepo(t.Context(), "ws", false); err == nil {
					t.Fatal("missing published metadata initialized")
				}
				if !mgr.HasPendingChanges() {
					t.Fatal("missing metadata reported healthy")
				}
				if _, err := os.Stat(filepath.Join(dir, "ws", ".git")); !os.IsNotExist(err) {
					t.Fatalf("metadata recreated: %v", err)
				}
				mgr.ForgetRepo("ws")
				if mgr.HasPendingChanges() {
					t.Fatal("forgotten unavailable workspace remains pending")
				}
			})
		}
	})
	t.Run("OwnerCancelsActiveGit", func(t *testing.T) {
		dir := t.TempDir()
		mgr := NewManager(dir, "Test", "test@example.com")
		repo, err := mgr.WorkspaceRepo(t.Context(), "ws", true)
		if err != nil {
			t.Fatal(err)
		}
		wsDir := filepath.Join(dir, "ws")
		node := ksid.NewID().String()
		if err := os.Mkdir(filepath.Join(wsDir, node), 0o700); err != nil {
			t.Fatal(err)
		}
		lock := filepath.Join(wsDir, ".git", "index.lock")
		if err := os.WriteFile(lock, []byte("owned"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
			return "saved", []string{node + "/index.md"}, os.WriteFile(filepath.Join(wsDir, node, "index.md"), []byte("saved"), 0o600)
		}); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(lock); err != nil {
			t.Fatal(err)
		}
		fifoPath := filepath.Join(wsDir, ".git", "recovery-started")
		if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
			t.Fatal(err)
		}
		fifo, err := os.OpenFile(fifoPath, os.O_RDWR, 0) //nolint:gosec // G304: test-owned FIFO in its temporary repository.
		if err != nil {
			t.Fatal(err)
		}
		var readers sync.WaitGroup
		t.Cleanup(func() {
			if err := fifo.Close(); err != nil {
				t.Error(err)
			}
			readers.Wait()
		})
		hook := "#!/bin/sh\nprintf '%s\\n' \"$$\" > .git/recovery-started\nexec sleep 300\n"
		if err := os.WriteFile(filepath.Join(wsDir, ".git", "hooks", "pre-commit"), []byte(hook), 0o700); err != nil { //nolint:gosec // G306: Git requires an executable hook.
			t.Fatal(err)
		}
		started := make(chan string, 1)
		readers.Go(func() {
			line, err := bufio.NewReader(fifo).ReadString('\n')
			if err != nil && !errors.Is(err, os.ErrClosed) {
				t.Error(err)
			}
			started <- line
		})
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		var workers sync.WaitGroup
		t.Cleanup(func() { cancel(); workers.Wait() })
		workers.Go(func() { mgr.RunCommitRecovery(ctx); close(done) })
		select {
		case line := <-started:
			pid, err := strconv.Atoi(strings.TrimSpace(line))
			if err != nil {
				t.Fatal(err)
			}
			proc, err := os.FindProcess(pid)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
					t.Error(err)
				}
			})
		case <-time.After(5 * time.Second):
			t.Fatal("recovery did not reach hook")
		}
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("active recovery ignored owner cancellation")
		}
		if !mgr.HasPendingChanges() {
			t.Fatal("canceled recovery lost pending history")
		}
	})
}
