// Persisted table properties stored in metadata.json.

package v1

import (
	"github.com/maruel/ksid"
)

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

	// Reference types.

	// PropertyTypeUser stores a reference to a workspace member by user ID.
	PropertyTypeUser PropertyType = "user"

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
