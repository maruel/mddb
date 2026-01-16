// Package storage implements the file system storage layer and business logic services.
package storage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	pagesDir string // Legacy default pages dir
	nextIDs  map[string]int // Next available numeric ID per organization
	mu       sync.Mutex
}

// NewFileStore initializes a FileStore with the given root directory.
func NewFileStore(rootDir string) (*FileStore, error) {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}

	pagesDir := filepath.Join(rootDir, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", pagesDir, err)
	}

	return &FileStore{
		rootDir:  rootDir,
		pagesDir: pagesDir,
		nextIDs:  make(map[string]int),
	}, nil
}

// NextID returns the next available numeric ID for an organization.
func (fs *FileStore) NextID(orgID string) string {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	id, ok := fs.nextIDs[orgID]
	if !ok {
		id = fs.findNextID(orgID)
	}

	fs.nextIDs[orgID] = id + 1
	return strconv.Itoa(id)
}

func (fs *FileStore) findNextID(orgID string) int {
	dir := fs.orgPagesDir(orgID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 1
	}

	maxID := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if id, err := strconv.Atoi(entry.Name()); err == nil && id > maxID {
			maxID = id
		}
	}

	return maxID + 1
}

func (fs *FileStore) orgPagesDir(orgID string) string {
	if orgID == "" {
		return fs.pagesDir
	}
	dir := filepath.Join(fs.rootDir, orgID, "pages")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// PageExists checks if a page directory exists.
func (fs *FileStore) PageExists(orgID, id string) bool {
	path := fs.pageDir(orgID, id)
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ReadPage reads a page from disk.
func (fs *FileStore) ReadPage(orgID, id string) (*models.Page, error) {
	filePath := fs.pageIndexFile(orgID, id)
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

// WritePage writes a page to disk.
func (fs *FileStore) WritePage(orgID, id, title, content string) (*models.Page, error) {
	now := time.Now()
	page := &models.Page{
		ID:       id,
		Title:    title,
		Content:  content,
		Created:  now,
		Modified: now,
		Path:     "index.md",
	}

	pageDir := fs.pageDir(orgID, id)
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := fs.pageIndexFile(orgID, id)
	data := formatMarkdownFile(page)
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write page: %w", err)
	}

	return page, nil
}

// UpdatePage updates an existing page.
func (fs *FileStore) UpdatePage(orgID, id, title, content string) (*models.Page, error) {
	filePath := fs.pageIndexFile(orgID, id)
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

// DeletePage deletes a page directory.
func (fs *FileStore) DeletePage(orgID, id string) error {
	pageDir := fs.pageDir(orgID, id)
	if err := os.RemoveAll(pageDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("page not found")
		}
		return fmt.Errorf("failed to delete page: %w", err)
	}
	return nil
}

// ListPages returns all pages for an organization.
func (fs *FileStore) ListPages(orgID string) ([]*models.Page, error) {
	var pages []*models.Page
	dir := fs.orgPagesDir(orgID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read pages directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		id := entry.Name()
		if _, err := strconv.Atoi(id); err != nil {
			continue
		}

		indexFile := fs.pageIndexFile(orgID, id)
		if _, err := os.Stat(indexFile); err == nil {
			page, err := fs.ReadPage(orgID, id)
			if err == nil {
				pages = append(pages, page)
			}
		}
	}

	return pages, nil
}

// ReadNode reads a unified node from disk.
func (fs *FileStore) ReadNode(orgID, id string) (*models.Node, error) {
	nodeDir := fs.pageDir(orgID, id)
	info, err := os.Stat(nodeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("node not found")
		}
		return nil, fmt.Errorf("failed to access node: %w", err)
	}

	node := &models.Node{
		ID:       id,
		Created:  info.ModTime(),
		Modified: info.ModTime(),
	}

	indexFile := fs.pageIndexFile(orgID, id)
	if _, err := os.Stat(indexFile); err == nil {
		page, err := fs.ReadPage(orgID, id)
		if err == nil {
			node.Title = page.Title
			node.Content = page.Content
			node.Created = page.Created
			node.Modified = page.Modified
			node.Tags = page.Tags
			node.Type = models.NodeTypeDocument
		}
	}

	schemaFile := fs.databaseSchemaFile(orgID, id)
	if _, err := os.Stat(schemaFile); err == nil {
		db, err := fs.ReadDatabase(orgID, id)
		if err == nil {
			if node.Type == models.NodeTypeDocument {
				node.Type = models.NodeTypeHybrid
			} else {
				node.Type = models.NodeTypeDatabase
				node.Title = db.Title
				node.Created = db.Created
				node.Modified = db.Modified
			}
			node.Columns = db.Columns
		}
	}

	return node, nil
}

// ReadNodeTree returns the full hierarchical tree of nodes.
func (fs *FileStore) ReadNodeTree(orgID string) ([]*models.Node, error) {
	return fs.readNodesRecursive(orgID, fs.orgPagesDir(orgID), "")
}

func (fs *FileStore) readNodesRecursive(orgID, dir, parentID string) ([]*models.Node, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var nodes []*models.Node
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		id := entry.Name()
		if _, err := strconv.Atoi(id); err != nil {
			continue
		}

		node, err := fs.ReadNodeFromPath(orgID, filepath.Join(dir, id), id, parentID)
		if err != nil {
			continue
		}

		children, _ := fs.readNodesRecursive(orgID, filepath.Join(dir, id), id)
		node.Children = children

		nodes = append(nodes, node)
	}
	return nodes, nil
}

// ReadNodeFromPath reads a node from a specific path.
func (fs *FileStore) ReadNodeFromPath(orgID, path, id, parentID string) (*models.Node, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	node := &models.Node{
		ID:       id,
		ParentID: parentID,
		Created:  info.ModTime(),
		Modified: info.ModTime(),
	}

	indexFile := filepath.Join(path, "index.md")
	if _, err := os.Stat(indexFile); err == nil {
		page, err := fs.ReadPage(orgID, id) // Note: This might need adjustment if path is deep
		if err == nil {
			node.Title = page.Title
			node.Content = page.Content
			node.Created = page.Created
			node.Modified = page.Modified
			node.Tags = page.Tags
			node.Type = models.NodeTypeDocument
		}
	}

	schemaFile := filepath.Join(path, "metadata.json")
	if _, err := os.Stat(schemaFile); err == nil {
		db, err := fs.ReadDatabase(orgID, id)
		if err == nil {
			if node.Type == models.NodeTypeDocument {
				node.Type = models.NodeTypeHybrid
			} else {
				node.Type = models.NodeTypeDatabase
				node.Title = db.Title
				node.Created = db.Created
				node.Modified = db.Modified
			}
			node.Columns = db.Columns
		}
	}

	return node, nil
}

func (fs *FileStore) pageDir(orgID, id string) string {
	return filepath.Join(fs.orgPagesDir(orgID), id)
}

func (fs *FileStore) pageIndexFile(orgID, id string) string {
	return filepath.Join(fs.pageDir(orgID, id), "index.md")
}

// Database operations

func (fs *FileStore) DatabaseExists(orgID, id string) bool {
	path := fs.databaseSchemaFile(orgID, id)
	_, err := os.Stat(path)
	return err == nil
}

func (fs *FileStore) ReadDatabase(orgID, id string) (*models.Database, error) {
	filePath := fs.databaseSchemaFile(orgID, id)
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

func (fs *FileStore) WriteDatabase(orgID string, db *models.Database) error {
	pageDir := fs.pageDir(orgID, db.ID)
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal database: %w", err)
	}

	filePath := fs.databaseSchemaFile(orgID, db.ID)
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write database: %w", err)
	}

	return nil
}

func (fs *FileStore) DeleteDatabase(orgID, id string) error {
	pageDir := fs.pageDir(orgID, id)
	if err := os.RemoveAll(pageDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete database: %w", err)
	}
	return nil
}

func (fs *FileStore) ListDatabases(orgID string) ([]*models.Database, error) {
	var databases []*models.Database
	dir := fs.orgPagesDir(orgID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read pages directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		id := entry.Name()
		if _, err := strconv.Atoi(id); err != nil {
			continue
		}

		schemaFile := fs.databaseSchemaFile(orgID, id)
		if _, err := os.Stat(schemaFile); err == nil {
			db, err := fs.ReadDatabase(orgID, id)
			if err == nil {
				databases = append(databases, db)
			}
		}
	}

	return databases, nil
}

func (fs *FileStore) AppendRecord(orgID, id string, record *models.Record) error {
	pageDir := fs.pageDir(orgID, id)
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}

	filePath := fs.databaseRecordsFile(orgID, id)
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open records file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write record: %w", err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

func (fs *FileStore) ReadRecords(orgID, id string) ([]*models.Record, error) {
	filePath := fs.databaseRecordsFile(orgID, id)
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*models.Record{}, nil
		}
		return nil, fmt.Errorf("failed to read records: %w", err)
	}
	defer func() { _ = f.Close() }()

	var records []*models.Record
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record models.Record
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("failed to parse record: %w", err)
		}
		records = append(records, &record)
	}
	return records, nil
}

func (fs *FileStore) ReadRecordsPage(orgID, id string, offset, limit int) ([]*models.Record, error) {
	filePath := fs.databaseRecordsFile(orgID, id)
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*models.Record{}, nil
		}
		return nil, fmt.Errorf("failed to read records: %w", err)
	}
	defer func() { _ = f.Close() }()

	var records []*models.Record
	scanner := bufio.NewScanner(f)
	currentIndex := 0
	count := 0
	for scanner.Scan() {
		if currentIndex < offset {
			currentIndex++
			continue
		}
		if limit > 0 && count >= limit {
			break
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record models.Record
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("failed to parse record: %w", err)
		}
		records = append(records, &record)
		currentIndex++
		count++
	}
	return records, nil
}

func (fs *FileStore) databaseSchemaFile(orgID, id string) string {
	return filepath.Join(fs.pageDir(orgID, id), "metadata.json")
}

func (fs *FileStore) databaseRecordsFile(orgID, id string) string {
	return filepath.Join(fs.pageDir(orgID, id), "data.jsonl")
}

// Asset operations

func (fs *FileStore) SaveAsset(orgID, pageID, assetName string, data []byte) (string, error) {
	pageDir := fs.pageDir(orgID, pageID)
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create page directory: %w", err)
	}

	assetPath := filepath.Join(pageDir, assetName)
	if err := os.WriteFile(assetPath, data, 0o644); err != nil {
		return "", fmt.Errorf("failed to write asset: %w", err)
	}

	return assetName, nil
}

func (fs *FileStore) ReadAsset(orgID, pageID, assetName string) ([]byte, error) {
	assetPath := filepath.Join(fs.pageDir(orgID, pageID), assetName)
	data, err := os.ReadFile(assetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("asset not found")
		}
		return nil, fmt.Errorf("failed to read asset: %w", err)
	}
	return data, nil
}

func (fs *FileStore) DeleteAsset(orgID, pageID, assetName string) error {
	assetPath := filepath.Join(fs.pageDir(orgID, pageID), assetName)
	if err := os.Remove(assetPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("asset not found")
		}
		return fmt.Errorf("failed to delete asset: %w", err)
	}
	return nil
}

func (fs *FileStore) ListAssets(orgID, pageID string) ([]*models.Asset, error) {
	pageDir := fs.pageDir(orgID, pageID)
	entries, err := os.ReadDir(pageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*models.Asset{}, nil
		}
		return nil, fmt.Errorf("failed to read assets: %w", err)
	}

	var assets []*models.Asset
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "index.md" || name == "metadata.json" || name == "data.jsonl" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		assets = append(assets, &models.Asset{
			ID:      name,
			Name:    name,
			Size:    info.Size(),
			Created: info.ModTime(),
			Path:    name,
		})
	}
	return assets, nil
}

// Helpers

func parseMarkdownFile(id string, data []byte) *models.Page {
	content := string(data)
	title := id
	var created, modified time.Time

	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content, "\n---", 2)
		if len(parts) == 2 {
			frontMatter := parts[0][4:]
			content = parts[1]
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

func formatMarkdownFile(page *models.Page) []byte {
	var buf bytes.Buffer
	buf.WriteString("---")
	buf.WriteString("\nid: " + page.ID + "\n")
	buf.WriteString("title: " + page.Title + "\n")
	buf.WriteString("created: " + page.Created.Format(time.RFC3339) + "\n")
	buf.WriteString("modified: " + page.Modified.Format(time.RFC3339) + "\n")
	if len(page.Tags) > 0 {
		buf.WriteString("tags: [" + strings.Join(page.Tags, ", ") + "]\n")
	}
	buf.WriteString("---")
	buf.WriteString("\n\n")
	buf.WriteString(page.Content)
	return buf.Bytes()
}