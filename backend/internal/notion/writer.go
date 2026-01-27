// Writes extracted Notion data to mddb storage format.

package notion

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/content"
)

// Writer writes extracted data to mddb storage format.
type Writer struct {
	OutputDir   string
	WorkspaceID string
}

// NewWriter creates a new writer for the given output directory and workspace.
func NewWriter(outputDir, workspaceID string) *Writer {
	return &Writer{
		OutputDir:   outputDir,
		WorkspaceID: workspaceID,
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

	// Write metadata.json for tables/hybrids
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

// writeMetadata writes the metadata.json file.
func (w *Writer) writeMetadata(nodeDir string, node *content.Node) error {
	path := filepath.Join(nodeDir, "metadata.json")

	meta := TableMetadata{
		Properties: node.Properties,
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	return os.WriteFile(path, data, 0o644) //nolint:gosec // G306: 0o644 is intentional for readable files
}

// TableMetadata is the structure stored in metadata.json.
type TableMetadata struct {
	Properties []content.Property `json:"properties"`
	Views      []any              `json:"views,omitempty"` // Placeholder for views
}

// WriteRecords writes records to a table's data.jsonl file.
func (w *Writer) WriteRecords(nodeID jsonldb.ID, records []*content.DataRecord) error {
	nodeDir := w.nodePath(nodeID)
	path := filepath.Join(nodeDir, "data.jsonl")

	f, err := os.Create(path) //nolint:gosec // G304: path is constructed from nodeID, not user input
	if err != nil {
		return fmt.Errorf("failed to create data file: %w", err)
	}
	defer func() { _ = f.Close() }()

	encoder := json.NewEncoder(f)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("failed to write record: %w", err)
		}
	}

	return nil
}

// AppendRecord appends a single record to a table's data.jsonl file.
func (w *Writer) AppendRecord(nodeID jsonldb.ID, record *content.DataRecord) error {
	nodeDir := w.nodePath(nodeID)
	path := filepath.Join(nodeDir, "data.jsonl")

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // G302: 0o644 is intentional for readable files
	if err != nil {
		return fmt.Errorf("failed to open data file: %w", err)
	}
	defer func() { _ = f.Close() }()

	encoder := json.NewEncoder(f)
	if err := encoder.Encode(record); err != nil {
		return fmt.Errorf("failed to write record: %w", err)
	}

	return nil
}
