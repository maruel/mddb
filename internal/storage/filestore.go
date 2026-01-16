package storage

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/maruel/mddb/internal/models"
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

// PageExists checks if a page file exists.
func (fs *FileStore) PageExists(id string) bool {
	path := fs.pageFilePath(id)
	_, err := os.Stat(path)
	return err == nil
}

// ReadPage reads a page from disk, parsing metadata and content.
func (fs *FileStore) ReadPage(id string) (*models.Page, error) {
	filePath := fs.pageFilePath(id)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("page not found")
		}
		return nil, fmt.Errorf("failed to read page: %w", err)
	}

	page, err := parseMarkdownFile(id, data, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse page: %w", err)
	}
	return page, nil
}

// WritePage writes a page to disk with metadata and content.
func (fs *FileStore) WritePage(id string, title, content string) (*models.Page, error) {
	now := time.Now()
	page := &models.Page{
		ID:       id,
		Title:    title,
		Content:  content,
		Created:  now,
		Modified: now,
		Path:     id + ".md",
	}

	filePath := fs.pageFilePath(id)

	// Create parent directory if needed
	if dir := filepath.Dir(filePath); dir != fs.pagesDir {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Format with YAML front matter
	data := formatMarkdownFile(page)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write page: %w", err)
	}

	return page, nil
}

// UpdatePage updates an existing page's content and metadata.
func (fs *FileStore) UpdatePage(id, title, content string) (*models.Page, error) {
	filePath := fs.pageFilePath(id)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("page not found")
		}
		return nil, fmt.Errorf("failed to read page: %w", err)
	}

	page, err := parseMarkdownFile(id, data, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse page: %w", err)
	}

	page.Title = title
	page.Content = content
	page.Modified = time.Now()

	updatedData := formatMarkdownFile(page)
	if err := os.WriteFile(filePath, updatedData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write page: %w", err)
	}

	return page, nil
}

// DeletePage deletes a page file.
func (fs *FileStore) DeletePage(id string) error {
	filePath := fs.pageFilePath(id)
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("page not found")
		}
		return fmt.Errorf("failed to delete page: %w", err)
	}
	return nil
}

// ListPages returns all pages in the pages directory.
func (fs *FileStore) ListPages() ([]*models.Page, error) {
	var pages []*models.Page

	err := filepath.Walk(fs.pagesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-.md files
		if info.IsDir() || !strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".db.md") {
			return nil
		}

		// Get relative path from pagesDir
		relPath, _ := filepath.Rel(fs.pagesDir, path)
		id := strings.TrimSuffix(relPath, ".md")
		id = filepath.ToSlash(id) // Normalize path separators

		page, err := fs.ReadPage(id)
		if err != nil {
			// Log but continue
			return nil
		}
		pages = append(pages, page)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list pages: %w", err)
	}

	return pages, nil
}

// pageFilePath constructs the full file path for a page ID.
// ID can include path separators for nested pages.
func (fs *FileStore) pageFilePath(id string) string {
	return filepath.Join(fs.pagesDir, id+".md")
}

// parseMarkdownFile parses a markdown file with YAML front matter.
func parseMarkdownFile(id string, data []byte, filePath string) (*models.Page, error) {
	content := string(data)
	title := id

	// Split front matter from content
	if strings.HasPrefix(content, "---\n") {
		parts := strings.SplitN(content, "\n---\n", 2)
		if len(parts) == 2 {
			frontMatter := parts[0][4:] // Remove "---\n"
			content = parts[1]

			// Parse title from front matter
			for _, line := range strings.Split(frontMatter, "\n") {
				if strings.HasPrefix(line, "title:") {
					title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
					break
				}
			}
		}
	}

	stat, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(filepath.Dir(filePath), filePath)

	return &models.Page{
		ID:       id,
		Title:    title,
		Content:  content,
		Created:  stat.ModTime(),
		Modified: stat.ModTime(),
		Path:     relPath,
	}, nil
}

// formatMarkdownFile formats a page into markdown with YAML front matter.
func formatMarkdownFile(page *models.Page) []byte {
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString("id: " + page.ID + "\n")
	buf.WriteString("title: " + page.Title + "\n")
	buf.WriteString("created: " + page.Created.Format(time.RFC3339) + "\n")
	buf.WriteString("modified: " + page.Modified.Format(time.RFC3339) + "\n")
	if len(page.Tags) > 0 {
		buf.WriteString("tags: [" + strings.Join(page.Tags, ", ") + "]\n")
	}
	buf.WriteString("---\n\n")
	buf.WriteString(page.Content)
	return buf.Bytes()
}
