// Package content provides the core file storage implementation for the mddb system.
package content

import (
	"bytes"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)


// FileStore handles all file system operations using directory-based storage.
// Storage model: Each page (document or database) is an ID-based directory.
//   - Pages: ID directory containing index.md with YAML front matter.
//   - Databases: ID directory containing data.jsonl (with schema header in first row).
//   - Assets: files within each page's directory namespace.
type FileStore struct {
	rootDir string
}

// page is an internal type for reading/writing page markdown files.
type page struct {
	id         jsonldb.ID
	title      string
	content    string
	created    time.Time
	modified   time.Time
	tags       []string
	faviconURL string
}

// NewFileStore initializes a FileStore with the given root directory.
func NewFileStore(rootDir string) (*FileStore, error) {
	if err := os.MkdirAll(rootDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
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
	_ = os.MkdirAll(dir, 0o755) //nolint:gosec // G301: 0o755 is intentional for user data directories
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

// ReadPage reads a page from disk and returns it as a Node.
func (fs *FileStore) ReadPage(orgID, id jsonldb.ID) (*entity.Node, error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	filePath := fs.pageIndexFile(orgID, id)
	data, err := os.ReadFile(filePath) //nolint:gosec // G304: filePath is constructed from validated orgID and id
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errPageNotFound
		}
		return nil, fmt.Errorf("failed to read page: %w", err)
	}

	p := parseMarkdownFile(id, data)
	return &entity.Node{
		ID:         p.id,
		Title:      p.title,
		Content:    p.content,
		Created:    p.created,
		Modified:   p.modified,
		Tags:       p.tags,
		FaviconURL: p.faviconURL,
		Type:       entity.NodeTypeDocument,
	}, nil
}

// WritePage writes a page to disk and returns it as a Node.
func (fs *FileStore) WritePage(orgID, id jsonldb.ID, title, content string) (*entity.Node, error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	now := time.Now()
	p := &page{
		id:       id,
		title:    title,
		content:  content,
		created:  now,
		modified: now,
	}

	pageDir := fs.pageDir(orgID, id)
	if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := fs.pageIndexFile(orgID, id)
	data := formatMarkdownFile(p)
	if err := os.WriteFile(filePath, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return nil, fmt.Errorf("failed to write page: %w", err)
	}

	return &entity.Node{
		ID:       p.id,
		Title:    p.title,
		Content:  p.content,
		Created:  p.created,
		Modified: p.modified,
		Tags:     p.tags,
		Type:     entity.NodeTypeDocument,
	}, nil
}

// UpdatePage updates an existing page and returns it as a Node.
func (fs *FileStore) UpdatePage(orgID, id jsonldb.ID, title, content string) (*entity.Node, error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	filePath := fs.pageIndexFile(orgID, id)
	data, err := os.ReadFile(filePath) //nolint:gosec // G304: filePath is constructed from validated orgID and id
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errPageNotFound
		}
		return nil, fmt.Errorf("failed to read page: %w", err)
	}

	p := parseMarkdownFile(id, data)
	p.title = title
	p.content = content
	p.modified = time.Now()

	updatedData := formatMarkdownFile(p)
	if err := os.WriteFile(filePath, updatedData, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return nil, fmt.Errorf("failed to write page: %w", err)
	}

	return &entity.Node{
		ID:         p.id,
		Title:      p.title,
		Content:    p.content,
		Created:    p.created,
		Modified:   p.modified,
		Tags:       p.tags,
		FaviconURL: p.faviconURL,
		Type:       entity.NodeTypeDocument,
	}, nil
}

// DeletePage deletes a page directory.
func (fs *FileStore) DeletePage(orgID, id jsonldb.ID) error {
	if orgID == 0 {
		return errOrgIDRequired
	}
	pageDir := fs.pageDir(orgID, id)
	if err := os.RemoveAll(pageDir); err != nil {
		if os.IsNotExist(err) {
			return errPageNotFound
		}
		return fmt.Errorf("failed to delete page: %w", err)
	}
	return nil
}

// IterPages returns an iterator over all pages for an organization as Nodes.
func (fs *FileStore) IterPages(orgID jsonldb.ID) (iter.Seq[*entity.Node], error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	dir := fs.orgPagesDir(orgID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read pages directory: %w", err)
	}

	return func(yield func(*entity.Node) bool) {
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
				if node, err := fs.ReadPage(orgID, id); err == nil {
					if !yield(node) {
						return
					}
				}
			}
		}
	}, nil
}

// ReadNode reads a unified node from disk.
func (fs *FileStore) ReadNode(orgID, id jsonldb.ID) (*entity.Node, error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	nodeDir := fs.pageDir(orgID, id)
	info, err := os.Stat(nodeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errNodeNotFound
		}
		return nil, fmt.Errorf("failed to access node: %w", err)
	}

	node := &entity.Node{
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
			node.Type = entity.NodeTypeDocument
		}
	}

	metadataFile := fs.databaseMetadataFile(orgID, id)
	if _, err := os.Stat(metadataFile); err == nil {
		db, err := fs.ReadDatabase(orgID, id)
		if err == nil {
			if node.Type == entity.NodeTypeDocument {
				node.Type = entity.NodeTypeHybrid
			} else {
				node.Type = entity.NodeTypeDatabase
				node.Title = db.Title
				node.Created = db.Created
				node.Modified = db.Modified
			}
			node.Properties = db.Properties
		}
	}

	return node, nil
}

// ReadNodeTree returns the full hierarchical tree of nodes.
func (fs *FileStore) ReadNodeTree(orgID jsonldb.ID) ([]*entity.Node, error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	return fs.readNodesRecursive(orgID, fs.orgPagesDir(orgID), 0)
}

func (fs *FileStore) readNodesRecursive(orgID jsonldb.ID, dir string, parentID jsonldb.ID) ([]*entity.Node, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var nodes []*entity.Node
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
func (fs *FileStore) ReadNodeFromPath(orgID jsonldb.ID, path string, id, parentID jsonldb.ID) (*entity.Node, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	node := &entity.Node{
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
			node.Type = entity.NodeTypeDocument
		}
	}

	schemaFile := filepath.Join(path, "metadata.json")
	if _, err := os.Stat(schemaFile); err == nil {
		db, err := fs.ReadDatabase(orgID, id)
		if err == nil {
			if node.Type == entity.NodeTypeDocument {
				node.Type = entity.NodeTypeHybrid
			} else {
				node.Type = entity.NodeTypeDatabase
				node.Title = db.Title
				node.Created = db.Created
				node.Modified = db.Modified
			}
			node.Properties = db.Properties
		}
	}

	return node, nil
}

// GetOrganizationUsage calculates the total number of pages and storage usage (in bytes) for an organization.
func (fs *FileStore) GetOrganizationUsage(orgID jsonldb.ID) (pageCount int, storageUsage int64, err error) {
	if orgID == 0 {
		return 0, 0, errOrgIDRequired
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
	path := fs.databaseMetadataFile(orgID, id)
	_, err := os.Stat(path)
	return err == nil
}

// ReadDatabase reads a database definition from metadata.json and returns it as a Node.
func (fs *FileStore) ReadDatabase(orgID, id jsonldb.ID) (*entity.Node, error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}

	metadataFile := fs.databaseMetadataFile(orgID, id)
	data, err := os.ReadFile(metadataFile) //nolint:gosec // G304: metadataFile is constructed from validated orgID and id
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errDatabaseNotFound
		}
		return nil, fmt.Errorf("failed to read database metadata: %w", err)
	}

	var metadata struct {
		Title      string            `json:"title"`
		Version    string            `json:"version"`
		Created    time.Time         `json:"created"`
		Modified   time.Time         `json:"modified"`
		Properties []entity.Property `json:"properties"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse database metadata: %w", err)
	}

	return &entity.Node{
		ID:         id,
		Title:      metadata.Title,
		Properties: metadata.Properties,
		Created:    metadata.Created,
		Modified:   metadata.Modified,
		Type:       entity.NodeTypeDatabase,
	}, nil
}

// WriteDatabase writes database metadata (including properties) to metadata.json.
// The JSONL records file is created lazily when the first record is added.
func (fs *FileStore) WriteDatabase(orgID jsonldb.ID, node *entity.Node) error {
	if orgID == 0 {
		return errOrgIDRequired
	}

	if err := os.MkdirAll(fs.pageDir(orgID, node.ID), 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write metadata.json with all database metadata including properties
	metadataFile := fs.databaseMetadataFile(orgID, node.ID)
	metadata := map[string]any{
		"title":      node.Title,
		"version":    "1.0",
		"created":    node.Created,
		"modified":   node.Modified,
		"properties": node.Properties,
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(metadataFile, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

// DeleteDatabase deletes a database and all its records.
func (fs *FileStore) DeleteDatabase(orgID, id jsonldb.ID) error {
	if orgID == 0 {
		return errOrgIDRequired
	}
	pageDir := fs.pageDir(orgID, id)
	if err := os.RemoveAll(pageDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete database: %w", err)
	}
	return nil
}

// IterDatabases returns an iterator over all databases for the given organization as Nodes.
func (fs *FileStore) IterDatabases(orgID jsonldb.ID) (iter.Seq[*entity.Node], error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	dir := fs.orgPagesDir(orgID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read pages directory: %w", err)
	}

	return func(yield func(*entity.Node) bool) {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			id, err := jsonldb.DecodeID(entry.Name())
			if err != nil {
				continue
			}
			metadataFile := fs.databaseMetadataFile(orgID, id)
			if _, err := os.Stat(metadataFile); err == nil {
				if node, err := fs.ReadDatabase(orgID, id); err == nil {
					if !yield(node) {
						return
					}
				}
			}
		}
	}, nil
}

// AppendRecord appends a record to a database using jsonldb abstraction.
func (fs *FileStore) AppendRecord(orgID, id jsonldb.ID, record *entity.DataRecord) error {
	if orgID == 0 {
		return errOrgIDRequired
	}
	pageDir := fs.pageDir(orgID, id)
	if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := fs.databaseRecordsFile(orgID, id)

	// Load or create table using jsonldb
	table, err := jsonldb.NewTable[*entity.DataRecord](filePath)
	if err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	// Append record
	if err := table.Append(record); err != nil {
		return fmt.Errorf("failed to append record: %w", err)
	}

	return nil
}

// IterRecords returns an iterator over all records for a database.
func (fs *FileStore) IterRecords(orgID, id jsonldb.ID) (iter.Seq[*entity.DataRecord], error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	filePath := fs.databaseRecordsFile(orgID, id)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return func(yield func(*entity.DataRecord) bool) {}, nil
	}

	table, err := jsonldb.NewTable[*entity.DataRecord](filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read records: %w", err)
	}
	return table.Iter(0), nil
}

// ReadRecordsPage reads a page of records for a database using jsonldb abstraction.
func (fs *FileStore) ReadRecordsPage(orgID, id jsonldb.ID, offset, limit int) ([]*entity.DataRecord, error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	filePath := fs.databaseRecordsFile(orgID, id)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return []*entity.DataRecord{}, nil
	}

	table, err := jsonldb.NewTable[*entity.DataRecord](filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read records: %w", err)
	}

	offset = max(0, offset)
	if offset >= table.Len() {
		return []*entity.DataRecord{}, nil
	}
	end := min(offset+limit, table.Len())

	var records []*entity.DataRecord
	idx := 0
	for r := range table.Iter(0) {
		if idx >= offset {
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
func (fs *FileStore) UpdateRecord(orgID, databaseID jsonldb.ID, record *entity.DataRecord) error {
	if orgID == 0 {
		return errOrgIDRequired
	}

	filePath := fs.databaseRecordsFile(orgID, databaseID)

	// Load using jsonldb abstraction
	table, err := jsonldb.NewTable[*entity.DataRecord](filePath)
	if err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	prev, err := table.Update(record)
	if err != nil {
		return fmt.Errorf("failed to update record: %w", err)
	}
	if prev == nil {
		return errRecordNotFound
	}
	return nil
}

// DeleteRecord deletes a record from a database using jsonldb abstraction.
func (fs *FileStore) DeleteRecord(orgID, databaseID, recordID jsonldb.ID) error {
	if orgID == 0 {
		return errOrgIDRequired
	}

	filePath := fs.databaseRecordsFile(orgID, databaseID)

	// Load using jsonldb abstraction
	table, err := jsonldb.NewTable[*entity.DataRecord](filePath)
	if err != nil {
		return fmt.Errorf("failed to load database: %w", err)
	}

	deleted, err := table.Delete(recordID)
	if err != nil {
		return fmt.Errorf("failed to delete record: %w", err)
	}
	if deleted == nil {
		return errRecordNotFound
	}
	return nil
}

func (fs *FileStore) databaseRecordsFile(orgID, id jsonldb.ID) string {
	return filepath.Join(fs.pageDir(orgID, id), "data.jsonl")
}

func (fs *FileStore) databaseMetadataFile(orgID, id jsonldb.ID) string {
	return filepath.Join(fs.pageDir(orgID, id), "metadata.json")
}

// Asset operations

// SaveAsset saves an asset associated with a page.
func (fs *FileStore) SaveAsset(orgID, pageID jsonldb.ID, assetName string, data []byte) (string, error) {
	if orgID == 0 {
		return "", errOrgIDRequired
	}
	pageDir := fs.pageDir(orgID, pageID)
	if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return "", fmt.Errorf("failed to create page directory: %w", err)
	}

	assetPath := filepath.Join(pageDir, assetName)
	if err := os.WriteFile(assetPath, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return "", fmt.Errorf("failed to write asset: %w", err)
	}

	return assetName, nil
}

// ReadAsset reads an asset associated with a page.
func (fs *FileStore) ReadAsset(orgID, pageID jsonldb.ID, assetName string) ([]byte, error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	assetPath := filepath.Join(fs.pageDir(orgID, pageID), assetName)
	data, err := os.ReadFile(assetPath) //nolint:gosec // G304: assetPath is constructed from validated IDs and filename
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errAssetNotFound
		}
		return nil, fmt.Errorf("failed to read asset: %w", err)
	}
	return data, nil
}

// DeleteAsset deletes an asset associated with a page.
func (fs *FileStore) DeleteAsset(orgID, pageID jsonldb.ID, assetName string) error {
	if orgID == 0 {
		return errOrgIDRequired
	}
	assetPath := filepath.Join(fs.pageDir(orgID, pageID), assetName)
	if err := os.Remove(assetPath); err != nil {
		if os.IsNotExist(err) {
			return errAssetNotFound
		}
		return fmt.Errorf("failed to delete asset: %w", err)
	}
	return nil
}

// IterAssets returns an iterator over all assets associated with a page.
func (fs *FileStore) IterAssets(orgID, pageID jsonldb.ID) (iter.Seq[*entity.Asset], error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	pageDir := fs.pageDir(orgID, pageID)
	entries, err := os.ReadDir(pageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return func(yield func(*entity.Asset) bool) {}, nil
		}
		return nil, fmt.Errorf("failed to read assets: %w", err)
	}

	return func(yield func(*entity.Asset) bool) {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if name == "index.md" || name == "data.jsonl" || name == "metadata.json" {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if !yield(&entity.Asset{
				ID:      name,
				Name:    name,
				Size:    info.Size(),
				Created: info.ModTime(),
				Path:    name,
			}) {
				return
			}
		}
	}, nil
}

// Helpers

func parseMarkdownFile(id jsonldb.ID, data []byte) *page {
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

	return &page{
		id:       id,
		title:    title,
		content:  content,
		created:  created,
		modified: modified,
	}
}

func formatMarkdownFile(p *page) []byte {
	var buf bytes.Buffer
	buf.WriteString("---")
	buf.WriteString("\nid: " + p.id.String() + "\n")
	buf.WriteString("title: " + p.title + "\n")
	buf.WriteString("created: " + p.created.Format(time.RFC3339) + "\n")
	buf.WriteString("modified: " + p.modified.Format(time.RFC3339) + "\n")
	if len(p.tags) > 0 {
		buf.WriteString("tags: [" + strings.Join(p.tags, ", ") + "]\n")
	}
	buf.WriteString("---")
	buf.WriteString("\n\n")
	buf.WriteString(p.content)
	return buf.Bytes()
}

