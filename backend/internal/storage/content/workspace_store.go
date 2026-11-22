// Owns quota-admitted page and record mutations, conditional writes, serialized imports, discovery, and canonical snapshots.

// Package content provides versioned file storage for the mddb system.
package content

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"maps"
	"mime"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	contentv1 "github.com/maruel/mddb/backend/internal/storage/content/data/v1"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/git"
)

// relativeLinkRe matches markdown links ending in /index.md (relative file paths on disk).
// Group 1: link text, Group 2: relative href ending in /index.md.
var relativeLinkRe = regexp.MustCompile(`\[([^\]]*)\]\(([^)]+/index\.md)\)`)

// WorkspaceFileStore is a versioned file storage system for a single workspace.
// All mutations are committed to git.
// Storage model: Each page (document or table) is an ID-based directory within the workspace.
//   - Pages: ID directory containing index.md with YAML front matter.
//   - Tables: ID directory containing metadata.json + data.jsonl.
//   - Assets: files within each page's directory namespace.
type WorkspaceFileStore struct {
	wsDir        string                    // Pre-computed: rootDir/wsID
	repo         git.Repository            // Cached git repository
	quotas       *storagev1.ResourceQuotas // Effective quotas (min of server/org/ws)
	mu           sync.RWMutex              // Protects ancestry and discovery indexes
	cache        map[ksid.ID]ksid.ID       // path identity -> parent, including retained containers
	pathChildren map[ksid.ID][]ksid.ID
	discoverable map[ksid.ID]struct{}
	indexParents map[ksid.ID]ksid.ID // Published ancestry; legacy refreshes never mutate it.
	indexReady   bool
	nodeIDs      []ksid.ID
	childIDs     map[ksid.ID][]ksid.ID
	links        linkCache // In-memory backlink index
	// snapshotRevisions is bounded and protected by the repository mutation gate.
	snapshotRevisions map[ksid.ID]snapshotRevision
}

// newWorkspaceFileStore creates a new workspace store.
// This is called internally by FileStoreService.GetWorkspaceStore.
func newWorkspaceFileStore(wsDir string, repo git.Repository, quotas *storagev1.ResourceQuotas) *WorkspaceFileStore {
	return &WorkspaceFileStore{
		wsDir:        wsDir,
		repo:         repo,
		quotas:       quotas,
		cache:        make(map[ksid.ID]ksid.ID),
		pathChildren: make(map[ksid.ID][]ksid.ID),
		discoverable: make(map[ksid.ID]struct{}),
	}
}

// ImportTx excludes readers while a background import replaces saved node files.
// Ordinary document edits must use revision-conditional UpdatePage or AppendPage.
// Existing-page snapshots refresh immediately; new-node discovery is published
// when the importer retires workspace indexes after completion or failure.
// The repository's recovery worker records imported changes in history later.
func (ws *WorkspaceFileStore) ImportTx(ctx context.Context, fn func() error) error {
	return ws.repo.WriteTx(ctx, fn)
}

// EffectiveQuotas returns the effective resource quotas for this workspace.
func (ws *WorkspaceFileStore) EffectiveQuotas() storagev1.ResourceQuotas {
	return *ws.quotas
}

// refreshCache rebuilds only the legacy parent cache. It must never change
// the published bounded index or the ancestry used by bounded readers.
func (ws *WorkspaceFileStore) refreshCache() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	paths := newNodePathIndex(false)
	if err := paths.scan(func() error { return nil }, ws.wsDir, 0); err != nil {
		return err
	}
	ws.cache = paths.parents
	return nil
}

// scanIndex publishes complete identity/ancestry data from a repository read transaction.
func (ws *WorkspaceFileStore) scanIndex(check func() error) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	paths := newNodePathIndex(true)
	if err := paths.scan(check, ws.wsDir, 0); err != nil {
		return err
	}
	ws.cache = maps.Clone(paths.parents)
	ws.indexParents = paths.parents
	ws.pathChildren = paths.children
	ws.discoverable = paths.discoverable
	ws.rebuildIndexLocked()
	return nil
}

// getParent returns the parent ID for a node.
// Returns 0 if the node is at the root or not found (caller handles not found via file system).
// Refreshes cache if node is not found.
func (ws *WorkspaceFileStore) getParent(id ksid.ID) ksid.ID {
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
func (ws *WorkspaceFileStore) setParent(id, parentID ksid.ID) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.setIndexParentLocked(id, parentID)
}

// deleteFromCache removes a node from the cache.
func (ws *WorkspaceFileStore) deleteFromCache(id ksid.ID) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.deleteIndexSubtreeLocked(id)
}

// checkPageQuota returns an error if creating a new page would exceed quota.
// MaxPages=0 means disabled (0 pages allowed). Server always provides a positive floor.
func (ws *WorkspaceFileStore) checkPageQuota() error {
	u, err := ws.scanUsage()
	if err != nil {
		return err
	}
	return quotaGrowth("pages", int64(u.pages), int64(u.pages+1), int64(ws.quotas.MaxPages))
}

// checkStorageQuota returns an error if adding the given bytes would exceed workspace storage quota.
// MaxStorageBytes=0 means disabled (0 bytes allowed). Server always provides a positive floor.
func (ws *WorkspaceFileStore) checkStorageQuota(additionalBytes int64) error {
	if additionalBytes <= 0 {
		return nil
	}
	u, err := ws.scanUsage()
	if err != nil {
		return err
	}
	return quotaGrowth("storage bytes", u.bytes, u.bytes+additionalBytes, ws.quotas.MaxStorageBytes)
}

// Path helpers

// relativeDir returns the relative directory path for a node.
// Top-level nodes (parentID=0) are stored directly in workspace dir.
// Nested nodes include their parent chain in the path.
func (ws *WorkspaceFileStore) relativeDir(id, parentID ksid.ID) string {
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
func (ws *WorkspaceFileStore) pageDir(id, parentID ksid.ID) string {
	return filepath.Join(ws.wsDir, ws.relativeDir(id, parentID))
}

func (ws *WorkspaceFileStore) pageIndexFile(id, parentID ksid.ID) string {
	return filepath.Join(ws.pageDir(id, parentID), "index.md")
}

func (ws *WorkspaceFileStore) tableRecordsFile(id, parentID ksid.ID) string {
	return filepath.Join(ws.pageDir(id, parentID), "data.jsonl")
}

func (ws *WorkspaceFileStore) tableMetadataFile(id, parentID ksid.ID) string {
	return filepath.Join(ws.pageDir(id, parentID), "metadata.json")
}

// gitPath builds a git-relative path by walking up the parent chain.
// parentID must be passed explicitly because during node creation,
// the node doesn't exist in the cache yet (it's added after gitPath is called).
// For root node (id=0), returns just the filename.
func (ws *WorkspaceFileStore) gitPath(parentID, id ksid.ID, fileName string) string {
	rel := ws.relativeDir(id, parentID)
	if rel == "" {
		return fileName
	}
	return filepath.Join(rel, fileName)
}

// PageExists checks if a page exists.
func (ws *WorkspaceFileStore) PageExists(id ksid.ID) bool {
	if id.IsZero() {
		return false // No node with ID 0 exists
	}
	parentID := ws.getParent(id)
	filePath := ws.pageIndexFile(id, parentID)
	_, err := os.Stat(filePath)
	return err == nil
}

// ReadPage reads a page by ID.
func (ws *WorkspaceFileStore) ReadPage(id ksid.ID) (*Node, error) {
	parentID := ws.getParent(id)
	filePath := ws.pageIndexFile(id, parentID)

	data, err := os.ReadFile(filePath) //nolint:gosec // G304: filePath is constructed from validated id
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNodeNotFound
		}
		return nil, fmt.Errorf("failed to read page: %w", err)
	}

	p := ParseMarkdown(data)
	return &Node{
		ID:       id,
		ParentID: parentID,
		Title:    p.title,
		Type:     NodeTypeDocument,
		Content:  p.content,
		Created:  p.created,
		Modified: p.modified,
		Icon:     p.icon,
		Cover:    p.cover,
	}, nil
}

// writePageFile writes the page file.
func (ws *WorkspaceFileStore) writePageFile(id, parentID ksid.ID, p *page) error {
	data := formatMarkdownFile(p)
	pageDir := ws.pageDir(id, parentID)
	filePath := ws.pageIndexFile(id, parentID)
	if err := ws.admitFile(filePath, int64(len(data))); err != nil {
		return err
	}
	if err := os.MkdirAll(pageDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return fmt.Errorf("failed to create directory: %w", err)
	}
	if err := storage.WriteFile(filePath, data); err != nil {
		return fmt.Errorf("failed to write page: %w", err)
	}
	return nil
}

// UpdatePage replaces a page only when expectedRevision matches its canonical content.
func (ws *WorkspaceFileStore) UpdatePage(ctx context.Context, id ksid.ID, title, body, expectedRevision string, author git.Author) (*Node, error) {
	return ws.mutatePage(ctx, id, expectedRevision, author, func(p *page) { p.title = title; p.content = body })
}

// AppendPage appends markdown atomically against the revision the caller read.
func (ws *WorkspaceFileStore) AppendPage(ctx context.Context, id ksid.ID, body, expectedRevision string, author git.Author) (*Node, error) {
	return ws.mutatePage(ctx, id, expectedRevision, author, func(p *page) {
		if p.content == "" {
			p.content = body
		} else {
			p.content = strings.TrimRight(p.content, "\n") + "\n\n" + body
		}
	})
}

func (ws *WorkspaceFileStore) mutatePage(ctx context.Context, id ksid.ID, expectedRevision string, author git.Author, mutate func(*page)) (*Node, error) {
	var node *Node
	err := ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		parent := ws.getParent(id)
		current, p, err := ws.readNodeFromPath(ws.pageDir(id, parent), id, parent)
		if err != nil {
			return "", nil, err
		}
		if p == nil {
			return "", nil, ErrNodeNotFound
		}
		revision, err := Revision(ctx, p.title, strings.NewReader(p.content), int64(len(p.content)))
		if err != nil {
			return "", nil, err
		}
		if expectedRevision == "" || revision != expectedRevision {
			return "", nil, &RevisionConflictError{CurrentRevision: revision}
		}
		previousPageTitle, previousBody := p.title, p.content
		previousTitle := current.Title
		mutate(p)
		p.modified = storagev1.Now()
		// Parse the exact bytes being persisted so normalization and the returned
		// revision describe one saved snapshot, without a post-transaction read.
		data := formatMarkdownFile(p)
		canonical := ParseMarkdown(data)
		revision, err = Revision(ctx, canonical.title, strings.NewReader(canonical.content), int64(len(canonical.content)))
		if err != nil {
			return "", nil, err
		}
		if previousBody == canonical.content && previousPageTitle == canonical.title {
			node = current
			node.Revision = revision
			return "", nil, nil
		}
		if err := ws.admitFile(ws.pageIndexFile(id, parent), int64(len(data))); err != nil {
			return "", nil, err
		}
		if err := storage.WriteFile(ws.pageIndexFile(id, parent), data); err != nil {
			return "", nil, err
		}
		node = current
		if node.Type == NodeTypeDocument {
			node.Title = canonical.title
		}
		node.Content, node.Revision = canonical.content, revision
		node.Created, node.Modified = canonical.created, canonical.modified
		node.Icon, node.Cover = canonical.icon, canonical.cover
		node.Changes = NodeChanges{Body: previousBody != canonical.content || previousPageTitle != canonical.title, Title: previousTitle != node.Title}
		ws.links.update(id, node.Content)
		return "update: page " + id.String(), []string{ws.gitPath(parent, id, "index.md")}, nil
	})
	return node, err
}

// UpdatePageFrontmatter updates the icon and cover fields of a page and commits to git.
// Pass empty string to clear a field.
func (ws *WorkspaceFileStore) UpdatePageFrontmatter(ctx context.Context, id ksid.ID, icon, cover string, author git.Author) (*Node, error) {
	var node *Node
	err := ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		parentID := ws.getParent(id)
		filePath := ws.pageIndexFile(id, parentID)

		data, err := os.ReadFile(filePath) //nolint:gosec // G304: filePath is constructed from validated id
		if err != nil {
			if os.IsNotExist(err) {
				return "", nil, ErrNodeNotFound
			}
			return "", nil, fmt.Errorf("failed to read page: %w", err)
		}

		p := ParseMarkdown(data)
		changed := p.icon != icon || p.cover != cover
		p.icon = icon
		p.cover = cover

		if changed {
			if err := ws.writePageFile(id, parentID, p); err != nil {
				return "", nil, err
			}
		}

		node = &Node{
			Changes:  NodeChanges{Metadata: changed},
			ID:       id,
			ParentID: parentID,
			Title:    p.title,
			Type:     NodeTypeDocument,
			Content:  p.content,
			Created:  p.created,
			Modified: p.modified,
			Icon:     p.icon,
			Cover:    p.cover,
		}
		files := []string{ws.gitPath(parentID, id, "index.md")}
		if !changed {
			return "", nil, nil
		}
		return "update: frontmatter " + id.String(), files, nil
	})
	return node, err
}

// DeletePage deletes a page and commits to git.
func (ws *WorkspaceFileStore) DeletePage(ctx context.Context, id ksid.ID, author git.Author) (deleted NodeDeletion, err error) {
	err = ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		deleted.NodeID, deleted.ParentID = id, ws.getParent(id)
		ws.mu.RLock()
		var collect func(ksid.ID)
		collect = func(node ksid.ID) {
			if len(deleted.IDs) == 128 {
				deleted.Resync = true
				return
			}
			deleted.IDs = append(deleted.IDs, node)
			for _, child := range ws.pathChildren[node] {
				collect(child)
				if deleted.Resync {
					return
				}
			}
		}
		collect(id)
		ws.mu.RUnlock()
		if err := ws.deletePage(id); err != nil {
			return "", nil, err
		}
		return "delete: page " + id.String(), []string{ws.gitPath(deleted.ParentID, id, "index.md")}, nil
	})
	if err == nil {
		ws.links.remove(id)
	}
	return deleted, err
}

// deletePage deletes a page without committing.
func (ws *WorkspaceFileStore) deletePage(id ksid.ID) error {
	if id.IsZero() {
		return ErrNodeNotFound // No node with ID 0 exists
	}

	parentID := ws.getParent(id)
	dir := ws.pageDir(id, parentID)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return ErrNodeNotFound
		}
		return fmt.Errorf("inspect page before deletion: %w", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		if os.IsNotExist(err) {
			return ErrNodeNotFound
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
func (ws *WorkspaceFileStore) iterPagesRecursive(dir string, parentID ksid.ID, yield func(*Node) bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id, err := ksid.Parse(entry.Name())
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
func (ws *WorkspaceFileStore) ReadNode(ctx context.Context, id ksid.ID) (*Node, error) {
	parentID := ws.getParent(id)
	nodeDir := ws.pageDir(id, parentID)
	node, p, err := ws.readNodeFromPath(nodeDir, id, parentID)
	if err != nil {
		return nil, err
	}

	if p != nil {
		node.Revision, err = Revision(ctx, p.title, strings.NewReader(p.content), int64(len(p.content)))
		if err != nil {
			return nil, err
		}
	}

	// Detect children for UI to show expand arrow
	entries, _ := os.ReadDir(nodeDir)
	for _, entry := range entries {
		if entry.IsDir() {
			if _, err := ksid.Parse(entry.Name()); err == nil {
				node.Children = []*Node{} // Has children - set empty slice to indicate expandable
				break
			}
		}
	}

	return node, nil
}

// readNodeFromPath reads a node and its canonical page from a specific path.
func (ws *WorkspaceFileStore) readNodeFromPath(path string, id, parentID ksid.ID) (*Node, *page, error) {
	indexFile := filepath.Join(path, "index.md")
	metadataFile := filepath.Join(path, "metadata.json")

	indexData, indexErr := os.ReadFile(indexFile) //nolint:gosec // G304: path is constructed from validated id
	hasIndex := indexErr == nil

	metadataData, metadataErr := os.ReadFile(metadataFile) //nolint:gosec // G304: path is constructed from validated id
	hasMetadata := metadataErr == nil

	if !hasIndex && !hasMetadata {
		return nil, nil, ErrNodeNotFound
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

	var p *page
	if hasIndex {
		p = ParseMarkdown(indexData)
		node.Title = p.title
		node.Content = p.content
		node.Created = p.created
		node.Modified = p.modified
		node.Icon = p.icon
		node.Cover = p.cover
	}

	if hasMetadata {
		var metadata map[string]any
		if err := json.Unmarshal(metadataData, &metadata); err != nil {
			return nil, nil, fmt.Errorf("failed to parse table metadata: %w", err)
		}
		if title, ok := metadata["title"].(string); ok {
			node.Title = title
		}
		if props, ok := metadata["properties"]; ok {
			propsData, err := json.Marshal(props)
			if err != nil {
				return nil, nil, fmt.Errorf("encode table properties: %w", err)
			}
			if err := json.Unmarshal(propsData, &node.Properties); err != nil {
				return nil, nil, fmt.Errorf("decode table properties: %w", err)
			}
		}
		if views, ok := metadata["views"]; ok {
			viewsData, err := json.Marshal(views)
			if err != nil {
				return nil, nil, fmt.Errorf("encode table views: %w", err)
			}
			if err := json.Unmarshal(viewsData, &node.Views); err != nil {
				return nil, nil, fmt.Errorf("decode table views: %w", err)
			}
		}
		var err error
		node.SchemaRevision, err = tableSchemaRevision(node.Properties)
		if err != nil {
			return nil, nil, err
		}
	}

	return node, p, nil
}

// GetWorkspaceUsage returns the page count and storage usage for the workspace.
func (ws *WorkspaceFileStore) GetWorkspaceUsage() (pageCount int, storageUsage int64, err error) {
	u, err := ws.scanUsage()
	return u.pages, u.bytes, err
}

// CommitCount returns the number of git commits in the workspace repository.
func (ws *WorkspaceFileStore) CommitCount(ctx context.Context) (int, error) {
	return ws.repo.CommitCount(ctx)
}

// TableExists checks if a table exists.
func (ws *WorkspaceFileStore) TableExists(id ksid.ID) bool {
	parentID := ws.getParent(id)
	metadataFile := ws.tableMetadataFile(id, parentID)
	_, err := os.Stat(metadataFile)
	return err == nil
}

// ReadTable reads a table by ID.
func (ws *WorkspaceFileStore) ReadTable(id ksid.ID) (*Node, error) {
	parentID := ws.getParent(id)
	metadataFile := ws.tableMetadataFile(id, parentID)

	data, err := os.ReadFile(metadataFile) //nolint:gosec // G304: metadataFile is constructed from validated id
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrTableNotFound
		}
		return nil, fmt.Errorf("failed to read table metadata: %w", err)
	}

	var metadata map[string]any
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse table metadata: %w", err)
	}
	if err := checkTableMetadataVersion(metadata["version"]); err != nil {
		return nil, err
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
			node.Created = storagev1.ToTime(t)
		}
	}

	if modified, ok := metadata["modified"].(string); ok {
		if t, err := time.Parse(time.RFC3339, modified); err == nil {
			node.Modified = storagev1.ToTime(t)
		}
	}

	if props, ok := metadata["properties"]; ok {
		propsData, err := json.Marshal(props)
		if err != nil {
			return nil, fmt.Errorf("encode table properties: %w", err)
		}
		if err := json.Unmarshal(propsData, &node.Properties); err != nil {
			return nil, fmt.Errorf("decode table properties: %w", err)
		}
	}

	if views, ok := metadata["views"]; ok {
		viewsData, err := json.Marshal(views)
		if err != nil {
			return nil, fmt.Errorf("encode table views: %w", err)
		}
		if err := json.Unmarshal(viewsData, &node.Views); err != nil {
			return nil, fmt.Errorf("decode table views: %w", err)
		}
	}
	node.SchemaRevision, err = tableSchemaRevision(node.Properties)
	if err != nil {
		return nil, err
	}

	return node, nil
}

func tableMetadataRevision(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func tableSchemaRevision(properties []contentv1.Property) (string, error) {
	if len(properties) == 0 {
		properties = nil
	}
	data, err := json.Marshal(properties)
	if err != nil {
		return "", fmt.Errorf("encode table schema revision: %w", err)
	}
	return tableMetadataRevision(data), nil
}

// WriteTable writes table metadata and commits to git.
func (ws *WorkspaceFileStore) WriteTable(ctx context.Context, node *Node, isNew bool, author git.Author) error {
	parentID := node.ParentID
	return ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		if !isNew {
			before, err := ws.readNodeSummary(ctx, node.ID)
			if err != nil {
				return "", nil, err
			}
			node.Changes = NodeChanges{Title: before.Title != node.Title, Schema: !reflect.DeepEqual(before.Properties, node.Properties) || !reflect.DeepEqual(before.Views, node.Views)}
			if !node.Changes.Title && !node.Changes.Schema {
				return "", nil, nil
			}
		}
		if err := ws.writeTable(node, isNew); err != nil {
			return "", nil, err
		}
		files := []string{ws.gitPath(parentID, node.ID, "metadata.json")}
		return "update: table " + node.ID.String(), files, nil
	})
}

// MutateTable reads and changes table metadata while holding the repository's
// mutation gate. The returned node describes the committed state.
func (ws *WorkspaceFileStore) MutateTable(ctx context.Context, id ksid.ID, author git.Author, mutate func(*Node) error) (*Node, error) {
	var node *Node
	err := ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		var err error
		node, err = ws.ReadTable(id)
		if err != nil {
			return "", nil, err
		}
		before, err := ws.readNodeSummary(ctx, id)
		if err != nil {
			return "", nil, err
		}
		if err := mutate(node); err != nil {
			return "", nil, err
		}
		node.Changes = NodeChanges{Title: before.Title != node.Title, Schema: !reflect.DeepEqual(before.Properties, node.Properties) || !reflect.DeepEqual(before.Views, node.Views)}
		if !node.Changes.Title && !node.Changes.Schema {
			return "", nil, nil
		}
		if err := ws.writeTable(node, false); err != nil {
			return "", nil, err
		}
		saved, err := ws.ReadTable(id)
		if err != nil {
			return "", nil, err
		}
		saved.Changes = node.Changes
		node = saved
		return "update: table " + id.String(), []string{ws.gitPath(node.ParentID, id, "metadata.json")}, nil
	})
	if err != nil {
		return nil, err
	}
	return node, nil
}

// tableMetadataVersion is the metadata.json format that writeTable emits.
const tableMetadataVersion = "1.0"

// checkTableMetadataVersion accepts any 1.x metadata.json version. A missing version is
// 1.0: the Notion importer and hand-edited workspaces omit it.
func checkTableMetadataVersion(v any) error {
	switch v := v.(type) {
	case nil:
		return nil
	case string:
		if strings.HasPrefix(v, "1.") {
			return nil
		}
	}
	return fmt.Errorf("unsupported table metadata version %v (expected 1.x)", v)
}

// writeTable writes table metadata without committing.
func (ws *WorkspaceFileStore) writeTable(node *Node, isNew bool) error {
	if err := ws.checkTableShape(node.ID, node.Properties); err != nil {
		return err
	}
	parentID := node.ParentID
	metadataFile := ws.tableMetadataFile(node.ID, parentID)

	metadata := map[string]any{
		"title":      node.Title,
		"version":    tableMetadataVersion,
		"modified":   storagev1.Now(),
		"properties": node.Properties,
		"views":      node.Views,
	}

	if isNew {
		metadata["created"] = storagev1.Now()
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

	if err := ws.admitFile(metadataFile, int64(len(data))); err != nil {
		return err
	}

	if err := os.MkdirAll(ws.pageDir(node.ID, parentID), 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := storage.WriteFile(metadataFile, data); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	return nil
}

// DeleteTable deletes a table and commits to git.
func (ws *WorkspaceFileStore) DeleteTable(ctx context.Context, id ksid.ID, author git.Author) error {
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
func (ws *WorkspaceFileStore) deleteTable(id ksid.ID) error {
	parentID := ws.getParent(id)
	metadataFile := ws.tableMetadataFile(id, parentID)
	if err := os.Remove(metadataFile); err != nil {
		if os.IsNotExist(err) {
			return ErrTableNotFound
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
func (ws *WorkspaceFileStore) iterTablesRecursive(dir string, parentID ksid.ID, yield func(*Node) bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id, err := ksid.Parse(entry.Name())
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
// Returns QuotaError if the limit is reached.
// MaxTablesPerWorkspace=0 means disabled (0 tables allowed).
func (ws *WorkspaceFileStore) CheckTableQuota() error {
	u, err := ws.scanUsage()
	if err != nil {
		return err
	}
	return quotaGrowth("tables per workspace", int64(u.tables), int64(u.tables+1), int64(ws.quotas.MaxTablesPerWorkspace))
}

// AppendRecord appends a record to a table and commits to git.
func (ws *WorkspaceFileStore) AppendRecord(ctx context.Context, tableID ksid.ID, record *contentv1.DataRecord, author git.Author) error {
	return ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		parentID := ws.getParent(tableID)
		if err := ws.appendRecord(tableID, parentID, record); err != nil {
			return "", nil, err
		}
		files := []string{ws.gitPath(parentID, tableID, "data.jsonl")}
		return "create: record " + record.ID.String(), files, nil
	})
}

// appendRecord appends a record to a table without committing.
func (ws *WorkspaceFileStore) appendRecord(tableID, tableParentID ksid.ID, record *contentv1.DataRecord) error {
	recordsFile := ws.tableRecordsFile(tableID, tableParentID)

	// A direct append can introduce the first JSONL table into an existing
	// page directory. The storage boundary must charge that table slot too.
	if _, err := os.Stat(recordsFile); os.IsNotExist(err) {
		if _, err := os.Stat(ws.tableMetadataFile(tableID, tableParentID)); os.IsNotExist(err) {
			if err := ws.CheckTableQuota(); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	table, err := jsonldb.NewTable[*contentv1.DataRecord](recordsFile)
	if err != nil {
		return fmt.Errorf("failed to open table: %w", err)
	}
	if table.Len() >= ws.quotas.MaxRecordsPerTable {
		return &QuotaError{Resource: "records per table", Limit: int64(ws.quotas.MaxRecordsPerTable)}
	}

	if err := table.Append(record, func(next int64) error { return ws.admitFile(recordsFile, next) }); err != nil {
		return fmt.Errorf("failed to append record: %w", err)
	}
	return nil
}

// IterRecords iterates over all records in a table.
func (ws *WorkspaceFileStore) IterRecords(id ksid.ID) (iter.Seq[*contentv1.DataRecord], error) {
	parentID := ws.getParent(id)
	filePath := ws.tableRecordsFile(id, parentID)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return func(yield func(*contentv1.DataRecord) bool) {}, nil
	}

	table, err := jsonldb.NewTable[*contentv1.DataRecord](filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read records: %w", err)
	}

	return table.Iter(0), nil
}

// CountRecords returns the number of records in a table.
func (ws *WorkspaceFileStore) CountRecords(id ksid.ID) (int, error) {
	parentID := ws.getParent(id)
	filePath := ws.tableRecordsFile(id, parentID)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return 0, nil
	}

	table, err := jsonldb.NewTable[*contentv1.DataRecord](filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to read records: %w", err)
	}

	return table.Len(), nil
}

// ReadRecordsPage reads a page of records for a table using jsonldb abstraction.
func (ws *WorkspaceFileStore) ReadRecordsPage(id ksid.ID, offset, limit int) ([]*contentv1.DataRecord, error) {
	parentID := ws.getParent(id)
	filePath := ws.tableRecordsFile(id, parentID)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return []*contentv1.DataRecord{}, nil
	}

	table, err := jsonldb.NewTable[*contentv1.DataRecord](filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read records: %w", err)
	}

	offset = max(0, offset)
	if offset >= table.Len() {
		return []*contentv1.DataRecord{}, nil
	}
	end := min(offset+limit, table.Len())

	var records []*contentv1.DataRecord
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

// ReplaceRecord conditionally replaces one record using its record and schema revisions.
func (ws *WorkspaceFileStore) ReplaceRecord(ctx context.Context, tableID, recordID ksid.ID, data map[string]any, expectedRevision, expectedSchemaRevision string, author git.Author) (*contentv1.DataRecord, error) {
	var record *contentv1.DataRecord
	err := ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		node, err := ws.ReadTable(tableID)
		if err != nil {
			return "", nil, err
		}
		if node.SchemaRevision != expectedSchemaRevision {
			return "", nil, &SchemaRevisionConflictError{CurrentRevision: node.SchemaRevision}
		}
		parentID := node.ParentID
		table, err := jsonldb.NewTable[*contentv1.DataRecord](ws.tableRecordsFile(tableID, parentID))
		if err != nil {
			return "", nil, fmt.Errorf("open records: %w", err)
		}
		current := table.Get(recordID)
		if current == nil {
			return "", nil, ErrRecordNotFound
		}
		revision, err := RecordRevision(current)
		if err != nil {
			return "", nil, err
		}
		if revision != expectedRevision {
			return "", nil, &RevisionConflictError{CurrentRevision: revision}
		}
		coerced := CoerceRecordData(data, node.Properties)
		if coerced == nil {
			coerced = make(map[string]any)
		}
		record = &contentv1.DataRecord{ID: recordID, Data: coerced, Created: current.Created, Modified: storagev1.Now()}
		canonical, err := CanonicalRecord(record)
		if err != nil {
			return "", nil, err
		}
		if _, err := table.Update(record, func(next int64) error { return ws.admitFile(ws.tableRecordsFile(tableID, parentID), next) }); err != nil {
			return "", nil, err
		}
		record = canonical
		files := []string{ws.gitPath(parentID, tableID, "data.jsonl")}
		return "update: record " + record.ID.String(), files, nil
	})
	if err != nil {
		return nil, err
	}
	return record, nil
}

// DeleteRecord deletes a record only if its current revision matches the caller's snapshot.
func (ws *WorkspaceFileStore) DeleteRecord(ctx context.Context, tableID, recordID ksid.ID, expectedRevision string, author git.Author) error {
	return ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		node, err := ws.ReadTable(tableID)
		if err != nil {
			return "", nil, err
		}
		table, err := jsonldb.NewTable[*contentv1.DataRecord](ws.tableRecordsFile(tableID, node.ParentID))
		if err != nil {
			return "", nil, fmt.Errorf("open records: %w", err)
		}
		current := table.Get(recordID)
		if current == nil {
			return "", nil, ErrRecordNotFound
		}
		revision, err := RecordRevision(current)
		if err != nil {
			return "", nil, err
		}
		if expectedRevision != revision {
			return "", nil, &RevisionConflictError{CurrentRevision: revision}
		}
		if _, err := table.Delete(recordID); err != nil {
			return "", nil, fmt.Errorf("delete record: %w", err)
		}
		return "delete: record " + recordID.String(), []string{ws.gitPath(node.ParentID, tableID, "data.jsonl")}, nil
	})
}

// SaveAsset admits and streams an exact-size asset before committing to git.
// Invalid stream lengths preserve the prior asset and release private staging.
func (ws *WorkspaceFileStore) SaveAsset(ctx context.Context, nodeID ksid.ID, assetName string, data io.Reader, size int64, author git.Author) (*Asset, error) {
	var asset *Asset
	err := ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		parentID := ws.getParent(nodeID)
		var err error
		asset, err = ws.saveAsset(nodeID, parentID, assetName, data, size)
		if err != nil {
			return "", nil, err
		}
		files := []string{ws.gitPath(parentID, nodeID, assetName)}
		return "create: asset " + assetName, files, nil
	})
	return asset, err
}

// saveAsset streams an admitted asset without committing.
func (ws *WorkspaceFileStore) saveAsset(nodeID, parentID ksid.ID, assetName string, data io.Reader, size int64) (*Asset, error) {
	if size < 0 {
		return nil, errors.New("negative asset size")
	}
	filePath, err := ws.assetPath(nodeID, parentID, assetName)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(filePath)
	old, err := fileBytes(filePath)
	if err != nil {
		return nil, err
	}
	if err := quotaGrowth("asset bytes", old, size, ws.quotas.MaxAssetSizeBytes); err != nil {
		return nil, err
	}
	if err := ws.admitFile(filePath, size); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	if err := storage.WriteFileReader(filePath, data, size); err != nil {
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
		Created:  storagev1.ToTime(info.ModTime()),
		Path:     filePath,
	}, nil
}

// ReadAsset reads an asset.
func (ws *WorkspaceFileStore) ReadAsset(nodeID ksid.ID, assetName string) ([]byte, error) {
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
func (ws *WorkspaceFileStore) DeleteAsset(ctx context.Context, nodeID ksid.ID, assetName string, author git.Author) error {
	return ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		parentID := ws.getParent(nodeID)
		if err := ws.deleteAsset(nodeID, parentID, assetName); err != nil {
			return "", nil, err
		}
		files := []string{ws.gitPath(parentID, nodeID, assetName)}
		return "delete: asset " + assetName, files, nil
	})
}

// deleteAsset deletes an asset without committing.
func (ws *WorkspaceFileStore) deleteAsset(nodeID, parentID ksid.ID, assetName string) error {
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
func (ws *WorkspaceFileStore) IterAssets(nodeID ksid.ID) (iter.Seq[*Asset], error) {
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
			if entry.IsDir() || strings.HasPrefix(entry.Name(), storage.AtomicFilePrefix) || entry.Name() == "index.md" || entry.Name() == "metadata.json" || entry.Name() == "data.jsonl" || strings.HasSuffix(entry.Name(), ".blobs") {
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
				Created:  storagev1.ToTime(info.ModTime()),
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
func (ws *WorkspaceFileStore) GetHistory(ctx context.Context, id ksid.ID, n int) ([]*git.Commit, error) {
	if id.IsZero() {
		return nil, ErrNodeNotFound // No node with ID 0 exists
	}
	// Use the full relative path including parent chain, otherwise nested nodes won't find their history.
	parentID := ws.getParent(id)
	path := ws.relativeDir(id, parentID)
	return ws.repo.GetHistory(ctx, path, n)
}

// GetPageContentAtCommit returns the content of a node's index.md file at a specific commit,
// with front matter stripped. This correctly handles nested nodes by resolving the git path.
func (ws *WorkspaceFileStore) GetPageContentAtCommit(ctx context.Context, hash string, id ksid.ID) (string, error) {
	parentID := ws.getParent(id)
	path := ws.gitPath(parentID, id, "index.md")
	data, err := ws.repo.GetFileAtCommit(ctx, hash, path)
	if err != nil {
		return "", err
	}
	p := ParseMarkdown(data)
	return p.content, nil
}

// CreateNode creates a new node (can be document, table, or hybrid) and commits to git.
// If parentID is zero, creates a top-level node in the workspace.
// Otherwise, creates a child under the specified parent node.
func (ws *WorkspaceFileStore) CreateNode(ctx context.Context, title string, nodeType NodeType, parentID ksid.ID, author git.Author) (*Node, error) {
	// Verify parent exists if specified.
	if !parentID.IsZero() && !ws.PageExists(parentID) && !ws.TableExists(parentID) {
		return nil, fmt.Errorf("parent node not found: %w", ErrNodeNotFound)
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
func (ws *WorkspaceFileStore) createNode(title string, nodeType NodeType, parentID ksid.ID) (*Node, []string, error) {
	if nodeType == NodeTypeDocument || nodeType == NodeTypeHybrid {
		if err := ws.checkPageQuota(); err != nil {
			return nil, nil, err
		}
	}
	if nodeType == NodeTypeTable || nodeType == NodeTypeHybrid {
		if err := ws.CheckTableQuota(); err != nil {
			return nil, nil, err
		}
	}

	id := ksid.NewID()
	now := storagev1.Now()

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
			"properties": []contentv1.Property{},
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
		if err := storage.WriteFile(filePath, pageData); err != nil {
			return nil, nil, fmt.Errorf("failed to write page: %w", err)
		}
		files = append(files, ws.gitPath(parentID, id, "index.md"))
	}

	if nodeType == NodeTypeTable || nodeType == NodeTypeHybrid {
		if err := os.MkdirAll(ws.pageDir(id, parentID), 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
			return nil, nil, fmt.Errorf("failed to create directory: %w", err)
		}

		metadataFile := ws.tableMetadataFile(id, parentID)
		if err := storage.WriteFile(metadataFile, metadataData); err != nil {
			return nil, nil, fmt.Errorf("failed to write metadata: %w", err)
		}
		files = append(files, ws.gitPath(parentID, id, "metadata.json"))
	}

	ws.setParent(id, parentID)

	return node, files, nil
}

// MoveNode reparents a node to a new parent (or root if newParentID is zero).
// It validates that the node exists, the new parent exists (unless root),
// and that the move would not create a cycle in the tree.
func (ws *WorkspaceFileStore) MoveNode(ctx context.Context, id, newParentID ksid.ID, author git.Author) (move NodeMove, err error) {
	err = ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		if id.IsZero() {
			return "", nil, ErrNodeNotFound
		}

		// Verify node exists.
		oldParentID := ws.getParent(id)
		if !ws.PageExists(id) && !ws.TableExists(id) {
			return "", nil, ErrNodeNotFound
		}

		// Verify new parent exists (unless root).
		if !newParentID.IsZero() && !ws.PageExists(newParentID) && !ws.TableExists(newParentID) {
			return "", nil, fmt.Errorf("new parent not found: %w", ErrNodeNotFound)
		}

		// No-op if already under the requested parent.
		if oldParentID == newParentID {
			return "", nil, nil
		}

		// Cycle check: walk from newParentID up to root; if we hit id, it's a cycle.
		for p := newParentID; !p.IsZero(); p = ws.getParent(p) {
			if p == id {
				return "", nil, errCycleDetected
			}
		}

		oldDir := ws.pageDir(id, oldParentID)
		oldRelDir := ws.relativeDir(id, oldParentID)

		// Compute new directory before moving (uses current cache state for newParentID chain).
		newRelDir := ws.relativeDir(id, newParentID)
		newDir := filepath.Join(ws.wsDir, newRelDir)

		if err := os.Rename(oldDir, newDir); err != nil {
			return "", nil, fmt.Errorf("failed to move node: %w", err)
		}
		ws.setParent(id, newParentID)
		move = NodeMove{OldParentID: oldParentID, ParentID: newParentID, Changed: true}

		files := []string{oldRelDir, newRelDir}
		return "move: node " + id.String() + " to parent " + newParentID.String(), files, nil
	})
	return move, err
}

// Repo returns the git Repository for the workspace. This is exported for handlers
// that need direct git operations (e.g., git remotes).
func (ws *WorkspaceFileStore) Repo() git.Repository {
	return ws.repo
}

// HasPage checks if a node has page content (index.md exists).
func (ws *WorkspaceFileStore) HasPage(id ksid.ID) bool {
	parentID := ws.getParent(id)
	filePath := ws.pageIndexFile(id, parentID)
	_, err := os.Stat(filePath)
	return err == nil
}

// HasTable checks if a node has table content (metadata.json exists).
func (ws *WorkspaceFileStore) HasTable(id ksid.ID) bool {
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
func (ws *WorkspaceFileStore) CreatePageUnderParent(ctx context.Context, parentID ksid.ID, title, content string, author git.Author) (*Node, error) {
	// Verify parent exists if specified.
	if !parentID.IsZero() && !ws.PageExists(parentID) && !ws.TableExists(parentID) {
		return nil, fmt.Errorf("parent node not found: %w", ErrNodeNotFound)
	}

	var node *Node
	err := ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := ws.checkPageQuota(); err != nil {
			return "", nil, err
		}
		id := ksid.NewID()
		now := storagev1.Now()

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
		if err := storage.WriteFile(filePath, pageData); err != nil {
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
	if err == nil {
		ws.links.update(node.ID, node.Content)
	}
	return node, err
}

// DeletePageFromNode removes the page content (index.md) from a node.
// The node directory is kept if table content exists.
func (ws *WorkspaceFileStore) DeletePageFromNode(ctx context.Context, id ksid.ID, author git.Author) (remaining *Node, deleted NodeDeletion, err error) {
	if id.IsZero() {
		return nil, deleted, ErrNodeNotFound // No node with ID 0 exists
	}

	err = ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		parentID := ws.getParent(id)
		deleted = NodeDeletion{NodeID: id, ParentID: parentID, IDs: []ksid.ID{id}}
		before, err := ws.readNodeSummary(ctx, id)
		if err != nil {
			return "", nil, err
		}
		if before.Type == NodeTypeHybrid {
			remaining = before
			remaining.Type = NodeTypeTable
			remaining.Icon, remaining.Cover = "", ""
			remaining.Changes = NodeChanges{Body: true, Metadata: true, Type: true}
		}
		filePath := ws.pageIndexFile(id, parentID)
		if err := os.Remove(filePath); err != nil {
			if os.IsNotExist(err) {
				return "", nil, ErrNodeNotFound
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
			} else {
				ws.hideNode(id)
			}
		}

		files := []string{ws.gitPath(parentID, id, "index.md")}
		return "delete: page " + id.String(), files, nil
	})
	if err == nil {
		ws.links.remove(id)
	}
	return remaining, deleted, err
}

// --- Table-specific operations ---

// CreateTableUnderParent creates a new table under a parent node and commits to git.
// Returns the new node with the table schema.
func (ws *WorkspaceFileStore) CreateTableUnderParent(ctx context.Context, parentID ksid.ID, title string, properties []contentv1.Property, author git.Author) (*Node, error) {
	// Verify parent exists if parentID is specified (non-root)
	if !parentID.IsZero() && !ws.PageExists(parentID) && !ws.TableExists(parentID) {
		return nil, fmt.Errorf("parent node not found: %w", ErrNodeNotFound)
	}

	var node *Node
	err := ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		if err := ws.CheckTableQuota(); err != nil {
			return "", nil, err
		}
		if err := quotaGrowth("columns per table", 0, int64(len(properties)), int64(ws.quotas.MaxColumnsPerTable)); err != nil {
			return "", nil, err
		}
		id := ksid.NewID()
		now := storagev1.Now()

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
		if err := storage.WriteFile(metadataFile, metadataData); err != nil {
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
func (ws *WorkspaceFileStore) DeleteTableFromNode(ctx context.Context, id ksid.ID, author git.Author) (remaining *Node, deleted NodeDeletion, err error) {
	if id.IsZero() {
		return nil, deleted, ErrTableNotFound // No node with ID 0 exists
	}

	err = ws.repo.CommitTx(ctx, author, func() (string, []string, error) {
		parentID := ws.getParent(id)
		deleted = NodeDeletion{NodeID: id, ParentID: parentID, IDs: []ksid.ID{id}}
		before, err := ws.readNodeSummary(ctx, id)
		if err != nil {
			return "", nil, err
		}
		if before.Type == NodeTypeHybrid {
			var header *page
			remaining, header, err = ws.readSummary(ctx, ws.pageDir(id, parentID), id, parentID)
			if err != nil {
				return "", nil, err
			}
			remaining.Title, remaining.Type = header.title, NodeTypeDocument
			remaining.Properties, remaining.Views = nil, nil
			remaining.Changes = NodeChanges{Schema: true, Metadata: true, Type: true, Title: before.Title != header.title}
		}
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
			} else {
				ws.hideNode(id)
			}
		}

		return "delete: table " + id.String(), files, nil
	})
	return remaining, deleted, err
}

// ListChildren returns all direct children of a node.
// In single-root model:
//   - If parentID is zero (root), returns children from workspace dir.
//   - Otherwise, returns children from parent's subdirectory.
func (ws *WorkspaceFileStore) ListChildren(parentID ksid.ID) ([]*Node, error) {
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
		id, err := ksid.Parse(entry.Name())
		if err != nil {
			continue
		}

		nodePath := filepath.Join(dir, entry.Name())
		node, _, err := ws.readNodeFromPath(nodePath, id, parentID)
		if err != nil {
			continue
		}

		// Check if this child has its own children
		childEntries, _ := os.ReadDir(nodePath)
		for _, childEntry := range childEntries {
			if childEntry.IsDir() {
				if _, decErr := ksid.Parse(childEntry.Name()); decErr == nil {
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

// ParseMarkdown parses a markdown file with optional YAML front matter.
func ParseMarkdown(data []byte) *page {
	content := string(data)
	var title, icon, cover string
	var created, modified storagev1.Time

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
						created = storagev1.ToTime(t)
					}
				case strings.HasPrefix(line, "modified:"):
					dateStr := strings.TrimSpace(strings.TrimPrefix(line, "modified:"))
					if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
						modified = storagev1.ToTime(t)
					}
				case strings.HasPrefix(line, "icon:"):
					icon = strings.TrimSpace(strings.TrimPrefix(line, "icon:"))
				case strings.HasPrefix(line, "cover:"):
					cover = strings.TrimSpace(strings.TrimPrefix(line, "cover:"))
				}
			}
		}
	}

	if created.IsZero() {
		created = storagev1.Now()
	}
	if modified.IsZero() {
		modified = storagev1.Now()
	}

	return &page{
		title:    title,
		content:  content,
		created:  created,
		modified: modified,
		icon:     icon,
		cover:    cover,
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
	if p.icon != "" {
		buf.WriteString("icon: " + p.icon + "\n")
	}
	if p.cover != "" {
		buf.WriteString("cover: " + p.cover + "\n")
	}
	buf.WriteString("---")
	buf.WriteString("\n\n")
	buf.WriteString(p.content)
	return buf.Bytes()
}

// --- Page Links Extraction and Backlinks Index ---

// ExtractLinkedNodeIDs extracts all node IDs from relative path links in markdown content.
// Matches links like [text](../nodeID/index.md) and extracts the nodeID from the directory name.
func ExtractLinkedNodeIDs(content string) []ksid.ID {
	matches := relativeLinkRe.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	var ids []ksid.ID

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		href := match[2]
		if strings.HasPrefix(href, "/") || strings.HasPrefix(href, "http") {
			continue
		}
		nodeIDStr := filepath.Base(filepath.Dir(href))
		if seen[nodeIDStr] {
			continue
		}
		seen[nodeIDStr] = true
		id, err := ksid.Parse(nodeIDStr)
		if err == nil && !id.IsZero() {
			ids = append(ids, id)
		}
	}
	return ids
}

// GetBacklinks returns all nodes that link to the given node.
// Uses an in-memory cache that is lazily built on first call and
// incrementally updated on page mutations.
func (ws *WorkspaceFileStore) GetBacklinks(ctx context.Context, targetID ksid.ID) ([]BacklinkInfo, error) {
	if err := ws.links.ensureBuilt(ws.IterPages); err != nil {
		return nil, err
	}
	sourceIDs := ws.links.backlinks(targetID)
	backlinks := make([]BacklinkInfo, 0, len(sourceIDs))
	for _, srcID := range sourceIDs {
		node, err := ws.ReadNode(ctx, srcID)
		if err != nil {
			continue // node may have been deleted between cache and read
		}
		backlinks = append(backlinks, BacklinkInfo{NodeID: srcID, Title: node.Title})
	}
	return backlinks, nil
}

// GetNodeTitles returns a map of node IDs to their titles for the given IDs.
func (ws *WorkspaceFileStore) GetNodeTitles(ctx context.Context, ids []ksid.ID) (map[ksid.ID]string, error) {
	titles := make(map[ksid.ID]string)
	for _, id := range ids {
		node, err := ws.ReadNode(ctx, id)
		if err != nil {
			// Node might have been deleted, skip it
			continue
		}
		titles[id] = node.Title
	}
	return titles, nil
}

// ReadPageSnapshot returns a complete schema and at most limit UTF-8 body bytes.
// A bounded digest cache reuses verified revisions across windows. Repository
// transactions invalidate changed files; atomic import replacement is detected
// through opened-file identity. Arbitrary external in-place writers are outside
// the saved-file snapshot contract. Metadata is read fresh on every request.
func (ws *WorkspaceFileStore) ReadPageSnapshot(ctx context.Context, id ksid.ID, offset int64, limit int) (out PageSnapshot, err error) {
	if err := validateContentWindow(offset, limit); err != nil {
		return out, err
	}
	err = ws.repo.ReadTx(ctx, func(saved *git.SavedFiles) (snapshotErr error) {
		parent, dir, e := ws.indexedNodePath(ctx, id)
		if e != nil {
			return e
		}
		n, p, e := ws.readSummary(ctx, dir, id, parent)
		if e != nil {
			return e
		}
		out.Node = *n
		if p == nil {
			if offset != 0 {
				return errors.New("table has no content continuation")
			}
			return nil
		}
		f, e := os.Open(filepath.Join(dir, "index.md")) //nolint:gosec // Validated node IDs construct the owned path.
		if e != nil {
			return e
		}
		defer func() { snapshotErr = errors.Join(snapshotErr, f.Close()) }()
		// Parse the same opened file used for hashing, including its canonical title.
		p, start, size, e := pageHeader(ctx, f)
		if e != nil {
			return e
		}
		out.Node.Content = ""
		st, e := f.Stat()
		if e != nil {
			return e
		}
		version := saved.Version(filepath.ToSlash(ws.gitPath(parent, id, "index.md")))
		cached, ok := ws.snapshotRevisions[id]
		if ok && cached.version == version && os.SameFile(cached.info, st) &&
			cached.info.Size() == st.Size() && cached.info.ModTime().Equal(st.ModTime()) {
			out.Revision = cached.revision
			start = cached.bodyStart
		} else {
			start, e = canonicalBodyStart(ctx, f, start, size)
			if e != nil {
				return e
			}
			out.Revision, e = Revision(ctx, p.title, io.NewSectionReader(f, start, size-start), size-start)
			if e != nil {
				return e
			}
			if len(ws.snapshotRevisions) >= 128 {
				clear(ws.snapshotRevisions)
			}
			if ws.snapshotRevisions == nil {
				ws.snapshotRevisions = make(map[ksid.ID]snapshotRevision)
			}
			ws.snapshotRevisions[id] = snapshotRevision{version: version, info: st, revision: out.Revision, bodyStart: start}
		}
		out.ContentBytes = size - start
		if offset > out.ContentBytes {
			return errors.New("content offset is outside document")
		}
		chunk := make([]byte, min(int64(limit+utf8.UTFMax), out.ContentBytes-offset))
		_, e = f.ReadAt(chunk, start+offset)
		if e != nil {
			return e
		}
		out.Node.Content, out.NextOffset, e = contentWindow(string(chunk), offset, limit)
		if e != nil {
			return e
		}
		out.Node.Created = p.created
		out.Node.Modified = p.modified
		out.Node.Icon = p.icon
		out.Node.Cover = p.cover
		if n.Type == NodeTypeDocument {
			out.Node.Title = p.title
		}
		return nil
	})
	return out, err
}

// ListNodeSummaries pages all nodes when parent is nil, otherwise direct children.
// Ordering is stable ascending node ID. Only the selected headers are opened.
func (ws *WorkspaceFileStore) ListNodeSummaries(ctx context.Context, parent *ksid.ID, after ksid.ID, limit int) (out NodeSummaryPage, err error) {
	err = ws.repo.ReadTx(ctx, func(*git.SavedFiles) error {
		var e error
		out, e = ws.listNodeSummaries(ctx, parent, after, limit)
		return e
	})
	return out, err
}

func (ws *WorkspaceFileStore) listNodeSummaries(ctx context.Context, parent *ksid.ID, after ksid.ID, limit int) (out NodeSummaryPage, err error) {
	if limit < 1 || limit > 100 {
		return out, errors.New("invalid node page size")
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	ws.mu.RLock()
	ready := ws.indexReady
	ws.mu.RUnlock()
	if !ready {
		if err := ws.scanIndex(ctx.Err); err != nil {
			return out, err
		}
	}
	ws.mu.RLock()
	ids := ws.nodeIDs
	if parent != nil {
		ids = ws.childIDs[*parent]
	}
	out.Total = len(ids)
	at, _ := slices.BinarySearch(ids, after)
	for at < len(ids) && ids[at] <= after {
		at++
	}
	selected := slices.Clone(ids[at:min(at+limit, len(ids))])
	more := at+len(selected) < len(ids)
	ws.mu.RUnlock()
	out.Nodes = make([]*Node, 0, len(selected))
	for _, id := range selected {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		n, e := ws.readNodeSummary(ctx, id)
		if e != nil {
			return out, fmt.Errorf("read node %s summary: %w", id, e)
		}
		out.Nodes = append(out.Nodes, n)
	}
	if more && len(selected) > 0 {
		out.Next = selected[len(selected)-1]
	}
	return out, nil
}

// NodeParent reads indexed ancestry without loading node content or metadata.
// It shares the repository read boundary with the discovery projection.
func (ws *WorkspaceFileStore) NodeParent(ctx context.Context, id ksid.ID) (parent ksid.ID, err error) {
	err = ws.repo.ReadTx(ctx, func(*git.SavedFiles) error {
		parent, _, err = ws.indexedNodePath(ctx, id)
		return err
	})
	return parent, err
}

// ReadNodeSummary reads bounded metadata without loading or hashing the body.
func (ws *WorkspaceFileStore) ReadNodeSummary(ctx context.Context, id ksid.ID) (out *Node, err error) {
	err = ws.repo.ReadTx(ctx, func(*git.SavedFiles) error { out, err = ws.readNodeSummary(ctx, id); return err })
	return out, err
}

func (ws *WorkspaceFileStore) readNodeSummary(ctx context.Context, id ksid.ID) (*Node, error) {
	p, dir, err := ws.indexedNodePath(ctx, id)
	if err != nil {
		return nil, err
	}
	n, _, err := ws.readSummary(ctx, dir, id, p)
	return n, err
}

func (ws *WorkspaceFileStore) readSummary(ctx context.Context, dir string, id, parent ksid.ID) (node *Node, header *page, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	n := &Node{ID: id, ParentID: parent, Type: NodeTypeDocument}
	var p *page
	f, err := os.Open(filepath.Join(dir, "index.md")) //nolint:gosec // The node directory is constructed from validated IDs.
	if err == nil {
		p, _, _, err = pageHeader(ctx, f)
		err = errors.Join(err, f.Close())
		if err != nil {
			return nil, nil, err
		}
		n.Title = p.title
		n.Created = p.created
		n.Modified = p.modified
		n.Icon = p.icon
		n.Cover = p.cover
	} else if !os.IsNotExist(err) {
		return nil, nil, err
	}
	f, err = os.Open(filepath.Join(dir, "metadata.json")) //nolint:gosec // The node directory is constructed from validated IDs.
	switch {
	case err == nil:
		data, e := io.ReadAll(io.LimitReader(f, MaxSnapshotMetadataBytes+1))
		e = errors.Join(e, f.Close())
		if e != nil {
			return nil, nil, e
		}
		if len(data) > MaxSnapshotMetadataBytes {
			return nil, nil, errors.New("table metadata exceeds snapshot limit")
		}
		var m map[string]json.RawMessage
		if e := json.Unmarshal(data, &m); e != nil {
			return nil, nil, e
		}
		if b := m["version"]; b != nil {
			var v any
			if e := json.Unmarshal(b, &v); e != nil {
				return nil, nil, e
			}
			if e := checkTableMetadataVersion(v); e != nil {
				return nil, nil, e
			}
		}
		if b := m["title"]; b != nil {
			if e := json.Unmarshal(b, &n.Title); e != nil {
				return nil, nil, e
			}
		}
		if b := m["properties"]; b != nil {
			if e := json.Unmarshal(b, &n.Properties); e != nil {
				return nil, nil, e
			}
		}
		if b := m["views"]; b != nil {
			if e := json.Unmarshal(b, &n.Views); e != nil {
				return nil, nil, e
			}
		}
		n.SchemaRevision, e = tableSchemaRevision(n.Properties)
		if e != nil {
			return nil, nil, e
		}
		n.Type = NodeTypeTable
		if p != nil {
			n.Type = NodeTypeHybrid
		}
	case !os.IsNotExist(err):
		return nil, nil, err
	case p == nil:
		return nil, nil, ErrNodeNotFound
	}
	ws.mu.RLock()
	n.HasChildren = len(ws.childIDs[id]) > 0
	ws.mu.RUnlock()
	return n, p, nil
}

func (ws *WorkspaceFileStore) rebuildIndexLocked() {
	ws.nodeIDs = make([]ksid.ID, 0, len(ws.discoverable))
	ws.childIDs = make(map[ksid.ID][]ksid.ID)
	for id := range ws.discoverable {
		p := ws.indexParents[id]
		ws.nodeIDs = append(ws.nodeIDs, id)
		ws.childIDs[p] = append(ws.childIDs[p], id)
	}
	slices.Sort(ws.nodeIDs)
	for _, ids := range ws.childIDs {
		slices.Sort(ids)
	}
	ws.indexReady = true
}

func (ws *WorkspaceFileStore) setIndexParentLocked(id, parent ksid.ID) {
	ws.cache[id] = parent
	if !ws.indexReady {
		return
	}
	old, ok := ws.indexParents[id]
	ws.indexParents[id] = parent
	ws.discoverable[id] = struct{}{}
	if ok {
		ws.pathChildren[old] = removeID(ws.pathChildren[old], id)
		ws.childIDs[old] = removeID(ws.childIDs[old], id)
	}
	ws.pathChildren[parent] = insertID(ws.pathChildren[parent], id)
	ws.nodeIDs = insertID(ws.nodeIDs, id)
	ws.childIDs[parent] = insertID(ws.childIDs[parent], id)
}

// hideNode keeps directory ancestry while withdrawing its missing content.
func (ws *WorkspaceFileStore) hideNode(id ksid.ID) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if !ws.indexReady {
		return
	}
	delete(ws.discoverable, id)
	ws.nodeIDs = removeID(ws.nodeIDs, id)
	p := ws.indexParents[id]
	ws.childIDs[p] = removeID(ws.childIDs[p], id)
}

func (ws *WorkspaceFileStore) deleteIndexSubtreeLocked(id ksid.ID) {
	if !ws.indexReady {
		children := make(map[ksid.ID][]ksid.ID)
		for child, p := range ws.cache {
			children[p] = append(children[p], child)
		}
		pending := []ksid.ID{id}
		for len(pending) > 0 {
			at := len(pending) - 1
			node := pending[at]
			pending = pending[:at]
			pending = append(pending, children[node]...)
			delete(ws.cache, node)
		}
		return
	}
	for len(ws.pathChildren[id]) > 0 {
		ws.deleteIndexSubtreeLocked(ws.pathChildren[id][0])
	}
	p := ws.indexParents[id]
	delete(ws.cache, id)
	delete(ws.indexParents, id)
	delete(ws.discoverable, id)
	delete(ws.pathChildren, id)
	ws.pathChildren[p] = removeID(ws.pathChildren[p], id)
	delete(ws.childIDs, id)
	ws.childIDs[p] = removeID(ws.childIDs[p], id)
	ws.nodeIDs = removeID(ws.nodeIDs, id)
}

// indexedNodePath resolves exclusively through published ancestry while the
// caller holds the repository read transaction. Legacy misses cannot change it.
func (ws *WorkspaceFileStore) indexedNodePath(ctx context.Context, id ksid.ID) (parent ksid.ID, dir string, err error) {
	ws.mu.RLock()
	ready := ws.indexReady
	ws.mu.RUnlock()
	if !ready {
		if err := ws.scanIndex(ctx.Err); err != nil {
			return 0, "", err
		}
	}
	ws.mu.RLock()
	p, found := ws.indexParents[id]
	if !found {
		ws.mu.RUnlock()
		return 0, "", ErrNodeNotFound
	}
	var parts []string
	for node := id; node != 0; node = ws.indexParents[node] {
		parts = append(parts, node.String())
	}
	ws.mu.RUnlock()
	slices.Reverse(parts)
	return p, filepath.Join(ws.wsDir, filepath.Join(parts...)), nil
}

// scanUsage observes current content, never Git/admin files or private staging.
// Callers admitting growth hold the repository mutation gate through publication;
// no independently cached counter can grant capacity after a store replacement.
func (ws *WorkspaceFileStore) scanUsage() (out workspaceUsage, err error) {
	tables := make(map[string]struct{})
	err = filepath.WalkDir(ws.wsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == ws.wsDir {
			return nil
		}
		name := entry.Name()
		rel, err := filepath.Rel(ws.wsDir, path)
		if err != nil {
			return err
		}
		if filepath.Dir(rel) == "." {
			if _, err := ksid.Parse(name); err != nil || !entry.IsDir() {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if name == ".git" || strings.HasPrefix(name, storage.AtomicFilePrefix) || strings.HasPrefix(name, ".jsonldb-") || (name == "tmp" && strings.HasSuffix(filepath.Dir(path), ".blobs")) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("cannot account non-regular content %s", rel)
		}
		out.bytes += info.Size()
		if _, parseErr := ksid.Parse(filepath.Base(filepath.Dir(path))); parseErr == nil {
			if name == "index.md" {
				out.pages++
			}
			if name == "metadata.json" || name == "data.jsonl" {
				tables[filepath.Dir(path)] = struct{}{}
			}
		}
		return nil
	})
	out.tables = len(tables)
	return out, err
}

// admitFile checks the serialized replacement, including headers and newlines.
// Staged bytes are excluded from the scan and never become retained usage until
// publication. Callers must hold the repository mutation gate until then.
func (ws *WorkspaceFileStore) admitFile(path string, next int64) error {
	old, err := fileBytes(path)
	if err != nil {
		return err
	}
	return ws.checkStorageQuota(next - old)
}

func (ws *WorkspaceFileStore) checkTableShape(id ksid.ID, properties []contentv1.Property) error {
	before := 0
	old, err := ws.ReadTable(id)
	if err == nil {
		before = len(old.Properties)
	} else if !errors.Is(err, ErrTableNotFound) {
		return err
	}
	if err := quotaGrowth("columns per table", int64(before), int64(len(properties)), int64(ws.quotas.MaxColumnsPerTable)); err != nil {
		return err
	}
	if errors.Is(err, ErrTableNotFound) {
		if _, err := os.Stat(ws.tableRecordsFile(id, ws.getParent(id))); os.IsNotExist(err) {
			return ws.CheckTableQuota()
		} else if err != nil {
			return err
		}
	}

	return nil
}

// AssetSize observes the exact regular-file size for replacement accounting,
// including assets omitted from presentation listings. Missing assets use zero.
func (ws *WorkspaceFileStore) AssetSize(ctx context.Context, nodeID ksid.ID, name string) (size int64, err error) {
	err = ws.repo.ReadTx(ctx, func(*git.SavedFiles) error {
		path, e := ws.assetPath(nodeID, ws.getParent(nodeID), name)
		if e != nil {
			return e
		}
		info, e := os.Stat(path)
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return errors.New("asset is not a regular file")
		}
		size = info.Size()
		return nil
	})
	return
}

func (ws *WorkspaceFileStore) assetPath(nodeID, parentID ksid.ID, name string) (string, error) {
	if nodeID.IsZero() {
		return "", ErrNodeNotFound
	}
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, "/\\") || name == "index.md" || name == "metadata.json" || name == "data.jsonl" || name == ".git" || strings.HasPrefix(name, storage.AtomicFilePrefix) || strings.HasPrefix(name, ".jsonldb-") {
		return "", errors.New("invalid asset name")
	}
	return filepath.Join(ws.pageDir(nodeID, parentID), name), nil
}
