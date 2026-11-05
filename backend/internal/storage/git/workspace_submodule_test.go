// Tests workspace history ownership after root submodule registration.

package git

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/maruel/ksid"
)

func TestWorkspaceSubmodule(t *testing.T) {
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			dir := t.TempDir()
			root, err := NewRootRepo(t.Context(), dir, "Test", "test@example.com")
			if err != nil {
				t.Fatal(err)
			}
			mgr := NewManagerWithBackend(dir, "Test", "test@example.com", b.backend)
			repo, err := mgr.WorkspaceRepo(t.Context(), "ws", true)
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
				return "initial", []string{"AGENTS.md"}, os.WriteFile(filepath.Join(dir, "ws", "AGENTS.md"), []byte("Workspace"), 0o600)
			}); err != nil {
				t.Fatal(err)
			}
			node := ksid.NewID().String()
			if err := os.Mkdir(filepath.Join(dir, "ws", node), 0o700); err != nil {
				t.Fatal(err)
			}
			index := filepath.Join(dir, "ws", ".git", "index")
			if err := os.Rename(index, index+".saved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(index, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := repo.CommitTx(t.Context(), Author{}, func() (string, []string, error) {
				return "saved", []string{node + "/index.md"}, os.WriteFile(filepath.Join(dir, "ws", node, "index.md"), []byte("saved"), 0o600)
			}); err != nil {
				t.Fatal(err)
			}
			if !mgr.HasPendingChanges() {
				t.Fatal("saved mutation was not pending")
			}
			if err := os.Remove(index); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(index+".saved", index); err != nil {
				t.Fatal(err)
			}
			if err := root.AddWorkspaceSubmodule(t.Context(), "ws"); err != nil {
				t.Fatal(err)
			}
			// Concurrent readers see one replacement handle, with pending state intact.
			var readers sync.WaitGroup
			for range 3 {
				readers.Go(func() {
					if _, err := repo.CommitCount(t.Context()); err != nil {
						t.Error(err)
					}
					if _, err := repo.GetHistory(t.Context(), "AGENTS.md", 10); err != nil {
						t.Error(err)
					}
					file, err := repo.FSAtCommit(t.Context(), "HEAD").Open("AGENTS.md")
					if err != nil {
						t.Error(err)
						return
					}
					if err := file.Close(); err != nil {
						t.Error(err)
					}
				})
			}
			readers.Wait()
			if !mgr.HasPendingChanges() {
				t.Fatal("backend replacement lost pending history")
			}
			if err := repo.Push(t.Context(), "missing", ""); err == nil {
				t.Fatal("missing remote unexpectedly succeeded")
			}

			if mgr.HasPendingChanges() {
				t.Fatal("registered workspace cannot record history")
			}
		})
	}
}
