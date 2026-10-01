// Tests that exec-backed commits wait for automatic Git maintenance.

package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecRepo(t *testing.T) {
	t.Run("CommitTx", func(t *testing.T) {
		t.Run("AutomaticMaintenance", func(t *testing.T) {
			dir := t.TempDir()
			ctx := t.Context()
			if out, err := gitOutput(dir, "init"); err != nil {
				t.Fatalf("init repo: %v\n%s", err, out)
			}

			// Force maintenance and request background execution in an existing repo.
			for _, cfg := range [][2]string{
				{"gc.autoDetach", "true"},
				{"maintenance.autoDetach", "true"},
				{"maintenance.commit-graph.auto", "-1"},
				{"maintenance.commit-graph.enabled", "true"},
				{"maintenance.gc.enabled", "false"},
				{"user.email", "test@example.com"},
				{"user.name", "Test User"},
			} {
				if out, err := gitOutput(dir, "config", cfg[0], cfg[1]); err != nil {
					t.Fatalf("configure %s: %v\n%s", cfg[0], err, out)
				}
			}
			mgr := NewManager(dir, "Test User", "test@example.com")
			repo, err := mgr.Repo(ctx, "")
			if err != nil {
				t.Fatal(err)
			}
			if err := repo.CommitTx(ctx, Author{}, func() (string, []string, error) {
				err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("content"), 0o600)
				return "Initial commit", []string{"test.txt"}, err
			}); err != nil {
				t.Fatal(err)
			}

			// The graph must exist before CommitTx returns, not after a delay.
			if _, err := os.Stat(filepath.Join(dir, ".git", "objects", "info", "commit-graphs", "commit-graph-chain")); err != nil {
				t.Fatalf("automatic maintenance did not finish before CommitTx returned: %v", err)
			}
			for _, key := range []string{"gc.autoDetach", "maintenance.autoDetach"} {
				out, err := gitOutput(dir, "config", "--local", "--bool", "--get", key)
				if err != nil {
					t.Fatalf("read %s: %v\n%s", key, err, out)
				}
				if strings.TrimSpace(string(out)) != "true" {
					t.Errorf("repository setting %s changed: %s", key, out)
				}
			}
		})
	})
}
