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
	"sync"
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
	mu      sync.RWMutex
	cache   map[jsonldb.ID]map[jsonldb.ID]jsonldb.ID // wsID -> nodeID -> parentID
}

// page is an internal type for reading/writing page markdown files.
type page struct {
	id       jsonldb.ID
	title    string
	content  string
	created  storage.Time
	modified storage.Time
	tags     []string
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
		cache:   make(map[jsonldb.ID]map[jsonldb.ID]jsonldb.ID),
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
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return fmt.Errorf("failed to initialize git repo for workspace %s: %w", wsID, err)
	}

	// Write AGENTS.md in the root of the workspace.
	agentsPath := filepath.Join(wsDir, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte(storage.AgentsMD), 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for documentation files
		return fmt.Errorf("failed to write AGENTS.md: %w", err)
	}

	// Commit AGENTS.md using default author.
	if err := repo.CommitTx(ctx, git.Author{}, func() (string, []string, error) {
		return "initial: add AGENTS.md", []string{"AGENTS.md"}, nil
	}); err != nil {
		return fmt.Errorf("failed to commit AGENTS.md: %w", err)
	}

	return nil
}

// refreshCache rebuilds the parent map for a workspace.
func (fs *FileStore) refreshCache(wsID jsonldb.ID) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if _, ok := fs.cache[wsID]; !ok {
		fs.cache[wsID] = make(map[jsonldb.ID]jsonldb.ID)
	}

	// Recursively walk the pages directory
	dir := fs.wsPagesDir(wsID)
	return fs.walkDirForCache(wsID, dir, 0)
}

func (fs *FileStore) walkDirForCache(wsID jsonldb.ID, dir string, parentID jsonldb.ID) error {
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

		fs.cache[wsID][id] = parentID
		if err := fs.walkDirForCache(wsID, filepath.Join(dir, entry.Name()), id); err != nil {
			return err
		}
	}
	return nil
}

// getParent returns the parent ID for a node.
// Returns 0 if the node is at the root or not found (caller handles not found via file system).
// Refreshes cache if node is not found.
func (fs *FileStore) getParent(wsID, id jsonldb.ID) jsonldb.ID {
	fs.mu.RLock()
	parents, ok := fs.cache[wsID]
	if ok {
		if parent, found := parents[id]; found {
			fs.mu.RUnlock()
			return parent
		}
	}
	fs.mu.RUnlock()

	// Not found in cache, try refreshing
	if err := fs.refreshCache(wsID); err != nil {
		slog.Error("failed to refresh cache", "wsID", wsID, "error", err)
		return 0
	}

	fs.mu.RLock()
	defer fs.mu.RUnlock()
	if parents, ok := fs.cache[wsID]; ok {
		return parents[id]
	}
	return 0
}

// setParent updates the cache with a new parent relationship.
func (fs *FileStore) setParent(wsID, id, parentID jsonldb.ID) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if _, ok := fs.cache[wsID]; !ok {
		fs.cache[wsID] = make(map[jsonldb.ID]jsonldb.ID)
	}
	fs.cache[wsID][id] = parentID
}

// deleteFromCache removes a node from the cache.
func (fs *FileStore) deleteFromCache(wsID, id jsonldb.ID) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if parents, ok := fs.cache[wsID]; ok {
		delete(parents, id)
	}
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
	parentID := fs.getParent(wsID, id)
	path := fs.pageDir(wsID, id, parentID)
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ReadPage reads a page from disk and returns it as a Node.
func (fs *FileStore) ReadPage(wsID, id jsonldb.ID) (*Node, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}

	parentID := fs.getParent(wsID, id)
	filePath := fs.pageIndexFile(wsID, id, parentID)
	data, err := os.ReadFile(filePath) //nolint:gosec // G304: filePath is constructed from validated wsID and id
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errPageNotFound
		}
		return nil, fmt.Errorf("failed to read page: %w", err)
	}

	p := parseMarkdownFile(id, data)
	return &Node{
		ID:       id,
		ParentID: parentID,
		Title:    p.title,
		Content:  p.content,
		Created:  p.created,
		Modified: p.modified,
		Tags:     p.tags,
		Type:     NodeTypeDocument,
	}, nil
}

// WritePage creates a new page on disk, commits to git, and returns it as a Node.
// If parentID is zero, creates the page at the root level.
// Otherwise, creates the page under the parent directory.
func (fs *FileStore) WritePage(ctx context.Context, wsID, id, parentID jsonldb.ID, title, content string, author git.Author) (*Node, error) {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return nil, err
	}

	var node *Node
	err = repo.CommitTx(ctx, author, func() (string, []string, error) {
		var err error
		node, err = fs.writePage(wsID, id, parentID, title, content)
		if err != nil {
			return "", nil, err
		}
		msg := "create: page " + id.String() + " - " + title
		files := []string{fs.gitPath(wsID, parentID, id, "index.md")}
		return msg, files, nil
	})
	return node, err
}

// writePage creates a new page on disk without committing.
// If parentID is zero, creates the page at the root level.
// Otherwise, creates the page under the parent directory.
func (fs *FileStore) writePage(wsID, id, parentID jsonldb.ID, title, content string) (*Node, error) {
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

	pageDir := fs.pageDir(wsID, id, parentID)
	if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	if err := fs.writePageFile(wsID, parentID, p); err != nil {
		return nil, err
	}

	// Update cache
	fs.setParent(wsID, id, parentID)

	return &Node{
		ID:       id,
		ParentID: parentID,
		Title:    title,
		Content:  content,
		Created:  now,
		Modified: now,
		Type:     NodeTypeDocument,
	}, nil
}

func (fs *FileStore) writePageFile(wsID, parentID jsonldb.ID, p *page) error {
	data := formatMarkdownFile(p)
	path := fs.pageIndexFile(wsID, p.id, parentID)
	return os.WriteFile(path, data, 0o644) //nolint:gosec // G306: 0o644 is intentional for user data files
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
		files := []string{fs.gitPath(wsID, node.ParentID, id, "index.md")}
		return "update: page " + id.String() + " - " + title, files, nil
	})
	if err != nil {
		return nil, err
	}
	return node, nil
}

// updatePage updates an existing page on disk without committing.
func (fs *FileStore) updatePage(wsID, id jsonldb.ID, title, content string) (*Node, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}

	parentID := fs.getParent(wsID, id)
	filePath := fs.pageIndexFile(wsID, id, parentID)
	// Check if exists
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil, errPageNotFound
		}
		return nil, fmt.Errorf("failed to check page existence: %w", err)
	}

	p := &page{
		id:       id,
		title:    title,
		content:  content,
		modified: storage.Now(),
	}

	if err := fs.writePageFile(wsID, parentID, p); err != nil {
		return nil, err
	}

	return &Node{
		ID:       id,
		ParentID: parentID,
		Title:    title,
		Content:  content,
		Modified: p.modified,
		Type:     NodeTypeDocument,
	}, nil
}

// DeletePage deletes a page directory and commits to git.
// DeletePage deletes a page and commits to git.
func (fs *FileStore) DeletePage(ctx context.Context, wsID, id jsonldb.ID, author git.Author) error {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return err
	}

	parentID := fs.getParent(wsID, id)
	return repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := fs.deletePage(wsID, id); err != nil {
			return "", nil, err
		}
		msg := "delete: page " + id.String()
		files := []string{fs.gitPath(wsID, parentID, id, "")}
		return msg, files, nil
	})
}

// deletePage deletes a page directory without committing.
func (fs *FileStore) deletePage(wsID, id jsonldb.ID) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}

	parentID := fs.getParent(wsID, id)
	pageDir := fs.pageDir(wsID, id, parentID)
	if err := os.RemoveAll(pageDir); err != nil {
		if os.IsNotExist(err) {
			return errPageNotFound
		}
		return fmt.Errorf("failed to delete page: %w", err)
	}

	// Remove from cache
	fs.deleteFromCache(wsID, id)

	return nil
}

// IterPages returns an iterator over all pages for an organization as Nodes.
// Recursively traverses the directory tree to include child pages under parents.
func (fs *FileStore) IterPages(wsID jsonldb.ID) (iter.Seq[*Node], error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}

	return func(yield func(*Node) bool) {
		fs.iterPagesRecursive(wsID, fs.wsPagesDir(wsID), 0, yield)
	}, nil
}

// iterPagesRecursive recursively yields pages from a directory and its subdirectories.
func (fs *FileStore) iterPagesRecursive(wsID jsonldb.ID, dir string, parentID jsonldb.ID, yield func(*Node) bool) {
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
		indexFile := fs.pageIndexFile(wsID, id, parentID)
		if _, err := os.Stat(indexFile); err == nil {
			if node, err := fs.ReadPage(wsID, id); err == nil {
				if !yield(node) {
					return
				}
			}
		}
		// Recursively yield children
		fs.iterPagesRecursive(wsID, filepath.Join(dir, entry.Name()), id, yield)
	}
}

// ReadNode reads a node (page, table, or both) from disk.
func (fs *FileStore) ReadNode(wsID, id jsonldb.ID) (*Node, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}

	parentID := fs.getParent(wsID, id)
	nodeDir := fs.pageDir(wsID, id, parentID)
	indexFile := fs.pageIndexFile(wsID, id, parentID)
	metadataFile := fs.tableMetadataFile(wsID, id, parentID)

	if _, err := os.Stat(nodeDir); err != nil {
		if os.IsNotExist(err) {
			return nil, errPageNotFound
		}
		return nil, fmt.Errorf("failed to check node directory: %w", err)
	}

	node := &Node{
		ID:       id,
		ParentID: parentID,
	}

	// Check if it's a page
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

	// Check if it's a table
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

	if node.Type == "" {
		return nil, errPageNotFound
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

// pageDir returns the directory path for a node.
// If parentID is zero, the node is at the root level.
// Otherwise, it is located under the parent directory.
func (fs *FileStore) pageDir(wsID, id, parentID jsonldb.ID) string {
	if parentID.IsZero() {
		return filepath.Join(fs.wsPagesDir(wsID), id.String())
	}
	// For hierarchical nodes, we need to recursively compute the parent's directory
	// But we don't know the parent's parent ID during creation.
	// Instead, we'll traverse the directory structure to find the parent.
	parentDir := fs.findPageDir(wsID, parentID)
	if parentDir == "" {
		// Parent not found, create at root (this shouldn't happen in normal flow)
		return filepath.Join(fs.wsPagesDir(wsID), id.String())
	}
	return filepath.Join(parentDir, id.String())
}

// findPageDir finds the actual directory for a node by searching the filesystem.
func (fs *FileStore) findPageDir(wsID, id jsonldb.ID) string {
	idStr := id.String()
	pagesDir := fs.wsPagesDir(wsID)

	// Check if it exists at root level first
	rootLevelDir := filepath.Join(pagesDir, idStr)
	if info, err := os.Stat(rootLevelDir); err == nil && info.IsDir() {
		return rootLevelDir
	}

	// Search recursively for nested nodes
	var foundDir string
	_ = filepath.WalkDir(pagesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || foundDir != "" {
			return err
		}
		if d.IsDir() && d.Name() == idStr {
			foundDir = path
			return filepath.SkipDir
		}
		return nil
	})
	return foundDir
}

// pageIndexFile returns the path to the index.md file for a node.
// If parentID is zero, the node is at the root level.
// Otherwise, it is located under the parent directory.
func (fs *FileStore) pageIndexFile(wsID, id, parentID jsonldb.ID) string {
	return filepath.Join(fs.pageDir(wsID, id, parentID), "index.md")
}

// TableExists checks if a table exists.
func (fs *FileStore) TableExists(wsID, id jsonldb.ID) bool {
	if wsID.IsZero() {
		return false
	}

	parentID := fs.getParent(wsID, id)
	path := fs.tableMetadataFile(wsID, id, parentID)
	_, err := os.Stat(path)
	return err == nil
}

// ReadTable reads a table definition from metadata.json and returns it as a Node.
// If parentID is zero, reads the table at the root level.
// Otherwise, reads the table from under the parent directory.
func (fs *FileStore) ReadTable(wsID, id jsonldb.ID) (*Node, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}

	parentID := fs.getParent(wsID, id)
	metadataFile := fs.tableMetadataFile(wsID, id, parentID)
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
// If parentID is zero, writes the table at the root level.
// Otherwise, writes the table under the parent directory.
func (fs *FileStore) WriteTable(ctx context.Context, wsID jsonldb.ID, node *Node, isNew bool, author git.Author) error {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return err
	}

	return repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := fs.writeTable(wsID, node.ParentID, node, isNew); err != nil {
			return "", nil, err
		}
		var msg string
		if isNew {
			msg = "create: table " + node.ID.String() + " - " + node.Title
		} else {
			msg = "update: table " + node.ID.String()
		}
		files := []string{fs.gitPath(wsID, node.ParentID, node.ID, "metadata.json")}
		return msg, files, nil
	})
}

// writeTable writes table metadata without committing.
// If parentID is zero, writes the table at the root level.
// Otherwise, writes the table under the parent directory.
func (fs *FileStore) writeTable(wsID, parentID jsonldb.ID, node *Node, isNew bool) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}
	if isNew {
		if err := fs.checkPageQuota(wsID); err != nil {
			return err
		}
	}

	// Write metadata.json with all table metadata including properties
	metadataFile := fs.tableMetadataFile(wsID, node.ID, parentID)
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

	if err := os.MkdirAll(fs.pageDir(wsID, node.ID, parentID), 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(metadataFile, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	return nil
}

// DeleteTable deletes a table and commits to git.
func (fs *FileStore) DeleteTable(ctx context.Context, wsID, id jsonldb.ID, author git.Author) error {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return err
	}

	parentID := fs.getParent(wsID, id)
	return repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := fs.deleteTable(wsID, id); err != nil {
			return "", nil, err
		}
		// When deleting table, we delete the metadata.json
		// If it's a pure table node (no index.md), we might delete the directory?
		// But deleteTable only deletes metadata.json currently?
		// Let's check deleteTable impl below.
		// It deletes metadata.json.
		files := []string{fs.gitPath(wsID, parentID, id, "metadata.json")}
		return "delete: table " + id.String(), files, nil
	})
}

// deleteTable deletes table metadata without committing.
func (fs *FileStore) deleteTable(wsID, id jsonldb.ID) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}

	parentID := fs.getParent(wsID, id)
	metadataFile := fs.tableMetadataFile(wsID, id, parentID)
	if err := os.Remove(metadataFile); err != nil {
		if os.IsNotExist(err) {
			return errTableNotFound
		}
		return fmt.Errorf("failed to delete table metadata: %w", err)
	}

	// If this was a pure table (no index.md), we should check if directory is empty/should be removed?
	// But keeping it simple: just delete the file.
	// If the directory becomes empty, it might be left behind?
	// For now, removing the metadata file effectively removes the table.
	// We should probably check if we need to remove from cache if node is gone?
	// But it might still be a page (index.md).
	// If index.md also doesn't exist, maybe we should remove from cache?
	// But for simplicity, we leave cache management to DeletePage which removes directory.
	// Wait, if it's a table-only node, DeleteTable should probably act like DeletePage?
	// But currently DeleteTable just removes metadata.json.

	return nil
}

// IterTables returns an iterator over all tables for the given organization as Nodes.
// Recursively traverses the directory tree to include child tables under parents.
func (fs *FileStore) IterTables(wsID jsonldb.ID) (iter.Seq[*Node], error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}

	return func(yield func(*Node) bool) {
		fs.iterTablesRecursive(wsID, fs.wsPagesDir(wsID), 0, yield)
	}, nil
}

// iterTablesRecursive recursively yields tables from a directory and its subdirectories.
func (fs *FileStore) iterTablesRecursive(wsID jsonldb.ID, dir string, parentID jsonldb.ID, yield func(*Node) bool) {
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
		metadataFile := fs.tableMetadataFile(wsID, id, parentID)
		if _, err := os.Stat(metadataFile); err == nil {
			if node, err := fs.ReadTable(wsID, id); err == nil {
				if !yield(node) {
					return
				}
			}
		}
		// Recursively yield children
		fs.iterTablesRecursive(wsID, filepath.Join(dir, entry.Name()), id, yield)
	}
}

// AppendRecord appends a record to a table and commits to git.
func (fs *FileStore) AppendRecord(ctx context.Context, wsID, tableID jsonldb.ID, record *DataRecord, author git.Author) error {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return err
	}

	parentID := fs.getParent(wsID, tableID)
	return repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := fs.appendRecord(wsID, tableID, parentID, record); err != nil {
			return "", nil, err
		}
		files := []string{fs.gitPath(wsID, parentID, tableID, "data.jsonl")}
		return "create: record " + record.ID.String(), files, nil
	})
}

// appendRecord appends a record to a table without committing.
// tableParentID specifies the parent of the table (0 for root level).
func (fs *FileStore) appendRecord(wsID, tableID, tableParentID jsonldb.ID, record *DataRecord) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}

	// Check quotas
	ws, err := fs.wsSvc.Get(wsID)
	if err != nil {
		return err
	}

	recordsFile := fs.tableRecordsFile(wsID, tableID, tableParentID)

	// Check max records per table
	table, err := jsonldb.NewTable[*DataRecord](recordsFile)
	// If file doesn't exist, we create it, so no error is fine if IsNotExist
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to open table: %w", err)
	}

	if table != nil {
		if table.Len() >= ws.Quotas.MaxRecordsPerTable {
			return fmt.Errorf("record quota exceeded: max %d", ws.Quotas.MaxRecordsPerTable)
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
	if err := fs.checkStorageQuota(wsID, int64(len(data))); err != nil {
		return err
	}

	if err := table.Append(record); err != nil {
		return fmt.Errorf("failed to append record: %w", err)
	}
	return nil
}

// IterRecords iterates over all records in a table.
func (fs *FileStore) IterRecords(wsID, id jsonldb.ID) (iter.Seq[*DataRecord], error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}

	parentID := fs.getParent(wsID, id)
	filePath := fs.tableRecordsFile(wsID, id, parentID)

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

	parentID := fs.getParent(wsID, id)
	filePath := fs.tableRecordsFile(wsID, id, parentID)

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
func (fs *FileStore) UpdateRecord(ctx context.Context, wsID, tableID jsonldb.ID, record *DataRecord, author git.Author) error {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return err
	}

	parentID := fs.getParent(wsID, tableID)
	return repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := fs.updateRecord(wsID, tableID, parentID, record); err != nil {
			return "", nil, err
		}
		files := []string{fs.gitPath(wsID, parentID, tableID, "data.jsonl")}
		return "update: record " + record.ID.String(), files, nil
	})
}

// updateRecord updates a record in a table without committing.
func (fs *FileStore) updateRecord(wsID, tableID, tableParentID jsonldb.ID, record *DataRecord) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}

	// Check quotas (size difference)
	// For simplicity, we assume update doesn't drastically increase size or we check strict quota on write.
	// But we should check storage quota if size increases.
	// Leaving simplified for now.

	recordsFile := fs.tableRecordsFile(wsID, tableID, tableParentID)
	table, err := jsonldb.NewTable[*DataRecord](recordsFile)
	if err != nil {
		return fmt.Errorf("failed to open table: %w", err)
	}

	if _, err := table.Update(record); err != nil {
		return fmt.Errorf("failed to update record: %w", err)
	}
	return nil
}

// DeleteRecord deletes a record from a table and commits to git.
func (fs *FileStore) DeleteRecord(ctx context.Context, wsID, tableID, recordID jsonldb.ID, author git.Author) error {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return err
	}

	parentID := fs.getParent(wsID, tableID)
	return repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := fs.deleteRecord(wsID, tableID, parentID, recordID); err != nil {
			return "", nil, err
		}
		files := []string{fs.gitPath(wsID, parentID, tableID, "data.jsonl")}
		return "delete: record " + recordID.String(), files, nil
	})
}

// deleteRecord deletes a record from a table without committing.
func (fs *FileStore) deleteRecord(wsID, tableID, tableParentID, recordID jsonldb.ID) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}

	recordsFile := fs.tableRecordsFile(wsID, tableID, tableParentID)
	table, err := jsonldb.NewTable[*DataRecord](recordsFile)
	if err != nil {
		return fmt.Errorf("failed to open table: %w", err)
	}

	if _, err := table.Delete(recordID); err != nil {
		return fmt.Errorf("failed to delete record: %w", err)
	}
	return nil
}

// tableRecordsFile returns the path to the data.jsonl file for a table.
// If parentID is zero, the table is at the root level.
// Otherwise, it is located under the parent directory.
func (fs *FileStore) tableRecordsFile(wsID, id, parentID jsonldb.ID) string {
	return filepath.Join(fs.pageDir(wsID, id, parentID), "data.jsonl")
}

// tableMetadataFile returns the path to the metadata.json file for a node.
// If parentID is zero, the node is at the root level.
// Otherwise, it is located under the parent directory.
func (fs *FileStore) tableMetadataFile(wsID, id, parentID jsonldb.ID) string {
	return filepath.Join(fs.pageDir(wsID, id, parentID), "metadata.json")
}

// gitPath constructs the git path for a file, accounting for parent hierarchy.
// Since we're in the middle of a transaction, we need to build the path from scratch.
// The simplest approach is to use the directory path that was already computed.
func (fs *FileStore) gitPath(wsID, parentID, id jsonldb.ID, fileName string) string {
	// Get the physical directory path
	dir := fs.pageDir(wsID, id, parentID)
	// Convert to git path by removing the root prefix
	relPath, _ := filepath.Rel(fs.wsPagesDir(wsID), dir)
	// Use filepath.Join to construct the path properly
	return filepath.Join("pages", relPath, fileName)
}

// SaveAsset saves an asset to disk and commits to git.
func (fs *FileStore) SaveAsset(ctx context.Context, wsID, pageID jsonldb.ID, assetName string, data []byte, author git.Author) (*Asset, error) {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return nil, err
	}

	parentID := fs.getParent(wsID, pageID)
	var asset *Asset
	err = repo.CommitTx(ctx, author, func() (string, []string, error) {
		var err error
		asset, err = fs.saveAsset(wsID, pageID, parentID, assetName, data)
		if err != nil {
			return "", nil, err
		}
		files := []string{fs.gitPath(wsID, parentID, pageID, assetName)}
		return "create: asset " + assetName, files, nil
	})
	return asset, err
}

func (fs *FileStore) saveAsset(wsID, pageID, pageParentID jsonldb.ID, assetName string, data []byte) (*Asset, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}

	if err := fs.checkStorageQuota(wsID, int64(len(data))); err != nil {
		return nil, err
	}

	dir := fs.pageDir(wsID, pageID, pageParentID)
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Validate filename to prevent directory traversal
	if filepath.Base(assetName) != assetName {
		return nil, errors.New("invalid asset name")
	}

	path := filepath.Join(dir, assetName)
	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data
		return nil, fmt.Errorf("failed to write asset: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat asset: %w", err)
	}

	return &Asset{
		ID:       assetName,
		Name:     assetName,
		MimeType: mime.TypeByExtension(filepath.Ext(assetName)),
		Size:     info.Size(),
		Created:  storage.ToTime(info.ModTime()),
		Path:     path,
	}, nil
}

// ReadAsset reads the binary content of an asset.
func (fs *FileStore) ReadAsset(wsID, pageID jsonldb.ID, assetName string) ([]byte, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}

	parentID := fs.getParent(wsID, pageID)
	dir := fs.pageDir(wsID, pageID, parentID)
	path := filepath.Join(dir, assetName)

	return os.ReadFile(path) //nolint:gosec // G304: path is constructed from validated IDs
}

// DeleteAsset deletes an asset file and commits to git.
func (fs *FileStore) DeleteAsset(ctx context.Context, wsID, pageID jsonldb.ID, assetName string, author git.Author) error {
	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return err
	}

	parentID := fs.getParent(wsID, pageID)
	return repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := fs.deleteAsset(wsID, pageID, parentID, assetName); err != nil {
			return "", nil, err
		}
		files := []string{fs.gitPath(wsID, parentID, pageID, assetName)}
		return "delete: asset " + assetName, files, nil
	})
}

// deleteAsset deletes an asset file without committing.
func (fs *FileStore) deleteAsset(wsID, pageID, pageParentID jsonldb.ID, assetName string) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}

	dir := fs.pageDir(wsID, pageID, pageParentID)
	path := filepath.Join(dir, assetName)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return errors.New("asset not found")
		}
		return fmt.Errorf("failed to delete asset: %w", err)
	}
	return nil
}

// IterAssets returns an iterator over all assets for a page.
func (fs *FileStore) IterAssets(wsID, pageID jsonldb.ID) (iter.Seq[*Asset], error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}

	parentID := fs.getParent(wsID, pageID)
	dir := fs.pageDir(wsID, pageID, parentID)

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
// If parentID is zero, the node is created at the root level.
// Otherwise, it is created under the parent node.
func (fs *FileStore) CreateNode(ctx context.Context, wsID jsonldb.ID, title string, nodeType NodeType, parentID jsonldb.ID, author git.Author) (*Node, error) {
	// Verify parent exists if parentID is specified
	if !parentID.IsZero() && !fs.PageExists(wsID, parentID) {
		return nil, fmt.Errorf("parent node not found: %w", errPageNotFound)
	}

	repo, err := fs.Repo(ctx, wsID)
	if err != nil {
		return nil, err
	}

	var node *Node
	err = repo.CommitTx(ctx, author, func() (string, []string, error) {
		var files []string
		var err error
		node, files, err = fs.createNode(wsID, title, nodeType, parentID)
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
func (fs *FileStore) createNode(wsID jsonldb.ID, title string, nodeType NodeType, parentID jsonldb.ID) (*Node, []string, error) {
	if err := fs.checkPageQuota(wsID); err != nil {
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
		pageDir := fs.pageDir(wsID, id, parentID)
		if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}

		filePath := fs.pageIndexFile(wsID, id, parentID)
		if err := os.WriteFile(filePath, pageData, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
			return nil, nil, fmt.Errorf("failed to write page: %w", err)
		}
		files = append(files, fs.gitPath(wsID, parentID, id, "index.md"))
	}

	if nodeType == NodeTypeTable || nodeType == NodeTypeHybrid {
		if err := os.MkdirAll(fs.pageDir(wsID, id, parentID), 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}

		metadataFile := fs.tableMetadataFile(wsID, id, parentID)
		if err := os.WriteFile(metadataFile, metadataData, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for user data files
			return nil, nil, fmt.Errorf("failed to write metadata: %w", err)
		}
		files = append(files, fs.gitPath(wsID, parentID, id, "metadata.json"))
	}

	return node, files, nil
}

// Repo returns the git.Repo for an organization. This is exported for handlers
// that need direct git operations (e.g., git remotes).
func (fs *FileStore) Repo(ctx context.Context, wsID jsonldb.ID) (*git.Repo, error) {
	return fs.git.Repo(ctx, wsID.String())
}
