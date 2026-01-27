// Writes extracted Notion data to mddb storage format.

package notion

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/content"
)

// Writer writes extracted data to mddb storage format.
type Writer struct {
	OutputDir   string
	WorkspaceID string

	mu     sync.Mutex
	tables map[jsonldb.ID]*jsonldb.Table[*content.DataRecord]
}

// NewWriter creates a new writer for the given output directory and workspace.
func NewWriter(outputDir, workspaceID string) *Writer {
	return &Writer{
		OutputDir:   outputDir,
		WorkspaceID: workspaceID,
		tables:      make(map[jsonldb.ID]*jsonldb.Table[*content.DataRecord]),
	}
}

// workspacePath returns the path to the workspace directory.
func (w *Writer) workspacePath() string {
	return filepath.Join(w.OutputDir, w.WorkspaceID)
}

// nodePath returns the path to a node's directory.
func (w *Writer) nodePath(nodeID jsonldb.ID) string {
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

// getTable returns the jsonldb.Table for a node, creating it if needed.
func (w *Writer) getTable(nodeID jsonldb.ID) (*jsonldb.Table[*content.DataRecord], error) {
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
func (w *Writer) WriteRecords(nodeID jsonldb.ID, properties []content.Property, records []*content.DataRecord) error {
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
func (w *Writer) AppendRecord(nodeID jsonldb.ID, record *content.DataRecord) error {
	table, err := w.getTable(nodeID)
	if err != nil {
		return err
	}

	if err := table.Append(record); err != nil {
		return fmt.Errorf("failed to append record: %w", err)
	}

	return nil
}
