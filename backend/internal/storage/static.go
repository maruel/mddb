// Embeds static files for the root data directory and workspace directories.

package storage

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// rootStaticFS contains static files written to the root data directory
// (.gitignore, etc.).
//
//go:embed all:static/root
var rootStaticFS embed.FS

// workspaceStaticFS contains static files written to each new workspace
// directory (AGENTS.md, etc.).
//
//go:embed static/workspace
var workspaceStaticFS embed.FS

// WriteRootStaticFiles writes embedded static files to the root data directory.
// It returns the list of relative paths written.
func WriteRootStaticFiles(targetDir string) ([]string, error) {
	return writeStaticFiles(targetDir, rootStaticFS, "static/root")
}

// WriteWorkspaceStaticFiles writes embedded static files to a workspace directory.
// It returns the list of relative paths written.
func WriteWorkspaceStaticFiles(targetDir string) ([]string, error) {
	return writeStaticFiles(targetDir, workspaceStaticFS, "static/workspace")
}

func writeStaticFiles(targetDir string, fsys embed.FS, root string) ([]string, error) {
	sub, err := fs.Sub(fsys, root)
	if err != nil {
		return nil, fmt.Errorf("sub fs %s: %w", root, err)
	}
	var written []string
	err = fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(sub, path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}
		target := filepath.Join(targetDir, path)
		existing, err := os.ReadFile(target) //nolint:gosec // G304: target constructed from validated targetDir
		if err == nil && bytes.Equal(existing, data) {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil { //nolint:gosec // G301: data directories
			return fmt.Errorf("mkdir for %s: %w", path, err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil { //nolint:gosec // G306: data files
			return fmt.Errorf("write %s: %w", path, err)
		}
		written = append(written, path)
		return nil
	})
	return written, err
}
