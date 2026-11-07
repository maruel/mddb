// Defines complete bounded MCP node resource schemas separately from storage runtime state.

package mcpschema

import "github.com/maruel/ksid"

// ResourceNode preserves the existing node resource shape, adding chunk continuation.
type ResourceNode struct {
	ID               ksid.ID        `json:"id"`
	ParentID         ksid.ID        `json:"parent_id,omitempty"`
	Title            string         `json:"title"`
	Content          string         `json:"content,omitempty"`
	Properties       []Property     `json:"properties,omitempty"`
	Views            []View         `json:"views,omitempty"`
	Created          float64        `json:"created"`
	Modified         float64        `json:"modified"`
	Tags             []string       `json:"tags,omitempty"`
	FaviconURL       string         `json:"favicon_url,omitempty"`
	Icon             string         `json:"icon,omitempty"`
	Cover            string         `json:"cover,omitempty"`
	Type             string         `json:"type"`
	HasChildren      bool           `json:"has_children,omitempty"`
	Children         []ResourceNode `json:"children,omitempty"`
	Revision         string         `json:"revision,omitempty"`
	ContentBytes     int64          `json:"contentBytes,omitempty"`
	ContentTruncated bool           `json:"contentTruncated,omitempty"`
	NextContentURI   string         `json:"nextContentURI,omitempty"`
}

// SelectOption preserves the node resource schema.
type SelectOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// Property preserves the node resource schema.
type Property struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`

	// Options contains the allowed values for select and multi_select properties.
	// Each option has an ID (used in storage), name (display), and optional color.
	Options []SelectOption `json:"options,omitempty"`

	// Type-specific configuration for relational properties.
	RelationConfig *RelationConfig `json:"relation_config,omitempty"`
	RollupConfig   *RollupConfig   `json:"rollup_config,omitempty"`
	FormulaConfig  *FormulaConfig  `json:"formula_config,omitempty"`
}

// RelationConfig preserves the node resource schema.
type RelationConfig struct {
	// TargetNodeID is the node (table) that this relation points to.
	TargetNodeID ksid.ID `json:"target_node_id"`
	// IsDualLink indicates if this is a bidirectional relation.
	IsDualLink bool `json:"is_dual_link,omitempty"`
	// DualPropertyName is the name of the corresponding property in the target table.
	DualPropertyName string `json:"dual_property_name,omitempty"`
}

// RollupConfig preserves the node resource schema.
type RollupConfig struct {
	// RelationProperty is the name of the relation property to traverse.
	RelationProperty string `json:"relation_property"`
	// TargetProperty is the property in the related table to aggregate.
	TargetProperty string `json:"target_property"`
	// Aggregation is the aggregation function to apply.
	Aggregation string `json:"aggregation"`
}

// FormulaConfig preserves the node resource schema.
type FormulaConfig struct {
	// Expression is the formula expression (stored for reference, not evaluated).
	Expression string `json:"expression"`
}

// View preserves the node resource schema.
type View struct {
	ID      ksid.ID `json:"id"`
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	Default bool    `json:"default,omitempty"`

	// Display configuration
	Columns []ViewColumn `json:"columns,omitempty"`

	// Data shaping
	Filters []Filter `json:"filters,omitempty"`
	Sorts   []Sort   `json:"sorts,omitempty"`
	Groups  []Group  `json:"groups,omitempty"`
}

// ViewColumn preserves the node resource schema.
type ViewColumn struct {
	Property string `json:"property"`
	Width    int    `json:"width,omitempty"`
	Visible  bool   `json:"visible"`
}

// Filter preserves the node resource schema.
type Filter struct {
	Property string `json:"property,omitempty"`
	Operator string `json:"operator,omitempty"`
	Value    any    `json:"value,omitempty"`

	// Compound filters (mutually exclusive with Property/Operator/Value)
	And []Filter `json:"and,omitempty"`
	Or  []Filter `json:"or,omitempty"`
}

// Sort preserves the node resource schema.
type Sort struct {
	Property  string `json:"property"`
	Direction string `json:"direction"`
}

// Group preserves the node resource schema.
type Group struct {
	Property string `json:"property"`
	Hidden   []any  `json:"hidden,omitempty"`
}
