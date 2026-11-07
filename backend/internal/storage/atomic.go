// Atomically replaces workspace snapshot files without exposing partial writes to readers.

package storage

import (
	"errors"
	"os"
	"path/filepath"
)

// AtomicFilePrefix identifies incomplete workspace writes that asset listings omit.
const AtomicFilePrefix = ".mddb-write-"

// WriteFile replaces path with a complete workspace file, preserving its permissions.
// New files use 0644. A failed write
// preserves the previous file and removes the same-directory temporary file.
func WriteFile(path string, data []byte) (err error) {
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
		_, err = f.Write(data)
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
