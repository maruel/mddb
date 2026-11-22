// Atomically replaces workspace snapshot files without exposing partial writes to readers.

package storage

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// AtomicFilePrefix identifies incomplete workspace writes that asset listings omit.
const AtomicFilePrefix = ".mddb-write-"

// WriteFile replaces path with a complete workspace file, preserving its permissions.
// New files use 0644. A failed write
// preserves the previous file and removes the same-directory temporary file.
func WriteFile(path string, data []byte) error {
	return writeFile(path, func(f *os.File) error { _, err := f.Write(data); return err })
}

// WriteFileReader atomically replaces path from an exact-size stream with bounded
// memory. A short, oversized, or failed stream preserves the prior publication
// and removes staging. Callers admit size before writing retained content.
func WriteFileReader(path string, data io.Reader, size int64) error {
	if size < 0 {
		return errors.New("negative file size")
	}
	return writeFile(path, func(f *os.File) error {
		// A full count does not prove successful input: the final bytes may
		// accompany a non-EOF source error, which must prevent publication.
		copied, err := io.Copy(f, io.LimitReader(data, size))
		if err != nil {
			return err
		}
		if copied != size {
			return io.ErrUnexpectedEOF
		}
		// Keep the probe's byte count and error independent too. An overlong
		// final byte can carry a source error that callers still need to see.
		extra, err := io.Copy(io.Discard, io.LimitReader(data, 1))
		if extra != 0 {
			return errors.Join(errors.New("stream exceeds declared file size"), err)
		}
		if err != nil {
			return err
		}
		return nil
	})
}

func writeFile(path string, write func(*os.File) error) (err error) {
	mode := os.FileMode(0o644)
	if st, e := os.Stat(path); e == nil {
		mode = st.Mode().Perm()
	} else if !os.IsNotExist(e) {
		return e
	}
	f, err := os.CreateTemp(filepath.Dir(path), AtomicFilePrefix+"*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if e := os.Remove(tmp); e != nil && !os.IsNotExist(e) {
			err = errors.Join(err, e)
		}
	}()
	if err = f.Chmod(mode); err == nil {
		err = write(f)
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
