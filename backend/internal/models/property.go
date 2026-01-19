package models

import "github.com/maruel/mddb/backend/internal/jsonldb"

// PropertyType represents the type of a database property.
type PropertyType string

const (
	// Primitive types (map directly to jsonldb storage types)

	// PropertyTypeText stores plain text values.
	PropertyTypeText PropertyType = "text"
	// PropertyTypeNumber stores numeric values (integer or float).
	PropertyTypeNumber PropertyType = "number"
	// PropertyTypeCheckbox stores boolean values.
	PropertyTypeCheckbox PropertyType = "checkbox"
	// PropertyTypeDate stores ISO8601 date strings.
	PropertyTypeDate PropertyType = "date"

	// Enumerated types (text/jsonb storage with options config)

	// PropertyTypeSelect stores a single selection from predefined options.
	PropertyTypeSelect PropertyType = "select"
	// PropertyTypeMultiSelect stores multiple selections from predefined options.
	PropertyTypeMultiSelect PropertyType = "multi_select"

	// Validated text types (text storage with validation)

	// PropertyTypeURL stores URLs with validation.
	PropertyTypeURL PropertyType = "url"
	// PropertyTypeEmail stores email addresses with validation.
	PropertyTypeEmail PropertyType = "email"
	// PropertyTypePhone stores phone numbers with validation.
	PropertyTypePhone PropertyType = "phone"
)

// StorageType returns the underlying jsonldb storage type for this property type.
func (pt PropertyType) StorageType() jsonldb.ColumnType {
	switch pt {
	case PropertyTypeText, PropertyTypeSelect, PropertyTypeURL, PropertyTypeEmail, PropertyTypePhone:
		return jsonldb.ColumnTypeText
	case PropertyTypeNumber:
		return jsonldb.ColumnTypeNumber
	case PropertyTypeCheckbox:
		return jsonldb.ColumnTypeBool
	case PropertyTypeDate:
		return jsonldb.ColumnTypeDate
	case PropertyTypeMultiSelect:
		return jsonldb.ColumnTypeJSONB
	default:
		return jsonldb.ColumnTypeText
	}
}

// SelectOption represents an option for select/multi_select properties.
type SelectOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// Property represents a database property (column) with its configuration.
type Property struct {
	Name     string       `json:"name"`
	Type     PropertyType `json:"type"`
	Required bool         `json:"required,omitempty"`

	// Options contains the allowed values for select and multi_select properties.
	// Each option has an ID (used in storage), name (display), and optional color.
	Options []SelectOption `json:"options,omitempty"`
}

// ToColumn converts a Property to a jsonldb.Column for storage.
func (p *Property) ToColumn() jsonldb.Column {
	return jsonldb.Column{
		Name:     p.Name,
		Type:     p.Type.StorageType(),
		Required: p.Required,
	}
}

// PropertiesToColumns converts a slice of Properties to jsonldb.Columns.
func PropertiesToColumns(props []Property) []jsonldb.Column {
	cols := make([]jsonldb.Column, len(props))
	for i, p := range props {
		cols[i] = p.ToColumn()
	}
	return cols
}
