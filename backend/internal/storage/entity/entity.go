// Package entity defines persistent domain models for storage in jsonldb.
//
// This package contains the core domain types that are persisted to disk via
// jsonldb. These types implement the jsonldb.Row interface (Clone, GetID,
// Validate) and use jsonldb.ID for unique identifiers.
//
// The entity package is the foundation of the data model:
//   - Node: Unified content entity (document, database, or hybrid)
//   - DataRecord: Individual records within database nodes
//
// Types in this package are storage-oriented and use jsonldb.ID (uint64) for
// identifiers. For API representations with string IDs and RFC3339 timestamps,
// see the dto package.
//
// Architecture note: The entity package has no dependencies on HTTP or API
// concerns. It only depends on jsonldb for ID types and is designed to be
// imported by both storage services and the dto package for conversions.
package entity

import (
	"errors"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

var errIDRequired = errors.New("id is required")

// Node represents the unified content entity (can be a Page, a Database, or both).
type Node struct {
	ID         jsonldb.ID `json:"id" jsonschema:"description=Unique node identifier"`
	ParentID   jsonldb.ID `json:"parent_id,omitempty" jsonschema:"description=Parent node ID for hierarchical structure"`
	Title      string     `json:"title" jsonschema:"description=Node title"`
	Content    string     `json:"content,omitempty" jsonschema:"description=Markdown content (Page part)"`
	Properties []Property `json:"properties,omitempty" jsonschema:"description=Schema definition (Database part)"`
	Created    time.Time  `json:"created" jsonschema:"description=Node creation timestamp"`
	Modified   time.Time  `json:"modified" jsonschema:"description=Last modification timestamp"`
	Tags       []string   `json:"tags,omitempty" jsonschema:"description=Node tags for categorization"`
	FaviconURL string     `json:"favicon_url,omitempty" jsonschema:"description=Custom favicon URL"`
	Type       NodeType   `json:"type" jsonschema:"description=Node type (document/database/hybrid)"`
	Children   []*Node    `json:"children,omitempty" jsonschema:"description=Nested child nodes"`
}

// NodeType defines what features are enabled for a node.
type NodeType string

const (
	// NodeTypeDocument represents a markdown document.
	NodeTypeDocument NodeType = "document"
	// NodeTypeDatabase represents a structured database.
	NodeTypeDatabase NodeType = "database"
	// NodeTypeHybrid represents an entity that is both a document and a database.
	NodeTypeHybrid NodeType = "hybrid"
)

// DataRecord represents a record in a database.
type DataRecord struct {
	ID       jsonldb.ID     `json:"id" jsonschema:"description=Unique record identifier"`
	Data     map[string]any `json:"data" jsonschema:"description=Record field values keyed by property name"`
	Created  time.Time      `json:"created" jsonschema:"description=Record creation timestamp"`
	Modified time.Time      `json:"modified" jsonschema:"description=Last modification timestamp"`
}

// Clone returns a deep copy of the DataRecord.
func (r *DataRecord) Clone() *DataRecord {
	c := *r
	if r.Data != nil {
		c.Data = make(map[string]any, len(r.Data))
		for k, v := range r.Data {
			c.Data[k] = v
		}
	}
	return &c
}

// GetID returns the DataRecord's ID.
func (r *DataRecord) GetID() jsonldb.ID {
	return r.ID
}

// Validate checks that the DataRecord is valid.
func (r *DataRecord) Validate() error {
	if r.ID.IsZero() {
		return errIDRequired
	}
	return nil
}

// Asset represents an uploaded file/image associated with a node.
// Asset IDs are filenames, not generated IDs, hence the string type.
type Asset struct {
	ID       string    `json:"id" jsonschema:"description=Asset identifier (filename)"`
	Name     string    `json:"name" jsonschema:"description=Original filename"`
	MimeType string    `json:"mime_type" jsonschema:"description=MIME type of the asset"`
	Size     int64     `json:"size" jsonschema:"description=File size in bytes"`
	Created  time.Time `json:"created" jsonschema:"description=Upload timestamp"`
	Path     string    `json:"path" jsonschema:"description=Storage path on disk"`
}

// SearchResult represents a single search result.
type SearchResult struct {
	Type     string            `json:"type" jsonschema:"description=Result type (page or record)"`
	NodeID   jsonldb.ID        `json:"node_id" jsonschema:"description=Node containing the result"`
	RecordID jsonldb.ID        `json:"record_id,omitempty" jsonschema:"description=Record ID if result is a database record"`
	Title    string            `json:"title" jsonschema:"description=Title of the matched item"`
	Snippet  string            `json:"snippet" jsonschema:"description=Text snippet with match context"`
	Score    float64           `json:"score" jsonschema:"description=Relevance score"`
	Matches  map[string]string `json:"matches" jsonschema:"description=Matched fields and their values"`
	Modified time.Time         `json:"modified" jsonschema:"description=Last modification timestamp"`
}

// SearchOptions defines parameters for a search.
type SearchOptions struct {
	Query       string `json:"query" jsonschema:"description=Search query string"`
	Limit       int    `json:"limit,omitempty" jsonschema:"description=Maximum number of results"`
	MatchTitle  bool   `json:"match_title,omitempty" jsonschema:"description=Search in titles"`
	MatchBody   bool   `json:"match_body,omitempty" jsonschema:"description=Search in body content"`
	MatchFields bool   `json:"match_fields,omitempty" jsonschema:"description=Search in database fields"`
}
