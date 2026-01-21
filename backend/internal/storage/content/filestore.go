// Package content provides versioned file storage for the mddb system.
package content

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/git"
)

// Author identifies who made a change for git commits.
type Author struct {
	Name  string
	Email string
}

// FileStore is a versioned file storage system. All mutations are committed to git.
// Storage model: Each page (document or table) is an ID-based directory.
//   - Pages: ID directory containing index.md with YAML front matter.
//   - Tables: ID directory containing metadata.json + data.jsonl.
//   - Assets: files within each page's directory namespace.
type FileStore struct {
	rootDir     string
	Git         *git.Client
	quotaGetter QuotaGetter
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

// NewFileStore creates a versioned file store.
// gitClient is required - all operations are versioned.
// quotaGetter provides quota limits for organizations.
func NewFileStore(rootDir string, gitClient *git.Client, quotaGetter QuotaGetter) (*FileStore, error) {
	if gitClient == nil {
		return nil, errors.New("git client is required")
	}
	if quotaGetter == nil {
		return nil, errors.New("quota getter is required")
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}

	return &FileStore{
		rootDir:     rootDir,
		Git:         gitClient,
		quotaGetter: quotaGetter,
	}, nil
}

// InitOrg initializes storage for a new organization.
// Creates the org directory structure and initializes git.
func (fs *FileStore) InitOrg(ctx context.Context, orgID jsonldb.ID) error {
	if orgID.IsZero() {
		return errOrgIDRequired
	}
	orgDir := filepath.Join(fs.rootDir, orgID.String())
	pagesDir := filepath.Join(orgDir, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return fmt.Errorf("failed to create organization directory: %w", err)
	}
	if err := fs.Git.Init(ctx, orgID.String()); err != nil {
		return fmt.Errorf("failed to initialize git repo for org %s: %w", orgID, err)
	}
	return nil
}

// checkPageQuota returns an error if creating a new page would exceed quota.
func (fs *FileStore) checkPageQuota(ctx context.Context, orgID jsonldb.ID) error {
	quota, err := fs.quotaGetter.GetQuota(ctx, orgID)
	if err != nil {
		return err
	}
	if quota.MaxPages <= 0 {
		return nil // No limit
	}
	count, _, err := fs.GetOrganizationUsage(orgID)
	if err != nil {
		return err
	}
	if count >= quota.MaxPages {
		return errQuotaExceeded
	}
	return nil
}

// checkStorageQuota returns an error if adding the given bytes would exceed storage quota.
func (fs *FileStore) checkStorageQuota(ctx context.Context, orgID jsonldb.ID, additionalBytes int64) error {
	quota, err := fs.quotaGetter.GetQuota(ctx, orgID)
	if err != nil {
		return err
	}
	if quota.MaxStorage <= 0 {
		return nil // No limit
	}
	_, usage, err := fs.GetOrganizationUsage(orgID)
	if err != nil {
		return err
	}
	if usage+additionalBytes > quota.MaxStorage {
		return errQuotaExceeded
	}
	return nil
}

func (fs *FileStore) orgPagesDir(orgID jsonldb.ID) string {
	if orgID == 0 {
		return ""
	}
	dir := filepath.Join(fs.rootDir, orgID.String(), "pages")
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		slog.Error("Failed to create organization pages directory", "dir", dir, "error", err)
	}
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
func (fs *FileStore) ReadPage(orgID, id jsonldb.ID) (*Node, error) {
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
	return &Node{
		ID:         p.id,
		Title:      p.title,
		Content:    p.content,
		Created:    p.created,
		Modified:   p.modified,
		Tags:       p.tags,
		FaviconURL: p.faviconURL,
		Type:       NodeTypeDocument,
	}, nil
}

// WritePage creates a new page on disk, commits to git, and returns it as a Node.
func (fs *FileStore) WritePage(ctx context.Context, orgID, id jsonldb.ID, title, content string, author Author) (*Node, error) {
	if orgID.IsZero() {
		return nil, errOrgIDRequired
	}
	if err := fs.checkPageQuota(ctx, orgID); err != nil {
		return nil, err
	}

	now := time.Now()
	p := &page{
		id:       id,
		title:    title,
		content:  content,
		created:  now,
		modified: now,
	}

	data := formatMarkdownFile(p)
	if err := fs.checkStorageQuota(ctx, orgID, int64(len(data))); err != nil {
		return nil, err
	}

	pageDir := fs.pageDir(orgID, id)
	if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := fs.pageIndexFile(orgID, id)
	if err := os.WriteFile(filePath, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return nil, fmt.Errorf("failed to write page: %w", err)
	}

	msg := "create: page " + id.String() + " - " + title
	files := []string{"pages/" + id.String() + "/index.md"}
	if err := fs.Git.Commit(ctx, orgID.String(), author.Name, author.Email, msg, files); err != nil {
		return nil, err
	}

	return &Node{
		ID:       p.id,
		Title:    p.title,
		Content:  p.content,
		Created:  p.created,
		Modified: p.modified,
		Tags:     p.tags,
		Type:     NodeTypeDocument,
	}, nil
}

// UpdatePage updates an existing page, commits to git, and returns it as a Node.
func (fs *FileStore) UpdatePage(ctx context.Context, orgID, id jsonldb.ID, title, content string, author Author) (*Node, error) {
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

	msg := "update: page " + id.String()
	files := []string{"pages/" + id.String() + "/index.md"}
	if err := fs.Git.Commit(ctx, orgID.String(), author.Name, author.Email, msg, files); err != nil {
		return nil, err
	}

	return &Node{
		ID:         p.id,
		Title:      p.title,
		Content:    p.content,
		Created:    p.created,
		Modified:   p.modified,
		Tags:       p.tags,
		FaviconURL: p.faviconURL,
		Type:       NodeTypeDocument,
	}, nil
}

// DeletePage deletes a page directory and commits to git.
func (fs *FileStore) DeletePage(ctx context.Context, orgID, id jsonldb.ID, author Author) error {
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

	msg := "delete: page " + id.String()
	files := []string{"pages/" + id.String()}
	return fs.Git.Commit(ctx, orgID.String(), author.Name, author.Email, msg, files)
}

// IterPages returns an iterator over all pages for an organization as Nodes.
func (fs *FileStore) IterPages(orgID jsonldb.ID) (iter.Seq[*Node], error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	dir := fs.orgPagesDir(orgID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read pages directory: %w", err)
	}

	return func(yield func(*Node) bool) {
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
func (fs *FileStore) ReadNode(orgID, id jsonldb.ID) (*Node, error) {
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

	node := &Node{
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
			node.Type = NodeTypeDocument
		}
	}

	metadataFile := fs.databaseMetadataFile(orgID, id)
	if _, err := os.Stat(metadataFile); err == nil {
		table, err := fs.ReadTable(orgID, id)
		if err == nil {
			if node.Type == NodeTypeDocument {
				node.Type = NodeTypeHybrid
			} else {
				node.Type = NodeTypeTable
				node.Title = table.Title
				node.Created = table.Created
				node.Modified = table.Modified
			}
			node.Properties = table.Properties
		}
	}

	return node, nil
}

// ReadNodeTree returns the full hierarchical tree of nodes.
func (fs *FileStore) ReadNodeTree(orgID jsonldb.ID) ([]*Node, error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	return fs.readNodesRecursive(orgID, fs.orgPagesDir(orgID), 0)
}

func (fs *FileStore) readNodesRecursive(orgID jsonldb.ID, dir string, parentID jsonldb.ID) ([]*Node, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var nodes []*Node
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
func (fs *FileStore) ReadNodeFromPath(orgID jsonldb.ID, path string, id, parentID jsonldb.ID) (*Node, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	node := &Node{
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
			node.Type = NodeTypeDocument
		}
	}

	schemaFile := filepath.Join(path, "metadata.json")
	if _, err := os.Stat(schemaFile); err == nil {
		table, err := fs.ReadTable(orgID, id)
		if err == nil {
			if node.Type == NodeTypeDocument {
				node.Type = NodeTypeHybrid
			} else {
				node.Type = NodeTypeTable
				node.Title = table.Title
				node.Created = table.Created
				node.Modified = table.Modified
			}
			node.Properties = table.Properties
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

// TableExists checks if a table exists for the given organization and ID.
func (fs *FileStore) TableExists(orgID, id jsonldb.ID) bool {
	path := fs.databaseMetadataFile(orgID, id)
	_, err := os.Stat(path)
	return err == nil
}

// ReadTable reads a table definition from metadata.json and returns it as a Node.
func (fs *FileStore) ReadTable(orgID, id jsonldb.ID) (*Node, error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}

	metadataFile := fs.databaseMetadataFile(orgID, id)
	data, err := os.ReadFile(metadataFile) //nolint:gosec // G304: metadataFile is constructed from validated orgID and id
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errTableNotFound
		}
		return nil, fmt.Errorf("failed to read table metadata: %w", err)
	}

	var metadata struct {
		Title      string     `json:"title"`
		Version    string     `json:"version"`
		Created    time.Time  `json:"created"`
		Modified   time.Time  `json:"modified"`
		Properties []Property `json:"properties"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse table metadata: %w", err)
	}

	return &Node{
		ID:         id,
		Title:      metadata.Title,
		Properties: metadata.Properties,
		Created:    metadata.Created,
		Modified:   metadata.Modified,
		Type:       NodeTypeTable,
	}, nil
}

// WriteTable writes table metadata to metadata.json and commits to git.
// The JSONL records file is created lazily when the first record is added.
// isNew should be true for create operations (triggers quota check), false for updates.
func (fs *FileStore) WriteTable(ctx context.Context, orgID jsonldb.ID, node *Node, isNew bool, author Author) error {
	if orgID.IsZero() {
		return errOrgIDRequired
	}
	if isNew {
		if err := fs.checkPageQuota(ctx, orgID); err != nil {
			return err
		}
	}

	// Write metadata.json with all table metadata including properties
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

	if isNew {
		if err := fs.checkStorageQuota(ctx, orgID, int64(len(data))); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(fs.pageDir(orgID, node.ID), 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(metadataFile, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	var msg string
	if isNew {
		msg = "create: table " + node.ID.String() + " - " + node.Title
	} else {
		msg = "update: table " + node.ID.String()
	}
	files := []string{"pages/" + node.ID.String() + "/metadata.json"}
	return fs.Git.Commit(ctx, orgID.String(), author.Name, author.Email, msg, files)
}

// DeleteTable deletes a table and all its records, commits to git.
func (fs *FileStore) DeleteTable(ctx context.Context, orgID, id jsonldb.ID, author Author) error {
	if orgID == 0 {
		return errOrgIDRequired
	}
	pageDir := fs.pageDir(orgID, id)
	if err := os.RemoveAll(pageDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete table: %w", err)
	}

	msg := "delete: table " + id.String()
	files := []string{"pages/" + id.String()}
	return fs.Git.Commit(ctx, orgID.String(), author.Name, author.Email, msg, files)
}

// IterTables returns an iterator over all tables for the given organization as Nodes.
func (fs *FileStore) IterTables(orgID jsonldb.ID) (iter.Seq[*Node], error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	dir := fs.orgPagesDir(orgID)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read pages directory: %w", err)
	}

	return func(yield func(*Node) bool) {
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
				if node, err := fs.ReadTable(orgID, id); err == nil {
					if !yield(node) {
						return
					}
				}
			}
		}
	}, nil
}

// AppendRecord appends a record to a table and commits to git.
func (fs *FileStore) AppendRecord(ctx context.Context, orgID, tableID jsonldb.ID, record *DataRecord, author Author) error {
	if orgID.IsZero() {
		return errOrgIDRequired
	}

	// Estimate storage impact
	recordData, _ := json.Marshal(record)
	if err := fs.checkStorageQuota(ctx, orgID, int64(len(recordData)+1)); err != nil {
		return err
	}

	pageDir := fs.pageDir(orgID, tableID)
	if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := fs.databaseRecordsFile(orgID, tableID)

	// Load or create table using jsonldb
	table, err := jsonldb.NewTable[*DataRecord](filePath)
	if err != nil {
		return fmt.Errorf("failed to load table: %w", err)
	}

	// Append record
	if err := table.Append(record); err != nil {
		return fmt.Errorf("failed to append record: %w", err)
	}

	msg := "create: record " + record.ID.String() + " in table " + tableID.String()
	files := []string{"pages/" + tableID.String() + "/data.jsonl"}
	return fs.Git.Commit(ctx, orgID.String(), author.Name, author.Email, msg, files)
}

// IterRecords returns an iterator over all records for a table.
func (fs *FileStore) IterRecords(orgID, id jsonldb.ID) (iter.Seq[*DataRecord], error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	filePath := fs.databaseRecordsFile(orgID, id)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return func(yield func(*DataRecord) bool) {}, nil
	}

	table, err := jsonldb.NewTable[*DataRecord](filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read records: %w", err)
	}
	return table.Iter(0), nil
}

// ReadRecordsPage reads a page of records for a table using jsonldb abstraction.
func (fs *FileStore) ReadRecordsPage(orgID, id jsonldb.ID, offset, limit int) ([]*DataRecord, error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	filePath := fs.databaseRecordsFile(orgID, id)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return []*DataRecord{}, nil
	}

	table, err := jsonldb.NewTable[*DataRecord](filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read records: %w", err)
	}

	offset = max(0, offset)
	if offset >= table.Len() {
		return []*DataRecord{}, nil
	}
	end := min(offset+limit, table.Len())

	var records []*DataRecord
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

// UpdateRecord updates an existing record in a table and commits to git.
func (fs *FileStore) UpdateRecord(ctx context.Context, orgID, tableID jsonldb.ID, record *DataRecord, author Author) error {
	if orgID.IsZero() {
		return errOrgIDRequired
	}

	// Estimate storage impact (simplified, assuming replacement might increase size)
	recordData, _ := json.Marshal(record)
	if err := fs.checkStorageQuota(ctx, orgID, int64(len(recordData))); err != nil {
		return err
	}

	filePath := fs.databaseRecordsFile(orgID, tableID)

	// Load using jsonldb abstraction
	table, err := jsonldb.NewTable[*DataRecord](filePath)
	if err != nil {
		return fmt.Errorf("failed to load table: %w", err)
	}

	prev, err := table.Update(record)
	if err != nil {
		return fmt.Errorf("failed to update record: %w", err)
	}
	if prev == nil {
		return errRecordNotFound
	}

	msg := "update: record " + record.ID.String() + " in table " + tableID.String()
	files := []string{"pages/" + tableID.String() + "/data.jsonl"}
	return fs.Git.Commit(ctx, orgID.String(), author.Name, author.Email, msg, files)
}

// DeleteRecord deletes a record from a table and commits to git.
func (fs *FileStore) DeleteRecord(ctx context.Context, orgID, tableID, recordID jsonldb.ID, author Author) error {
	if orgID == 0 {
		return errOrgIDRequired
	}

	filePath := fs.databaseRecordsFile(orgID, tableID)

	// Load using jsonldb abstraction
	table, err := jsonldb.NewTable[*DataRecord](filePath)
	if err != nil {
		return fmt.Errorf("failed to load table: %w", err)
	}

	deleted, err := table.Delete(recordID)
	if err != nil {
		return fmt.Errorf("failed to delete record: %w", err)
	}
	if deleted == nil {
		return errRecordNotFound
	}

	msg := "delete: record " + recordID.String() + " from table " + tableID.String()
	files := []string{"pages/" + tableID.String() + "/data.jsonl"}
	return fs.Git.Commit(ctx, orgID.String(), author.Name, author.Email, msg, files)
}

func (fs *FileStore) databaseRecordsFile(orgID, id jsonldb.ID) string {
	return filepath.Join(fs.pageDir(orgID, id), "data.jsonl")
}

func (fs *FileStore) databaseMetadataFile(orgID, id jsonldb.ID) string {
	return filepath.Join(fs.pageDir(orgID, id), "metadata.json")
}

// Asset operations

// SaveAsset saves an asset associated with a page and commits to git.
func (fs *FileStore) SaveAsset(ctx context.Context, orgID, pageID jsonldb.ID, assetName string, data []byte, author Author) (*Asset, error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	if err := fs.checkStorageQuota(ctx, orgID, int64(len(data))); err != nil {
		return nil, err
	}

	pageDir := fs.pageDir(orgID, pageID)
	if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return nil, fmt.Errorf("failed to create page directory: %w", err)
	}

	assetPath := filepath.Join(pageDir, assetName)
	if err := os.WriteFile(assetPath, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return nil, fmt.Errorf("failed to write asset: %w", err)
	}

	msg := "create: asset " + assetName + " in page " + pageID.String()
	files := []string{"pages/" + pageID.String() + "/" + assetName}
	if err := fs.Git.Commit(ctx, orgID.String(), author.Name, author.Email, msg, files); err != nil {
		return nil, err
	}

	// Detect MIME type from filename
	mimeType := mime.TypeByExtension(filepath.Ext(assetName))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	return &Asset{
		ID:       assetName,
		Name:     assetName,
		MimeType: mimeType,
		Size:     int64(len(data)),
		Path:     assetName,
	}, nil
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

// DeleteAsset deletes an asset associated with a page and commits to git.
func (fs *FileStore) DeleteAsset(ctx context.Context, orgID, pageID jsonldb.ID, assetName string, author Author) error {
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

	msg := "delete: asset " + assetName + " from page " + pageID.String()
	files := []string{"pages/" + pageID.String() + "/" + assetName}
	return fs.Git.Commit(ctx, orgID.String(), author.Name, author.Email, msg, files)
}

// IterAssets returns an iterator over all assets associated with a page.
func (fs *FileStore) IterAssets(orgID, pageID jsonldb.ID) (iter.Seq[*Asset], error) {
	if orgID == 0 {
		return nil, errOrgIDRequired
	}
	pageDir := fs.pageDir(orgID, pageID)
	entries, err := os.ReadDir(pageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return func(yield func(*Asset) bool) {}, nil
		}
		return nil, fmt.Errorf("failed to read assets: %w", err)
	}

	return func(yield func(*Asset) bool) {
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
			if !yield(&Asset{
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

// History operations

// GetHistory returns the commit history for a page, limited to n commits.
// n is capped at 1000. If n <= 0, defaults to 1000.
func (fs *FileStore) GetHistory(ctx context.Context, orgID, id jsonldb.ID, n int) ([]*git.Commit, error) {
	return fs.Git.GetHistory(ctx, orgID.String(), "pages/"+id.String(), n)
}

// GetFileAtCommit returns the content of a file at a specific commit.
func (fs *FileStore) GetFileAtCommit(ctx context.Context, orgID jsonldb.ID, hash, path string) ([]byte, error) {
	return fs.Git.GetFileAtCommit(ctx, orgID.String(), hash, path)
}

// CreateNode creates a new node (can be document, database, or hybrid) and commits to git.
func (fs *FileStore) CreateNode(ctx context.Context, orgID jsonldb.ID, title string, nodeType NodeType, author Author) (*Node, error) {
	if err := fs.checkPageQuota(ctx, orgID); err != nil {
		return nil, err
	}

	id := jsonldb.NewID()
	now := time.Now()

	node := &Node{
		ID:       id,
		Title:    title,
		Type:     nodeType,
		Created:  now,
		Modified: now,
	}

	var files []string

	if nodeType == NodeTypeDocument || nodeType == NodeTypeHybrid {
		p := &page{
			id:       id,
			title:    title,
			content:  "",
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
		files = append(files, "pages/"+id.String()+"/index.md")
	}

	if nodeType == NodeTypeTable || nodeType == NodeTypeHybrid {
		if err := os.MkdirAll(fs.pageDir(orgID, id), 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}

		metadataFile := fs.databaseMetadataFile(orgID, id)
		metadata := map[string]any{
			"title":      title,
			"version":    "1.0",
			"created":    now,
			"modified":   now,
			"properties": []Property{},
		}
		data, err := json.Marshal(metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		if err := os.WriteFile(metadataFile, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
			return nil, fmt.Errorf("failed to write metadata: %w", err)
		}
		files = append(files, "pages/"+id.String()+"/metadata.json")
	}

	msg := "create: " + string(nodeType) + " " + id.String() + " - " + title
	if err := fs.Git.Commit(ctx, orgID.String(), author.Name, author.Email, msg, files); err != nil {
		return nil, err
	}

	return node, nil
}
