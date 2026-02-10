// Defines the core data models for content (Node, DataRecord, Asset).

package content

import (
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/storage"
)

// Node represents the unified content entity (can be a Page, a Table, or both).
type Node struct {
	ID          ksid.ID      `json:"id" jsonschema:"description=Unique node identifier"`
	ParentID    ksid.ID      `json:"parent_id,omitempty" jsonschema:"description=Parent node ID for hierarchical structure"`
	Title       string       `json:"title" jsonschema:"description=Node title"`
	Content     string       `json:"content,omitempty" jsonschema:"description=Markdown content (Page part)"`
	Properties  []Property   `json:"properties,omitempty" jsonschema:"description=Schema definition (Table part)"`
	Views       []View       `json:"views,omitempty" jsonschema:"description=Saved view configurations (Table part)"`
	Created     storage.Time `json:"created" jsonschema:"description=Node creation timestamp"`
	Modified    storage.Time `json:"modified" jsonschema:"description=Last modification timestamp"`
	Tags        []string     `json:"tags,omitempty" jsonschema:"description=Node tags for categorization"`
	FaviconURL  string       `json:"favicon_url,omitempty" jsonschema:"description=Custom favicon URL"`
	Icon        string       `json:"icon,omitempty" jsonschema:"description=Node icon (emoji or local asset path)"`
	Cover       string       `json:"cover,omitempty" jsonschema:"description=Cover image (local asset path)"`
	Type        NodeType     `json:"type" jsonschema:"description=Node type (document/table/hybrid)"`
	HasChildren bool         `json:"has_children,omitempty" jsonschema:"description=Whether this node has child nodes"`
	Children    []*Node      `json:"children,omitempty" jsonschema:"description=Nested child nodes"`
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

// DataRecord represents a record in a table.
type DataRecord struct {
	ID       ksid.ID        `json:"id" jsonschema:"description=Unique record identifier"`
	Data     map[string]any `json:"data" jsonschema:"description=Record field values keyed by property name"`
	Created  storage.Time   `json:"created" jsonschema:"description=Record creation timestamp"`
	Modified storage.Time   `json:"modified" jsonschema:"description=Last modification timestamp"`
}

// Clone returns a deep copy of the DataRecord.
func (r *DataRecord) Clone() *DataRecord {
	c := *r
	if r.Data != nil {
		c.Data = cloneDataMap(r.Data)
	}
	return &c
}

// cloneDataMap performs a deep clone of a record data map.
func cloneDataMap(m map[string]any) map[string]any {
	c := make(map[string]any, len(m))
	for k, v := range m {
		c[k] = cloneValue(v)
	}
	return c
}

// cloneValue deep clones a value that may appear in record data.
func cloneValue(v any) any {
	switch val := v.(type) {
	case []any:
		c := make([]any, len(val))
		for i, item := range val {
			c[i] = cloneValue(item)
		}
		return c
	case []string:
		return append([]string(nil), val...)
	case map[string]any:
		return cloneDataMap(val)
	default:
		// Primitives (string, float64, bool, nil) are immutable
		return v
	}
}

// GetID returns the DataRecord's ID.
func (r *DataRecord) GetID() ksid.ID {
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
	ID       string       `json:"id" jsonschema:"description=Asset identifier (filename)"`
	Name     string       `json:"name" jsonschema:"description=Original filename"`
	MimeType string       `json:"mime_type" jsonschema:"description=MIME type of the asset"`
	Size     int64        `json:"size" jsonschema:"description=File size in bytes"`
	Created  storage.Time `json:"created" jsonschema:"description=Upload timestamp"`
	Path     string       `json:"path" jsonschema:"description=Storage path on disk"`
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
	Modified storage.Time      `json:"modified" jsonschema:"description=Last modification timestamp"`
}

// SearchOptions defines parameters for a search.
type SearchOptions struct {
	Query       string `json:"query" jsonschema:"description=Search query string"`
	Limit       int    `json:"limit,omitempty" jsonschema:"description=Maximum number of results"`
	MatchTitle  bool   `json:"match_title,omitempty" jsonschema:"description=Search in titles"`
	MatchBody   bool   `json:"match_body,omitempty" jsonschema:"description=Search in body content"`
	MatchFields bool   `json:"match_fields,omitempty" jsonschema:"description=Search in table fields"`
}

// PropertyType represents the type of a table property.
type PropertyType string

const (
	// Primitive types.

	// PropertyTypeText stores plain text values.
	PropertyTypeText PropertyType = "text"
	// PropertyTypeMarkdown stores inline markdown (bold, italic, links, code).
	PropertyTypeMarkdown PropertyType = "markdown"
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

	// Relational types.

	// PropertyTypeRelation links to records in another table.
	PropertyTypeRelation PropertyType = "relation"
	// PropertyTypeRollup aggregates values from related records.
	PropertyTypeRollup PropertyType = "rollup"
	// PropertyTypeFormula computes a value from other properties.
	PropertyTypeFormula PropertyType = "formula"
)

// SelectOption represents an option for select/multi_select properties.
type SelectOption struct {
	ID    string `json:"id" jsonschema:"description=Unique option identifier"`
	Name  string `json:"name" jsonschema:"description=Display name of the option"`
	Color string `json:"color,omitempty" jsonschema:"description=Color for visual distinction"`
}

// Property represents a table column with its configuration.
type Property struct {
	Name     string       `json:"name" jsonschema:"description=Property name (column header)"`
	Type     PropertyType `json:"type" jsonschema:"description=Property type (text/number/select/etc)"`
	Required bool         `json:"required,omitempty" jsonschema:"description=Whether this property is required"`

	// Options contains the allowed values for select and multi_select properties.
	// Each option has an ID (used in storage), name (display), and optional color.
	Options []SelectOption `json:"options,omitempty" jsonschema:"description=Allowed values for select properties"`

	// Type-specific configuration for relational properties.
	RelationConfig *RelationConfig `json:"relation_config,omitempty" jsonschema:"description=Configuration for relation properties"`
	RollupConfig   *RollupConfig   `json:"rollup_config,omitempty" jsonschema:"description=Configuration for rollup properties"`
	FormulaConfig  *FormulaConfig  `json:"formula_config,omitempty" jsonschema:"description=Configuration for formula properties"`
}

// RelationConfig defines the target and behavior of a relation property.
type RelationConfig struct {
	// TargetNodeID is the node (table) that this relation points to.
	TargetNodeID ksid.ID `json:"target_node_id" jsonschema:"description=Target table node ID"`
	// IsDualLink indicates if this is a bidirectional relation.
	IsDualLink bool `json:"is_dual_link,omitempty" jsonschema:"description=Whether this is a bidirectional relation"`
	// DualPropertyName is the name of the corresponding property in the target table.
	DualPropertyName string `json:"dual_property_name,omitempty" jsonschema:"description=Name of the dual property in target table"`
}

// RollupConfig defines how to aggregate values from related records.
type RollupConfig struct {
	// RelationProperty is the name of the relation property to traverse.
	RelationProperty string `json:"relation_property" jsonschema:"description=Name of the relation property to aggregate through"`
	// TargetProperty is the property in the related table to aggregate.
	TargetProperty string `json:"target_property" jsonschema:"description=Name of the property to aggregate in related records"`
	// Aggregation is the aggregation function to apply.
	Aggregation RollupAggregation `json:"aggregation" jsonschema:"description=Aggregation function (count/sum/average/min/max/show_all)"`
}

// RollupAggregation defines the aggregation function for rollup properties.
type RollupAggregation string

const (
	// RollupCount counts the number of related records.
	RollupCount RollupAggregation = "count"
	// RollupCountValues counts non-empty values.
	RollupCountValues RollupAggregation = "count_values"
	// RollupSum sums numeric values.
	RollupSum RollupAggregation = "sum"
	// RollupAverage calculates the average of numeric values.
	RollupAverage RollupAggregation = "average"
	// RollupMin finds the minimum value.
	RollupMin RollupAggregation = "min"
	// RollupMax finds the maximum value.
	RollupMax RollupAggregation = "max"
	// RollupShowAll shows all values as a list.
	RollupShowAll RollupAggregation = "show_all"
)

// FormulaConfig defines a computed property expression.
type FormulaConfig struct {
	// Expression is the formula expression (stored for reference, not evaluated).
	Expression string `json:"expression" jsonschema:"description=Formula expression"`
}

// BacklinkInfo represents a page that links to another page.
type BacklinkInfo struct {
	NodeID ksid.ID `json:"node_id" jsonschema:"description=ID of the page linking to this page"`
	Title  string  `json:"title" jsonschema:"description=Title of the linking page"`
}
