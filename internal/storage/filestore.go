package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileStore handles all file system operations.
// All content is stored in the pages directory:
// - Pages: files ending with .md
// - Databases: files ending with .db.md
// - Assets: any file not ending with .md
type FileStore struct {
	rootDir  string
	pagesDir string
}

// NewFileStore initializes a FileStore with the given root directory.
// Creates pages/ subdirectory where all content is stored.
func NewFileStore(rootDir string) (*FileStore, error) {
	// Create root directory if it doesn't exist
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}

	fs := &FileStore{
		rootDir:  rootDir,
		pagesDir: filepath.Join(rootDir, "pages"),
	}

	// Create pages directory
	if err := os.MkdirAll(fs.pagesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", fs.pagesDir, err)
	}

	return fs, nil
}

// PagesDir returns the pages directory path where all content is stored.
func (fs *FileStore) PagesDir() string {
	return fs.pagesDir
}

// RootDir returns the root directory path.
func (fs *FileStore) RootDir() string {
	return fs.rootDir
}
