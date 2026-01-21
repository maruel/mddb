package content

import (
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// Node represents the unified content entity (can be a Page, a Database, or both).
type Node struct {
	ID         jsonldb.ID `json:"id" jsonschema:"description=Unique node identifier"`
	ParentID   jsonldb.ID `json:"parent_id,omitempty" jsonschema:"description=Parent node ID for hierarchical structure"`
	Title      string     `json:"title" jsonschema:"description=Node title"`
	Content    string     `json:"content,omitempty" jsonschema:"description=Markdown content (Page part)"`
	Properties []Property `json:"properties,omitempty" jsonschema:"description=Schema definition (Table part)"`
	Created    time.Time  `json:"created" jsonschema:"description=Node creation timestamp"`
	Modified   time.Time  `json:"modified" jsonschema:"description=Last modification timestamp"`
	Tags       []string   `json:"tags,omitempty" jsonschema:"description=Node tags for categorization"`
	FaviconURL string     `json:"favicon_url,omitempty" jsonschema:"description=Custom favicon URL"`
	Type       NodeType   `json:"type" jsonschema:"description=Node type (document/table/hybrid)"`
	Children   []*Node    `json:"children,omitempty" jsonschema:"description=Nested child nodes"`
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
	RecordID jsonldb.ID        `json:"record_id,omitempty" jsonschema:"description=Record ID if result is a table record"`
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

// PropertyType represents the type of a database property.
type PropertyType string

const (
	// Primitive types.

	// PropertyTypeText stores plain text values.
	PropertyTypeText PropertyType = "text"
	// PropertyTypeNumber stores numeric values (integer or float).
	PropertyTypeNumber PropertyType = "number"
	// PropertyTypeCheckbox stores boolean values.
	PropertyTypeCheckbox PropertyType = "checkbox"
	// PropertyTypeDate stores ISO8601 date strings.
	PropertyTypeDate PropertyType = "date"

	// Enumerated types (with predefined options).

	// PropertyTypeSelect stores a single selection from predefined options.
	PropertyTypeSelect PropertyType = "select"
	// PropertyTypeMultiSelect stores multiple selections from predefined options.
	PropertyTypeMultiSelect PropertyType = "multi_select"

	// Validated text types.

	// PropertyTypeURL stores URLs with validation.
	PropertyTypeURL PropertyType = "url"
	// PropertyTypeEmail stores email addresses with validation.
	PropertyTypeEmail PropertyType = "email"
	// PropertyTypePhone stores phone numbers with validation.
	PropertyTypePhone PropertyType = "phone"
)

// SelectOption represents an option for select/multi_select properties.
type SelectOption struct {
	ID    string `json:"id" jsonschema:"description=Unique option identifier"`
	Name  string `json:"name" jsonschema:"description=Display name of the option"`
	Color string `json:"color,omitempty" jsonschema:"description=Color for visual distinction"`
}

// Property represents a table property (column) with its configuration.
type Property struct {
	Name     string       `json:"name" jsonschema:"description=Property name (column header)"`
	Type     PropertyType `json:"type" jsonschema:"description=Property type (text/number/select/etc)"`
	Required bool         `json:"required,omitempty" jsonschema:"description=Whether this property is required"`

	// Options contains the allowed values for select and multi_select properties.
	// Each option has an ID (used in storage), name (display), and optional color.
	Options []SelectOption `json:"options,omitempty" jsonschema:"description=Allowed values for select properties"`
}
