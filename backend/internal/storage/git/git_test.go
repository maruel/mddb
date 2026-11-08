// Tests for Git operations.

package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var backends = []struct {
	name    string
	backend Backend
}{
	{"Exec", BackendExec},
	{"GoGit", BackendGoGit},
}

func TestRepo(t *testing.T) {
	t.Parallel()

	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()
			backend := b.backend
			testRepoAll(t, backend)
		})
	}
}

func testRepoAll(t *testing.T, backend Backend) {
	t.Helper()

	t.Run("SavedFileVersions", func(t *testing.T) {
		dir := t.TempDir()
		repo, err := NewManagerWithBackend(dir, "Test", "test@example.com", backend).Repo(t.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		readVersion := func(path string) *FileVersion {
			var v *FileVersion
			if err := repo.ReadTx(t.Context(), func(s *SavedFiles) error { v = s.Version(path); return nil }); err != nil {
				t.Fatal(err)
			}
			return v
		}
		first := readVersion("page/index.md")
		if again := readVersion("page/index.md"); again != first {
			t.Fatal("unchanged file lost its generation")
		}
		if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
			return "unrelated", []string{"other"}, os.WriteFile(filepath.Join(dir, "other"), []byte("other"), 0o600)
		}); err != nil {
			t.Fatal(err)
		}
		if readVersion("page/index.md") != first {
			t.Fatal("unrelated write invalidated page")
		}
		if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
			if err := os.MkdirAll(filepath.Join(dir, "page"), 0o700); err != nil {
				return "", nil, err
			}
			return "page", []string{"page"}, os.WriteFile(filepath.Join(dir, "page", "index.md"), []byte("saved"), 0o600)
		}); err != nil {
			t.Fatal(err)
		}
		second := readVersion("page/index.md")
		if second == first {
			t.Fatal("directory mutation retained child generation")
		}
		failed := errors.New("failed after saving")
		if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
			if err := os.WriteFile(filepath.Join(dir, "page", "index.md"), []byte("partial"), 0o600); err != nil {
				return "", nil, err
			}
			return "", nil, failed
		}); !errors.Is(err, failed) {
			t.Fatal(err)
		}
		third := readVersion("page/index.md")
		if third == second {
			t.Fatal("failed callback retained potentially changed generation")
		}
		if _, err := repo.Pull(t.Context(), "missing", "main"); err == nil {
			t.Fatal("pull unexpectedly succeeded")
		}
		if readVersion("page/index.md") == third {
			t.Fatal("pull retained potentially changed generation")
		}
		beforeFailedCommit := readVersion("page/index.md")
		if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
			if err := os.WriteFile(filepath.Join(dir, "page", "index.md"), []byte("saved without history"), 0o600); err != nil {
				return "", nil, err
			}
			return "cannot stage missing path", []string{"page/index.md", "missing/path"}, nil
		}); err == nil {
			t.Fatal("commit unexpectedly succeeded")
		}
		if readVersion("page/index.md") == beforeFailedCommit {
			t.Fatal("failed history recording retained saved file generation")
		}

		beforeEviction := readVersion("page/index.md")
		for i := range 256 {
			readVersion(fmt.Sprintf("eviction-%d", i))
		}
		if readVersion("page/index.md") == beforeEviction {
			t.Fatal("evicted generation reused")
		}
	})

	t.Run("UnbornHistory", func(t *testing.T) {
		repo, err := NewManagerWithBackend(t.TempDir(), "Test", "test@example.com", backend).Repo(t.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		n, err := repo.CommitCount(t.Context())
		if err != nil || n != 0 {
			t.Fatalf("unborn count=%d err=%v", n, err)
		}
		h, err := repo.GetHistory(t.Context(), "row", 10)
		if err != nil || len(h) != 0 {
			t.Fatalf("unborn history=%v err=%v", h, err)
		}
	})

	t.Run("HistoryReadFailures", func(t *testing.T) {
		for _, mode := range []string{"canceled", "missing HEAD object", "missing ancestor object", "invalid HEAD"} {
			t.Run(mode, func(t *testing.T) {
				dir := t.TempDir()
				repo, err := NewManagerWithBackend(dir, "Test", "test@example.com", backend).Repo(t.Context(), "")
				if err != nil {
					t.Fatal(err)
				}
				if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
					return "first", []string{"row"}, os.WriteFile(filepath.Join(dir, "row"), []byte("first"), 0o600)
				}); err != nil {
					t.Fatal(err)
				}

				ctx := t.Context()
				switch mode {
				case "canceled":
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
				case "missing HEAD object", "missing ancestor object":
					out, err := gitOutput(dir, "rev-parse", "HEAD")
					if err != nil {
						t.Fatal(err)
					}
					hash := strings.TrimSpace(string(out))
					if mode == "missing ancestor object" {
						if err := repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
							return "second", []string{"row"}, os.WriteFile(filepath.Join(dir, "row"), []byte("second"), 0o600)
						}); err != nil {
							t.Fatal(err)
						}
					}
					if err := os.Remove(filepath.Join(dir, ".git", "objects", hash[:2], hash[2:])); err != nil {
						t.Fatal(err)
					}
					// Reopen to avoid a previously loaded object's cache masking on-disk corruption.
					repo, err = NewManagerWithBackend(dir, "Test", "test@example.com", backend).Repo(ctx, "")
					if err != nil {
						t.Fatal(err)
					}
				case "invalid HEAD":
					if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("invalid reference\n"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				n, countErr := repo.CommitCount(ctx)
				h, historyErr := repo.GetHistory(ctx, "row", 10)

				if countErr == nil {
					t.Errorf("CommitCount hid %s: count=%d", mode, n)
				}
				if historyErr == nil {
					t.Errorf("GetHistory hid %s: history length=%d", mode, len(h))
				}
				if mode == "canceled" && (!errors.Is(countErr, context.Canceled) || !errors.Is(historyErr, context.Canceled)) {
					t.Errorf("cancellation lost: %v / %v", countErr, historyErr)
				}
			})
		}
	})
	t.Run("SavedWriteSurvivesCancellation", func(t *testing.T) {
		dir := t.TempDir()
		repo, err := NewManagerWithBackend(dir, "Test", "test@example.com", backend).Repo(t.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		if err := repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
			err := os.WriteFile(filepath.Join(dir, "row"), []byte("saved"), 0o600)
			cancel()
			return "saved", []string{"row"}, err
		}); err != nil {
			t.Fatal(err)
		}
		out, err := repo.GetFileAtCommit(t.Context(), "HEAD", "row")
		if err != nil || string(out) != "saved" {
			t.Fatalf("saved write not committed: %q %v", out, err)
		}
	})
	t.Run("CanceledInitialization", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := NewManagerWithBackend(t.TempDir(), "Test", "test@example.com", backend).Repo(ctx, "")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("initialization lost cancellation: %v", err)
		}
	})
	t.Run("CanceledRemoteOperations", func(t *testing.T) {
		dir := t.TempDir()
		repo, err := NewManagerWithBackend(dir, "Test", "test@example.com", backend).Repo(t.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := repo.Fetch(ctx, "missing", ""); !errors.Is(err, context.Canceled) {
			t.Errorf("Fetch lost cancellation: %v", err)
		}
		if err := repo.Push(ctx, "missing", ""); !errors.Is(err, context.Canceled) {
			t.Errorf("Push lost cancellation: %v", err)
		}
		if _, err := repo.Pull(ctx, "missing", ""); !errors.Is(err, context.Canceled) {
			t.Errorf("Pull lost cancellation: %v", err)
		}
	})

	t.Run("ActiveRemoteCancellation", func(t *testing.T) {
		for _, operation := range []string{"fetch", "push"} {
			t.Run(operation, func(t *testing.T) {
				dir := t.TempDir()
				repo, err := NewManagerWithBackend(dir, "Test", "test@example.com", backend).Repo(t.Context(), "")
				if err != nil {
					t.Fatal(err)
				}
				if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
					return "saved", []string{"row"}, os.WriteFile(filepath.Join(dir, "row"), []byte("saved"), 0o600)
				}); err != nil {
					t.Fatal(err)
				}
				started := make(chan struct{}, 1)
				release := make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					select {
					case started <- struct{}{}:
					default:
					}
					<-release
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
				t.Cleanup(server.Close)
				if err := repo.SetRemote(t.Context(), "origin", server.URL); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(t.Context())
				t.Cleanup(cancel)
				var wg sync.WaitGroup
				t.Cleanup(func() { close(release); wg.Wait() })
				done := make(chan error, 1)
				wg.Go(func() {
					if operation == "fetch" {
						done <- repo.Fetch(ctx, "origin", "")
					} else {
						done <- repo.Push(ctx, "origin", "")
					}
				})
				select {
				case <-started:
				case <-time.After(3 * time.Second):
					t.Fatal("remote operation never reached server")
				}
				cancel()
				select {
				case err := <-done:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("remote cancellation was lost: %v", err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("remote operation ignored active owner cancellation")
				}
			})
		}
	})
	t.Run("UnavailableNestedRepository", func(t *testing.T) {
		root := t.TempDir()
		parent, err := NewManagerWithBackend(root, "Test", "test@example.com", backend).Repo(t.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		if err := parent.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
			return "parent", []string{"parent"}, os.WriteFile(filepath.Join(root, "parent"), []byte("parent"), 0o600)
		}); err != nil {
			t.Fatal(err)
		}
		repo, err := NewManagerWithBackend(root, "Test", "test@example.com", backend).Repo(t.Context(), "child")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(root, "child", ".git"), filepath.Join(root, "child", "unavailable")); err != nil {
			t.Fatal(err)
		}
		if n, err := repo.CommitCount(t.Context()); err == nil {
			t.Errorf("unavailable child history count=%d, no error", n)
		}
		if h, err := repo.GetHistory(t.Context(), ".", 10); err == nil {
			t.Errorf("unavailable child history=%v, no error", h)
		}
	})

	t.Run("HistoryMessageSeparators", func(t *testing.T) {
		dir := t.TempDir()
		repo, err := NewManagerWithBackend(dir, "Test", "test@example.com", backend).Repo(t.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		message := "record separator \x1e in subject\n\nBody with \x1e separator."
		if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
			return message, []string{"row"}, os.WriteFile(filepath.Join(dir, "row"), []byte("saved"), 0o600)
		}); err != nil {
			t.Fatal(err)
		}
		h, err := repo.GetHistory(t.Context(), "row", 10)
		if err != nil || len(h) != 1 {
			t.Fatalf("history=%v err=%v", h, err)
		}
		if h[0].Message != "record separator \x1e in subject" || h[0].Body != "Body with \x1e separator." {
			t.Fatalf("message changed: %+v", h[0])
		}
	})
	t.Run("CanceledMutationDoesNotRunCallback", func(t *testing.T) {
		repo, err := NewManagerWithBackend(t.TempDir(), "Test", "test@example.com", backend).Repo(t.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		called := false
		err = repo.CommitTx(ctx, Author{}, func() (string, []string, error) { called = true; return "", nil, nil })
		if !errors.Is(err, context.Canceled) || called {
			t.Fatalf("canceled mutation: callback=%v err=%v", called, err)
		}
	})
	t.Run("CanceledCommitFS", func(t *testing.T) {
		repo, err := NewManagerWithBackend(t.TempDir(), "Test", "test@example.com", backend).Repo(t.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = repo.FSAtCommit(ctx, "HEAD").Open("row")
		if !errors.Is(err, context.Canceled) || errors.Is(err, os.ErrNotExist) {
			t.Fatalf("commit filesystem lost cancellation: %v", err)
		}
	})

	t.Run("Init", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()

		mgr := NewManagerWithBackend(tmpDir, "Test User", "test@example.com", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatalf("Repo() failed: %v", err)
		}
		_ = repo

		// Check .git exists
		if _, err := os.Stat(filepath.Join(tmpDir, ".git")); os.IsNotExist(err) {
			t.Error(".git directory not created")
		}

		// Check config
		checkConfig(t, tmpDir, "user.name", "Test User")
		checkConfig(t, tmpDir, "user.email", "test@example.com")

		// Test subdir init
		subDir := filepath.Join(tmpDir, "subdir")
		if err := os.Mkdir(subDir, 0o700); err != nil {
			t.Fatalf("failed to create subdir: %v", err)
		}

		_, err = mgr.Repo(ctx, "subdir")
		if err != nil {
			t.Fatalf("Repo(subdir) failed: %v", err)
		}

		if _, err := os.Stat(filepath.Join(subDir, ".git")); os.IsNotExist(err) {
			t.Error("subdir .git directory not created")
		}
	})

	t.Run("Commit", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()

		mgr := NewManagerWithBackend(tmpDir, "Test User", "test@example.com", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatalf("Repo() failed: %v", err)
		}

		// Create a file
		testFile := "test.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("hello world"), 0o600); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		// Commit using CommitTx
		author := Author{Name: "Author", Email: "author@example.com"}
		err = repo.CommitTx(ctx, author, func() (string, []string, error) {
			return "Initial commit", []string{testFile}, nil
		})
		if err != nil {
			t.Fatalf("CommitTx() failed: %v", err)
		}

		// Verify log
		history, err := repo.GetHistory(ctx, testFile, 1)
		if err != nil {
			t.Fatalf("GetHistory() failed: %v", err)
		}

		if len(history) != 1 {
			t.Fatalf("expected 1 commit, got %d", len(history))
		}
		if history[0].Message != "Initial commit" {
			t.Errorf("expected message 'Initial commit', got '%s'", history[0].Message)
		}
		if history[0].Author != "Author" {
			t.Errorf("expected author 'Author', got '%s'", history[0].Author)
		}
	})

	t.Run("GetHistory", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "Test User", "test@example.com", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatalf("Repo() failed: %v", err)
		}

		testFile := "test.txt"
		author := Author{}

		// Commit 1
		if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("v1"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := repo.CommitTx(ctx, author, func() (string, []string, error) {
			return "Commit 1", []string{testFile}, nil
		}); err != nil {
			t.Fatal(err)
		}

		// Commit 2
		if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("v2"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := repo.CommitTx(ctx, author, func() (string, []string, error) {
			return "Commit 2", []string{testFile}, nil
		}); err != nil {
			t.Fatal(err)
		}

		history, err := repo.GetHistory(ctx, testFile, 10)
		if err != nil {
			t.Fatalf("GetHistory() failed: %v", err)
		}

		if len(history) != 2 {
			t.Fatalf("expected 2 commits, got %d", len(history))
		}
		if history[0].Message != "Commit 2" {
			t.Errorf("expected first commit to be 'Commit 2', got '%s'", history[0].Message)
		}
		if history[1].Message != "Commit 1" {
			t.Errorf("expected second commit to be 'Commit 1', got '%s'", history[1].Message)
		}
	})

	t.Run("GetFileAtCommit", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "Test User", "test@example.com", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatalf("Repo() failed: %v", err)
		}

		testFile := "test.txt"
		author := Author{}

		// Commit 1
		if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("content v1"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := repo.CommitTx(ctx, author, func() (string, []string, error) {
			return "Commit 1", []string{testFile}, nil
		}); err != nil {
			t.Fatal(err)
		}

		history, err := repo.GetHistory(ctx, testFile, 1)
		if err != nil {
			t.Fatal(err)
		}
		v1Hash := history[0].Hash

		// Commit 2
		if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("content v2"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := repo.CommitTx(ctx, author, func() (string, []string, error) {
			return "Commit 2", []string{testFile}, nil
		}); err != nil {
			t.Fatal(err)
		}

		// Get v1 content
		content, err := repo.GetFileAtCommit(ctx, v1Hash, testFile)
		if err != nil {
			t.Fatalf("GetFileAtCommit() failed: %v", err)
		}

		if string(content) != "content v1" {
			t.Errorf("expected 'content v1', got '%s'", string(content))
		}
	})

	t.Run("SetRemote", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "Test User", "test@example.com", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		remoteURL := "https://github.com/example/repo.git"
		if err := repo.SetRemote(ctx, "origin", remoteURL); err != nil {
			t.Fatalf("SetRemote() failed: %v", err)
		}

		// #nosec G204
		cmd := exec.Command("git", "remote", "get-url", "origin")
		cmd.Dir = tmpDir
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git remote get-url failed: %v", err)
		}

		if strings.TrimSpace(string(out)) != remoteURL {
			t.Errorf("expected remote url %s, got %s", remoteURL, string(out))
		}

		// Test removing remote
		if err := repo.SetRemote(ctx, "origin", ""); err != nil {
			t.Fatalf("SetRemote(empty) failed: %v", err)
		}

		// #nosec G204
		cmd = exec.Command("git", "remote")
		cmd.Dir = tmpDir
		out, err = cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "origin") {
			t.Error("remote origin should have been removed")
		}
	})

	t.Run("Push", func(t *testing.T) {
		t.Parallel()
		// Create "remote" bare repo
		remoteDir := t.TempDir()
		// #nosec G204
		initBare := exec.Command("git", "init", "--bare")
		initBare.Dir = remoteDir
		if err := initBare.Run(); err != nil {
			t.Fatalf("failed to init bare repo: %v", err)
		}

		// Local repo
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "Test User", "test@example.com", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		// Commit something
		testFile := "test.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
			return "Initial commit", []string{testFile}, nil
		}); err != nil {
			t.Fatal(err)
		}

		// Add remote
		if err := repo.SetRemote(ctx, "origin", remoteDir); err != nil {
			t.Fatal(err)
		}

		// Push
		if err := repo.Push(ctx, "origin", "master"); err != nil {
			t.Fatalf("Push() failed: %v", err)
		}

		// Verify in remote
		// #nosec G204
		cmd := exec.Command("git", "log", "master", "-n", "1", "--format=%s")
		cmd.Dir = remoteDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("failed to read log from remote: %v, output: %s", err, string(out))
		}
		if strings.TrimSpace(string(out)) != "Initial commit" {
			t.Errorf("expected remote to have commit 'Initial commit', got '%s'", string(out))
		}
	})

	t.Run("SeparateRepos", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()

		mgr := NewManagerWithBackend(tmpDir, "Root User", "root@example.com", backend)

		// Create root repo
		rootRepo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		// Create subdir repo
		subDirName := "subrepo"
		subRepo, err := mgr.Repo(ctx, subDirName)
		if err != nil {
			t.Fatal(err)
		}

		// File in subdir
		subDir := filepath.Join(tmpDir, subDirName)
		testFile := "file_in_sub.txt"
		if err := os.WriteFile(filepath.Join(subDir, testFile), []byte("content"), 0o600); err != nil {
			t.Fatal(err)
		}

		author := Author{Name: "Sub User", Email: "sub@example.com"}
		if err := subRepo.CommitTx(ctx, author, func() (string, []string, error) {
			return "Sub commit", []string{testFile}, nil
		}); err != nil {
			t.Fatalf("CommitTx(subdir) failed: %v", err)
		}

		// Verify subdir commit
		history, err := subRepo.GetHistory(ctx, testFile, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) == 0 {
			t.Fatal("subdir should have history")
		}
		if history[0].Message != "Sub commit" {
			t.Errorf("expected 'Sub commit', got '%s'", history[0].Message)
		}

		// Root repo should NOT have subdir commits (they are independent now)
		rootHistory, err := rootRepo.GetHistory(ctx, ".", 5)
		if err != nil {
			t.Fatal(err)
		}

		for _, h := range rootHistory {
			if strings.Contains(h.Message, "Sub commit") || strings.Contains(h.Message, "sync:") {
				t.Errorf("root repo should not have subdir or sync commits, found: %s", h.Message)
			}
		}
	})

	t.Run("NewManagerDefaults", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()

		mgr := NewManagerWithBackend(tmpDir, "", "", backend)
		_, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatalf("Repo() failed: %v", err)
		}

		// Verify defaults were set in git config
		checkConfig(t, tmpDir, "user.name", "mddb")
		checkConfig(t, tmpDir, "user.email", "mddb@localhost")
	})

	t.Run("PushAutoBranch", func(t *testing.T) {
		t.Parallel()
		// Remote
		remoteDir := t.TempDir()
		// #nosec G204
		if err := exec.Command("git", "-C", remoteDir, "init", "--bare").Run(); err != nil {
			t.Fatalf("failed to init bare repo: %v", err)
		}

		// Local
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "User", "email", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		// Commit
		file := "f.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, file), []byte("content"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
			return "msg", []string{file}, nil
		}); err != nil {
			t.Fatal(err)
		}

		// Add remote
		if err := repo.SetRemote(ctx, "origin", remoteDir); err != nil {
			t.Fatal(err)
		}

		// Push with empty branch (should detect master/main)
		if err := repo.Push(ctx, "origin", ""); err != nil {
			t.Fatalf("Push(empty branch) failed: %v", err)
		}

		// Verify - check all branches/refs because HEAD might point to an unborn branch (e.g. master) while we pushed main
		// #nosec G204
		out, err := exec.Command("git", "-C", remoteDir, "log", "--all", "-n", "1", "--format=%s").CombinedOutput()
		if err != nil {
			t.Fatalf("failed to read log from remote: %v, output: %s", err, string(out))
		}
		if strings.TrimSpace(string(out)) != "msg" {
			t.Errorf("expected remote to have commit 'msg', got '%s'", string(out))
		}
	})

	t.Run("CommitTxEdgeCases", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "User", "email", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		// Empty files list
		if err := repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
			return "msg", nil, nil
		}); err != nil {
			t.Errorf("CommitTx(nil) failed: %v", err)
		}

		// Commit a file
		file := "f.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, file), []byte("content"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
			return "msg", []string{file}, nil
		}); err != nil {
			t.Fatal(err)
		}

		// Try to commit same file again without changes - should be no-op
		if err := repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
			return "msg2", []string{file}, nil
		}); err != nil {
			t.Errorf("CommitTx(no changes) failed: %v", err)
		}

		// Verify no new commit
		history, err := repo.GetHistory(ctx, file, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) != 1 {
			t.Errorf("expected 1 commit, got %d", len(history))
		}
	})

	t.Run("GetHistoryEdgeCases", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "User", "email", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		file := "f.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, file), []byte("c"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
			return "msg", []string{file}, nil
		}); err != nil {
			t.Fatal(err)
		}

		// n=0 => default to 1000
		h, err := repo.GetHistory(ctx, file, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(h) != 1 {
			t.Errorf("expected 1 commit, got %d", len(h))
		}

		// invalid path
		h, err = repo.GetHistory(ctx, "nonexistent", 1)
		if err != nil {
			t.Errorf("GetHistory(nonexistent) should not error but returned: %v", err)
		}
		if len(h) != 0 {
			t.Error("expected empty history for nonexistent file")
		}
	})

	t.Run("ParseDate", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "User", "email", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		file := "date.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, file), []byte("d"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
			return "msg", []string{file}, nil
		}); err != nil {
			t.Fatal(err)
		}

		history, err := repo.GetHistory(ctx, file, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) > 0 {
			if history[0].AuthorDate.IsZero() {
				t.Error("AuthorDate should not be zero")
			}
			if history[0].CommitDate.IsZero() {
				t.Error("CommitDate should not be zero")
			}
		}
	})

	t.Run("GetFileAtCommitFailure", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "User", "email", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		// No commit yet
		if _, err := repo.GetFileAtCommit(ctx, "HEAD", "file"); err == nil {
			t.Error("GetFileAtCommit(HEAD) should fail on empty repo")
		}

		// Commit
		file := "f.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, file), []byte("c"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
			return "msg", []string{file}, nil
		}); err != nil {
			t.Fatal(err)
		}

		// Invalid hash
		if _, err := repo.GetFileAtCommit(ctx, "invalidhash", file); err == nil {
			t.Error("GetFileAtCommit(invalidhash) should fail")
		}

		// File not in commit
		if _, err := repo.GetFileAtCommit(ctx, "HEAD", "missing.txt"); err == nil {
			t.Error("GetFileAtCommit(missing) should fail")
		}
	})

	t.Run("RepoIdempotent", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "User", "email", backend)

		repo1, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		// Get again - should return cached repo
		repo2, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatalf("Repo() second time failed: %v", err)
		}

		// Should be same instance
		if repo1 != repo2 {
			t.Error("expected same repo instance")
		}

		// Verify config still there (not overwritten or errored)
		checkConfig(t, tmpDir, "user.name", "User")
	})

	t.Run("CommitTxNonExistentFile", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "User", "email", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		if err := repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
			return "msg", []string{"missing.txt"}, nil
		}); err == nil {
			t.Error("CommitTx(missing file) should fail")
		}
	})

	t.Run("SetRemoteRemoveNonExistent", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "User", "email", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		// Remove non-existent remote should be no-op
		if err := repo.SetRemote(ctx, "origin", ""); err != nil {
			t.Errorf("SetRemote(remove non-existent) failed: %v", err)
		}
	})

	t.Run("CommitTx", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "User", "user@example.com", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		// Create multiple files
		file1 := "file1.txt"
		file2 := "file2.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, file1), []byte("content1"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, file2), []byte("content2"), 0o600); err != nil {
			t.Fatal(err)
		}

		// Commit both files in a single transaction
		author := Author{Name: "Tx Author", Email: "tx@example.com"}
		err = repo.CommitTx(ctx, author, func() (string, []string, error) {
			return "create: file1 and file2", []string{file1, file2}, nil
		})
		if err != nil {
			t.Fatalf("CommitTx() failed: %v", err)
		}

		// Verify single commit
		history, err := repo.GetHistory(ctx, ".", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) != 1 {
			t.Fatalf("expected 1 commit, got %d", len(history))
		}
		if history[0].Message != "create: file1 and file2" {
			t.Errorf("expected message 'create: file1 and file2', got '%s'", history[0].Message)
		}
		if history[0].Author != "Tx Author" {
			t.Errorf("expected author 'Tx Author', got '%s'", history[0].Author)
		}
	})

	t.Run("CommitTxError", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "User", "user@example.com", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		// Create a file
		file := "file.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, file), []byte("content"), 0o600); err != nil {
			t.Fatal(err)
		}

		// Transaction that returns error - should not commit
		testErr := errors.New("test error")
		err = repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
			return "msg", []string{file}, testErr
		})
		if !errors.Is(err, testErr) {
			t.Errorf("expected testErr, got %v", err)
		}

		// Verify no commit was made
		history, err := repo.GetHistory(ctx, file, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) != 0 {
			t.Errorf("expected 0 commits after error, got %d", len(history))
		}
	})

	t.Run("CommitTxEmpty", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "User", "user@example.com", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		// Transaction that returns no files - should be no-op
		err = repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
			return "", nil, nil
		})
		if err != nil {
			t.Errorf("CommitTx(empty) failed: %v", err)
		}
	})

	t.Run("FS", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "User", "user@example.com", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		// Create a file
		file := "test.txt"
		content := []byte("hello world")
		if err := os.WriteFile(filepath.Join(tmpDir, file), content, 0o600); err != nil {
			t.Fatal(err)
		}

		// Get FS and read file
		fs := repo.FS()
		f, err := fs.Open(file)
		if err != nil {
			t.Fatalf("FS.Open() failed: %v", err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				t.Errorf("Close() failed: %v", err)
			}
		}()

		data := make([]byte, len(content))
		n, err := f.Read(data)
		if err != nil {
			t.Fatalf("Read() failed: %v", err)
		}
		if n != len(content) || !bytes.Equal(data, content) {
			t.Errorf("expected %q, got %q", string(content), string(data))
		}
	})

	t.Run("FSAtCommit", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		mgr := NewManagerWithBackend(tmpDir, "User", "user@example.com", backend)
		repo, err := mgr.Repo(ctx, "")
		if err != nil {
			t.Fatal(err)
		}

		file := "test.txt"
		v1Content := []byte("version 1")
		v2Content := []byte("version 2")

		// Commit v1
		if err := os.WriteFile(filepath.Join(tmpDir, file), v1Content, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
			return "v1", []string{file}, nil
		}); err != nil {
			t.Fatal(err)
		}

		history, err := repo.GetHistory(ctx, file, 1)
		if err != nil {
			t.Fatal(err)
		}
		v1Hash := history[0].Hash

		// Commit v2
		if err := os.WriteFile(filepath.Join(tmpDir, file), v2Content, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
			return "v2", []string{file}, nil
		}); err != nil {
			t.Fatal(err)
		}

		// Get FS at v1 commit
		fs := repo.FSAtCommit(ctx, v1Hash)
		f, err := fs.Open(file)
		if err != nil {
			t.Fatalf("FSAtCommit.Open() failed: %v", err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				t.Errorf("Close() failed: %v", err)
			}
		}()

		data := make([]byte, len(v1Content))
		n, err := f.Read(data)
		if err != nil {
			t.Fatalf("Read() failed: %v", err)
		}
		if n != len(v1Content) || !bytes.Equal(data, v1Content) {
			t.Errorf("expected %q, got %q", string(v1Content), string(data))
		}
	})
}

func checkConfig(t *testing.T, dir, key, expected string) {
	t.Helper()
	// #nosec G204
	cmd := exec.Command("git", "config", key)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git config %s failed: %v", key, err)
	}
	if strings.TrimSpace(string(out)) != expected {
		t.Errorf("expected %s=%s, got %s", key, expected, string(out))
	}
}
