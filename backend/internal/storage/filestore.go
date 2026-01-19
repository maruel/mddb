// Package storage implements the file-system persistence layer and business services.
//
// It provides services for nodes, pages, databases, users, organizations, and
// assets. Data is stored as markdown files and JSONL databases with optional
// git versioning. The package handles caching, quota management, and search.
package storage

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/models"
)

// FileStore handles all file system operations using directory-based storage.
// Storage model: Each page (document or database) is a numeric directory (1, 2, 3, etc.)
// - Pages: numeric directory containing index.md with YAML front matter
// - Databases: numeric directory containing data.jsonl (with schema header in first row)
// - Assets: files within each page's directory namespace
type FileStore struct {
	rootDir string
	nextIDs map[string]int // Next available numeric ID per organization
	mu      sync.Mutex
}

// NewFileStore initializes a FileStore with the given root directory.
func NewFileStore(rootDir string) (*FileStore, error) {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}

	return &FileStore{
		rootDir: rootDir,
		nextIDs: make(map[string]int),
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
	return EncodeID(uint64(id))
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
		if id, err := DecodeID(entry.Name()); err == nil && int(id) > maxID {
			maxID = int(id)
		}
	}

	return maxID + 1
}

func (fs *FileStore) orgPagesDir(orgID string) string {
	if orgID == "" {
		return ""
	}
	dir := filepath.Join(fs.rootDir, orgID, "pages")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// PageExists checks if a page directory exists.
func (fs *FileStore) PageExists(orgID, id string) bool {
	if orgID == "" {
		return false
	}
	path := fs.pageDir(orgID, id)
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ReadPage reads a page from disk.
func (fs *FileStore) ReadPage(orgID, id string) (*models.Page, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
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
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
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
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
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
	if orgID == "" {
		return fmt.Errorf("organization ID is required")
	}
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
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
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
		if _, err := DecodeID(id); err != nil {
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
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
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

	recordsFile := fs.databaseRecordsFile(orgID, id)
	if _, err := os.Stat(recordsFile); err == nil {
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
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
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
		if _, err := DecodeID(id); err != nil {
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

// GetOrganizationUsage calculates the total number of pages and storage usage (in bytes) for an organization.
func (fs *FileStore) GetOrganizationUsage(orgID string) (pageCount int, storageUsage int64, err error) {
	if orgID == "" {
		return 0, 0, fmt.Errorf("organization ID is required")
	}
	dir := fs.orgPagesDir(orgID)
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			storageUsage += info.Size()
			if info.Name() == "index.md" || info.Name() == "data.jsonl" {
				// We count unique directories that have index.md or data.jsonl
				// But Walk is recursive. Let's simplify and just count index.md as "pages"
				if info.Name() == "index.md" {
					pageCount++
				}
			}
		}
		return nil
	})
	return
}

func (fs *FileStore) pageDir(orgID, id string) string {
	return filepath.Join(fs.orgPagesDir(orgID), id)
}

func (fs *FileStore) pageIndexFile(orgID, id string) string {
	return filepath.Join(fs.pageDir(orgID, id), "index.md")
}

// Database operations

// DatabaseExists checks if a database exists for the given organization and ID.
func (fs *FileStore) DatabaseExists(orgID, id string) bool {
	path := fs.databaseRecordsFile(orgID, id)
	_, err := os.Stat(path)
	return err == nil
}

// ReadDatabase reads a database definition from the JSONL file using jsonldb abstraction.
func (fs *FileStore) ReadDatabase(orgID, id string) (*models.Database, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	filePath := fs.databaseRecordsFile(orgID, id)

	// Load using jsonldb abstraction
	db, err := jsonldb.NewDatabase(filePath, id, "", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read database: %w", err)
	}

	if db.Header == nil {
		return nil, fmt.Errorf("database file is empty or invalid")
	}

	// Convert from jsonldb to models
	return &models.Database{
		ID:       db.Header.ID,
		Title:    db.Header.Title,
		Columns:  columnsFromJSONLDB(db.Header.Columns),
		Created:  db.Header.Created,
		Modified: db.Header.Modified,
		Version:  db.Header.Version,
	}, nil
}

// WriteDatabase updates a database schema in the JSONL file using jsonldb abstraction.
func (fs *FileStore) WriteDatabase(orgID string, db *models.Database) error {
	if orgID == "" {
		return fmt.Errorf("organization ID is required")
	}

	pageDir := fs.pageDir(orgID, db.ID)
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := fs.databaseRecordsFile(orgID, db.ID)

	// Convert columns to jsonldb format
	jsonldbCols := columnsToJSONLDB(db.Columns)

	// Load existing database using jsonldb
	jsonDb, err := jsonldb.NewDatabase(filePath, db.ID, db.Title, jsonldbCols)
	if err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	// Update schema
	if err := jsonDb.UpdateSchema(db.Title, jsonldbCols); err != nil {
		return fmt.Errorf("failed to update schema: %w", err)
	}

	return nil
}

// DeleteDatabase deletes a database and all its records.
func (fs *FileStore) DeleteDatabase(orgID, id string) error {
	if orgID == "" {
		return fmt.Errorf("organization ID is required")
	}
	pageDir := fs.pageDir(orgID, id)
	if err := os.RemoveAll(pageDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete database: %w", err)
	}
	return nil
}

// ListDatabases returns all databases for the given organization.
func (fs *FileStore) ListDatabases(orgID string) ([]*models.Database, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
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
		if _, err := DecodeID(id); err != nil {
			continue
		}

		// Check if this is a database (has data.jsonl file)
		recordsFile := fs.databaseRecordsFile(orgID, id)
		if _, err := os.Stat(recordsFile); err == nil {
			db, err := fs.ReadDatabase(orgID, id)
			if err == nil {
				databases = append(databases, db)
			}
		}
	}

	return databases, nil
}

// AppendRecord appends a record to a database using jsonldb abstraction.
func (fs *FileStore) AppendRecord(orgID, id string, record *models.DataRecord) error {
	if orgID == "" {
		return fmt.Errorf("organization ID is required")
	}
	pageDir := fs.pageDir(orgID, id)
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := fs.databaseRecordsFile(orgID, id)

	// Load or create database using jsonldb
	db, err := jsonldb.NewDatabase(filePath, id, "", nil)
	if err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	// Append record (convert to jsonldb format)
	if err := db.AppendRecord(recordToJSONLDB(record)); err != nil {
		return fmt.Errorf("failed to append record: %w", err)
	}

	return nil
}

// ReadRecords reads all records for a database using jsonldb abstraction.
func (fs *FileStore) ReadRecords(orgID, id string) ([]*models.DataRecord, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
	filePath := fs.databaseRecordsFile(orgID, id)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return []*models.DataRecord{}, nil
	}

	// Load using jsonldb abstraction
	db, err := jsonldb.NewDatabase(filePath, id, "", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read records: %w", err)
	}

	records := db.GetRecords()
	// Convert jsonldb records to models records
	result := make([]*models.DataRecord, len(records))
	for i := range records {
		result[i] = recordFromJSONLDB(records[i])
	}
	return result, nil
}

// ReadRecordsPage reads a page of records for a database using jsonldb abstraction.
func (fs *FileStore) ReadRecordsPage(orgID, id string, offset, limit int) ([]*models.DataRecord, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
	filePath := fs.databaseRecordsFile(orgID, id)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return []*models.DataRecord{}, nil
	}

	// Load using jsonldb abstraction
	db, err := jsonldb.NewDatabase(filePath, id, "", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read records: %w", err)
	}

	records := db.GetRecordsPage(offset, limit)
	// Convert jsonldb records to models records
	result := make([]*models.DataRecord, len(records))
	for i := range records {
		result[i] = recordFromJSONLDB(records[i])
	}
	return result, nil
}

// UpdateRecord updates an existing record in a database using jsonldb abstraction.
func (fs *FileStore) UpdateRecord(orgID, databaseID string, record *models.DataRecord) error {
	if orgID == "" {
		return fmt.Errorf("organization ID is required")
	}

	filePath := fs.databaseRecordsFile(orgID, databaseID)

	// Load using jsonldb abstraction
	db, err := jsonldb.NewDatabase(filePath, databaseID, "", nil)
	if err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	// Update record (convert to jsonldb format)
	if err := db.UpdateRecord(recordToJSONLDB(record)); err != nil {
		return fmt.Errorf("failed to update record: %w", err)
	}

	return nil
}

// DeleteRecord deletes a record from a database using jsonldb abstraction.
func (fs *FileStore) DeleteRecord(orgID, databaseID, recordID string) error {
	if orgID == "" {
		return fmt.Errorf("organization ID is required")
	}

	filePath := fs.databaseRecordsFile(orgID, databaseID)

	// Load using jsonldb abstraction
	db, err := jsonldb.NewDatabase(filePath, databaseID, "", nil)
	if err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	// Delete record
	if err := db.DeleteRecord(recordID); err != nil {
		return fmt.Errorf("failed to delete record: %w", err)
	}

	return nil
}

func (fs *FileStore) databaseRecordsFile(orgID, id string) string {
	return filepath.Join(fs.pageDir(orgID, id), "data.jsonl")
}

// Asset operations

// SaveAsset saves an asset associated with a page.
func (fs *FileStore) SaveAsset(orgID, pageID, assetName string, data []byte) (string, error) {
	if orgID == "" {
		return "", fmt.Errorf("organization ID is required")
	}
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

// ReadAsset reads an asset associated with a page.
func (fs *FileStore) ReadAsset(orgID, pageID, assetName string) ([]byte, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
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

// DeleteAsset deletes an asset associated with a page.
func (fs *FileStore) DeleteAsset(orgID, pageID, assetName string) error {
	if orgID == "" {
		return fmt.Errorf("organization ID is required")
	}
	assetPath := filepath.Join(fs.pageDir(orgID, pageID), assetName)
	if err := os.Remove(assetPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("asset not found")
		}
		return fmt.Errorf("failed to delete asset: %w", err)
	}
	return nil
}

// ListAssets lists all assets associated with a page.
func (fs *FileStore) ListAssets(orgID, pageID string) ([]*models.Asset, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
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
		if name == "index.md" || name == "data.jsonl" {
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

// Converters between jsonldb and models types

// columnsToJSONLDB converts models.Column to jsonldb.Column.
func columnsToJSONLDB(cols []models.Column) []jsonldb.Column {
	result := make([]jsonldb.Column, len(cols))
	for i, col := range cols {
		result[i] = jsonldb.Column{
			ID:       col.ID,
			Name:     col.Name,
			Type:     col.Type,
			Options:  col.Options,
			Required: col.Required,
		}
	}
	return result
}

// columnsFromJSONLDB converts jsonldb.Column to models.Column.
func columnsFromJSONLDB(cols []jsonldb.Column) []models.Column {
	result := make([]models.Column, len(cols))
	for i, col := range cols {
		result[i] = models.Column{
			ID:       col.ID,
			Name:     col.Name,
			Type:     col.Type,
			Options:  col.Options,
			Required: col.Required,
		}
	}
	return result
}

// recordToJSONLDB converts models.DataRecord to jsonldb.DataRecord.
func recordToJSONLDB(rec *models.DataRecord) jsonldb.DataRecord {
	return jsonldb.DataRecord{
		ID:       rec.ID,
		Data:     rec.Data,
		Created:  rec.Created,
		Modified: rec.Modified,
	}
}

// recordFromJSONLDB converts jsonldb.DataRecord to models.DataRecord.
func recordFromJSONLDB(rec jsonldb.DataRecord) *models.DataRecord {
	return &models.DataRecord{
		ID:       rec.ID,
		Data:     rec.Data,
		Created:  rec.Created,
		Modified: rec.Modified,
	}
}
