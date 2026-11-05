// Tests recovery preserves conflicts and literal, scoped staged changes.

package git

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maruel/ksid"
)

func TestWorkspaceIndex(t *testing.T) {
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			t.Run("UnresolvedMerge", func(t *testing.T) {
				mgr, repo, dir, node := workspaceIndexFixture(t, b.backend, []string{"index.md"})
				branch := strings.TrimSpace(string(workspaceGit(t, dir, "branch", "--show-current")))
				workspaceGit(t, dir, "checkout", "-b", "other")
				path := filepath.Join(dir, node, "index.md")
				if err := os.WriteFile(path, []byte("other branch\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				workspaceGit(t, dir, "commit", "-am", "other branch")
				workspaceGit(t, dir, "checkout", branch)
				if err := os.WriteFile(path, []byte("current branch\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				workspaceGit(t, dir, "commit", "-am", "current branch")
				if out, err := gitOutput(dir, "merge", "other"); err == nil {
					t.Fatalf("fixture did not conflict: %s", out)
				}
				head := workspaceGit(t, dir, "rev-parse", "HEAD")
				index, err := os.ReadFile(filepath.Join(dir, ".git", "index")) //nolint:gosec // G304: file in the test-owned repository.
				if err != nil {
					t.Fatal(err)
				}
				content, err := os.ReadFile(path) //nolint:gosec // G304: file in the test-owned repository.
				if err != nil {
					t.Fatal(err)
				}
				if err := repo.Push(t.Context(), "missing", ""); err == nil {
					t.Fatal("conflicted recovery succeeded")
				}
				if !mgr.HasPendingChanges() {
					t.Error("unresolved merge was reported healthy")
				}
				if got := workspaceGit(t, dir, "rev-parse", "HEAD"); !bytes.Equal(got, head) {
					t.Error("recovery committed an unresolved merge")
				}
				got, err := os.ReadFile(filepath.Join(dir, ".git", "index")) //nolint:gosec // G304: file in the test-owned repository.
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, index) {
					t.Error("recovery rewrote conflict index entries")
				}
				got, err = os.ReadFile(path) //nolint:gosec // G304: file in the test-owned repository.
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, content) {
					t.Error("recovery changed conflict content")
				}
				if len(workspaceGit(t, dir, "ls-files", "--unmerged")) == 0 {
					t.Error("recovery resolved the merge")
				}
			})
			t.Run("LiteralDeletedMetacharacter", func(t *testing.T) {
				mgr, repo, dir, node := workspaceIndexFixture(t, b.backend, []string{"*"})
				if err := os.Remove(filepath.Join(dir, node, "*")); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, node, ".jsonldb-secret"), []byte("unpublished"), 0o600); err != nil {
					t.Fatal(err)
				}
				temp := filepath.Join(dir, node, "data.blobs", "tmp")
				if err := os.MkdirAll(temp, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(temp, "upload"), []byte("unpublished"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := repo.Push(t.Context(), "missing", ""); err == nil {
					t.Fatal("missing remote succeeded")
				}
				if mgr.HasPendingChanges() {
					t.Error("literal deletion did not recover")
				}
				if got := workspaceGit(t, dir, "ls-tree", "-r", "--name-only", "HEAD"); len(got) != 0 {
					t.Errorf("excluded files entered history: %q", got)
				}
				count, err := repo.CommitCount(t.Context())
				if err != nil || count != 2 {
					t.Errorf("deletion history count=%d err=%v", count, err)
				}
			})
			t.Run("WhollyStagedDeletion", func(t *testing.T) {
				mgr, repo, dir, node := workspaceIndexFixture(t, b.backend, []string{"index.md"})
				workspaceGit(t, dir, "rm", "--", node+"/index.md")
				if err := repo.Push(t.Context(), "missing", ""); err == nil {
					t.Fatal("missing remote succeeded")
				}
				if mgr.HasPendingChanges() {
					t.Error("staged deletion did not recover")
				}
				if got := workspaceGit(t, dir, "ls-tree", "-r", "--name-only", "HEAD"); len(got) != 0 {
					t.Errorf("deletion was not recorded: %q", got)
				}
				if got := workspaceGit(t, dir, "diff", "--cached", "--name-only"); len(got) != 0 {
					t.Errorf("pending deletion hidden: %q", got)
				}
				count, err := repo.CommitCount(t.Context())
				if err != nil || count != 2 {
					t.Errorf("deletion history count=%d err=%v", count, err)
				}
			})
			t.Run("UnsafeStagedRename", func(t *testing.T) {
				mgr, repo, dir, node := workspaceIndexFixture(t, b.backend, []string{"index.md"})
				scratch := filepath.Join(dir, "scratch")
				if err := os.WriteFile(scratch, []byte("tracked scratch"), 0o600); err != nil {
					t.Fatal(err)
				}
				workspaceGit(t, dir, "add", "--", "scratch")
				workspaceGit(t, dir, "commit", "-m", "existing scratch")
				workspaceGit(t, dir, "mv", "--", "scratch", node+"/asset")
				head := workspaceGit(t, dir, "rev-parse", "HEAD")
				index, err := os.ReadFile(filepath.Join(dir, ".git", "index")) //nolint:gosec // G304: file in the test-owned repository.
				if err != nil {
					t.Fatal(err)
				}
				if err := repo.Push(t.Context(), "missing", ""); err == nil {
					t.Fatal("unsafe recovery succeeded")
				}
				if !mgr.HasPendingChanges() {
					t.Error("unsafe staged rename reported healthy")
				}
				if got := workspaceGit(t, dir, "rev-parse", "HEAD"); !bytes.Equal(got, head) {
					t.Error("unsafe staged rename was committed")
				}
				got, err := os.ReadFile(filepath.Join(dir, ".git", "index")) //nolint:gosec // G304: file in the test-owned repository.
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, index) {
					t.Error("unsafe index was changed")
				}
			})
		})
	}
}

func workspaceIndexFixture(t *testing.T, backend Backend, files []string) (mgr *Manager, repo Repository, dir, node string) {
	dir = t.TempDir()
	mgr = NewManagerWithBackend(dir, "Test", "test@example.com", backend)
	_, err := mgr.Repo(t.Context(), "ws")
	if err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(dir, "ws")
	node = ksid.NewID().String()
	if err := os.Mkdir(filepath.Join(dir, node), 0o700); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		path := node + "/" + file
		if err := os.WriteFile(filepath.Join(dir, path), []byte("initial\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	// Git's CLI creates literal fixture paths even when their names contain '*'.
	args := append([]string{"--literal-pathspecs", "add", "--"}, paths...)
	workspaceGit(t, dir, args...)
	workspaceGit(t, dir, "commit", "-m", "initial")
	repo, err = mgr.WorkspaceRepo(t.Context(), "ws", false)
	if err != nil {
		t.Fatal(err)
	}
	if mgr.HasPendingChanges() {
		t.Fatal("clean fixture has pending history")
	}
	return mgr, repo, dir, node
}

func workspaceGit(t *testing.T, dir string, args ...string) []byte {
	out, err := gitOutput(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out
}
