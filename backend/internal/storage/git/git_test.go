package git

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepo(t *testing.T) {
	t.Parallel()

	t.Run("Init", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()

		mgr := NewManager(tmpDir, "Test User", "test@example.com")
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

		mgr := NewManager(tmpDir, "Test User", "test@example.com")
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
		mgr := NewManager(tmpDir, "Test User", "test@example.com")
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
		mgr := NewManager(tmpDir, "Test User", "test@example.com")
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
		mgr := NewManager(tmpDir, "Test User", "test@example.com")
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
		mgr := NewManager(tmpDir, "Test User", "test@example.com")
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

		mgr := NewManager(tmpDir, "Root User", "root@example.com")

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

		mgr := NewManager(tmpDir, "", "")
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
		mgr := NewManager(tmpDir, "User", "email")
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
		mgr := NewManager(tmpDir, "User", "email")
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
		mgr := NewManager(tmpDir, "User", "email")
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
		mgr := NewManager(tmpDir, "User", "email")
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
		mgr := NewManager(tmpDir, "User", "email")
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
		mgr := NewManager(tmpDir, "User", "email")

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
		mgr := NewManager(tmpDir, "User", "email")
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
		mgr := NewManager(tmpDir, "User", "email")
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
		mgr := NewManager(tmpDir, "User", "user@example.com")
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
		mgr := NewManager(tmpDir, "User", "user@example.com")
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
		mgr := NewManager(tmpDir, "User", "user@example.com")
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
		mgr := NewManager(tmpDir, "User", "user@example.com")
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
		mgr := NewManager(tmpDir, "User", "user@example.com")
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
