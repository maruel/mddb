// Package storage implements the file-system persistence layer and business services.
//
// It provides services for nodes, pages, databases, users, organizations, and
// assets. Data is stored as markdown files and JSONL databases with optional
// git versioning. The package handles caching, quota management, and search.
package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/models"
)

// FileStore handles all file system operations using directory-based storage.
// Storage model: Each page (document or database) is an ID-based directory.
// - Pages: ID directory containing index.md with YAML front matter
// - Databases: ID directory containing data.jsonl (with schema header in first row)
// - Assets: files within each page's directory namespace
type FileStore struct {
	rootDir string
}

// NewFileStore initializes a FileStore with the given root directory.
func NewFileStore(rootDir string) (*FileStore, error) {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}

	return &FileStore{
		rootDir: rootDir,
	}, nil
}

func (fs *FileStore) orgPagesDir(orgID jsonldb.ID) string {
	if orgID == 0 {
		return ""
	}
	dir := filepath.Join(fs.rootDir, orgID.String(), "pages")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// PageExists checks if a page directory exists.
func (fs *FileStore) PageExists(orgID, id jsonldb.ID) bool {
	if orgID == 0 {
		return false
	}
	path := fs.pageDir(orgID, id)
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ReadPage reads a page from disk.
func (fs *FileStore) ReadPage(orgID, id jsonldb.ID) (*models.Page, error) {
	if orgID == 0 {
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
func (fs *FileStore) WritePage(orgID, id jsonldb.ID, title, content string) (*models.Page, error) {
	if orgID == 0 {
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
func (fs *FileStore) UpdatePage(orgID, id jsonldb.ID, title, content string) (*models.Page, error) {
	if orgID == 0 {
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
func (fs *FileStore) DeletePage(orgID, id jsonldb.ID) error {
	if orgID == 0 {
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
func (fs *FileStore) ListPages(orgID jsonldb.ID) ([]*models.Page, error) {
	if orgID == 0 {
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

		id, err := jsonldb.DecodeID(entry.Name())
		if err != nil {
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
func (fs *FileStore) ReadNode(orgID, id jsonldb.ID) (*models.Node, error) {
	if orgID == 0 {
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
func (fs *FileStore) ReadNodeTree(orgID jsonldb.ID) ([]*models.Node, error) {
	if orgID == 0 {
		return nil, fmt.Errorf("organization ID is required")
	}
	return fs.readNodesRecursive(orgID, fs.orgPagesDir(orgID), 0)
}

func (fs *FileStore) readNodesRecursive(orgID jsonldb.ID, dir string, parentID jsonldb.ID) ([]*models.Node, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var nodes []*models.Node
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		id, err := jsonldb.DecodeID(entry.Name())
		if err != nil {
			continue
		}

		node, err := fs.ReadNodeFromPath(orgID, filepath.Join(dir, entry.Name()), id, parentID)
		if err != nil {
			continue
		}

		children, _ := fs.readNodesRecursive(orgID, filepath.Join(dir, entry.Name()), id)
		node.Children = children

		nodes = append(nodes, node)
	}
	return nodes, nil
}

// ReadNodeFromPath reads a node from a specific path.
func (fs *FileStore) ReadNodeFromPath(orgID jsonldb.ID, path string, id, parentID jsonldb.ID) (*models.Node, error) {
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
func (fs *FileStore) GetOrganizationUsage(orgID jsonldb.ID) (pageCount int, storageUsage int64, err error) {
	if orgID == 0 {
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

func (fs *FileStore) pageDir(orgID, id jsonldb.ID) string {
	return filepath.Join(fs.orgPagesDir(orgID), id.String())
}

func (fs *FileStore) pageIndexFile(orgID, id jsonldb.ID) string {
	return filepath.Join(fs.pageDir(orgID, id), "index.md")
}

// Database operations

// DatabaseExists checks if a database exists for the given organization and ID.
func (fs *FileStore) DatabaseExists(orgID, id jsonldb.ID) bool {
	path := fs.databaseRecordsFile(orgID, id)
	_, err := os.Stat(path)
	return err == nil
}

// ReadDatabase reads a database definition from the JSONL file using jsonldb abstraction.
func (fs *FileStore) ReadDatabase(orgID, id jsonldb.ID) (*models.Database, error) {
	if orgID == 0 {
		return nil, fmt.Errorf("organization ID is required")
	}

	filePath := fs.databaseRecordsFile(orgID, id)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("database not found")
	}

	// Load using jsonldb abstraction
	table, err := jsonldb.NewTable[*models.DataRecord](filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read database: %w", err)
	}

	schema := table.Schema()
	if schema.Version == "" {
		return nil, fmt.Errorf("database file is empty or invalid")
	}

	// Read metadata.json for title and other fields
	pageDir := fs.pageDir(orgID, id)
	metadataFile := filepath.Join(pageDir, "metadata.json")
	var title string
	if data, err := os.ReadFile(metadataFile); err == nil {
		var metadata map[string]any
		if err := json.Unmarshal(data, &metadata); err == nil {
			if t, ok := metadata["title"].(string); ok {
				title = t
			}
		}
	}

	// Convert from jsonldb to models
	return &models.Database{
		ID:      id,
		Title:   title,
		Columns: columnsFromJSONLDB(schema.Columns),
		Version: schema.Version,
	}, nil
}

// WriteDatabase updates a database schema in the JSONL file using jsonldb abstraction.
func (fs *FileStore) WriteDatabase(orgID jsonldb.ID, db *models.Database) error {
	if orgID == 0 {
		return fmt.Errorf("organization ID is required")
	}

	pageDir := fs.pageDir(orgID, db.ID)
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := fs.databaseRecordsFile(orgID, db.ID)

	// Convert columns to jsonldb format
	jsonldbCols := columnsToJSONLDB(db.Columns)

	// Load existing database using jsonldb Table
	table, err := jsonldb.NewTable[*models.DataRecord](filePath)
	if err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	// Update schema
	if err := table.UpdateSchema(jsonldbCols); err != nil {
		return fmt.Errorf("failed to update schema: %w", err)
	}

	// Write metadata.json with title and other db metadata
	metadataFile := filepath.Join(pageDir, "metadata.json")
	metadata := map[string]any{
		"title":    db.Title,
		"version":  db.Version,
		"created":  db.Created,
		"modified": db.Modified,
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(metadataFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

// DeleteDatabase deletes a database and all its records.
func (fs *FileStore) DeleteDatabase(orgID, id jsonldb.ID) error {
	if orgID == 0 {
		return fmt.Errorf("organization ID is required")
	}
	pageDir := fs.pageDir(orgID, id)
	if err := os.RemoveAll(pageDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete database: %w", err)
	}
	return nil
}

// ListDatabases returns all databases for the given organization.
func (fs *FileStore) ListDatabases(orgID jsonldb.ID) ([]*models.Database, error) {
	if orgID == 0 {
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

		id, err := jsonldb.DecodeID(entry.Name())
		if err != nil {
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
func (fs *FileStore) AppendRecord(orgID, id jsonldb.ID, record *models.DataRecord) error {
	if orgID == 0 {
		return fmt.Errorf("organization ID is required")
	}
	pageDir := fs.pageDir(orgID, id)
	if err := os.MkdirAll(pageDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := fs.databaseRecordsFile(orgID, id)

	// Load or create table using jsonldb
	table, err := jsonldb.NewTable[*models.DataRecord](filePath)
	if err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	// Append record
	if err := table.Append(record); err != nil {
		return fmt.Errorf("failed to append record: %w", err)
	}

	return nil
}

// ReadRecords reads all records for a database using jsonldb abstraction.
func (fs *FileStore) ReadRecords(orgID, id jsonldb.ID) ([]*models.DataRecord, error) {
	if orgID == 0 {
		return nil, fmt.Errorf("organization ID is required")
	}
	filePath := fs.databaseRecordsFile(orgID, id)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return []*models.DataRecord{}, nil
	}

	// Load using jsonldb abstraction
	table, err := jsonldb.NewTable[*models.DataRecord](filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read records: %w", err)
	}

	// Collect all records from table
	var records []*models.DataRecord
	for r := range table.All() {
		records = append(records, r)
	}
	return records, nil
}

// ReadRecordsPage reads a page of records for a database using jsonldb abstraction.
func (fs *FileStore) ReadRecordsPage(orgID, id jsonldb.ID, offset, limit int) ([]*models.DataRecord, error) {
	if orgID == 0 {
		return nil, fmt.Errorf("organization ID is required")
	}
	filePath := fs.databaseRecordsFile(orgID, id)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return []*models.DataRecord{}, nil
	}

	// Load using jsonldb abstraction
	table, err := jsonldb.NewTable[*models.DataRecord](filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read records: %w", err)
	}

	// Paginate records
	if offset < 0 {
		offset = 0
	}
	if offset >= table.Len() {
		return []*models.DataRecord{}, nil
	}

	end := offset + limit
	if end > table.Len() {
		end = table.Len()
	}

	// Collect paginated records from table
	var records []*models.DataRecord
	idx := 0
	for r := range table.All() {
		if idx >= offset && idx < end {
			records = append(records, r)
		}
		idx++
		if idx >= end {
			break
		}
	}
	return records, nil
}

// UpdateRecord updates an existing record in a database using jsonldb abstraction.
func (fs *FileStore) UpdateRecord(orgID, databaseID jsonldb.ID, record *models.DataRecord) error {
	if orgID == 0 {
		return fmt.Errorf("organization ID is required")
	}

	filePath := fs.databaseRecordsFile(orgID, databaseID)

	// Load using jsonldb abstraction
	table, err := jsonldb.NewTable[*models.DataRecord](filePath)
	if err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	// Find and update the record
	var updated []*models.DataRecord
	found := false
	for r := range table.All() {
		if r.ID == record.ID {
			updated = append(updated, record)
			found = true
		} else {
			updated = append(updated, r)
		}
	}

	if !found {
		return fmt.Errorf("record not found")
	}

	return table.Replace(updated)
}

// DeleteRecord deletes a record from a database using jsonldb abstraction.
func (fs *FileStore) DeleteRecord(orgID, databaseID, recordID jsonldb.ID) error {
	if orgID == 0 {
		return fmt.Errorf("organization ID is required")
	}

	filePath := fs.databaseRecordsFile(orgID, databaseID)

	// Load using jsonldb abstraction
	table, err := jsonldb.NewTable[*models.DataRecord](filePath)
	if err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	// Find and remove the record
	var updated []*models.DataRecord
	found := false
	for r := range table.All() {
		if r.ID == recordID {
			found = true
		} else {
			updated = append(updated, r)
		}
	}

	if !found {
		return fmt.Errorf("record not found")
	}

	return table.Replace(updated)
}

func (fs *FileStore) databaseRecordsFile(orgID, id jsonldb.ID) string {
	return filepath.Join(fs.pageDir(orgID, id), "data.jsonl")
}

// Asset operations

// SaveAsset saves an asset associated with a page.
func (fs *FileStore) SaveAsset(orgID, pageID jsonldb.ID, assetName string, data []byte) (string, error) {
	if orgID == 0 {
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
func (fs *FileStore) ReadAsset(orgID, pageID jsonldb.ID, assetName string) ([]byte, error) {
	if orgID == 0 {
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
func (fs *FileStore) DeleteAsset(orgID, pageID jsonldb.ID, assetName string) error {
	if orgID == 0 {
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
func (fs *FileStore) ListAssets(orgID, pageID jsonldb.ID) ([]*models.Asset, error) {
	if orgID == 0 {
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

func parseMarkdownFile(id jsonldb.ID, data []byte) *models.Page {
	content := string(data)
	title := id.String()
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
	buf.WriteString("\nid: " + page.ID.String() + "\n")
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

// columnsToJSONLDB converts models.Column to jsonldb.Column for storage.
// High-level types (select, multi_select) are mapped to their storage types (text).
// Options are not stored in jsonldb schema - they must be stored separately.
func columnsToJSONLDB(cols []models.Column) []jsonldb.Column {
	result := make([]jsonldb.Column, len(cols))
	for i, col := range cols {
		result[i] = jsonldb.Column{
			Name:     col.Name,
			Type:     col.Type.StorageType(),
			Required: col.Required,
		}
	}
	return result
}

// columnsFromJSONLDB converts jsonldb.Column to models.Column.
// This only recovers primitive types - high-level types (select, multi_select)
// and their options must be merged from metadata storage.
func columnsFromJSONLDB(cols []jsonldb.Column) []models.Column {
	result := make([]models.Column, len(cols))
	for i, col := range cols {
		result[i] = models.Column{
			Name:     col.Name,
			Type:     storageTypeToModelType(col.Type),
			Required: col.Required,
		}
	}
	return result
}

// storageTypeToModelType converts a jsonldb storage type to a models column type.
// Since select/multi_select are stored as text, this only returns primitive types.
// Blob and JSONB storage types don't have high-level equivalents yet.
func storageTypeToModelType(st jsonldb.ColumnType) models.ColumnType {
	switch st {
	case jsonldb.ColumnTypeText:
		return models.ColumnTypeText
	case jsonldb.ColumnTypeNumber:
		return models.ColumnTypeNumber
	case jsonldb.ColumnTypeBool:
		return models.ColumnTypeCheckbox
	case jsonldb.ColumnTypeDate:
		return models.ColumnTypeDate
	case jsonldb.ColumnTypeBlob, jsonldb.ColumnTypeJSONB:
		// No high-level equivalent yet, treat as text
		return models.ColumnTypeText
	default:
		return models.ColumnTypeText
	}
}
