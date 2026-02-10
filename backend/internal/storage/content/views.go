// Defines view types for saved table configurations.

package content

import "github.com/maruel/mddb/backend/internal/ksid"

// View represents a saved table view configuration.
type View struct {
	ID      ksid.ID  `json:"id" jsonschema:"description=Unique view identifier"`
	Name    string   `json:"name" jsonschema:"description=View display name"`
	Type    ViewType `json:"type" jsonschema:"description=View layout type (table/board/gallery/list/calendar)"`
	Default bool     `json:"default,omitempty" jsonschema:"description=Whether this is the default view"`

	// Display configuration
	Columns []ViewColumn `json:"columns,omitempty" jsonschema:"description=Column visibility and ordering"`

	// Data shaping
	Filters []Filter `json:"filters,omitempty" jsonschema:"description=Filter conditions"`
	Sorts   []Sort   `json:"sorts,omitempty" jsonschema:"description=Sort order"`
	Groups  []Group  `json:"groups,omitempty" jsonschema:"description=Grouping configuration"`
}

// GetID returns the View's ID.
func (v *View) GetID() ksid.ID {
	return v.ID
}

// Validate checks that the View is valid.
func (v *View) Validate() error {
	if v.ID.IsZero() {
		return errIDRequired
	}
	if v.Name == "" {
		return errNameRequired
	}
	return nil
}

// ViewType defines the layout type for a view.
type ViewType string

const (
	// ViewTypeTable displays records in a spreadsheet-like table.
	ViewTypeTable ViewType = "table"
	// ViewTypeBoard displays records in a kanban board grouped by a property.
	ViewTypeBoard ViewType = "board"
	// ViewTypeGallery displays records as cards in a grid.
	ViewTypeGallery ViewType = "gallery"
	// ViewTypeList displays records in a simple list.
	ViewTypeList ViewType = "list"
	// ViewTypeCalendar displays records on a calendar by date property.
	ViewTypeCalendar ViewType = "calendar"
)

// ViewColumn defines the visibility and width of a property column.
type ViewColumn struct {
	Property string `json:"property" jsonschema:"description=Property name to display"`
	Width    int    `json:"width,omitempty" jsonschema:"description=Column width in pixels"`
	Visible  bool   `json:"visible" jsonschema:"description=Whether the column is visible"`
}

// Filter defines a condition for filtering records.
type Filter struct {
	Property string   `json:"property,omitempty" jsonschema:"description=Property name to filter on"`
	Operator FilterOp `json:"operator,omitempty" jsonschema:"description=Filter operator"`
	Value    any      `json:"value,omitempty" jsonschema:"description=Value to compare against"`

	// Compound filters (mutually exclusive with Property/Operator/Value)
	And []Filter `json:"and,omitempty" jsonschema:"description=All conditions must match (AND)"`
	Or  []Filter `json:"or,omitempty" jsonschema:"description=Any condition must match (OR)"`
}

// FilterOp defines the comparison operator for a filter.
type FilterOp string

const (
	// FilterOpEquals matches if value equals the filter value.
	FilterOpEquals FilterOp = "equals"
	// FilterOpNotEquals matches if value does not equal the filter value.
	FilterOpNotEquals FilterOp = "not_equals"
	// FilterOpContains matches if value contains the filter value (text).
	FilterOpContains FilterOp = "contains"
	// FilterOpNotContains matches if value does not contain the filter value.
	FilterOpNotContains FilterOp = "not_contains"
	// FilterOpStartsWith matches if value starts with the filter value.
	FilterOpStartsWith FilterOp = "starts_with"
	// FilterOpEndsWith matches if value ends with the filter value.
	FilterOpEndsWith FilterOp = "ends_with"
	// FilterOpGreaterThan matches if value is greater than the filter value.
	FilterOpGreaterThan FilterOp = "gt"
	// FilterOpLessThan matches if value is less than the filter value.
	FilterOpLessThan FilterOp = "lt"
	// FilterOpGreaterEqual matches if value is greater than or equal to the filter value.
	FilterOpGreaterEqual FilterOp = "gte"
	// FilterOpLessEqual matches if value is less than or equal to the filter value.
	FilterOpLessEqual FilterOp = "lte"
	// FilterOpIsEmpty matches if value is empty/null.
	FilterOpIsEmpty FilterOp = "is_empty"
	// FilterOpIsNotEmpty matches if value is not empty/null.
	FilterOpIsNotEmpty FilterOp = "is_not_empty"
)

// Sort defines the sort order for a property.
type Sort struct {
	Property  string  `json:"property" jsonschema:"description=Property name to sort by"`
	Direction SortDir `json:"direction" jsonschema:"description=Sort direction (asc/desc)"`
}

// SortDir defines the sort direction.
type SortDir string

const (
	// SortAsc sorts in ascending order (A-Z, 0-9, oldest-newest).
	SortAsc SortDir = "asc"
	// SortDesc sorts in descending order (Z-A, 9-0, newest-oldest).
	SortDesc SortDir = "desc"
)

// Group defines how to group records by a property.
type Group struct {
	Property string `json:"property" jsonschema:"description=Property name to group by"`
	Hidden   []any  `json:"hidden,omitempty" jsonschema:"description=Group values to hide"`
}
