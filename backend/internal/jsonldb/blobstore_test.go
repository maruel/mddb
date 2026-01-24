package jsonldb

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlobWriter(t *testing.T) {
	t.Run("WriteAndClose", func(t *testing.T) {
		store := &blobStore{dir: filepath.Join(t.TempDir(), "blobs")}

		t.Run("write data", func(t *testing.T) {
			w, err := store.newBlob()
			if err != nil {
				t.Fatalf("NewWriter() error = %v", err)
			}

			data := []byte("hello, world!")
			n, err := w.Write(data)
			if err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			if n != len(data) {
				t.Errorf("Write() n = %d, want %d", n, len(data))
			}

			blob, err := w.Close()
			if err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if blob.IsZero() {
				t.Error("Close() returned unset blob")
			}
			// SHA-256 of "hello, world!" with size 13 (base32 hex encoded).
			wantHash := "sha256:D3J5DCIHSPV86M5UV143LC6L3HJ1JSV7K6KV1PQO73A1VSR8USK0-13"
			if string(blob.Ref) != wantHash {
				t.Errorf("Close() hash = %q, want %q", blob.Ref, wantHash)
			}

			// Verify file exists at correct location
			path := store.pathForRef(blob.Ref)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("blob file not found at %s: %v", path, err)
			}
		})

		t.Run("empty write", func(t *testing.T) {
			w, err := store.newBlob()
			if err != nil {
				t.Fatal(err)
			}
			blob, err := w.Close()
			if err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			// Empty blob returns the hardcoded empty hash with size 0.
			if blob.Ref != "sha256:SEOC8GKOVGE196NRUJ49IRTP4GJQSGF4CIDP6J54IMCHMU2IN1AG-0" {
				t.Errorf("Close() with no data should return empty hash, got %q", blob.Ref)
			}

			// Reading empty blob should return empty content
			r, err := blob.Reader()
			if err != nil {
				t.Fatalf("Reader() error = %v", err)
			}
			content, err := io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			if len(content) != 0 {
				t.Errorf("expected empty content, got %d bytes", len(content))
			}
		})

		t.Run("streaming write", func(t *testing.T) {
			w, err := store.newBlob()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte("part1")); err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte("part2")); err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte("part3")); err != nil {
				t.Fatal(err)
			}
			blob, err := w.Close()
			if err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			// Read back and verify
			r, err := blob.Reader()
			if err != nil {
				t.Fatalf("Reader() error = %v", err)
			}
			content, err := io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			if string(content) != "part1part2part3" {
				t.Errorf("read content = %q, want %q", content, "part1part2part3")
			}
		})
	})

	t.Run("Abort", func(t *testing.T) {
		store := &blobStore{dir: filepath.Join(t.TempDir(), "blobs")}

		w, err := store.newBlob()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("some data")); err != nil {
			t.Fatal(err)
		}
		tmpPath := w.tmpPath

		if err := w.Abort(); err != nil {
			t.Fatalf("Abort() error = %v", err)
		}

		// Temp file should be removed
		if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
			t.Error("temp file not removed after Abort()")
		}
	})

	t.Run("DoubleClose", func(t *testing.T) {
		store := &blobStore{dir: filepath.Join(t.TempDir(), "blobs")}

		w, err := store.newBlob()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("data")); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Close(); err != nil {
			t.Fatal(err)
		}

		_, err = w.Close()
		if !errors.Is(err, fs.ErrClosed) {
			t.Errorf("second Close() error = %v, want fs.ErrClosed", err)
		}

		_, err = w.Write([]byte("more"))
		if !errors.Is(err, fs.ErrClosed) {
			t.Errorf("Write after Close() error = %v, want fs.ErrClosed", err)
		}
	})
}

func TestBlobStore(t *testing.T) {
	t.Run("Open", func(t *testing.T) {
		store := &blobStore{dir: filepath.Join(t.TempDir(), "blobs")}

		// Create a blob
		w, err := store.newBlob()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("test content")); err != nil {
			t.Fatal(err)
		}
		blob, err := w.Close()
		if err != nil {
			t.Fatal(err)
		}

		t.Run("existing", func(t *testing.T) {
			r, err := store.open(blob.Ref)
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			content, err := io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			if string(content) != "test content" {
				t.Errorf("content = %q, want %q", content, "test content")
			}
		})

		t.Run("non-existent", func(t *testing.T) {
			_, err := store.open(BlobRef("sha256:" + strings.Repeat("A", 52) + "-100"))
			if !errors.Is(err, os.ErrNotExist) {
				t.Errorf("Open() error = %v, want os.ErrNotExist", err)
			}
		})

		t.Run("invalid hash", func(t *testing.T) {
			_, err := store.open("invalid")
			if err == nil {
				t.Error("Open() invalid hash should error")
			}
		})
	})

	t.Run("Delete", func(t *testing.T) {
		store := &blobStore{dir: filepath.Join(t.TempDir(), "blobs")}

		w, err := store.newBlob()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("to delete")); err != nil {
			t.Fatal(err)
		}
		blob, err := w.Close()
		if err != nil {
			t.Fatal(err)
		}

		if err := store.remove(blob.Ref); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify blob file was deleted
		if _, err := os.Stat(store.pathForRef(blob.Ref)); !os.IsNotExist(err) {
			t.Error("blob still exists after Delete()")
		}

		// Delete non-existent should not error.
		if err := store.remove(BlobRef("sha256:" + strings.Repeat("C", 52) + "-100")); err != nil {
			t.Errorf("Delete() non-existent error = %v", err)
		}

		// Delete invalid hash should error
		if err := store.remove("invalid"); err == nil {
			t.Error("Delete() invalid hash should error")
		}
	})

	t.Run("GC", func(t *testing.T) {
		store := &blobStore{dir: filepath.Join(t.TempDir(), "blobs")}

		// Create blobs
		refs := make([]BlobRef, 0, 3)
		for i := range 3 {
			w, err := store.newBlob()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte{byte(i)}); err != nil {
				t.Fatal(err)
			}
			blob, err := w.Close()
			if err != nil {
				t.Fatal(err)
			}
			refs = append(refs, blob.Ref)
		}

		// Keep only first blob
		if err := store.gc(map[BlobRef]int{refs[0]: 1}); err != nil {
			t.Fatalf("GC() error = %v", err)
		}

		// First blob should exist
		if _, err := os.Stat(store.pathForRef(refs[0])); err != nil {
			t.Error("kept blob was deleted")
		}

		// Others should be gone
		for _, ref := range refs[1:] {
			if _, err := os.Stat(store.pathForRef(ref)); !os.IsNotExist(err) {
				t.Errorf("orphan blob %s still exists", ref)
			}
		}
	})

	t.Run("GCCleansTmpDir", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "blobs")
		store := &blobStore{dir: dir}

		// Create a temp file manually (simulating failed write)
		tmpDir := filepath.Join(dir, "tmp")
		if err := os.MkdirAll(tmpDir, 0o750); err != nil {
			t.Fatal(err)
		}
		tmpFile := filepath.Join(tmpDir, "orphan.tmp")
		if err := os.WriteFile(tmpFile, []byte("orphan"), 0o600); err != nil {
			t.Fatal(err)
		}

		// gc should clean up tmp files
		if err := store.gc(map[BlobRef]int{}); err != nil {
			t.Fatalf("gc() error = %v", err)
		}

		if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
			t.Error("orphan temp file not removed")
		}
	})

	t.Run("EmptyBlobOptimization", func(t *testing.T) {
		store := &blobStore{dir: filepath.Join(t.TempDir(), "blobs")}

		// Write empty content
		w, err := store.newBlob()
		if err != nil {
			t.Fatal(err)
		}
		blob, err := w.Close()
		if err != nil {
			t.Fatal(err)
		}

		// Should have the hardcoded empty hash with size 0.
		const emptyHash = "sha256:SEOC8GKOVGE196NRUJ49IRTP4GJQSGF4CIDP6J54IMCHMU2IN1AG-0"
		if blob.Ref != emptyHash {
			t.Errorf("empty blob hash = %q, want %q", blob.Ref, emptyHash)
		}

		// No file should be created for empty blob
		path := store.pathForRef(blob.Ref)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("empty blob should not create a file")
		}

		// Reading empty blob should return empty content (virtual existence)
		r, err := store.open(blob.Ref)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		content, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		if len(content) != 0 {
			t.Errorf("empty blob content length = %d, want 0", len(content))
		}

		// Delete should be a no-op (no error)
		if err := store.remove(blob.Ref); err != nil {
			t.Errorf("Delete() error = %v", err)
		}
	})

	t.Run("DeduplicatesSameContent", func(t *testing.T) {
		store := &blobStore{dir: filepath.Join(t.TempDir(), "blobs")}

		content := []byte("duplicate content")

		// Write same content twice
		w1, err := store.newBlob()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w1.Write(content); err != nil {
			t.Fatal(err)
		}
		blob1, err := w1.Close()
		if err != nil {
			t.Fatal(err)
		}

		w2, err := store.newBlob()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w2.Write(content); err != nil {
			t.Fatal(err)
		}
		blob2, err := w2.Close()
		if err != nil {
			t.Fatal(err)
		}

		if blob1.Ref != blob2.Ref {
			t.Error("same content produced different hashes")
		}

		// Should still be only one file
		path := store.pathForRef(blob1.Ref)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("blob file not found: %v", err)
		}
		if info.Size() != int64(len(content)) {
			t.Errorf("blob size = %d, want %d", info.Size(), len(content))
		}
	})
}
