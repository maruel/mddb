package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/maruel/mddb/internal/models"
)

// FileStore handles all file system operations using directory-based storage.
// Storage model: Each page (document or database) is a numeric directory (1, 2, 3, etc.)
// - Pages: numeric directory containing index.md with YAML front matter
// - Databases: numeric directory containing metadata.json + data.jsonl
// - Assets: files within each page's directory namespace
type FileStore struct {
	rootDir  string
	pagesDir string
	nextID   int // Next available numeric ID (cached)
}

// NewFileStore initializes a FileStore with the given root directory.
// Creates pages/ subdirectory where all content is stored in numeric directories.
func NewFileStore(rootDir string) (*FileStore, error) {
	// Create root directory if it doesn't exist
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}

	pagesDir := filepath.Join(rootDir, "pages")

	// Create pages directory
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", pagesDir, err)
	}

	fs := &FileStore{
		rootDir:  rootDir,
		pagesDir: pagesDir,
		nextID:   1,
	}

	// Calculate next ID by finding highest existing numeric directory
	fs.nextID = fs.findNextID()

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

// NextID returns the next available numeric ID and increments the counter.
func (fs *FileStore) NextID() string {
	id := fs.nextID
	fs.nextID++
	return strconv.Itoa(id)
}

// findNextID scans the pages directory to find the highest numeric ID
// and returns the next available ID.
func (fs *FileStore) findNextID() int {
	entries, err := os.ReadDir(fs.pagesDir)
	if err != nil {
		return 1
	}

	maxID := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Try to parse as numeric ID
		if id, err := strconv.Atoi(entry.Name()); err == nil && id > maxID {
			maxID = id
		}
	}

	return maxID + 1
}

// PageExists checks if a page directory exists.
func (fs *FileStore) PageExists(id string) bool {
	path := fs.pageDir(id)
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ReadPage reads a page from disk, parsing metadata and content.
// Page is stored as: {id}/index.md with YAML front matter
func (fs *FileStore) ReadPage(id string) (*models.Page, error) {
	filePath := fs.pageIndexFile(id)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("page not found")
		}
		return nil, fmt.Errorf("failed to read page: %w", err)
	}

	page := parseMarkdownFile(id, data)
	return page, nil
}

// WritePage writes a page to disk with metadata and content.
// Page is stored as: {id}/index.md with YAML front matter
func (fs *FileStore) WritePage(id, title, content string) (*models.Page, error) {
	now := time.Now()
	page := &models.Page{
		ID:       id,
		Title:    title,
		Content:  content,
		Created:  now,
		Modified: now,
		Path:     "index.md",
	}

	pageDir := fs.pageDir(id)

	// Create page directory
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Write index.md with YAML front matter
	filePath := fs.pageIndexFile(id)
	data := formatMarkdownFile(page)
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write page: %w", err)
	}

	return page, nil
}

// UpdatePage updates an existing page's content and metadata.
func (fs *FileStore) UpdatePage(id, title, content string) (*models.Page, error) {
	filePath := fs.pageIndexFile(id)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("page not found")
		}
		return nil, fmt.Errorf("failed to read page: %w", err)
	}

	page := parseMarkdownFile(id, data)

	page.Title = title
	page.Content = content
	page.Modified = time.Now()

	updatedData := formatMarkdownFile(page)
	if err := os.WriteFile(filePath, updatedData, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write page: %w", err)
	}

	return page, nil
}

// DeletePage deletes a page directory and all its contents.
func (fs *FileStore) DeletePage(id string) error {
	pageDir := fs.pageDir(id)
	if err := os.RemoveAll(pageDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("page not found")
		}
		return fmt.Errorf("failed to delete page: %w", err)
	}
	return nil
}

// ListPages returns all document pages in the pages directory.
// Only considers numeric directories containing index.md (not databases).
func (fs *FileStore) ListPages() ([]*models.Page, error) {
	var pages []*models.Page

	entries, err := os.ReadDir(fs.pagesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read pages directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Only consider numeric IDs (skip non-numeric directories)
		id := entry.Name()
		if _, err := strconv.Atoi(id); err != nil {
			continue
		}

		// Skip if it's a database (has metadata.json but not index.md)
		indexFile := fs.pageIndexFile(id)
		_, err := os.Stat(indexFile)
		if os.IsNotExist(err) {
			continue // Not a page, skip
		}
		if err != nil {
			continue // Error checking, skip
		}

		page, err := fs.ReadPage(id)
		if err != nil {
			continue // Log but continue
		}
		pages = append(pages, page)
	}

	return pages, nil
}

// pageDir returns the directory path for a page ID.
func (fs *FileStore) pageDir(id string) string {
	return filepath.Join(fs.pagesDir, id)
}

// pageIndexFile returns the index.md file path for a page ID.
func (fs *FileStore) pageIndexFile(id string) string {
	return filepath.Join(fs.pageDir(id), "index.md")
}

// parseMarkdownFile parses a markdown file with YAML front matter.
func parseMarkdownFile(id string, data []byte) *models.Page {
	content := string(data)
	title := id
	var created, modified time.Time

	// Split front matter from content
	if strings.HasPrefix(content, "---\n") {
		parts := strings.SplitN(content, "\n---\n", 2)
		if len(parts) == 2 {
			frontMatter := parts[0][4:] // Remove "---\n"
			content = parts[1]

			// Parse front matter
			for _, line := range strings.Split(frontMatter, "\n") {
				switch {
				case strings.HasPrefix(line, "title:"):
					title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
				case strings.HasPrefix(line, "created:"):
					dateStr := strings.TrimSpace(strings.TrimPrefix(line, "created:"))
					if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
						created = t
					}
				case strings.HasPrefix(line, "modified:"):
					dateStr := strings.TrimSpace(strings.TrimPrefix(line, "modified:"))
					if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
						modified = t
					}
				}
			}
		}
	}

	// Use current time if not found in front matter
	if created.IsZero() {
		created = time.Now()
	}
	if modified.IsZero() {
		modified = time.Now()
	}

	return &models.Page{
		ID:       id,
		Title:    title,
		Content:  content,
		Created:  created,
		Modified: modified,
		Path:     "index.md",
	}
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

// Database operations
// Databases are stored as: {id}/metadata.json (schema) + {id}/data.jsonl (records)

// DatabaseExists checks if a database directory and schema file exist.
func (fs *FileStore) DatabaseExists(id string) bool {
	path := fs.databaseSchemaFile(id)
	_, err := os.Stat(path)
	return err == nil
}

// ReadDatabase reads a database schema from disk.
func (fs *FileStore) ReadDatabase(id string) (*models.Database, error) {
	filePath := fs.databaseSchemaFile(id)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("database not found")
		}
		return nil, fmt.Errorf("failed to read database: %w", err)
	}

	var db models.Database
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("failed to parse database: %w", err)
	}

	return &db, nil
}

// WriteDatabase writes a database schema to disk.
// Creates {id}/metadata.json in the page directory.
func (fs *FileStore) WriteDatabase(db *models.Database) error {
	pageDir := fs.pageDir(db.ID)

	// Create page directory
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal database: %w", err)
	}

	filePath := fs.databaseSchemaFile(db.ID)
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write database: %w", err)
	}

	return nil
}

// DeleteDatabase deletes a database directory and all its contents.
func (fs *FileStore) DeleteDatabase(id string) error {
	pageDir := fs.pageDir(id)
	if err := os.RemoveAll(pageDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete database: %w", err)
	}
	return nil
}

// ListDatabases returns all databases in the pages directory.
// Only considers numeric directories containing metadata.json (not pages).
func (fs *FileStore) ListDatabases() ([]*models.Database, error) {
	var databases []*models.Database

	entries, err := os.ReadDir(fs.pagesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read pages directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Only consider numeric IDs (skip non-numeric directories)
		id := entry.Name()
		if _, err := strconv.Atoi(id); err != nil {
			continue
		}

		// Check if it's a database (has metadata.json)
		schemaFile := fs.databaseSchemaFile(id)
		_, err := os.Stat(schemaFile)
		if os.IsNotExist(err) {
			continue // Not a database, skip
		}
		if err != nil {
			continue // Error checking, skip
		}

		db, err := fs.ReadDatabase(id)
		if err != nil {
			continue // Log but continue
		}
		databases = append(databases, db)
	}

	return databases, nil
}

// AppendRecord appends a record to a database's JSONL records file.
// Stored as: {id}/data.jsonl
func (fs *FileStore) AppendRecord(id string, record *models.Record) error {
	pageDir := fs.pageDir(id)

	// Create page directory if needed
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}

	// Open file in append mode, create if doesn't exist
	filePath := fs.databaseRecordsFile(id)
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open records file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Write JSON and newline
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write record: %w", err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

// ReadRecords reads all records from a database's JSONL file.
func (fs *FileStore) ReadRecords(id string) ([]*models.Record, error) {
	filePath := fs.databaseRecordsFile(id)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*models.Record{}, nil // Empty database
		}
		return nil, fmt.Errorf("failed to read records: %w", err)
	}

	var records []*models.Record
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record models.Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("failed to parse record: %w", err)
		}
		records = append(records, &record)
	}

	return records, nil
}

// databaseSchemaFile returns the path for a database schema file.
// Stored as: {id}/metadata.json
func (fs *FileStore) databaseSchemaFile(id string) string {
	return filepath.Join(fs.pageDir(id), "metadata.json")
}

// databaseRecordsFile returns the path for a database records file.
// Stored as: {id}/data.jsonl
func (fs *FileStore) databaseRecordsFile(id string) string {
	return filepath.Join(fs.pageDir(id), "data.jsonl")
}

// Asset operations
// Assets are files stored within a page's directory namespace.
// Examples: {id}/image.png, {id}/favicon.ico, {id}/document.pdf

// SaveAsset saves an asset file to a page's directory.
// Returns the relative path from the page directory (e.g., "image.png").
func (fs *FileStore) SaveAsset(pageID, assetName string, data []byte) (string, error) {
	pageDir := fs.pageDir(pageID)

	// Create page directory if needed
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create page directory: %w", err)
	}

	assetPath := filepath.Join(pageDir, assetName)
	if err := os.WriteFile(assetPath, data, 0o644); err != nil {
		return "", fmt.Errorf("failed to write asset: %w", err)
	}

	return assetName, nil
}

// ReadAsset reads an asset file from a page's directory.
func (fs *FileStore) ReadAsset(pageID, assetName string) ([]byte, error) {
	assetPath := filepath.Join(fs.pageDir(pageID), assetName)
	data, err := os.ReadFile(assetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("asset not found")
		}
		return nil, fmt.Errorf("failed to read asset: %w", err)
	}
	return data, nil
}

// DeleteAsset deletes an asset file from a page's directory.
func (fs *FileStore) DeleteAsset(pageID, assetName string) error {
	assetPath := filepath.Join(fs.pageDir(pageID), assetName)
	if err := os.Remove(assetPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("asset not found")
		}
		return fmt.Errorf("failed to delete asset: %w", err)
	}
	return nil
}

// ListAssets lists all asset files in a page's directory, excluding index.md, metadata.json, and data.jsonl.
func (fs *FileStore) ListAssets(pageID string) ([]*models.Asset, error) {
	pageDir := fs.pageDir(pageID)
	entries, err := os.ReadDir(pageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*models.Asset{}, nil // Page doesn't exist yet, return empty list
		}
		return nil, fmt.Errorf("failed to read assets: %w", err)
	}

	var assets []*models.Asset
	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip directories
		}

		name := entry.Name()
		// Skip index files
		if name == "index.md" || name == "metadata.json" || name == "data.jsonl" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue // Skip if unable to get info
		}

		assets = append(assets, &models.Asset{
			ID:      name, // Use filename as ID for now
			Name:    name,
			Size:    info.Size(),
			Created: info.ModTime(),
			Path:    name,
		})
	}

	return assets, nil
}
