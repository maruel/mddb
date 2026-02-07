package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootRepo(t *testing.T) {
	t.Parallel()

	t.Run("Init", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		ctx := t.Context()

		// Create db dir and a file to ensure initial commit has content
		dbDir := filepath.Join(dir, "db")
		if err := os.MkdirAll(dbDir, 0o755); err != nil { //nolint:gosec // G301: test data
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dbDir, "users.jsonl"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "server_config.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}

		rr, err := NewRootRepo(ctx, dir, "Test", "test@example.com")
		if err != nil {
			t.Fatalf("NewRootRepo() failed: %v", err)
		}
		_ = rr

		// Verify .git exists
		if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
			t.Error(".git directory not created")
		}

		// Verify .gitignore
		data, err := os.ReadFile(filepath.Join(dir, ".gitignore")) //nolint:gosec // G304: test path
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), ".env") {
			t.Error(".gitignore should contain .env")
		}

		// Verify initial commit
		out, err := gitOutput(dir, "log", "--oneline")
		if err != nil {
			t.Fatalf("git log failed: %v", err)
		}
		if !strings.Contains(string(out), "initial commit") {
			t.Errorf("expected initial commit, got: %s", string(out))
		}

		// Verify files are tracked
		out, err = gitOutput(dir, "ls-files")
		if err != nil {
			t.Fatal(err)
		}
		tracked := string(out)
		if !strings.Contains(tracked, ".gitignore") {
			t.Error(".gitignore should be tracked")
		}
		if !strings.Contains(tracked, "db/users.jsonl") {
			t.Error("db/users.jsonl should be tracked")
		}
		if !strings.Contains(tracked, "server_config.json") {
			t.Error("server_config.json should be tracked")
		}
	})

	t.Run("InitIdempotent", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		ctx := t.Context()

		rr1, err := NewRootRepo(ctx, dir, "Test", "test@example.com")
		if err != nil {
			t.Fatal(err)
		}

		// Second init should not fail or create duplicate commits
		rr2, err := NewRootRepo(ctx, dir, "Test", "test@example.com")
		if err != nil {
			t.Fatalf("second NewRootRepo() failed: %v", err)
		}
		_, _ = rr1, rr2

		out, err := gitOutput(dir, "log", "--oneline")
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) != 1 {
			t.Errorf("expected 1 commit, got %d: %s", len(lines), string(out))
		}
	})

	t.Run("CommitDBChanges", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		ctx := t.Context()

		dbDir := filepath.Join(dir, "db")
		if err := os.MkdirAll(dbDir, 0o755); err != nil { //nolint:gosec // G301: test data
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dbDir, "users.jsonl"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "server_config.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}

		rr, err := NewRootRepo(ctx, dir, "Test", "test@example.com")
		if err != nil {
			t.Fatal(err)
		}

		// Modify a db file
		if err := os.WriteFile(filepath.Join(dbDir, "users.jsonl"), []byte("{\"id\":1}\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		author := Author{Name: "User", Email: "user@example.com"}
		if err := rr.CommitDBChanges(ctx, author, "POST /api/v1/auth/register"); err != nil {
			t.Fatalf("CommitDBChanges() failed: %v", err)
		}

		// Verify commit
		out, err := gitOutput(dir, "log", "-1", "--format=%s")
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(out)) != "POST /api/v1/auth/register" {
			t.Errorf("unexpected commit message: %s", string(out))
		}

		// Verify author
		out, err = gitOutput(dir, "log", "-1", "--format=%an <%ae>")
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(out)) != "User <user@example.com>" {
			t.Errorf("unexpected author: %s", string(out))
		}
	})

	t.Run("CommitDBChangesNoop", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		ctx := t.Context()

		if err := os.MkdirAll(filepath.Join(dir, "db"), 0o755); err != nil { //nolint:gosec // G301: test data
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "server_config.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}

		rr, err := NewRootRepo(ctx, dir, "Test", "test@example.com")
		if err != nil {
			t.Fatal(err)
		}

		// Commit without changes — should be a no-op
		if err := rr.CommitDBChanges(ctx, Author{}, "noop"); err != nil {
			t.Fatalf("CommitDBChanges(noop) failed: %v", err)
		}

		out, err := gitOutput(dir, "log", "--oneline")
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) != 1 {
			t.Errorf("expected only initial commit, got %d", len(lines))
		}
	})

	t.Run("AddAndRemoveSubmodule", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		ctx := t.Context()

		if err := os.MkdirAll(filepath.Join(dir, "db"), 0o755); err != nil { //nolint:gosec // G301: test data
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "server_config.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}

		rr, err := NewRootRepo(ctx, dir, "Test", "test@example.com")
		if err != nil {
			t.Fatal(err)
		}

		// Create a workspace git repo
		wsID := "ws-abc123"
		wsDir := filepath.Join(dir, wsID)
		if err := os.MkdirAll(wsDir, 0o755); err != nil { //nolint:gosec // G301: test data
			t.Fatal(err)
		}
		initGitRepo(t, wsDir)

		// Add as submodule
		if err := rr.AddWorkspaceSubmodule(ctx, wsID); err != nil {
			t.Fatalf("AddWorkspaceSubmodule() failed: %v", err)
		}

		// Verify .gitmodules lists the submodule
		data, err := os.ReadFile(filepath.Join(dir, ".gitmodules")) //nolint:gosec // G304: test path
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), wsID) {
			t.Errorf(".gitmodules should contain %s, got: %s", wsID, string(data))
		}

		// Verify submodule status
		out, err := gitOutput(dir, "submodule", "status")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), wsID) {
			t.Errorf("submodule status should mention %s, got: %s", wsID, string(out))
		}

		// Remove submodule
		if err := rr.RemoveWorkspaceSubmodule(ctx, wsID); err != nil {
			t.Fatalf("RemoveWorkspaceSubmodule() failed: %v", err)
		}

		// Verify .gitmodules is empty or gone
		if data, err := os.ReadFile(filepath.Join(dir, ".gitmodules")); err == nil { //nolint:gosec // G304: test path
			if strings.Contains(string(data), wsID) {
				t.Errorf(".gitmodules should not contain %s after removal", wsID)
			}
		}
	})

	t.Run("MigrateExistingWorkspaces", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		ctx := t.Context()

		if err := os.MkdirAll(filepath.Join(dir, "db"), 0o755); err != nil { //nolint:gosec // G301: test data
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "server_config.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}

		// Create a workspace git repo BEFORE initializing root repo
		wsID := "ws-migrate"
		wsDir := filepath.Join(dir, wsID)
		if err := os.MkdirAll(wsDir, 0o755); err != nil { //nolint:gosec // G301: test data
			t.Fatal(err)
		}
		initGitRepo(t, wsDir)

		// Initialize root repo — should auto-detect and add ws-migrate as submodule
		rr, err := NewRootRepo(ctx, dir, "Test", "test@example.com")
		if err != nil {
			t.Fatalf("NewRootRepo() failed: %v", err)
		}
		_ = rr

		// Verify submodule was added
		data, err := os.ReadFile(filepath.Join(dir, ".gitmodules")) //nolint:gosec // G304: test path
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), wsID) {
			t.Errorf(".gitmodules should contain %s after migration", wsID)
		}
	})

	t.Run("CommitDBChangesDefaultAuthor", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		ctx := t.Context()

		if err := os.MkdirAll(filepath.Join(dir, "db"), 0o755); err != nil { //nolint:gosec // G301: test data
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "db", "sessions.jsonl"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "server_config.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}

		rr, err := NewRootRepo(ctx, dir, "", "")
		if err != nil {
			t.Fatal(err)
		}

		// Modify file
		if err := os.WriteFile(filepath.Join(dir, "db", "sessions.jsonl"), []byte("{\"id\":1}\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		// Commit with empty author — should use defaults
		if err := rr.CommitDBChanges(ctx, Author{}, "POST /api/v1/auth/login"); err != nil {
			t.Fatalf("CommitDBChanges() failed: %v", err)
		}

		out, err := gitOutput(dir, "log", "-1", "--format=%an <%ae>")
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(out)) != "mddb <mddb@localhost>" {
			t.Errorf("expected default author, got: %s", string(out))
		}
	})
}

// initGitRepo initializes a git repo in dir with a single commit.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		// #nosec G204
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# ws"), 0o600); err != nil {
		t.Fatal(err)
	}
	// #nosec G204
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	// #nosec G204
	cmd = exec.Command("git", "commit", "-m", "init ws")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

func gitOutput(dir string, args ...string) ([]byte, error) {
	// #nosec G204
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}
