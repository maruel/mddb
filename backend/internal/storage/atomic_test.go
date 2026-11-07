// Tests complete atomic workspace file replacement and permission/error preservation.

package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFile(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "index.md")
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path) //nolint:gosec // Test-owned temporary file.
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := f.Close(); err != nil {
				t.Error(err)
			}
		})
		if err := WriteFile(path, []byte("new")); err != nil {
			t.Fatal(err)
		}
		b := make([]byte, 3)
		if _, err := f.Read(b); err != nil {
			t.Fatal(err)
		}
		if string(b) != "old" {
			t.Fatalf("opened snapshot changed: %q", b)
		}
		b, err = os.ReadFile(path) //nolint:gosec // Test-owned temporary file.
		if err != nil || string(b) != "new" {
			t.Fatalf("replacement: %q, %v", b, err)
		}
		st, err := os.Stat(path)
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("permissions: %v, %v", st, err)
		}
	})
	t.Run("error", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "target")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := WriteFile(dir, []byte("fail")); err == nil {
			t.Fatal("replaced directory")
		}
		entries, err := os.ReadDir(filepath.Dir(dir))
		if err != nil || len(entries) != 1 {
			t.Fatalf("temporary file leaked: %v, %v", entries, err)
		}
	})
}
