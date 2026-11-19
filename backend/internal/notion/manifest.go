// Parses view manifest YAML files for import.

package notion

import (
	"errors"
	"fmt"
	"os"

	contentv1 "github.com/maruel/mddb/backend/internal/storage/content/data/v1"

	"github.com/maruel/ksid"
	"gopkg.in/yaml.v3"
)

// ViewManifest defines the views to create for imported databases, because the
// Notion API does not expose database views. It is a YAML file:
//
//	version: 1
//	databases:
//	  - notion_id: "0123456789abcdef0123456789abcdef"
//	    views:
//	      - name: By status
//	        type: board        # table, list, gallery, board, or calendar
//	        default: true
//	        group_by: Status   # property names, not IDs
//	        hidden_groups: [Archived]
//	        columns: [{property: Title, width: 240, visible: true}]
//	        sorts: [{property: Due, direction: asc}]   # asc or desc
//	        filters: [{property: Status, operator: not_equals, value: Done}]
//
// Filters nest with and/or lists; operators are listed in mapFilterOp.
// Validate requires version 1, a notion_id per database, and a name and a
// valid type per view.
//
// TODO: date_property, cover_property, and title_property parse but
// configToView ignores them, and mapFilterOp silently maps an unknown operator
// to equals. Apply the properties once views support them, and reject unknown
// operators in Validate.
type ViewManifest struct {
	Version   int                  `yaml:"version"`
	Databases []DatabaseViewConfig `yaml:"databases"`
}

// ParseManifest reads and parses a view manifest from a file.
// The path is provided by the CLI user, so file inclusion is expected.
func ParseManifest(path string) (*ViewManifest, error) {
	data, err := os.ReadFile(path) //nolint:gosec // User-specified manifest path
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest ViewManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	return &manifest, nil
}

// ParseManifestBytes parses a view manifest from bytes.
func ParseManifestBytes(data []byte) (*ViewManifest, error) {
	var manifest ViewManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	return &manifest, nil
}

// Validate checks that the manifest is valid.
func (m *ViewManifest) Validate() error {
	if m.Version != 1 {
		return fmt.Errorf("unsupported manifest version: %d", m.Version)
	}

	for i := range m.Databases {
		db := &m.Databases[i]
		if db.NotionID == "" {
			return fmt.Errorf("database %d: notion_id is required", i)
		}

		for j := range db.Views {
			v := &db.Views[j]
			if v.Name == "" {
				return fmt.Errorf("database %s, view %d: name is required", db.NotionID, j)
			}
			if v.Type == "" {
				return fmt.Errorf("database %s, view %q: type is required", db.NotionID, v.Name)
			}
			if !isValidViewType(v.Type) {
				return fmt.Errorf("database %s, view %q: invalid type %q", db.NotionID, v.Name, v.Type)
			}
		}
	}

	return nil
}

// ViewsForDatabase returns the view configs for a given Notion database ID.
func (m *ViewManifest) ViewsForDatabase(notionID string) []ViewConfig {
	for i := range m.Databases {
		if m.Databases[i].NotionID == notionID {
			return m.Databases[i].Views
		}
	}
	return nil
}

// ToContentViews converts manifest view configs to content.View structs.
func (m *ViewManifest) ToContentViews(notionID string) []contentv1.View {
	configs := m.ViewsForDatabase(notionID)
	if configs == nil {
		return nil
	}

	views := make([]contentv1.View, 0, len(configs))
	for i := range configs {
		views = append(views, configToView(&configs[i]))
	}
	return views
}

// DatabaseViewConfig defines views for a specific database.
type DatabaseViewConfig struct {
	NotionID string       `yaml:"notion_id"`
	Views    []ViewConfig `yaml:"views"`
}

// ViewConfig defines a single view configuration.
type ViewConfig struct {
	Name         string         `yaml:"name"`
	Type         string         `yaml:"type"`
	Default      bool           `yaml:"default,omitempty"`
	Columns      []ColumnConfig `yaml:"columns,omitempty"`
	Sorts        []SortConfig   `yaml:"sorts,omitempty"`
	Filters      []FilterConfig `yaml:"filters,omitempty"`
	GroupBy      string         `yaml:"group_by,omitempty"`
	HiddenGroups []string       `yaml:"hidden_groups,omitempty"`
	DateProperty string         `yaml:"date_property,omitempty"`
	// Gallery-specific
	CoverProperty string `yaml:"cover_property,omitempty"`
	TitleProperty string `yaml:"title_property,omitempty"`
}

// ColumnConfig defines column visibility and width.
type ColumnConfig struct {
	Property string `yaml:"property"`
	Width    int    `yaml:"width,omitempty"`
	Visible  *bool  `yaml:"visible,omitempty"` // nil means visible
}

// SortConfig defines a sort criterion.
type SortConfig struct {
	Property  string `yaml:"property"`
	Direction string `yaml:"direction"` // "asc" or "desc"
}

// FilterConfig defines a filter condition.
type FilterConfig struct {
	Property string         `yaml:"property,omitempty"`
	Operator string         `yaml:"operator,omitempty"`
	Value    any            `yaml:"value,omitempty"`
	And      []FilterConfig `yaml:"and,omitempty"`
	Or       []FilterConfig `yaml:"or,omitempty"`
}

// configToView converts a ViewConfig to a content.View.
func configToView(cfg *ViewConfig) contentv1.View {
	view := contentv1.View{
		ID:      ksid.NewID(),
		Name:    cfg.Name,
		Type:    mapViewType(cfg.Type),
		Default: cfg.Default,
	}

	// Convert columns
	for i := range cfg.Columns {
		col := &cfg.Columns[i]
		visible := true
		if col.Visible != nil {
			visible = *col.Visible
		}
		view.Columns = append(view.Columns, contentv1.ViewColumn{
			Property: col.Property,
			Width:    col.Width,
			Visible:  visible,
		})
	}

	// Convert sorts
	for i := range cfg.Sorts {
		sort := &cfg.Sorts[i]
		view.Sorts = append(view.Sorts, contentv1.Sort{
			Property:  sort.Property,
			Direction: mapSortDir(sort.Direction),
		})
	}

	// Convert filters
	for i := range cfg.Filters {
		filter := &cfg.Filters[i]
		view.Filters = append(view.Filters, configToFilter(filter))
	}

	// Convert groups
	if cfg.GroupBy != "" {
		group := contentv1.Group{
			Property: cfg.GroupBy,
		}
		// Convert hidden groups to any slice
		for _, h := range cfg.HiddenGroups {
			group.Hidden = append(group.Hidden, h)
		}
		view.Groups = append(view.Groups, group)
	}

	return view
}

// configToFilter converts a FilterConfig to a content.Filter.
func configToFilter(cfg *FilterConfig) contentv1.Filter {
	filter := contentv1.Filter{
		Property: cfg.Property,
		Operator: mapFilterOp(cfg.Operator),
		Value:    cfg.Value,
	}

	// Handle compound filters
	for i := range cfg.And {
		filter.And = append(filter.And, configToFilter(&cfg.And[i]))
	}
	for i := range cfg.Or {
		filter.Or = append(filter.Or, configToFilter(&cfg.Or[i]))
	}

	return filter
}

// isValidViewType checks if a view type string is valid.
func isValidViewType(t string) bool {
	switch t {
	case "table", "board", "gallery", "list", "calendar":
		return true
	default:
		return false
	}
}

// mapViewType converts a string view type to content.ViewType.
func mapViewType(t string) contentv1.ViewType {
	switch t {
	case "table":
		return contentv1.ViewTypeTable
	case "board":
		return contentv1.ViewTypeBoard
	case "gallery":
		return contentv1.ViewTypeGallery
	case "list":
		return contentv1.ViewTypeList
	case "calendar":
		return contentv1.ViewTypeCalendar
	default:
		return contentv1.ViewTypeTable
	}
}

// mapSortDir converts a string sort direction to content.SortDir.
func mapSortDir(dir string) contentv1.SortDir {
	switch dir {
	case "desc":
		return contentv1.SortDesc
	default:
		return contentv1.SortAsc
	}
}

// mapFilterOp converts a string filter operator to content.FilterOp.
func mapFilterOp(op string) contentv1.FilterOp {
	switch op {
	case "equals":
		return contentv1.FilterOpEquals
	case "not_equals":
		return contentv1.FilterOpNotEquals
	case "contains":
		return contentv1.FilterOpContains
	case "not_contains":
		return contentv1.FilterOpNotContains
	case "starts_with":
		return contentv1.FilterOpStartsWith
	case "ends_with":
		return contentv1.FilterOpEndsWith
	case "gt":
		return contentv1.FilterOpGreaterThan
	case "lt":
		return contentv1.FilterOpLessThan
	case "gte":
		return contentv1.FilterOpGreaterEqual
	case "lte":
		return contentv1.FilterOpLessEqual
	case "is_empty":
		return contentv1.FilterOpIsEmpty
	case "is_not_empty":
		return contentv1.FilterOpIsNotEmpty
	default:
		return contentv1.FilterOpEquals
	}
}

// ErrNoManifest is returned when no manifest file is provided.
var ErrNoManifest = errors.New("no manifest file provided")
