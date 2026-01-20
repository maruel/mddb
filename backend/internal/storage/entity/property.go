package entity

// PropertyType represents the type of a database property.
type PropertyType string

const (
	// Primitive types

	// PropertyTypeText stores plain text values.
	PropertyTypeText PropertyType = "text"
	// PropertyTypeNumber stores numeric values (integer or float).
	PropertyTypeNumber PropertyType = "number"
	// PropertyTypeCheckbox stores boolean values.
	PropertyTypeCheckbox PropertyType = "checkbox"
	// PropertyTypeDate stores ISO8601 date strings.
	PropertyTypeDate PropertyType = "date"

	// Enumerated types (with predefined options)

	// PropertyTypeSelect stores a single selection from predefined options.
	PropertyTypeSelect PropertyType = "select"
	// PropertyTypeMultiSelect stores multiple selections from predefined options.
	PropertyTypeMultiSelect PropertyType = "multi_select"

	// Validated text types

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

// Property represents a database property (column) with its configuration.
type Property struct {
	Name     string       `json:"name" jsonschema:"description=Property name (column header)"`
	Type     PropertyType `json:"type" jsonschema:"description=Property type (text/number/select/etc)"`
	Required bool         `json:"required,omitempty" jsonschema:"description=Whether this property is required"`

	// Options contains the allowed values for select and multi_select properties.
	// Each option has an ID (used in storage), name (display), and optional color.
	Options []SelectOption `json:"options,omitempty" jsonschema:"description=Allowed values for select properties"`
}
