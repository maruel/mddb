// Defines runtime content models (Node, Asset, search results) and record canonicalization.

package content

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	contentv1 "github.com/maruel/mddb/backend/internal/storage/content/data/v1"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"

	"github.com/maruel/ksid"
)

// Node represents the unified content entity (can be a Page, a Table, or both).
type Node struct {
	// Changes describes this mutation's committed effects, not persisted content.
	Changes        NodeChanges          `json:"-"`
	Revision       string               `json:"revision,omitempty" jsonschema:"description=Canonical page content revision"`
	SchemaRevision string               `json:"-"`
	ID             ksid.ID              `json:"id" jsonschema:"description=Unique node identifier"`
	ParentID       ksid.ID              `json:"parent_id,omitempty" jsonschema:"description=Parent node ID for hierarchical structure"`
	Title          string               `json:"title" jsonschema:"description=Node title"`
	Content        string               `json:"content,omitempty" jsonschema:"description=Markdown content (Page part)"`
	Properties     []contentv1.Property `json:"properties,omitempty" jsonschema:"description=Schema definition (Table part)"`
	Views          []contentv1.View     `json:"views,omitempty" jsonschema:"description=Saved view configurations (Table part)"`
	Created        storagev1.Time       `json:"created" jsonschema:"description=Node creation timestamp"`
	Modified       storagev1.Time       `json:"modified" jsonschema:"description=Last modification timestamp"`
	Tags           []string             `json:"tags,omitempty" jsonschema:"description=Node tags for categorization"`
	FaviconURL     string               `json:"favicon_url,omitempty" jsonschema:"description=Custom favicon URL"`
	Icon           string               `json:"icon,omitempty" jsonschema:"description=Node icon (emoji or local asset path)"`
	Cover          string               `json:"cover,omitempty" jsonschema:"description=Cover image (local asset path)"`
	Type           NodeType             `json:"type" jsonschema:"description=Node type (document/table/hybrid)"`
	HasChildren    bool                 `json:"has_children,omitempty" jsonschema:"description=Whether this node has child nodes"`
	Children       []*Node              `json:"children,omitempty" jsonschema:"description=Nested child nodes"`
}

// NodeChanges identifies effects computed while the mutation owns its snapshot.
// A title change affects discovery descriptors as well as page content.
type NodeChanges struct {
	Body     bool
	Metadata bool
	Schema   bool
	Type     bool
	Title    bool
}

// NodeDeletion describes the removed resource set from one committed subtree
// deletion. A large subtree requires resync rather than an unbounded event.
type NodeDeletion struct {
	NodeID   ksid.ID
	ParentID ksid.ID
	IDs      []ksid.ID
	Resync   bool
}

// NodeMove records the ancestry transition from the committed mutation.
type NodeMove struct {
	OldParentID ksid.ID
	ParentID    ksid.ID
	Changed     bool
}

// NodeType defines what features are enabled for a node.
type NodeType string

const (
	// NodeTypeDocument represents a markdown document.
	NodeTypeDocument NodeType = "document"
	// NodeTypeTable represents a structured table.
	NodeTypeTable NodeType = "table"
	// NodeTypeHybrid represents an entity that is both a document and a table.
	NodeTypeHybrid NodeType = "hybrid"
)

// CanonicalRecord returns the representation read back from JSONL storage.
func CanonicalRecord(record *contentv1.DataRecord) (*contentv1.DataRecord, error) {
	canonical := *record
	if record.Data == nil {
		return &canonical, nil
	}
	canonical.Data = make(map[string]any, len(record.Data))
	for key, value := range record.Data {
		switch v := value.(type) {
		case nil, string, bool, float64:
			canonical.Data[key] = v
		case int64:
			canonical.Data[key] = float64(v)
		default:
			// Preserve JSON semantics for nested values and less common Go types.
			encoded, err := json.Marshal(record)
			if err != nil {
				return nil, fmt.Errorf("encode record: %w", err)
			}
			if err := json.Unmarshal(encoded, &canonical); err != nil {
				return nil, fmt.Errorf("normalize record: %w", err)
			}
			return &canonical, nil
		}
	}
	return &canonical, nil
}

// RecordRevision hashes the canonical JSON representation of a record.
func RecordRevision(record *contentv1.DataRecord) (string, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("encode record revision: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// Asset represents an uploaded file/image associated with a node.
// Asset IDs are filenames, not generated IDs, hence the string type.
type Asset struct {
	ID       string         `json:"id" jsonschema:"description=Asset identifier (filename)"`
	Name     string         `json:"name" jsonschema:"description=Original filename"`
	MimeType string         `json:"mime_type" jsonschema:"description=MIME type of the asset"`
	Size     int64          `json:"size" jsonschema:"description=File size in bytes"`
	Created  storagev1.Time `json:"created" jsonschema:"description=Upload timestamp"`
	Path     string         `json:"path" jsonschema:"description=Storage path on disk"`
}

// SearchResult represents a single search result.
type SearchResult struct {
	Type     string            `json:"type" jsonschema:"description=Result type (page or record)"`
	NodeID   ksid.ID           `json:"node_id" jsonschema:"description=Node containing the result"`
	RecordID ksid.ID           `json:"record_id,omitempty" jsonschema:"description=Record ID if result is a table record"`
	Title    string            `json:"title" jsonschema:"description=Title of the matched item"`
	Snippet  string            `json:"snippet" jsonschema:"description=Text snippet with match context"`
	Score    float64           `json:"score" jsonschema:"description=Relevance score"`
	Matches  map[string]string `json:"matches" jsonschema:"description=Matched fields and their values"`
	Modified storagev1.Time    `json:"modified" jsonschema:"description=Last modification timestamp"`
}

// SearchOptions defines parameters for a search.
type SearchOptions struct {
	Query       string `json:"query" jsonschema:"description=Search query string"`
	Limit       int    `json:"limit,omitempty" jsonschema:"description=Maximum number of results"`
	MatchTitle  bool   `json:"match_title,omitempty" jsonschema:"description=Search in titles"`
	MatchBody   bool   `json:"match_body,omitempty" jsonschema:"description=Search in body content"`
	MatchFields bool   `json:"match_fields,omitempty" jsonschema:"description=Search in table fields"`
}

// BacklinkInfo represents a page that links to another page.
type BacklinkInfo struct {
	NodeID ksid.ID `json:"node_id" jsonschema:"description=ID of the page linking to this page"`
	Title  string  `json:"title" jsonschema:"description=Title of the linking page"`
}
