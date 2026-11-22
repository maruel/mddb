// Tests complete atomic workspace file replacement and permission/error preservation.

package storage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestWriteFileReader(t *testing.T) {
	t.Run("valid exact stream retains an opened snapshot", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "asset")
		if err := WriteFile(path, []byte("before")); err != nil {
			t.Fatal(err)
		}
		snapshot, err := os.Open(path) //nolint:gosec // G304: Test-owned temporary path.
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := snapshot.Close(); err != nil {
				t.Error(err)
			}
		})
		if err := WriteFileReader(path, strings.NewReader("after"), 5); err != nil {
			t.Fatal(err)
		}
		old, err := io.ReadAll(snapshot)
		if err != nil || string(old) != "before" {
			t.Fatalf("opened publication changed: %q (%v)", old, err)
		}
		current, err := os.ReadFile(path) //nolint:gosec // G304: Test-owned temporary path.
		if err != nil || string(current) != "after" {
			t.Fatalf("new publication: %q (%v)", current, err)
		}
	})
	t.Run("final bytes retain their source error", func(t *testing.T) {
		for _, sample := range []struct {
			name, data string
			size       int64
		}{{"exact", "new", 3}, {"short", "ne", 3}, {"overlong probe", "new!", 3}, {"empty", "", 0}} {
			t.Run(sample.name, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "asset")
				if err := WriteFile(path, []byte("before")); err != nil {
					t.Fatal(err)
				}
				failure := errors.New("source failed with final bytes")
				source := &atomicFinalErrorReader{data: []byte(sample.data), err: failure}
				if err := WriteFileReader(path, source, sample.size); !errors.Is(err, failure) {
					t.Fatalf("source error lost: %v", err)
				}
				current, err := os.ReadFile(path) //nolint:gosec // G304: Test-owned temporary path.
				if err != nil || string(current) != "before" {
					t.Fatalf("failed stream published: %q (%v)", current, err)
				}
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) != 1 || entries[0].Name() != "asset" {
					t.Fatalf("failed stream retained staging: %v (%v)", entries, err)
				}
			})
		}
	})
	t.Run("EOF accompanying final bytes completes a valid stream", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "asset")
		source := &atomicFinalErrorReader{data: []byte("new"), err: io.EOF}
		if err := WriteFileReader(path, source, 3); err != nil {
			t.Fatal(err)
		}
		current, err := os.ReadFile(path) //nolint:gosec // G304: Test-owned temporary path.
		if err != nil || string(current) != "new" {
			t.Fatalf("valid final EOF stream: %q (%v)", current, err)
		}
	})

	t.Run("invalid stream preserves publication and removes staging", func(t *testing.T) {
		for _, kind := range []string{"short", "long", "failed", "negative"} {
			t.Run(kind, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "asset")
				if err := WriteFile(path, []byte("before")); err != nil {
					t.Fatal(err)
				}
				var source io.Reader = strings.NewReader("after")
				size := int64(5)
				switch kind {
				case "short":
					size = 6
				case "long":
					size = 4
				case "failed":
					source = io.MultiReader(strings.NewReader("a"), atomicErrorReader{})
				case "negative":
					size = -1
				}
				if err := WriteFileReader(path, source, size); err == nil {
					t.Fatal("invalid stream succeeded")
				}
				current, err := os.ReadFile(path) //nolint:gosec // G304: Test-owned temporary path.
				if err != nil || string(current) != "before" {
					t.Fatalf("publication changed: %q (%v)", current, err)
				}
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) != 1 || entries[0].Name() != "asset" {
					t.Fatalf("staging not removed: %v (%v)", entries, err)
				}
			})
		}
	})
}

type atomicErrorReader struct{}

func (atomicErrorReader) Read([]byte) (int, error) { return 0, errors.New("source failed") }

// atomicFinalErrorReader returns its final payload and error together, then EOF.
type atomicFinalErrorReader struct {
	data []byte
	err  error
	done bool
}

func (r *atomicFinalErrorReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		r.done = true
		return n, r.err
	}
	return n, nil
}
