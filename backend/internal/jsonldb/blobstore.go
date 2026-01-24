package jsonldb

import (
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// base32Enc uses base32 "Extended Hex" alphabet (0-9A-V) which is ASCII-sorted
// and case-insensitive safe for filesystems.
var base32Enc = base32.HexEncoding.WithPadding(base32.NoPadding)

// BlobWriter streams data to a blob, computing the SHA-256 hash as data is written.
//
// Create via [Table.NewBlob]. Write data using [BlobWriter.Write], then call
// [BlobWriter.Close] to finalize and get the [Blob] reference. If an error occurs
// during writing, call [BlobWriter.Abort] to clean up the temporary file.
type BlobWriter struct {
	store   *blobStore
	tmpPath string
	file    io.WriteCloser // nil after Close or Abort
	hasher  hash.Hash
	size    int64
}

// Write implements io.Writer, writing to temp file and updating the hash.
func (w *BlobWriter) Write(p []byte) (n int, err error) {
	if w.file == nil {
		return 0, fs.ErrClosed
	}
	n, err = w.file.Write(p)
	if n > 0 {
		w.size += int64(n)
		w.hasher.Write(p[:n])
	}
	return n, err
}

// Close finalizes the blob: closes the temp file, computes the final ref,
// and renames to the content-addressed location.
//
// Returns the finalized Blob with ref and store reference set.
// If no data was written, returns an empty Blob with the empty content ref.
func (w *BlobWriter) Close() (Blob, error) {
	if w.file == nil {
		return Blob{}, fs.ErrClosed
	}
	if err := w.file.Close(); err != nil {
		w.file = nil
		return Blob{}, errors.Join(fmt.Errorf("failed to close temp file: %w", err), os.Remove(w.tmpPath))
	}
	w.file = nil

	// Empty blob optimization: return hardcoded ref, no file created.
	if w.size == 0 {
		if err := os.Remove(w.tmpPath); err != nil {
			return Blob{}, fmt.Errorf("failed to remove temp file: %w", err)
		}
		return Blob{Ref: emptyBlobRef, store: w.store}, nil
	}

	// Compute final ref: "sha256:<base32>-<size>" (always lowercase).
	ref := BlobRef(fmt.Sprintf("%s%s-%d", blobRefPrefix, base32Enc.EncodeToString(w.hasher.Sum(nil)), w.size))

	// Create target directory (fan-out by first 2 base32 chars of hash).
	if err := os.MkdirAll(filepath.Join(w.store.dir, string(ref)[7:9]), 0o750); err != nil {
		return Blob{}, errors.Join(fmt.Errorf("failed to create blob subdirectory: %w", err), os.Remove(w.tmpPath))
	}

	// If blob already exists (same content), just remove temp.
	targetPath := w.store.pathForRef(ref)
	if _, err := os.Stat(targetPath); err == nil {
		if err := os.Remove(w.tmpPath); err != nil {
			return Blob{}, fmt.Errorf("failed to remove temp file: %w", err)
		}
		return Blob{Ref: ref, store: w.store}, nil
	}
	if err := os.Rename(w.tmpPath, targetPath); err != nil {
		return Blob{}, errors.Join(fmt.Errorf("failed to rename blob to final location: %w", err), os.Remove(w.tmpPath))
	}
	return Blob{Ref: ref, store: w.store}, nil
}

// Abort cancels the write and cleans up the temp file.
func (w *BlobWriter) Abort() error {
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return errors.Join(err, os.Remove(w.tmpPath))
}

//

const (
	blobDirSuffix = ".blobs"
	tmpDirName    = "tmp"

	// emptyBlobRef is the ref for empty content (SHA-256 of nothing with size 0).
	// Used as an optimization to avoid file I/O for empty blobs.
	emptyBlobRef = BlobRef("sha256:SEOC8GKOVGE196NRUJ49IRTP4GJQSGF4CIDP6J54IMCHMU2IN1AG-0")
)

// blobStore manages content-addressed files in a directory.
//
// Files are organized with 256-way fan-out: <dir>/<ref[:2]>/<ref[2:]>.
// Temporary files during write are stored in <dir>/tmp/<random>.tmp.
type blobStore struct {
	dir string
}

// newBlob creates a BlobWriter for streaming blob creation.
//
// Data is written to a temp file; Close() finalizes the hash and renames
// to the content-addressed location.
func (bs *blobStore) newBlob() (*BlobWriter, error) {
	if err := os.MkdirAll(filepath.Join(bs.dir, tmpDirName), 0o750); err != nil {
		return nil, fmt.Errorf("failed to create tmp directory: %w", err)
	}
	f, err := os.CreateTemp(filepath.Join(bs.dir, tmpDirName), "*.tmp")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	return &BlobWriter{
		store:   bs,
		file:    f,
		tmpPath: f.Name(),
		hasher:  sha256.New(),
	}, nil
}

// open returns a ReadCloser for the blob with the given ref.
func (bs *blobStore) open(ref BlobRef) (io.ReadCloser, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	// Optimization: empty blob has no file, return empty reader.
	if ref == emptyBlobRef {
		return io.NopCloser(strings.NewReader("")), nil
	}
	f, err := os.Open(bs.pathForRef(ref))
	if err != nil {
		return nil, fmt.Errorf("failed to open blob: %w", err)
	}
	return f, nil
}

// remove removes a blob by ref.
//
// Returns nil if the blob doesn't exist.
func (bs *blobStore) remove(ref BlobRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	// Optimization: empty blob has no file, nothing to delete.
	if ref == emptyBlobRef {
		return nil
	}
	if err := os.Remove(bs.pathForRef(ref)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete blob: %w", err)
	}
	return nil
}

// gc removes blobs not in usedRefs and cleans up unknown entries.
//
// This is a stop-the-world GC: caller should ensure no writes are in progress.
// Returns all errors encountered joined together.
func (bs *blobStore) gc(usedRefs map[BlobRef]int) error {
	entries, err := os.ReadDir(bs.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read blob directory: %w", err)
	}

	var errs []error
	for _, entry := range entries {
		name := entry.Name()

		// Clean up tmp directory contents.
		if name == tmpDirName {
			if err := bs.cleanupTmpDir(filepath.Join(bs.dir, name)); err != nil {
				errs = append(errs, err)
			}
			continue
		}

		// Delete unknown subdirectories or files at root level.
		if !entry.IsDir() || !isValidBase32Prefix(name) {
			if err := os.RemoveAll(filepath.Join(bs.dir, name)); err != nil {
				errs = append(errs, fmt.Errorf("failed to remove unknown entry %s: %w", name, err))
			}
			continue
		}

		files, err := os.ReadDir(filepath.Join(bs.dir, name))
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to read subdir %s: %w", name, err))
			continue
		}
		for _, file := range files {
			filePath := filepath.Join(bs.dir, name, file.Name())

			// Remove subdirectories inside ref dirs.
			if file.IsDir() {
				if err := os.RemoveAll(filePath); err != nil {
					errs = append(errs, fmt.Errorf("failed to remove subdir in %s: %w", name, err))
				}
				continue
			}

			// Reconstruct full ref from directory name + filename.
			ref := BlobRef(blobRefPrefix + name + file.Name())
			if ref.Validate() != nil {
				if err := os.Remove(filePath); err != nil {
					errs = append(errs, fmt.Errorf("failed to remove unknown file %s: %w", file.Name(), err))
				}
				continue
			}

			// Remove orphaned blobs.
			if usedRefs[ref] == 0 {
				if err := os.Remove(filePath); err != nil {
					errs = append(errs, fmt.Errorf("failed to remove orphan blob %s: %w", ref, err))
				}
			}
		}
	}
	return errors.Join(errs...)
}

// cleanupTmpDir removes all .tmp files from the given directory.
func (bs *blobStore) cleanupTmpDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read tmp directory: %w", err)
	}
	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tmp") {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				errs = append(errs, fmt.Errorf("failed to remove temp file %s: %w", entry.Name(), err))
			}
		}
	}
	return errors.Join(errs...)
}

// pathForRef returns the file path for a blob ref.
// Extracts hash portion after "sha256:" prefix for fan-out directory structure.
func (bs *blobStore) pathForRef(ref BlobRef) string {
	hashPart := string(ref)[7:] // Skip "sha256:" prefix
	return filepath.Join(bs.dir, hashPart[:2], hashPart[2:])
}

// isValidBase32Prefix checks if a string is a valid 2-character base32 hex prefix.
func isValidBase32Prefix(s string) bool {
	return len(s) == 2 && isBase32HexChar(s[0]) && isBase32HexChar(s[1])
}

// isBase32HexChar checks if a byte is a valid base32 hex character (0-9, A-V uppercase only).
func isBase32HexChar(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'A' && c <= 'V')
}
