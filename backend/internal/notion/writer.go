// Writes extracted Notion data to mddb storage format.

package notion

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
)

// Writer writes extracted data to mddb storage format.
type Writer struct {
	OutputDir   string
	WorkspaceID string

	mu     sync.Mutex
	tables map[ksid.ID]*jsonldb.Table[*content.DataRecord]
}

// NewWriter creates a new writer for the given output directory and workspace.
func NewWriter(outputDir, workspaceID string) *Writer {
	return &Writer{
		OutputDir:   outputDir,
		WorkspaceID: workspaceID,
		tables:      make(map[ksid.ID]*jsonldb.Table[*content.DataRecord]),
	}
}

// workspacePath returns the path to the workspace directory.
func (w *Writer) workspacePath() string {
	return filepath.Join(w.OutputDir, w.WorkspaceID)
}

// nodePath returns the path to a node's directory.
func (w *Writer) nodePath(nodeID ksid.ID) string {
	return filepath.Join(w.workspacePath(), nodeID.String())
}

// EnsureWorkspace creates the workspace directory if it doesn't exist.
func (w *Writer) EnsureWorkspace() error {
	return os.MkdirAll(w.workspacePath(), 0o755) //nolint:gosec // G301: 0o755 is intentional for data directories
}

// WriteNode writes a node (page or table) to the filesystem.
func (w *Writer) WriteNode(node *content.Node, markdownContent string) error {
	nodeDir := w.nodePath(node.ID)
	if err := os.MkdirAll(nodeDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return fmt.Errorf("failed to create node directory: %w", err)
	}

	// Write index.md for documents/hybrids
	if node.Type == content.NodeTypeDocument || node.Type == content.NodeTypeHybrid {
		if err := w.writeMarkdown(nodeDir, node.Title, markdownContent); err != nil {
			return err
		}
	}

	// Write metadata.json for tables/hybrids (views only, properties go in data.jsonl)
	if node.Type == content.NodeTypeTable || node.Type == content.NodeTypeHybrid {
		if err := w.writeMetadata(nodeDir, node); err != nil {
			return err
		}
	}

	return nil
}

// writeMarkdown writes the index.md file with front matter.
func (w *Writer) writeMarkdown(nodeDir, title, mdContent string) error {
	path := filepath.Join(nodeDir, "index.md")

	// Create markdown with YAML front matter
	md := fmt.Sprintf("---\ntitle: %q\n---\n\n%s", title, mdContent)

	return os.WriteFile(path, []byte(md), 0o644) //nolint:gosec // G306: 0o644 is intentional for readable files
}

// writeMetadata writes the metadata.json file (views only, properties go in data.jsonl).
func (w *Writer) writeMetadata(nodeDir string, node *content.Node) error {
	// Only write metadata.json if there are views
	if len(node.Views) == 0 {
		return nil
	}

	path := filepath.Join(nodeDir, "metadata.json")
	meta := TableMetadata{
		Views: node.Views,
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	return os.WriteFile(path, data, 0o644) //nolint:gosec // G306: 0o644 is intentional for readable files
}

// TableMetadata is the structure stored in metadata.json.
type TableMetadata struct {
	Views []content.View `json:"views,omitempty"`
}

// NodeEntry is a manifest entry for a node (stored in nodes.jsonl).
type NodeEntry struct {
	ID       ksid.ID      `json:"id"`
	ParentID ksid.ID      `json:"parent_id,omitempty"`
	Title    string       `json:"title"`
	Type     string       `json:"type"`
	Icon     string       `json:"icon,omitempty"`
	Cover    string       `json:"cover,omitempty"`
	Created  storage.Time `json:"created"`
	Modified storage.Time `json:"modified"`
}

// ClearNodesManifest removes the nodes.jsonl file to prepare for a fresh import.
// Call this at the start of an import to avoid duplicate entries.
func (w *Writer) ClearNodesManifest() error {
	path := filepath.Join(w.workspacePath(), "nodes.jsonl")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove nodes.jsonl: %w", err)
	}
	return nil
}

// WriteNodeEntry appends a node entry to the manifest file (nodes.jsonl).
func (w *Writer) WriteNodeEntry(node *content.Node) (rerr error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	path := filepath.Join(w.workspacePath(), "nodes.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // G304: path is constructed from validated input
	if err != nil {
		return fmt.Errorf("failed to open nodes.jsonl: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && rerr == nil {
			rerr = fmt.Errorf("failed to close nodes.jsonl: %w", cerr)
		}
	}()

	entry := NodeEntry{
		ID:       node.ID,
		ParentID: node.ParentID,
		Title:    node.Title,
		Type:     string(node.Type),
		Icon:     node.Icon,
		Cover:    node.Cover,
		Created:  node.Created,
		Modified: node.Modified,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal node entry: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write node entry: %w", err)
	}

	return nil
}

// ClearNodeData removes the data.jsonl file for a node to prepare for re-import.
func (w *Writer) ClearNodeData(nodeID ksid.ID) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Remove from cache if present
	delete(w.tables, nodeID)

	nodeDir := w.nodePath(nodeID)
	path := filepath.Join(nodeDir, "data.jsonl")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove data.jsonl: %w", err)
	}
	return nil
}

// getTable returns the jsonldb.Table for a node, creating it if needed.
func (w *Writer) getTable(nodeID ksid.ID) (*jsonldb.Table[*content.DataRecord], error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if table, ok := w.tables[nodeID]; ok {
		return table, nil
	}

	nodeDir := w.nodePath(nodeID)
	path := filepath.Join(nodeDir, "data.jsonl")

	table, err := jsonldb.NewTable[*content.DataRecord](path)
	if err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	w.tables[nodeID] = table
	return table, nil
}

// WriteRecords writes records to a table's data.jsonl file using jsonldb.Table.
// Properties are stored in the table header for schema-aware deserialization.
func (w *Writer) WriteRecords(nodeID ksid.ID, properties []content.Property, records []*content.DataRecord) error {
	table, err := w.getTable(nodeID)
	if err != nil {
		return err
	}

	// Store properties in the table header
	if len(properties) > 0 {
		propsJSON, err := json.Marshal(properties)
		if err != nil {
			return fmt.Errorf("failed to marshal properties: %w", err)
		}
		if err := table.SetProperties(propsJSON); err != nil {
			return fmt.Errorf("failed to set properties: %w", err)
		}
	}

	for _, record := range records {
		if err := table.Append(record); err != nil {
			return fmt.Errorf("failed to append record: %w", err)
		}
	}

	return nil
}

// AppendRecord appends a single record to a table's data.jsonl file.
func (w *Writer) AppendRecord(nodeID ksid.ID, record *content.DataRecord) error {
	table, err := w.getTable(nodeID)
	if err != nil {
		return err
	}

	if err := table.Append(record); err != nil {
		return fmt.Errorf("failed to append record: %w", err)
	}

	return nil
}

// IDMapping stores the Notion ID to mddb ID mapping for incremental imports.
type IDMapping struct {
	Version int                `json:"version"`
	IDs     map[string]ksid.ID `json:"ids"` // Notion ID -> mddb ID
}

// idMappingPath returns the path to the ID mapping file.
func (w *Writer) idMappingPath() string {
	return filepath.Join(w.workspacePath(), "notion_id_mapping.json")
}

// LoadIDMapping loads the ID mapping from disk if it exists.
// Returns an empty map if the file doesn't exist.
func (w *Writer) LoadIDMapping() (map[string]ksid.ID, error) {
	path := w.idMappingPath()
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed from validated input
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]ksid.ID), nil
		}
		return nil, fmt.Errorf("failed to read ID mapping: %w", err)
	}

	var mapping IDMapping
	if err := json.Unmarshal(data, &mapping); err != nil {
		return nil, fmt.Errorf("failed to parse ID mapping: %w", err)
	}

	return mapping.IDs, nil
}

// SaveIDMapping saves the ID mapping to disk.
func (w *Writer) SaveIDMapping(ids map[string]ksid.ID) error {
	mapping := IDMapping{
		Version: 1,
		IDs:     ids,
	}

	data, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal ID mapping: %w", err)
	}

	path := w.idMappingPath()
	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional
		return fmt.Errorf("failed to write ID mapping: %w", err)
	}

	return nil
}
