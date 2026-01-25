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
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// FileStore is a versioned file storage system. All mutations are committed to git.
// Storage model: Each page (document or table) is an ID-based directory within a workspace.
//   - Pages: ID directory containing index.md with YAML front matter.
//   - Tables: ID directory containing metadata.json + data.jsonl.
//   - Assets: files within each page's directory namespace.
type FileStore struct {
	rootDir string
	git     *git.Manager
	wsSvc   *identity.WorkspaceService
	orgSvc  *identity.OrganizationService
}

// page is an internal type for reading/writing page markdown files.
type page struct {
	id         jsonldb.ID
	title      string
	content    string
	created    storage.Time
	modified   storage.Time
	tags       []string
	faviconURL string
}

// NewFileStore creates a versioned file store.
// gitMgr is required - all operations are versioned.
// wsSvc provides quota limits for workspaces.
// orgSvc provides quota limits for organizations.
func NewFileStore(rootDir string, gitMgr *git.Manager, wsSvc *identity.WorkspaceService, orgSvc *identity.OrganizationService) (*FileStore, error) {
	if gitMgr == nil {
		return nil, errors.New("git manager is required")
	}
	if wsSvc == nil {
		return nil, errors.New("workspace service is required")
	}
	if orgSvc == nil {
		return nil, errors.New("organization service is required")
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}

	return &FileStore{
		rootDir: rootDir,
		git:     gitMgr,
		wsSvc:   wsSvc,
		orgSvc:  orgSvc,
	}, nil
}

// InitWorkspace initializes storage for a new workspace.
// Creates the workspace directory structure and initializes git.
func (fs *FileStore) InitWorkspace(ctx context.Context, wsID jsonldb.ID) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}
	wsDir := filepath.Join(fs.rootDir, wsID.String())
	pagesDir := filepath.Join(wsDir, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return fmt.Errorf("failed to create workspace directory: %w", err)
	}
	// Getting the repo initializes git if needed
	if _, err := fs.Repo(ctx, wsID); err != nil {
		return fmt.Errorf("failed to initialize git repo for workspace %s: %w", wsID, err)
	}
	return nil
}

// checkPageQuota returns an error if creating a new page would exceed quota.
func (fs *FileStore) checkPageQuota(wsID jsonldb.ID) error {
	ws, err := fs.wsSvc.Get(wsID)
	if err != nil {
		return err
	}
	count, _, err := fs.GetWorkspaceUsage(wsID)
	if err != nil {
		return err
	}
	if count >= ws.Quotas.MaxPages {
		return errQuotaExceeded
	}
	return nil
}

// checkStorageQuota returns an error if adding the given bytes would exceed workspace or organization storage quota.
func (fs *FileStore) checkStorageQuota(wsID jsonldb.ID, additionalBytes int64) error {
	// Check workspace-level storage quota
	ws, err := fs.wsSvc.Get(wsID)
	if err != nil {
		return err
	}
	_, usage, err := fs.GetWorkspaceUsage(wsID)
	if err != nil {
		return err
	}
	maxStorageBytes := int64(ws.Quotas.MaxStorageMB) * 1024 * 1024
	if usage+additionalBytes > maxStorageBytes {
		return errQuotaExceeded
	}

	// Check organization-level storage quota
	if err := fs.checkOrgStorageQuota(wsID, additionalBytes); err != nil {
		return err
	}

	return nil
}

// checkRecordQuota returns an error if adding a new record would exceed the table's record quota.
func (fs *FileStore) checkRecordQuota(wsID jsonldb.ID, currentCount int) error {
	ws, err := fs.wsSvc.Get(wsID)
	if err != nil {
		return err
	}
	if currentCount >= ws.Quotas.MaxRecordsPerTable {
		return errQuotaExceeded
	}
	return nil
}

// checkAssetSizeQuota returns an error if the given size exceeds the workspace's single asset size quota.
func (fs *FileStore) checkAssetSizeQuota(wsID jsonldb.ID, size int64) error {
	ws, err := fs.wsSvc.Get(wsID)
	if err != nil {
		return err
	}
	maxAssetSizeBytes := int64(ws.Quotas.MaxAssetSizeMB) * 1024 * 1024
	if size > maxAssetSizeBytes {
		return errQuotaExceeded
	}
	return nil
}

// checkOrgStorageQuota returns an error if adding the given bytes would exceed the organization's total storage quota.
// This checks the sum of storage usage across all workspaces in the organization.
func (fs *FileStore) checkOrgStorageQuota(wsID jsonldb.ID, additionalBytes int64) error {
	ws, err := fs.wsSvc.Get(wsID)
	if err != nil {
		return err
	}
	org, err := fs.orgSvc.Get(ws.OrganizationID)
	if err != nil {
		return err
	}

	orgUsage, err := fs.GetOrganizationUsage(ws.OrganizationID)
	if err != nil {
		return err
	}

	maxOrgStorageBytes := int64(org.Quotas.MaxTotalStorageGB) * 1024 * 1024 * 1024
	if orgUsage+additionalBytes > maxOrgStorageBytes {
		return errQuotaExceeded
	}
	return nil
}

func (fs *FileStore) wsPagesDir(wsID jsonldb.ID) string {
	if wsID.IsZero() {
		return ""
	}
	dir := filepath.Join(fs.rootDir, wsID.String(), "pages")
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		slog.Error("Failed to create organization pages directory", "dir", dir, "error", err)
	}
	return dir
}

// PageExists checks if a page directory exists.
func (fs *FileStore) PageExists(wsID, id jsonldb.ID) bool {
	if wsID.IsZero() {
		return false
	}
	path := fs.pageDir(wsID, id)
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ReadPage reads a page from disk and returns it as a Node.
func (fs *FileStore) ReadPage(wsID, id jsonldb.ID) (*Node, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}
	filePath := fs.pageIndexFile(wsID, id)
	data, err := os.ReadFile(filePath) //nolint:gosec // G304: filePath is constructed from validated wsID and id
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
func (fs *FileStore) WritePage(ctx context.Context, wsID, id jsonldb.ID, title, content string, author git.Author) (*Node, error) {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return nil, err
	}

	var node *Node
	err = repo.CommitTx(ctx, author, func() (string, []string, error) {
		var err error
		node, err = fs.writePage(wsID, id, title, content)
		if err != nil {
			return "", nil, err
		}
		msg := "create: page " + id.String() + " - " + title
		files := []string{"pages/" + id.String() + "/index.md"}
		return msg, files, nil
	})
	return node, err
}

// writePage creates a new page on disk without committing.
func (fs *FileStore) writePage(wsID, id jsonldb.ID, title, content string) (*Node, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}
	if err := fs.checkPageQuota(wsID); err != nil {
		return nil, err
	}

	now := storage.Now()
	p := &page{
		id:       id,
		title:    title,
		content:  content,
		created:  now,
		modified: now,
	}

	data := formatMarkdownFile(p)
	if err := fs.checkStorageQuota(wsID, int64(len(data))); err != nil {
		return nil, err
	}

	pageDir := fs.pageDir(wsID, id)
	if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := fs.pageIndexFile(wsID, id)
	if err := os.WriteFile(filePath, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return nil, fmt.Errorf("failed to write page: %w", err)
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
func (fs *FileStore) UpdatePage(ctx context.Context, wsID, id jsonldb.ID, title, content string, author git.Author) (*Node, error) {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return nil, err
	}

	var node *Node
	err = repo.CommitTx(ctx, author, func() (string, []string, error) {
		var err error
		node, err = fs.updatePage(wsID, id, title, content)
		if err != nil {
			return "", nil, err
		}
		msg := "update: page " + id.String()
		files := []string{"pages/" + id.String() + "/index.md"}
		return msg, files, nil
	})
	return node, err
}

// updatePage updates an existing page on disk without committing.
func (fs *FileStore) updatePage(wsID, id jsonldb.ID, title, content string) (*Node, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}
	filePath := fs.pageIndexFile(wsID, id)
	data, err := os.ReadFile(filePath) //nolint:gosec // G304: filePath is constructed from validated wsID and id
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errPageNotFound
		}
		return nil, fmt.Errorf("failed to read page: %w", err)
	}

	p := parseMarkdownFile(id, data)
	p.title = title
	p.content = content
	p.modified = storage.Now()

	updatedData := formatMarkdownFile(p)

	// Check storage quota for the additional bytes (new size - old size)
	additionalBytes := int64(len(updatedData)) - int64(len(data))
	if additionalBytes > 0 {
		if err := fs.checkStorageQuota(wsID, additionalBytes); err != nil {
			return nil, err
		}
	}

	if err := os.WriteFile(filePath, updatedData, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return nil, fmt.Errorf("failed to write page: %w", err)
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
func (fs *FileStore) DeletePage(ctx context.Context, wsID, id jsonldb.ID, author git.Author) error {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return err
	}

	return repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := fs.deletePage(wsID, id); err != nil {
			return "", nil, err
		}
		msg := "delete: page " + id.String()
		files := []string{"pages/" + id.String()}
		return msg, files, nil
	})
}

// deletePage deletes a page directory without committing.
func (fs *FileStore) deletePage(wsID, id jsonldb.ID) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}
	pageDir := fs.pageDir(wsID, id)
	if err := os.RemoveAll(pageDir); err != nil {
		if os.IsNotExist(err) {
			return errPageNotFound
		}
		return fmt.Errorf("failed to delete page: %w", err)
	}
	return nil
}

// IterPages returns an iterator over all pages for an organization as Nodes.
func (fs *FileStore) IterPages(wsID jsonldb.ID) (iter.Seq[*Node], error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}
	dir := fs.wsPagesDir(wsID)

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
			indexFile := fs.pageIndexFile(wsID, id)
			if _, err := os.Stat(indexFile); err == nil {
				if node, err := fs.ReadPage(wsID, id); err == nil {
					if !yield(node) {
						return
					}
				}
			}
		}
	}, nil
}

// ReadNode reads a unified node from disk.
func (fs *FileStore) ReadNode(wsID, id jsonldb.ID) (*Node, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}
	nodeDir := fs.pageDir(wsID, id)
	info, err := os.Stat(nodeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errNodeNotFound
		}
		return nil, fmt.Errorf("failed to access node: %w", err)
	}

	node := &Node{
		ID:       id,
		Created:  storage.ToTime(info.ModTime()),
		Modified: storage.ToTime(info.ModTime()),
	}

	indexFile := fs.pageIndexFile(wsID, id)
	if _, err := os.Stat(indexFile); err == nil {
		page, err := fs.ReadPage(wsID, id)
		if err == nil {
			node.Title = page.Title
			node.Content = page.Content
			node.Created = page.Created
			node.Modified = page.Modified
			node.Tags = page.Tags
			node.Type = NodeTypeDocument
		}
	}

	metadataFile := fs.tableMetadataFile(wsID, id)
	if _, err := os.Stat(metadataFile); err == nil {
		table, err := fs.ReadTable(wsID, id)
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
func (fs *FileStore) ReadNodeTree(wsID jsonldb.ID) ([]*Node, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}
	return fs.readNodesRecursive(wsID, fs.wsPagesDir(wsID), 0)
}

func (fs *FileStore) readNodesRecursive(wsID jsonldb.ID, dir string, parentID jsonldb.ID) ([]*Node, error) {
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

		node, err := fs.ReadNodeFromPath(wsID, filepath.Join(dir, entry.Name()), id, parentID)
		if err != nil {
			continue
		}

		children, _ := fs.readNodesRecursive(wsID, filepath.Join(dir, entry.Name()), id)
		node.Children = children

		nodes = append(nodes, node)
	}
	return nodes, nil
}

// ReadNodeFromPath reads a node from a specific path.
func (fs *FileStore) ReadNodeFromPath(wsID jsonldb.ID, path string, id, parentID jsonldb.ID) (*Node, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	node := &Node{
		ID:       id,
		ParentID: parentID,
		Created:  storage.ToTime(info.ModTime()),
		Modified: storage.ToTime(info.ModTime()),
	}

	indexFile := filepath.Join(path, "index.md")
	if _, err := os.Stat(indexFile); err == nil {
		page, err := fs.ReadPage(wsID, id)
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
		table, err := fs.ReadTable(wsID, id)
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

// GetWorkspaceUsage calculates the total number of pages and storage usage (in bytes) for a workspace.
// Pages are counted as directories containing index.md (documents) or metadata.json (tables).
// Hybrid nodes (both files) are counted once.
func (fs *FileStore) GetWorkspaceUsage(wsID jsonldb.ID) (pageCount int, storageUsage int64, err error) {
	if wsID.IsZero() {
		return 0, 0, errWSIDRequired
	}
	dir := fs.wsPagesDir(wsID)
	counted := make(map[string]bool)
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			storageUsage += info.Size()
			// Count directories containing index.md or metadata.json as pages/tables
			if info.Name() == "index.md" || info.Name() == "metadata.json" {
				parentDir := filepath.Dir(path)
				if !counted[parentDir] {
					counted[parentDir] = true
					pageCount++
				}
			}
		}
		return nil
	})
	return
}

// GetOrganizationUsage calculates the total storage usage (in bytes) across all workspaces in an organization.
// This iterates through all workspaces belonging to the organization and sums their storage usage.
func (fs *FileStore) GetOrganizationUsage(orgID jsonldb.ID) (int64, error) {
	if orgID.IsZero() {
		return 0, errOrgIDRequired
	}

	var totalUsage int64
	for ws := range fs.wsSvc.IterByOrg(orgID) {
		_, usage, err := fs.GetWorkspaceUsage(ws.ID)
		if err != nil {
			return 0, fmt.Errorf("failed to get workspace usage for %s: %w", ws.ID, err)
		}
		totalUsage += usage
	}
	return totalUsage, nil
}

func (fs *FileStore) pageDir(wsID, id jsonldb.ID) string {
	return filepath.Join(fs.wsPagesDir(wsID), id.String())
}

func (fs *FileStore) pageIndexFile(wsID, id jsonldb.ID) string {
	return filepath.Join(fs.pageDir(wsID, id), "index.md")
}

// Table operations

// TableExists checks if a table exists for the given organization and ID.
func (fs *FileStore) TableExists(wsID, id jsonldb.ID) bool {
	path := fs.tableMetadataFile(wsID, id)
	_, err := os.Stat(path)
	return err == nil
}

// ReadTable reads a table definition from metadata.json and returns it as a Node.
func (fs *FileStore) ReadTable(wsID, id jsonldb.ID) (*Node, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}

	metadataFile := fs.tableMetadataFile(wsID, id)
	data, err := os.ReadFile(metadataFile) //nolint:gosec // G304: metadataFile is constructed from validated wsID and id
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errTableNotFound
		}
		return nil, fmt.Errorf("failed to read table metadata: %w", err)
	}

	var metadata struct {
		Title      string       `json:"title"`
		Version    string       `json:"version"`
		Created    storage.Time `json:"created"`
		Modified   storage.Time `json:"modified"`
		Properties []Property   `json:"properties"`
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
func (fs *FileStore) WriteTable(ctx context.Context, wsID jsonldb.ID, node *Node, isNew bool, author git.Author) error {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return err
	}

	return repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := fs.writeTable(wsID, node, isNew); err != nil {
			return "", nil, err
		}
		var msg string
		if isNew {
			msg = "create: table " + node.ID.String() + " - " + node.Title
		} else {
			msg = "update: table " + node.ID.String()
		}
		files := []string{"pages/" + node.ID.String() + "/metadata.json"}
		return msg, files, nil
	})
}

// writeTable writes table metadata without committing.
func (fs *FileStore) writeTable(wsID jsonldb.ID, node *Node, isNew bool) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}
	if isNew {
		if err := fs.checkPageQuota(wsID); err != nil {
			return err
		}
	}

	// Write metadata.json with all table metadata including properties
	metadataFile := fs.tableMetadataFile(wsID, node.ID)
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
		if err := fs.checkStorageQuota(wsID, int64(len(data))); err != nil {
			return err
		}
	} else {
		// For updates, check quota for additional bytes only
		oldData, err := os.ReadFile(metadataFile) //nolint:gosec // G304: metadataFile is constructed from validated wsID and id
		if err == nil {
			additionalBytes := int64(len(data)) - int64(len(oldData))
			if additionalBytes > 0 {
				if err := fs.checkStorageQuota(wsID, additionalBytes); err != nil {
					return err
				}
			}
		}
	}

	if err := os.MkdirAll(fs.pageDir(wsID, node.ID), 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(metadataFile, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	return nil
}

// DeleteTable deletes a table and all its records, commits to git.
func (fs *FileStore) DeleteTable(ctx context.Context, wsID, id jsonldb.ID, author git.Author) error {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return err
	}

	return repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := fs.deleteTable(wsID, id); err != nil {
			return "", nil, err
		}
		msg := "delete: table " + id.String()
		files := []string{"pages/" + id.String()}
		return msg, files, nil
	})
}

// deleteTable deletes a table directory without committing.
func (fs *FileStore) deleteTable(wsID, id jsonldb.ID) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}
	pageDir := fs.pageDir(wsID, id)
	if err := os.RemoveAll(pageDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete table: %w", err)
	}
	return nil
}

// IterTables returns an iterator over all tables for the given organization as Nodes.
func (fs *FileStore) IterTables(wsID jsonldb.ID) (iter.Seq[*Node], error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}
	dir := fs.wsPagesDir(wsID)

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
			metadataFile := fs.tableMetadataFile(wsID, id)
			if _, err := os.Stat(metadataFile); err == nil {
				if node, err := fs.ReadTable(wsID, id); err == nil {
					if !yield(node) {
						return
					}
				}
			}
		}
	}, nil
}

// AppendRecord appends a record to a table and commits to git.
func (fs *FileStore) AppendRecord(ctx context.Context, wsID, tableID jsonldb.ID, record *DataRecord, author git.Author) error {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return err
	}

	return repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := fs.appendRecord(wsID, tableID, record); err != nil {
			return "", nil, err
		}
		msg := "create: record " + record.ID.String() + " in table " + tableID.String()
		files := []string{"pages/" + tableID.String() + "/data.jsonl"}
		return msg, files, nil
	})
}

// appendRecord appends a record without committing.
func (fs *FileStore) appendRecord(wsID, tableID jsonldb.ID, record *DataRecord) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}

	// Estimate storage impact
	recordData, _ := json.Marshal(record)
	if err := fs.checkStorageQuota(wsID, int64(len(recordData)+1)); err != nil {
		return err
	}

	pageDir := fs.pageDir(wsID, tableID)
	if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := fs.tableRecordsFile(wsID, tableID)

	// Load or create table using jsonldb
	table, err := jsonldb.NewTable[*DataRecord](filePath)
	if err != nil {
		return fmt.Errorf("failed to load table: %w", err)
	}

	if err := fs.checkRecordQuota(wsID, table.Len()); err != nil {
		return err
	}

	// Append record
	if err := table.Append(record); err != nil {
		return fmt.Errorf("failed to append record: %w", err)
	}
	return nil
}

// IterRecords returns an iterator over all records for a table.
func (fs *FileStore) IterRecords(wsID, id jsonldb.ID) (iter.Seq[*DataRecord], error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}
	filePath := fs.tableRecordsFile(wsID, id)

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
func (fs *FileStore) ReadRecordsPage(wsID, id jsonldb.ID, offset, limit int) ([]*DataRecord, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}
	filePath := fs.tableRecordsFile(wsID, id)

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
func (fs *FileStore) UpdateRecord(ctx context.Context, wsID, tableID jsonldb.ID, record *DataRecord, author git.Author) error {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return err
	}

	return repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := fs.updateRecord(wsID, tableID, record); err != nil {
			return "", nil, err
		}
		msg := "update: record " + record.ID.String() + " in table " + tableID.String()
		files := []string{"pages/" + tableID.String() + "/data.jsonl"}
		return msg, files, nil
	})
}

// updateRecord updates an existing record without committing.
func (fs *FileStore) updateRecord(wsID, tableID jsonldb.ID, record *DataRecord) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}

	filePath := fs.tableRecordsFile(wsID, tableID)

	// Load using jsonldb abstraction
	table, err := jsonldb.NewTable[*DataRecord](filePath)
	if err != nil {
		return fmt.Errorf("failed to load table: %w", err)
	}

	// Get the old record to calculate size delta
	oldRecord := table.Get(record.ID)
	if oldRecord == nil {
		return errRecordNotFound
	}

	// Check storage quota for additional bytes only
	newData, _ := json.Marshal(record)
	oldData, _ := json.Marshal(oldRecord)
	additionalBytes := int64(len(newData)) - int64(len(oldData))
	if additionalBytes > 0 {
		if err := fs.checkStorageQuota(wsID, additionalBytes); err != nil {
			return err
		}
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

// DeleteRecord deletes a record from a table and commits to git.
func (fs *FileStore) DeleteRecord(ctx context.Context, wsID, tableID, recordID jsonldb.ID, author git.Author) error {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return err
	}

	return repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := fs.deleteRecord(wsID, tableID, recordID); err != nil {
			return "", nil, err
		}
		msg := "delete: record " + recordID.String() + " from table " + tableID.String()
		files := []string{"pages/" + tableID.String() + "/data.jsonl"}
		return msg, files, nil
	})
}

// deleteRecord deletes a record without committing.
func (fs *FileStore) deleteRecord(wsID, tableID, recordID jsonldb.ID) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}

	filePath := fs.tableRecordsFile(wsID, tableID)

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
	return nil
}

func (fs *FileStore) tableRecordsFile(wsID, id jsonldb.ID) string {
	return filepath.Join(fs.pageDir(wsID, id), "data.jsonl")
}

func (fs *FileStore) tableMetadataFile(wsID, id jsonldb.ID) string {
	return filepath.Join(fs.pageDir(wsID, id), "metadata.json")
}

// Asset operations

// SaveAsset saves an asset associated with a page and commits to git.
func (fs *FileStore) SaveAsset(ctx context.Context, wsID, pageID jsonldb.ID, assetName string, data []byte, author git.Author) (*Asset, error) {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return nil, err
	}

	var asset *Asset
	err = repo.CommitTx(ctx, author, func() (string, []string, error) {
		var err error
		asset, err = fs.saveAsset(wsID, pageID, assetName, data)
		if err != nil {
			return "", nil, err
		}
		msg := "create: asset " + assetName + " in page " + pageID.String()
		files := []string{"pages/" + pageID.String() + "/" + assetName}
		return msg, files, nil
	})
	return asset, err
}

// saveAsset saves an asset without committing.
func (fs *FileStore) saveAsset(wsID, pageID jsonldb.ID, assetName string, data []byte) (*Asset, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}
	if err := fs.checkAssetSizeQuota(wsID, int64(len(data))); err != nil {
		return nil, err
	}
	if err := fs.checkStorageQuota(wsID, int64(len(data))); err != nil {
		return nil, err
	}

	pageDir := fs.pageDir(wsID, pageID)
	if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return nil, fmt.Errorf("failed to create page directory: %w", err)
	}

	assetPath := filepath.Join(pageDir, assetName)
	if err := os.WriteFile(assetPath, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return nil, fmt.Errorf("failed to write asset: %w", err)
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
func (fs *FileStore) ReadAsset(wsID, pageID jsonldb.ID, assetName string) ([]byte, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}
	assetPath := filepath.Join(fs.pageDir(wsID, pageID), assetName)
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
func (fs *FileStore) DeleteAsset(ctx context.Context, wsID, pageID jsonldb.ID, assetName string, author git.Author) error {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return err
	}

	return repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := fs.deleteAsset(wsID, pageID, assetName); err != nil {
			return "", nil, err
		}
		msg := "delete: asset " + assetName + " from page " + pageID.String()
		files := []string{"pages/" + pageID.String() + "/" + assetName}
		return msg, files, nil
	})
}

// deleteAsset deletes an asset without committing.
func (fs *FileStore) deleteAsset(wsID, pageID jsonldb.ID, assetName string) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}
	assetPath := filepath.Join(fs.pageDir(wsID, pageID), assetName)
	if err := os.Remove(assetPath); err != nil {
		if os.IsNotExist(err) {
			return errAssetNotFound
		}
		return fmt.Errorf("failed to delete asset: %w", err)
	}
	return nil
}

// IterAssets returns an iterator over all assets associated with a page.
func (fs *FileStore) IterAssets(wsID, pageID jsonldb.ID) (iter.Seq[*Asset], error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}
	pageDir := fs.pageDir(wsID, pageID)
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
				Created: storage.ToTime(info.ModTime()),
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
	var created, modified storage.Time

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

// History operations

// GetHistory returns the commit history for a page, limited to n commits.
// n is capped at 1000. If n <= 0, defaults to 1000.
func (fs *FileStore) GetHistory(ctx context.Context, wsID, id jsonldb.ID, n int) ([]*git.Commit, error) {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return nil, err
	}
	return repo.GetHistory(ctx, "pages/"+id.String(), n)
}

// GetFileAtCommit returns the content of a file at a specific commit.
func (fs *FileStore) GetFileAtCommit(ctx context.Context, wsID jsonldb.ID, hash, path string) ([]byte, error) {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return nil, err
	}
	return repo.GetFileAtCommit(ctx, hash, path)
}

// CreateNode creates a new node (can be document, table, or hybrid) and commits to git.
func (fs *FileStore) CreateNode(ctx context.Context, wsID jsonldb.ID, title string, nodeType NodeType, author git.Author) (*Node, error) {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return nil, err
	}

	var node *Node
	err = repo.CommitTx(ctx, author, func() (string, []string, error) {
		var files []string
		var err error
		node, files, err = fs.createNode(wsID, title, nodeType)
		if err != nil {
			return "", nil, err
		}
		msg := "create: " + string(nodeType) + " " + node.ID.String() + " - " + title
		return msg, files, nil
	})
	return node, err
}

// createNode creates a new node without committing.
func (fs *FileStore) createNode(wsID jsonldb.ID, title string, nodeType NodeType) (*Node, []string, error) {
	if err := fs.checkPageQuota(wsID); err != nil {
		return nil, nil, err
	}

	id := jsonldb.NewID()
	now := storage.Now()

	node := &Node{
		ID:       id,
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
			id:       id,
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
	if err := fs.checkStorageQuota(wsID, totalSize); err != nil {
		return nil, nil, err
	}

	var files []string

	if nodeType == NodeTypeDocument || nodeType == NodeTypeHybrid {
		pageDir := fs.pageDir(wsID, id)
		if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}

		filePath := fs.pageIndexFile(wsID, id)
		if err := os.WriteFile(filePath, pageData, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
			return nil, nil, fmt.Errorf("failed to write page: %w", err)
		}
		files = append(files, "pages/"+id.String()+"/index.md")
	}

	if nodeType == NodeTypeTable || nodeType == NodeTypeHybrid {
		if err := os.MkdirAll(fs.pageDir(wsID, id), 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}

		metadataFile := fs.tableMetadataFile(wsID, id)
		if err := os.WriteFile(metadataFile, metadataData, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
			return nil, nil, fmt.Errorf("failed to write metadata: %w", err)
		}
		files = append(files, "pages/"+id.String()+"/metadata.json")
	}

	return node, files, nil
}

// Repo returns the git.Repo for an organization. This is exported for handlers
// that need direct git operations (e.g., git remotes).
func (fs *FileStore) Repo(ctx context.Context, wsID jsonldb.ID) (*git.Repo, error) {
	return fs.git.Repo(ctx, wsID.String())
}
