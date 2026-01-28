// Handles file operations within a specific workspace directory.

// Package content provides versioned file storage for the mddb system.
package content

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// WorkspaceFileStore is a versioned file storage system for a single workspace.
// All mutations are committed to git.
// Storage model: Each page (document or table) is an ID-based directory within the workspace.
//   - Pages: ID directory containing index.md with YAML front matter.
//   - Tables: ID directory containing metadata.json + data.jsonl.
//   - Assets: files within each page's directory namespace.
type WorkspaceFileStore struct {
	wsDir  string                    // Pre-computed: rootDir/wsID
	repo   *git.Repo                 // Cached git repository
	quotas *identity.WorkspaceQuotas // Workspace quotas
	mu     sync.RWMutex              // Protects cache
	cache  map[jsonldb.ID]jsonldb.ID // nodeID -> parentID
}

// newWorkspaceFileStore creates a new workspace store.
// This is called internally by FileStoreService.GetWorkspaceStore.
func newWorkspaceFileStore(wsDir string, repo *git.Repo, quotas *identity.WorkspaceQuotas) *WorkspaceFileStore {
	return &WorkspaceFileStore{
		wsDir:  wsDir,
		repo:   repo,
		quotas: quotas,
		cache:  make(map[jsonldb.ID]jsonldb.ID),
	}
}

// refreshCache rebuilds the parent map for the workspace.
func (ws *WorkspaceFileStore) refreshCache() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.cache = make(map[jsonldb.ID]jsonldb.ID)
	return ws.walkDirForCache(ws.wsDir, 0)
}

func (ws *WorkspaceFileStore) walkDirForCache(dir string, parentID jsonldb.ID) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		id, err := jsonldb.DecodeID(entry.Name())
		if err != nil {
			continue
		}

		ws.cache[id] = parentID
		if err := ws.walkDirForCache(filepath.Join(dir, entry.Name()), id); err != nil {
			return err
		}
	}
	return nil
}

// getParent returns the parent ID for a node.
// Returns 0 if the node is at the root or not found (caller handles not found via file system).
// Refreshes cache if node is not found.
func (ws *WorkspaceFileStore) getParent(id jsonldb.ID) jsonldb.ID {
	ws.mu.RLock()
	if parent, found := ws.cache[id]; found {
		ws.mu.RUnlock()
		return parent
	}
	ws.mu.RUnlock()

	// Not found in cache, try refreshing
	if err := ws.refreshCache(); err != nil {
		slog.Error("failed to refresh cache", "error", err)
		return 0
	}

	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return ws.cache[id]
}

// setParent updates the cache with a new parent relationship.
func (ws *WorkspaceFileStore) setParent(id, parentID jsonldb.ID) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.cache[id] = parentID
}

// deleteFromCache removes a node from the cache.
func (ws *WorkspaceFileStore) deleteFromCache(id jsonldb.ID) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	delete(ws.cache, id)
}

// checkPageQuota returns an error if creating a new page would exceed quota.
func (ws *WorkspaceFileStore) checkPageQuota() error {
	count, _, err := ws.GetWorkspaceUsage()
	if err != nil {
		return err
	}
	if count >= ws.quotas.MaxPages {
		return errQuotaExceeded
	}
	return nil
}

// checkStorageQuota returns an error if adding the given bytes would exceed workspace storage quota.
func (ws *WorkspaceFileStore) checkStorageQuota(additionalBytes int64) error {
	_, usage, err := ws.GetWorkspaceUsage()
	if err != nil {
		return err
	}
	maxStorageBytes := int64(ws.quotas.MaxStorageMB) * 1024 * 1024
	if usage+additionalBytes > maxStorageBytes {
		return errQuotaExceeded
	}
	return nil
}

// Path helpers

// relativeDir returns the relative directory path for a node.
// Top-level nodes (parentID=0) are stored directly in workspace dir.
// Nested nodes include their parent chain in the path.
func (ws *WorkspaceFileStore) relativeDir(id, parentID jsonldb.ID) string {
	// Build path from parent chain, stopping at workspace level (parentID=0).
	var parts []string
	for p := parentID; !p.IsZero(); p = ws.getParent(p) {
		parts = append(parts, p.String())
	}
	slices.Reverse(parts)
	parts = append(parts, id.String())
	return filepath.Join(parts...)
}

// pageDir returns the absolute directory path for a node.
func (ws *WorkspaceFileStore) pageDir(id, parentID jsonldb.ID) string {
	return filepath.Join(ws.wsDir, ws.relativeDir(id, parentID))
}

func (ws *WorkspaceFileStore) pageIndexFile(id, parentID jsonldb.ID) string {
	return filepath.Join(ws.pageDir(id, parentID), "index.md")
}

func (ws *WorkspaceFileStore) tableRecordsFile(id, parentID jsonldb.ID) string {
	return filepath.Join(ws.pageDir(id, parentID), "data.jsonl")
}

func (ws *WorkspaceFileStore) tableMetadataFile(id, parentID jsonldb.ID) string {
	return filepath.Join(ws.pageDir(id, parentID), "metadata.json")
}

// gitPath builds a git-relative path by walking up the parent chain.
// parentID must be passed explicitly because during node creation,
// the node doesn't exist in the cache yet (it's added after gitPath is called).
// For root node (id=0), returns just the filename.
func (ws *WorkspaceFileStore) gitPath(parentID, id jsonldb.ID, fileName string) string {
	rel := ws.relativeDir(id, parentID)
	if rel == "" {
		return fileName
	}
	return filepath.Join(rel, fileName)
}

// PageExists checks if a page exists.
func (ws *WorkspaceFileStore) PageExists(id jsonldb.ID) bool {
	if id.IsZero() {
		return false // No node with ID 0 exists
	}
	parentID := ws.getParent(id)
	filePath := ws.pageIndexFile(id, parentID)
	_, err := os.Stat(filePath)
	return err == nil
}

// ReadPage reads a page by ID.
func (ws *WorkspaceFileStore) ReadPage(id jsonldb.ID) (*Node, error) {
	parentID := ws.getParent(id)
	filePath := ws.pageIndexFile(id, parentID)

	data, err := os.ReadFile(filePath) //nolint:gosec // G304: filePath is constructed from validated id
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errPageNotFound
		}
		return nil, fmt.Errorf("failed to read page: %w", err)
	}

	p := parseMarkdownFile(data)
	return &Node{
		ID:       id,
		ParentID: parentID,
		Title:    p.title,
		Type:     NodeTypeDocument,
		Content:  p.content,
		Created:  p.created,
		Modified: p.modified,
	}, nil
}

// WritePage writes a page and commits to git.
func (ws *WorkspaceFileStore) WritePage(ctx context.Context, id, parentID jsonldb.ID, title, content string, author git.Author) (*Node, error) {
	var node *Node
	err := ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		var err error
		node, err = ws.writePage(id, parentID, title, content)
		if err != nil {
			return "", nil, err
		}
		files := []string{ws.gitPath(parentID, id, "index.md")}
		return "update: page " + id.String(), files, nil
	})
	return node, err
}

// writePage writes a page without committing.
func (ws *WorkspaceFileStore) writePage(id, parentID jsonldb.ID, title, content string) (*Node, error) {
	now := storage.Now()
	p := &page{
		title:    title,
		content:  content,
		created:  now,
		modified: now,
	}

	if err := ws.writePageFile(id, parentID, p); err != nil {
		return nil, err
	}

	return &Node{
		ID:       id,
		ParentID: parentID,
		Title:    p.title,
		Type:     NodeTypeDocument,
		Content:  p.content,
		Created:  p.created,
		Modified: p.modified,
	}, nil
}

// writePageFile writes the page file.
func (ws *WorkspaceFileStore) writePageFile(id, parentID jsonldb.ID, p *page) error {
	data := formatMarkdownFile(p)
	pageDir := ws.pageDir(id, parentID)
	filePath := ws.pageIndexFile(id, parentID)
	if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return fmt.Errorf("failed to create directory: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return fmt.Errorf("failed to write page: %w", err)
	}
	return nil
}

// UpdatePage updates a page and commits to git.
func (ws *WorkspaceFileStore) UpdatePage(ctx context.Context, id jsonldb.ID, title, content string, author git.Author) (*Node, error) {
	var node *Node
	err := ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		var err error
		node, err = ws.updatePage(id, title, content)
		if err != nil {
			return "", nil, err
		}
		parentID := ws.getParent(id)
		files := []string{ws.gitPath(parentID, id, "index.md")}
		return "update: page " + id.String(), files, nil
	})
	return node, err
}

// updatePage updates a page without committing.
func (ws *WorkspaceFileStore) updatePage(id jsonldb.ID, title, content string) (*Node, error) {
	parentID := ws.getParent(id)
	filePath := ws.pageIndexFile(id, parentID)

	data, err := os.ReadFile(filePath) //nolint:gosec // G304: filePath is constructed from validated id
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errPageNotFound
		}
		return nil, fmt.Errorf("failed to read page: %w", err)
	}

	p := parseMarkdownFile(data)
	p.title = title
	p.content = content
	p.modified = storage.Now()

	if err := ws.writePageFile(id, parentID, p); err != nil {
		return nil, err
	}

	return &Node{
		ID:       id,
		ParentID: parentID,
		Title:    p.title,
		Type:     NodeTypeDocument,
		Content:  p.content,
		Created:  p.created,
		Modified: p.modified,
	}, nil
}

// DeletePage deletes a page and commits to git.
func (ws *WorkspaceFileStore) DeletePage(ctx context.Context, id jsonldb.ID, author git.Author) error {
	parentID := ws.getParent(id)
	gitPathFile := ws.gitPath(parentID, id, "index.md")

	return ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := ws.deletePage(id); err != nil {
			return "", nil, err
		}
		return "delete: page " + id.String(), []string{gitPathFile}, nil
	})
}

// deletePage deletes a page without committing.
func (ws *WorkspaceFileStore) deletePage(id jsonldb.ID) error {
	if id.IsZero() {
		return errPageNotFound // No node with ID 0 exists
	}

	parentID := ws.getParent(id)
	dir := ws.pageDir(id, parentID)
	if err := os.RemoveAll(dir); err != nil {
		if os.IsNotExist(err) {
			return errPageNotFound
		}
		return fmt.Errorf("failed to delete page: %w", err)
	}
	ws.deleteFromCache(id)
	return nil
}

// IterPages returns an iterator over all pages in the workspace.
// Recursively traverses the directory tree to include child pages under parents.
func (ws *WorkspaceFileStore) IterPages() (iter.Seq[*Node], error) {
	return func(yield func(*Node) bool) {
		ws.iterPagesRecursive(ws.wsDir, 0, yield)
	}, nil
}

// iterPagesRecursive recursively yields pages from a directory and its subdirectories.
func (ws *WorkspaceFileStore) iterPagesRecursive(dir string, parentID jsonldb.ID, yield func(*Node) bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id, err := jsonldb.DecodeID(entry.Name())
		if err != nil {
			continue
		}
		indexFile := ws.pageIndexFile(id, parentID)
		if _, err := os.Stat(indexFile); err == nil {
			if node, err := ws.ReadPage(id); err == nil {
				if !yield(node) {
					return
				}
			}
		}
		// Recursively yield children
		ws.iterPagesRecursive(filepath.Join(dir, entry.Name()), id, yield)
	}
}

// ReadNode reads a node (page or table or hybrid) by ID.
func (ws *WorkspaceFileStore) ReadNode(id jsonldb.ID) (*Node, error) {
	parentID := ws.getParent(id)
	nodeDir := ws.pageDir(id, parentID)
	node, err := ws.ReadNodeFromPath(nodeDir, id, parentID)
	if err != nil {
		return nil, err
	}

	// Detect children for UI to show expand arrow
	entries, _ := os.ReadDir(nodeDir)
	for _, entry := range entries {
		if entry.IsDir() {
			if _, err := jsonldb.DecodeID(entry.Name()); err == nil {
				node.Children = []*Node{} // Has children - set empty slice to indicate expandable
				break
			}
		}
	}

	return node, nil
}

// ReadNodeFromPath reads a node from a specific path.
func (ws *WorkspaceFileStore) ReadNodeFromPath(path string, id, parentID jsonldb.ID) (*Node, error) {
	indexFile := filepath.Join(path, "index.md")
	metadataFile := filepath.Join(path, "metadata.json")

	indexData, indexErr := os.ReadFile(indexFile) //nolint:gosec // G304: path is constructed from validated id
	hasIndex := indexErr == nil

	metadataData, metadataErr := os.ReadFile(metadataFile) //nolint:gosec // G304: path is constructed from validated id
	hasMetadata := metadataErr == nil

	if !hasIndex && !hasMetadata {
		return nil, errPageNotFound
	}

	var nodeType NodeType
	switch {
	case hasIndex && hasMetadata:
		nodeType = NodeTypeHybrid
	case hasMetadata:
		nodeType = NodeTypeTable
	default:
		nodeType = NodeTypeDocument
	}

	node := &Node{
		ID:       id,
		ParentID: parentID,
		Type:     nodeType,
	}

	if hasIndex {
		p := parseMarkdownFile(indexData)
		node.Title = p.title
		node.Content = p.content
		node.Created = p.created
		node.Modified = p.modified
	}

	if hasMetadata {
		var metadata map[string]any
		if err := json.Unmarshal(metadataData, &metadata); err != nil {
			return nil, fmt.Errorf("failed to parse table metadata: %w", err)
		}
		if title, ok := metadata["title"].(string); ok {
			node.Title = title
		}
		if props, ok := metadata["properties"].([]any); ok {
			for _, prop := range props {
				if propMap, ok := prop.(map[string]any); ok {
					node.Properties = append(node.Properties, Property{
						Name: propMap["name"].(string),
						Type: PropertyType(propMap["type"].(string)),
					})
				}
			}
		}
	}

	return node, nil
}

// GetWorkspaceUsage returns the page count and storage usage for the workspace.
func (ws *WorkspaceFileStore) GetWorkspaceUsage() (pageCount int, storageUsage int64, err error) {
	pages, err := ws.IterPages()
	if err != nil {
		return 0, 0, err
	}
	for range pages {
		pageCount++
	}

	err = filepath.Walk(ws.wsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			storageUsage += info.Size()
		}
		return nil
	})
	return
}

// TableExists checks if a table exists.
func (ws *WorkspaceFileStore) TableExists(id jsonldb.ID) bool {
	parentID := ws.getParent(id)
	metadataFile := ws.tableMetadataFile(id, parentID)
	_, err := os.Stat(metadataFile)
	return err == nil
}

// ReadTable reads a table by ID.
func (ws *WorkspaceFileStore) ReadTable(id jsonldb.ID) (*Node, error) {
	parentID := ws.getParent(id)
	metadataFile := ws.tableMetadataFile(id, parentID)

	data, err := os.ReadFile(metadataFile) //nolint:gosec // G304: metadataFile is constructed from validated id
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errTableNotFound
		}
		return nil, fmt.Errorf("failed to read table metadata: %w", err)
	}

	var metadata map[string]any
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse table metadata: %w", err)
	}

	node := &Node{
		ID:       id,
		ParentID: parentID,
		Type:     NodeTypeTable,
	}

	if title, ok := metadata["title"].(string); ok {
		node.Title = title
	}

	if created, ok := metadata["created"].(string); ok {
		if t, err := time.Parse(time.RFC3339, created); err == nil {
			node.Created = storage.ToTime(t)
		}
	}

	if modified, ok := metadata["modified"].(string); ok {
		if t, err := time.Parse(time.RFC3339, modified); err == nil {
			node.Modified = storage.ToTime(t)
		}
	}

	if props, ok := metadata["properties"].([]any); ok {
		for _, prop := range props {
			if propMap, ok := prop.(map[string]any); ok {
				node.Properties = append(node.Properties, Property{
					Name: propMap["name"].(string),
					Type: PropertyType(propMap["type"].(string)),
				})
			}
		}
	}

	return node, nil
}

// WriteTable writes table metadata and commits to git.
func (ws *WorkspaceFileStore) WriteTable(ctx context.Context, node *Node, isNew bool, author git.Author) error {
	parentID := node.ParentID
	return ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := ws.writeTable(node, isNew); err != nil {
			return "", nil, err
		}
		files := []string{ws.gitPath(parentID, node.ID, "metadata.json")}
		return "update: table " + node.ID.String(), files, nil
	})
}

// writeTable writes table metadata without committing.
func (ws *WorkspaceFileStore) writeTable(node *Node, isNew bool) error {
	parentID := node.ParentID
	metadataFile := ws.tableMetadataFile(node.ID, parentID)

	metadata := map[string]any{
		"title":      node.Title,
		"version":    "1.0",
		"modified":   storage.Now(),
		"properties": node.Properties,
	}

	if isNew {
		metadata["created"] = storage.Now()
	} else {
		// For updates, preserve existing created time if possible
		if oldData, err := os.ReadFile(metadataFile); err == nil { //nolint:gosec // G304: metadataFile is constructed from validated id
			var oldMetadata map[string]any
			if err := json.Unmarshal(oldData, &oldMetadata); err == nil {
				if created, ok := oldMetadata["created"]; ok {
					metadata["created"] = created
				}
			}
		}
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Check storage quota for new or updated metadata
	if isNew {
		if err := ws.checkStorageQuota(int64(len(data))); err != nil {
			return err
		}
	} else {
		// For updates, check quota for additional bytes only
		oldData, err := os.ReadFile(metadataFile) //nolint:gosec // G304: metadataFile is constructed from validated id
		if err == nil {
			additionalBytes := int64(len(data)) - int64(len(oldData))
			if additionalBytes > 0 {
				if err := ws.checkStorageQuota(additionalBytes); err != nil {
					return err
				}
			}
		}
	}

	if err := os.MkdirAll(ws.pageDir(node.ID, parentID), 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(metadataFile, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	return nil
}

// DeleteTable deletes a table and commits to git.
func (ws *WorkspaceFileStore) DeleteTable(ctx context.Context, id jsonldb.ID, author git.Author) error {
	parentID := ws.getParent(id)
	return ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := ws.deleteTable(id); err != nil {
			return "", nil, err
		}
		files := []string{ws.gitPath(parentID, id, "metadata.json")}
		return "delete: table " + id.String(), files, nil
	})
}

// deleteTable deletes table metadata without committing.
func (ws *WorkspaceFileStore) deleteTable(id jsonldb.ID) error {
	parentID := ws.getParent(id)
	metadataFile := ws.tableMetadataFile(id, parentID)
	if err := os.Remove(metadataFile); err != nil {
		if os.IsNotExist(err) {
			return errTableNotFound
		}
		return fmt.Errorf("failed to delete table metadata: %w", err)
	}
	return nil
}

// IterTables returns an iterator over all tables for the workspace as Nodes.
// Recursively traverses the directory tree to include child tables under parents.
func (ws *WorkspaceFileStore) IterTables() (iter.Seq[*Node], error) {
	return func(yield func(*Node) bool) {
		ws.iterTablesRecursive(ws.wsDir, 0, yield)
	}, nil
}

// iterTablesRecursive recursively yields tables from a directory and its subdirectories.
func (ws *WorkspaceFileStore) iterTablesRecursive(dir string, parentID jsonldb.ID, yield func(*Node) bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id, err := jsonldb.DecodeID(entry.Name())
		if err != nil {
			continue
		}
		metadataFile := ws.tableMetadataFile(id, parentID)
		if _, err := os.Stat(metadataFile); err == nil {
			if node, err := ws.ReadTable(id); err == nil {
				if !yield(node) {
					return
				}
			}
		}
		// Recursively yield children
		ws.iterTablesRecursive(filepath.Join(dir, entry.Name()), id, yield)
	}
}

// CheckTableQuota checks if the workspace has reached its table limit.
// Returns ErrTableQuotaExceeded if the limit is reached.
func (ws *WorkspaceFileStore) CheckTableQuota(maxTables int) error {
	tables, err := ws.IterTables()
	if err != nil {
		return err
	}
	count := 0
	for range tables {
		count++
		if count >= maxTables {
			return ErrTableQuotaExceeded
		}
	}
	return nil
}

// AppendRecord appends a record to a table and commits to git.
func (ws *WorkspaceFileStore) AppendRecord(ctx context.Context, tableID jsonldb.ID, record *DataRecord, author git.Author) error {
	parentID := ws.getParent(tableID)
	return ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := ws.appendRecord(tableID, parentID, record); err != nil {
			return "", nil, err
		}
		files := []string{ws.gitPath(parentID, tableID, "data.jsonl")}
		return "create: record " + record.ID.String(), files, nil
	})
}

// appendRecord appends a record to a table without committing.
func (ws *WorkspaceFileStore) appendRecord(tableID, tableParentID jsonldb.ID, record *DataRecord) error {
	recordsFile := ws.tableRecordsFile(tableID, tableParentID)

	// Check max records per table
	table, err := jsonldb.NewTable[*DataRecord](recordsFile)
	// If file doesn't exist, we create it, so no error is fine if IsNotExist
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to open table: %w", err)
	}

	if table != nil {
		if table.Len() >= ws.quotas.MaxRecordsPerTable {
			return fmt.Errorf("record quota exceeded: max %d", ws.quotas.MaxRecordsPerTable)
		}
	} else {
		// New table
		table, err = jsonldb.NewTable[*DataRecord](recordsFile)
		if err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	// Calculate size for storage quota
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}
	if err := ws.checkStorageQuota(int64(len(data))); err != nil {
		return err
	}

	if err := table.Append(record); err != nil {
		return fmt.Errorf("failed to append record: %w", err)
	}
	return nil
}

// IterRecords iterates over all records in a table.
func (ws *WorkspaceFileStore) IterRecords(id jsonldb.ID) (iter.Seq[*DataRecord], error) {
	parentID := ws.getParent(id)
	filePath := ws.tableRecordsFile(id, parentID)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return func(yield func(*DataRecord) bool) {}, nil
	}

	table, err := jsonldb.NewTable[*DataRecord](filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read records: %w", err)
	}

	return table.Iter(0), nil
}

// CountRecords returns the number of records in a table.
func (ws *WorkspaceFileStore) CountRecords(id jsonldb.ID) (int, error) {
	parentID := ws.getParent(id)
	filePath := ws.tableRecordsFile(id, parentID)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return 0, nil
	}

	table, err := jsonldb.NewTable[*DataRecord](filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to read records: %w", err)
	}

	return table.Len(), nil
}

// ReadRecordsPage reads a page of records for a table using jsonldb abstraction.
func (ws *WorkspaceFileStore) ReadRecordsPage(id jsonldb.ID, offset, limit int) ([]*DataRecord, error) {
	parentID := ws.getParent(id)
	filePath := ws.tableRecordsFile(id, parentID)

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

// UpdateRecord updates a record in a table and commits to git.
func (ws *WorkspaceFileStore) UpdateRecord(ctx context.Context, tableID jsonldb.ID, record *DataRecord, author git.Author) error {
	parentID := ws.getParent(tableID)
	return ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := ws.updateRecord(tableID, parentID, record); err != nil {
			return "", nil, err
		}
		files := []string{ws.gitPath(parentID, tableID, "data.jsonl")}
		return "update: record " + record.ID.String(), files, nil
	})
}

// updateRecord updates a record in a table without committing.
func (ws *WorkspaceFileStore) updateRecord(tableID, tableParentID jsonldb.ID, record *DataRecord) error {
	recordsFile := ws.tableRecordsFile(tableID, tableParentID)

	table, err := jsonldb.NewTable[*DataRecord](recordsFile)
	if err != nil {
		return fmt.Errorf("failed to open table: %w", err)
	}

	_, err = table.Update(record)
	if err != nil {
		return fmt.Errorf("failed to update record: %w", err)
	}
	return nil
}

// DeleteRecord deletes a record and commits to git.
func (ws *WorkspaceFileStore) DeleteRecord(ctx context.Context, tableID, recordID jsonldb.ID, author git.Author) error {
	parentID := ws.getParent(tableID)
	return ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := ws.deleteRecord(tableID, parentID, recordID); err != nil {
			return "", nil, err
		}
		files := []string{ws.gitPath(parentID, tableID, "data.jsonl")}
		return "delete: record " + recordID.String(), files, nil
	})
}

// deleteRecord deletes a record without committing.
func (ws *WorkspaceFileStore) deleteRecord(tableID, tableParentID, recordID jsonldb.ID) error {
	recordsFile := ws.tableRecordsFile(tableID, tableParentID)

	table, err := jsonldb.NewTable[*DataRecord](recordsFile)
	if err != nil {
		return fmt.Errorf("failed to open table: %w", err)
	}

	_, err = table.Delete(recordID)
	if err != nil {
		return fmt.Errorf("failed to delete record: %w", err)
	}
	return nil
}

// SaveAsset saves an asset and commits to git.
func (ws *WorkspaceFileStore) SaveAsset(ctx context.Context, nodeID jsonldb.ID, assetName string, data []byte, author git.Author) (*Asset, error) {
	parentID := ws.getParent(nodeID)
	var asset *Asset
	err := ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		var err error
		asset, err = ws.saveAsset(nodeID, parentID, assetName, data)
		if err != nil {
			return "", nil, err
		}
		files := []string{ws.gitPath(parentID, nodeID, assetName)}
		return "create: asset " + assetName, files, nil
	})
	return asset, err
}

// saveAsset saves an asset without committing.
func (ws *WorkspaceFileStore) saveAsset(nodeID, parentID jsonldb.ID, assetName string, data []byte) (*Asset, error) {
	if err := ws.checkStorageQuota(int64(len(data))); err != nil {
		return nil, err
	}

	dir := ws.pageDir(nodeID, parentID)
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := filepath.Join(dir, assetName)
	if err := os.WriteFile(filePath, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return nil, fmt.Errorf("failed to write asset: %w", err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat asset: %w", err)
	}

	return &Asset{
		ID:       assetName,
		Name:     assetName,
		MimeType: mime.TypeByExtension(filepath.Ext(assetName)),
		Size:     info.Size(),
		Created:  storage.ToTime(info.ModTime()),
		Path:     filePath,
	}, nil
}

// ReadAsset reads an asset.
func (ws *WorkspaceFileStore) ReadAsset(nodeID jsonldb.ID, assetName string) ([]byte, error) {
	parentID := ws.getParent(nodeID)
	filePath := filepath.Join(ws.pageDir(nodeID, parentID), assetName)

	data, err := os.ReadFile(filePath) //nolint:gosec // G304: filePath is constructed from validated ids
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errAssetNotFound
		}
		return nil, fmt.Errorf("failed to read asset: %w", err)
	}
	return data, nil
}

// DeleteAsset deletes an asset and commits to git.
func (ws *WorkspaceFileStore) DeleteAsset(ctx context.Context, nodeID jsonldb.ID, assetName string, author git.Author) error {
	parentID := ws.getParent(nodeID)
	return ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := ws.deleteAsset(nodeID, parentID, assetName); err != nil {
			return "", nil, err
		}
		files := []string{ws.gitPath(parentID, nodeID, assetName)}
		return "delete: asset " + assetName, files, nil
	})
}

// deleteAsset deletes an asset without committing.
func (ws *WorkspaceFileStore) deleteAsset(nodeID, parentID jsonldb.ID, assetName string) error {
	filePath := filepath.Join(ws.pageDir(nodeID, parentID), assetName)

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return errAssetNotFound
		}
		return fmt.Errorf("failed to delete asset: %w", err)
	}
	return nil
}

// IterAssets returns an iterator over all assets for a page.
func (ws *WorkspaceFileStore) IterAssets(nodeID jsonldb.ID) (iter.Seq[*Asset], error) {
	parentID := ws.getParent(nodeID)
	dir := ws.pageDir(nodeID, parentID)

	// Check if directory exists
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			// Page directory doesn't exist, so no assets
			return func(yield func(*Asset) bool) {}, nil
		}
		return nil, fmt.Errorf("failed to list assets: %w", err)
	}

	return func(yield func(*Asset) bool) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}

		for _, entry := range entries {
			if entry.IsDir() || entry.Name() == "index.md" || entry.Name() == "metadata.json" || entry.Name() == "data.jsonl" || strings.HasSuffix(entry.Name(), ".blobs") {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			asset := &Asset{
				ID:       entry.Name(),
				Name:     entry.Name(),
				MimeType: mime.TypeByExtension(filepath.Ext(entry.Name())),
				Size:     info.Size(),
				Created:  storage.ToTime(info.ModTime()),
				Path:     filepath.Join(dir, entry.Name()),
			}

			if !yield(asset) {
				return
			}
		}
	}, nil
}

// History operations

// GetHistory returns the commit history for a node, limited to n commits.
// n is capped at 1000. If n <= 0, defaults to 1000.
func (ws *WorkspaceFileStore) GetHistory(ctx context.Context, id jsonldb.ID, n int) ([]*git.Commit, error) {
	if id.IsZero() {
		return nil, errPageNotFound // No node with ID 0 exists
	}
	// Use the full relative path including parent chain, otherwise nested nodes won't find their history.
	parentID := ws.getParent(id)
	path := ws.relativeDir(id, parentID)
	return ws.repo.GetHistory(ctx, path, n)
}

// GetFileAtCommit returns the content of a file at a specific commit.
func (ws *WorkspaceFileStore) GetFileAtCommit(ctx context.Context, hash, path string) ([]byte, error) {
	return ws.repo.GetFileAtCommit(ctx, hash, path)
}

// CreateNode creates a new node (can be document, table, or hybrid) and commits to git.
// If parentID is zero, creates a top-level node in the workspace.
// Otherwise, creates a child under the specified parent node.
func (ws *WorkspaceFileStore) CreateNode(ctx context.Context, title string, nodeType NodeType, parentID jsonldb.ID, author git.Author) (*Node, error) {
	// Verify parent exists if specified.
	if !parentID.IsZero() && !ws.PageExists(parentID) && !ws.TableExists(parentID) {
		return nil, fmt.Errorf("parent node not found: %w", errPageNotFound)
	}

	var node *Node
	err := ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		var files []string
		var err error
		node, files, err = ws.createNode(title, nodeType, parentID)
		if err != nil {
			return "", nil, err
		}
		msg := "create: " + string(nodeType) + " " + node.ID.String() + " - " + title
		if !parentID.IsZero() {
			msg += " (parent: " + parentID.String() + ")"
		}
		return msg, files, nil
	})
	return node, err
}

// createNode creates a new node without committing.
// If parentID is zero, the node is created at the root level.
// Otherwise, it is created under the parent directory.
func (ws *WorkspaceFileStore) createNode(title string, nodeType NodeType, parentID jsonldb.ID) (*Node, []string, error) {
	if err := ws.checkPageQuota(); err != nil {
		return nil, nil, err
	}

	id := jsonldb.NewID()
	now := storage.Now()

	node := &Node{
		ID:       id,
		ParentID: parentID,
		Title:    title,
		Type:     nodeType,
		Created:  now,
		Modified: now,
	}

	// Calculate total storage needed before writing
	var totalSize int64
	var pageData []byte
	var metadataData []byte

	if nodeType == NodeTypeDocument || nodeType == NodeTypeHybrid {
		p := &page{
			title:    title,
			content:  "",
			created:  now,
			modified: now,
		}
		pageData = formatMarkdownFile(p)
		totalSize += int64(len(pageData))
	}

	if nodeType == NodeTypeTable || nodeType == NodeTypeHybrid {
		metadata := map[string]any{
			"title":      title,
			"version":    "1.0",
			"created":    now,
			"modified":   now,
			"properties": []Property{},
		}
		var err error
		metadataData, err = json.Marshal(metadata)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		totalSize += int64(len(metadataData))
	}

	// Check storage quota before writing any files
	if err := ws.checkStorageQuota(totalSize); err != nil {
		return nil, nil, err
	}

	var files []string

	if nodeType == NodeTypeDocument || nodeType == NodeTypeHybrid {
		pageDir := ws.pageDir(id, parentID)
		if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}

		filePath := ws.pageIndexFile(id, parentID)
		if err := os.WriteFile(filePath, pageData, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
			return nil, nil, fmt.Errorf("failed to write page: %w", err)
		}
		files = append(files, ws.gitPath(parentID, id, "index.md"))
	}

	if nodeType == NodeTypeTable || nodeType == NodeTypeHybrid {
		if err := os.MkdirAll(ws.pageDir(id, parentID), 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}

		metadataFile := ws.tableMetadataFile(id, parentID)
		if err := os.WriteFile(metadataFile, metadataData, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
			return nil, nil, fmt.Errorf("failed to write metadata: %w", err)
		}
		files = append(files, ws.gitPath(parentID, id, "metadata.json"))
	}

	ws.setParent(id, parentID)

	return node, files, nil
}

// Repo returns the git.Repo for the workspace. This is exported for handlers
// that need direct git operations (e.g., git remotes).
func (ws *WorkspaceFileStore) Repo() *git.Repo {
	return ws.repo
}

// HasPage checks if a node has page content (index.md exists).
func (ws *WorkspaceFileStore) HasPage(id jsonldb.ID) bool {
	parentID := ws.getParent(id)
	filePath := ws.pageIndexFile(id, parentID)
	_, err := os.Stat(filePath)
	return err == nil
}

// HasTable checks if a node has table content (metadata.json exists).
func (ws *WorkspaceFileStore) HasTable(id jsonldb.ID) bool {
	parentID := ws.getParent(id)
	metadataFile := ws.tableMetadataFile(id, parentID)
	_, err := os.Stat(metadataFile)
	return err == nil
}

// --- Page-specific operations ---

// CreatePageUnderParent creates a new page under a parent node and commits to git.
// If parentID is zero, creates a top-level page in the workspace.
// Otherwise, creates a child under the specified parent node.
// Returns the new node with the page content.
func (ws *WorkspaceFileStore) CreatePageUnderParent(ctx context.Context, parentID jsonldb.ID, title, content string, author git.Author) (*Node, error) {
	// Verify parent exists if specified.
	if !parentID.IsZero() && !ws.PageExists(parentID) && !ws.TableExists(parentID) {
		return nil, fmt.Errorf("parent node not found: %w", errPageNotFound)
	}

	if err := ws.checkPageQuota(); err != nil {
		return nil, err
	}

	var node *Node
	err := ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		id := jsonldb.NewID()
		now := storage.Now()

		p := &page{
			title:    title,
			content:  content,
			created:  now,
			modified: now,
		}
		pageData := formatMarkdownFile(p)

		if err := ws.checkStorageQuota(int64(len(pageData))); err != nil {
			return "", nil, err
		}

		pageDir := ws.pageDir(id, parentID)
		if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional
			return "", nil, fmt.Errorf("failed to create directory: %w", err)
		}

		filePath := ws.pageIndexFile(id, parentID)
		if err := os.WriteFile(filePath, pageData, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional
			return "", nil, fmt.Errorf("failed to write page: %w", err)
		}

		ws.setParent(id, parentID)

		node = &Node{
			ID:       id,
			ParentID: parentID,
			Title:    title,
			Content:  content,
			Type:     NodeTypeDocument,
			Created:  now,
			Modified: now,
		}

		files := []string{ws.gitPath(parentID, id, "index.md")}
		msg := "create: page " + id.String() + " - " + title + " (parent: " + parentID.String() + ")"
		return msg, files, nil
	})
	return node, err
}

// DeletePageFromNode removes the page content (index.md) from a node.
// The node directory is kept if table content exists.
func (ws *WorkspaceFileStore) DeletePageFromNode(ctx context.Context, id jsonldb.ID, author git.Author) error {
	if id.IsZero() {
		return errPageNotFound // No node with ID 0 exists
	}

	parentID := ws.getParent(id)
	return ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		filePath := ws.pageIndexFile(id, parentID)
		if err := os.Remove(filePath); err != nil {
			if os.IsNotExist(err) {
				return "", nil, errPageNotFound
			}
			return "", nil, fmt.Errorf("failed to delete page: %w", err)
		}

		// If no table content exists, remove the directory too
		if !ws.TableExists(id) {
			dir := ws.pageDir(id, parentID)
			// Check if directory is empty (besides the removed index.md)
			entries, _ := os.ReadDir(dir)
			if len(entries) == 0 {
				_ = os.Remove(dir)
				ws.deleteFromCache(id)
			}
		}

		files := []string{ws.gitPath(parentID, id, "index.md")}
		return "delete: page " + id.String(), files, nil
	})
}

// --- Table-specific operations ---

// CreateTableUnderParent creates a new table under a parent node and commits to git.
// Returns the new node with the table schema.
func (ws *WorkspaceFileStore) CreateTableUnderParent(ctx context.Context, parentID jsonldb.ID, title string, properties []Property, author git.Author) (*Node, error) {
	// Verify parent exists if parentID is specified (non-root)
	if !parentID.IsZero() && !ws.PageExists(parentID) && !ws.TableExists(parentID) {
		return nil, fmt.Errorf("parent node not found: %w", errPageNotFound)
	}

	if err := ws.checkPageQuota(); err != nil {
		return nil, err
	}

	var node *Node
	err := ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		id := jsonldb.NewID()
		now := storage.Now()

		metadata := map[string]any{
			"title":      title,
			"version":    "1.0",
			"created":    now,
			"modified":   now,
			"properties": properties,
		}
		metadataData, err := json.Marshal(metadata)
		if err != nil {
			return "", nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}

		if err := ws.checkStorageQuota(int64(len(metadataData))); err != nil {
			return "", nil, err
		}

		pageDir := ws.pageDir(id, parentID)
		if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional
			return "", nil, fmt.Errorf("failed to create directory: %w", err)
		}

		metadataFile := ws.tableMetadataFile(id, parentID)
		if err := os.WriteFile(metadataFile, metadataData, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional
			return "", nil, fmt.Errorf("failed to write metadata: %w", err)
		}

		ws.setParent(id, parentID)

		node = &Node{
			ID:         id,
			ParentID:   parentID,
			Title:      title,
			Type:       NodeTypeTable,
			Properties: properties,
			Created:    now,
			Modified:   now,
		}

		files := []string{ws.gitPath(parentID, id, "metadata.json")}
		msg := "create: table " + id.String() + " - " + title
		if !parentID.IsZero() {
			msg += " (parent: " + parentID.String() + ")"
		}
		return msg, files, nil
	})
	return node, err
}

// DeleteTableFromNode removes the table content (metadata.json + data.jsonl) from a node.
// The node directory is kept if page content exists.
func (ws *WorkspaceFileStore) DeleteTableFromNode(ctx context.Context, id jsonldb.ID, author git.Author) error {
	if id.IsZero() {
		return errTableNotFound // No node with ID 0 exists
	}

	parentID := ws.getParent(id)
	return ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		var files []string

		metadataFile := ws.tableMetadataFile(id, parentID)
		if err := os.Remove(metadataFile); err != nil && !os.IsNotExist(err) {
			return "", nil, fmt.Errorf("failed to delete metadata: %w", err)
		}
		files = append(files, ws.gitPath(parentID, id, "metadata.json"))

		recordsFile := ws.tableRecordsFile(id, parentID)
		if err := os.Remove(recordsFile); err != nil && !os.IsNotExist(err) {
			return "", nil, fmt.Errorf("failed to delete records: %w", err)
		}
		if _, err := os.Stat(recordsFile); err == nil {
			files = append(files, ws.gitPath(parentID, id, "data.jsonl"))
		}

		// If no page content exists, remove the directory too
		if !ws.PageExists(id) {
			dir := ws.pageDir(id, parentID)
			// Check if directory is empty
			entries, _ := os.ReadDir(dir)
			if len(entries) == 0 {
				_ = os.Remove(dir)
				ws.deleteFromCache(id)
			}
		}

		return "delete: table " + id.String(), files, nil
	})
}

// ListChildren returns all direct children of a node.
// In single-root model:
//   - If parentID is zero (root), returns children from workspace dir.
//   - Otherwise, returns children from parent's subdirectory.
func (ws *WorkspaceFileStore) ListChildren(parentID jsonldb.ID) ([]*Node, error) {
	var dir string
	if parentID.IsZero() {
		// Children of root are stored directly in workspace dir.
		dir = ws.wsDir
	} else {
		// Children of non-root nodes are in subdirectories.
		parentParentID := ws.getParent(parentID)
		dir = ws.pageDir(parentID, parentParentID)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
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

		nodePath := filepath.Join(dir, entry.Name())
		node, err := ws.ReadNodeFromPath(nodePath, id, parentID)
		if err != nil {
			continue
		}

		// Check if this child has its own children
		childEntries, _ := os.ReadDir(nodePath)
		for _, childEntry := range childEntries {
			if childEntry.IsDir() {
				if _, decErr := jsonldb.DecodeID(childEntry.Name()); decErr == nil {
					node.HasChildren = true
					break
				}
			}
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// Helper functions

func parseMarkdownFile(data []byte) *page {
	content := string(data)
	var title string
	var created, modified storage.Time

	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content, "\n---", 2)
		if len(parts) == 2 {
			frontMatter := parts[0][4:]
			content = strings.TrimLeft(parts[1], "\n")
			for _, line := range strings.Split(frontMatter, "\n") {
				switch {
				case strings.HasPrefix(line, "title:"):
					title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
				case strings.HasPrefix(line, "created:"):
					dateStr := strings.TrimSpace(strings.TrimPrefix(line, "created:"))
					if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
						created = storage.ToTime(t)
					}
				case strings.HasPrefix(line, "modified:"):
					dateStr := strings.TrimSpace(strings.TrimPrefix(line, "modified:"))
					if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
						modified = storage.ToTime(t)
					}
				}
			}
		}
	}

	if created.IsZero() {
		created = storage.Now()
	}
	if modified.IsZero() {
		modified = storage.Now()
	}

	return &page{
		title:    title,
		content:  content,
		created:  created,
		modified: modified,
	}
}

func formatMarkdownFile(p *page) []byte {
	var buf bytes.Buffer
	buf.WriteString("---")
	buf.WriteString("\ntitle: " + p.title + "\n")
	buf.WriteString("created: " + p.created.AsTime().Format(time.RFC3339) + "\n")
	buf.WriteString("modified: " + p.modified.AsTime().Format(time.RFC3339) + "\n")
	if len(p.tags) > 0 {
		buf.WriteString("tags: [" + strings.Join(p.tags, ", ") + "]\n")
	}
	buf.WriteString("---")
	buf.WriteString("\n\n")
	buf.WriteString(p.content)
	return buf.Bytes()
}
