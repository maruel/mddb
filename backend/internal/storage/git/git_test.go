package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestClient(t *testing.T) {
	t.Parallel()

	t.Run("Init", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()

		client, err := New(ctx, tmpDir, "Test User", "test@example.com")
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

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

		if err := client.Init(ctx, "subdir"); err != nil {
			t.Fatalf("Init(subdir) failed: %v", err)
		}

		if _, err := os.Stat(filepath.Join(subDir, ".git")); os.IsNotExist(err) {
			t.Error("subdir .git directory not created")
		}
	})

	t.Run("Commit", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()

		client, err := New(ctx, tmpDir, "Test User", "test@example.com")
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		// Create a file
		testFile := "test.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("hello world"), 0o600); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		// Commit
		if err := client.Commit(ctx, "", "Author", "author@example.com", "Initial commit", []string{testFile}); err != nil {
			t.Fatalf("Commit() failed: %v", err)
		}

		// Verify log
		history, err := client.GetHistory(ctx, "", testFile, 1)
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
		client, err := New(ctx, tmpDir, "Test User", "test@example.com")
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		testFile := "test.txt"
		// Commit 1
		if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("v1"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := client.Commit(ctx, "", "", "", "Commit 1", []string{testFile}); err != nil {
			t.Fatal(err)
		}

		// Commit 2
		if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("v2"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := client.Commit(ctx, "", "", "", "Commit 2", []string{testFile}); err != nil {
			t.Fatal(err)
		}

		history, err := client.GetHistory(ctx, "", testFile, 10)
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
		client, err := New(ctx, tmpDir, "Test User", "test@example.com")
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		testFile := "test.txt"
		// Commit 1
		if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("content v1"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := client.Commit(ctx, "", "", "", "Commit 1", []string{testFile}); err != nil {
			t.Fatal(err)
		}

		history, err := client.GetHistory(ctx, "", testFile, 1)
		if err != nil {
			t.Fatal(err)
		}
		v1Hash := history[0].Hash

		// Commit 2
		if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("content v2"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := client.Commit(ctx, "", "", "", "Commit 2", []string{testFile}); err != nil {
			t.Fatal(err)
		}

		// Get v1 content
		content, err := client.GetFileAtCommit(ctx, "", v1Hash, testFile)
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
		client, err := New(ctx, tmpDir, "Test User", "test@example.com")
		if err != nil {
			t.Fatal(err)
		}

		remoteURL := "https://github.com/example/repo.git"
		if err := client.SetRemote(ctx, "", "origin", remoteURL); err != nil {
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
		if err := client.SetRemote(ctx, "", "origin", ""); err != nil {
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
		client, err := New(ctx, tmpDir, "Test User", "test@example.com")
		if err != nil {
			t.Fatal(err)
		}

		// Commit something
		testFile := "test.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, testFile), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := client.Commit(ctx, "", "", "", "Initial commit", []string{testFile}); err != nil {
			t.Fatal(err)
		}

		// Add remote
		if err := client.SetRemote(ctx, "", "origin", remoteDir); err != nil {
			t.Fatal(err)
		}

		// Push
		if err := client.Push(ctx, "", "origin", "master"); err != nil {
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

	t.Run("SubdirCommitSync", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()

		client, err := New(ctx, tmpDir, "Root User", "root@example.com")
		if err != nil {
			t.Fatal(err)
		}

		// Create subdir as a separate repo
		subDirName := "subrepo"
		subDir := filepath.Join(tmpDir, subDirName)
		if err := os.Mkdir(subDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := client.Init(ctx, subDirName); err != nil {
			t.Fatal(err)
		}

		// File in subdir
		testFile := "file_in_sub.txt"
		if err := os.WriteFile(filepath.Join(subDir, testFile), []byte("content"), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := client.Commit(ctx, subDirName, "Sub User", "sub@example.com", "Sub commit", []string{testFile}); err != nil {
			t.Fatalf("Commit(subdir) failed: %v", err)
		}

		// Verify subdir commit
		history, err := client.GetHistory(ctx, subDirName, testFile, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) == 0 {
			t.Fatal("subdir should have history")
		}

		rootHistory, err := client.GetHistory(ctx, "", ".", 5)
		if err != nil {
			t.Fatal(err)
		}

		foundSync := false
		for _, h := range rootHistory {
			if strings.Contains(h.Message, fmt.Sprintf("sync: %s update", subDirName)) {
				foundSync = true
				break
			}
		}

		if !foundSync {
			t.Error("root repo should have a sync commit for the subdir")
		}
	})

	t.Run("NewDefaults", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()

		_, err := New(ctx, tmpDir, "", "")
		if err != nil {
			t.Fatalf("New() failed: %v", err)
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
		client, err := New(ctx, tmpDir, "User", "email")
		if err != nil {
			t.Fatal(err)
		}

		// Commit
		file := "f.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, file), []byte("content"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := client.Commit(ctx, "", "", "", "msg", []string{file}); err != nil {
			t.Fatal(err)
		}

		// Add remote
		if err := client.SetRemote(ctx, "", "origin", remoteDir); err != nil {
			t.Fatal(err)
		}

		// Push with empty branch (should detect master/main)
		if err := client.Push(ctx, "", "origin", ""); err != nil {
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

	t.Run("CommitEdgeCases", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		client, err := New(ctx, tmpDir, "User", "email")
		if err != nil {
			t.Fatal(err)
		}

		// Empty files list
		if err := client.Commit(ctx, "", "", "", "msg", nil); err != nil {
			t.Errorf("Commit(nil) failed: %v", err)
		}

		// No changes
		file := "f.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, file), []byte("content"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := client.Commit(ctx, "", "", "", "msg", []string{file}); err != nil {
			t.Fatal(err)
		}

		// Try to commit same file again without changes
		if err := client.Commit(ctx, "", "", "", "msg2", []string{file}); err != nil {
			t.Errorf("Commit(no changes) failed: %v", err)
		}

		// Verify no new commit
		history, err := client.GetHistory(ctx, "", file, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) != 1 {
			t.Errorf("expected 1 commit, got %d", len(history))
		}
	})

	t.Run("DirEscape", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		client, err := New(ctx, tmpDir, "User", "email")
		if err != nil {
			t.Fatal(err)
		}

		// Test Init with escape
		if err := client.Init(ctx, "../escape"); err == nil {
			t.Error("Init(../escape) should fail")
		}

		// Test Commit with escape
		if err := client.Commit(ctx, "/abs/path", "", "", "msg", nil); err == nil {
			t.Error("Commit(/abs/path) should fail")
		}
	})

	t.Run("GetHistoryEdgeCases", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		client, err := New(ctx, tmpDir, "User", "email")
		if err != nil {
			t.Fatal(err)
		}

		file := "f.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, file), []byte("c"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := client.Commit(ctx, "", "", "", "msg", []string{file}); err != nil {
			t.Fatal(err)
		}

		// n=0 => default to 1000
		h, err := client.GetHistory(ctx, "", file, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(h) != 1 {
			t.Errorf("expected 1 commit, got %d", len(h))
		}

		// invalid path
		h, err = client.GetHistory(ctx, "", "nonexistent", 1)
		if err != nil {
			t.Errorf("GetHistory(nonexistent) should not error but returned: %v", err)
		}
		if len(h) != 0 {
			t.Error("expected empty history for nonexistent file")
		}
	})

	t.Run("ParseDate", func(t *testing.T) {
		t.Parallel()
		// However, we can verify that the time.Time objects in GetHistory are valid

		tmpDir := t.TempDir()
		ctx := t.Context()
		client, err := New(ctx, tmpDir, "User", "email")
		if err != nil {
			t.Fatal(err)
		}

		file := "date.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, file), []byte("d"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := client.Commit(ctx, "", "", "", "msg", []string{file}); err != nil {
			t.Fatal(err)
		}

		history, err := client.GetHistory(ctx, "", file, 1)
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
		client, err := New(ctx, tmpDir, "User", "email")
		if err != nil {
			t.Fatal(err)
		}

		// No commit yet
		if _, err := client.GetFileAtCommit(ctx, "", "HEAD", "file"); err == nil {
			t.Error("GetFileAtCommit(HEAD) should fail on empty repo")
		}

		// Commit
		file := "f.txt"
		if err := os.WriteFile(filepath.Join(tmpDir, file), []byte("c"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := client.Commit(ctx, "", "", "", "msg", []string{file}); err != nil {
			t.Fatal(err)
		}

		// Invalid hash
		if _, err := client.GetFileAtCommit(ctx, "", "invalidhash", file); err == nil {
			t.Error("GetFileAtCommit(invalidhash) should fail")
		}

		// File not in commit
		if _, err := client.GetFileAtCommit(ctx, "", "HEAD", "missing.txt"); err == nil {
			t.Error("GetFileAtCommit(missing) should fail")
		}
	})

	t.Run("InitIdempotent", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		client, err := New(ctx, tmpDir, "User", "email")
		if err != nil {
			t.Fatal(err)
		}

		// Init again
		if err := client.Init(ctx, ""); err != nil {
			t.Fatalf("Init() second time failed: %v", err)
		}

		// Verify config still there (not overwritten or errored)
		checkConfig(t, tmpDir, "user.name", "User")
	})

	t.Run("CommitNonExistentFile", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		client, err := New(ctx, tmpDir, "User", "email")
		if err != nil {
			t.Fatal(err)
		}

		if err := client.Commit(ctx, "", "", "", "msg", []string{"missing.txt"}); err == nil {
			t.Error("Commit(missing file) should fail")
		}
	})

	t.Run("SetRemoteRemoveNonExistent", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		ctx := t.Context()
		client, err := New(ctx, tmpDir, "User", "email")
		if err != nil {
			t.Fatal(err)
		}

		// Remove non-existent remote should be no-op
		if err := client.SetRemote(ctx, "", "origin", ""); err != nil {
			t.Errorf("SetRemote(remove non-existent) failed: %v", err)
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
