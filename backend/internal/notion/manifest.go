// Parses view manifest YAML files for import.

package notion

import (
	"errors"
	"fmt"
	"os"

	"github.com/maruel/mddb/backend/internal/ksid"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"gopkg.in/yaml.v3"
)

// ViewManifest defines the structure of a view manifest file.
type ViewManifest struct {
	Version   int                  `yaml:"version"`
	Databases []DatabaseViewConfig `yaml:"databases"`
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
func (m *ViewManifest) ToContentViews(notionID string) []content.View {
	configs := m.ViewsForDatabase(notionID)
	if configs == nil {
		return nil
	}

	views := make([]content.View, 0, len(configs))
	for i := range configs {
		views = append(views, configToView(&configs[i]))
	}
	return views
}

// configToView converts a ViewConfig to a content.View.
func configToView(cfg *ViewConfig) content.View {
	view := content.View{
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
		view.Columns = append(view.Columns, content.ViewColumn{
			Property: col.Property,
			Width:    col.Width,
			Visible:  visible,
		})
	}

	// Convert sorts
	for i := range cfg.Sorts {
		sort := &cfg.Sorts[i]
		view.Sorts = append(view.Sorts, content.Sort{
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
		group := content.Group{
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
func configToFilter(cfg *FilterConfig) content.Filter {
	filter := content.Filter{
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
func mapViewType(t string) content.ViewType {
	switch t {
	case "table":
		return content.ViewTypeTable
	case "board":
		return content.ViewTypeBoard
	case "gallery":
		return content.ViewTypeGallery
	case "list":
		return content.ViewTypeList
	case "calendar":
		return content.ViewTypeCalendar
	default:
		return content.ViewTypeTable
	}
}

// mapSortDir converts a string sort direction to content.SortDir.
func mapSortDir(dir string) content.SortDir {
	switch dir {
	case "desc":
		return content.SortDesc
	default:
		return content.SortAsc
	}
}

// mapFilterOp converts a string filter operator to content.FilterOp.
func mapFilterOp(op string) content.FilterOp {
	switch op {
	case "equals":
		return content.FilterOpEquals
	case "not_equals":
		return content.FilterOpNotEquals
	case "contains":
		return content.FilterOpContains
	case "not_contains":
		return content.FilterOpNotContains
	case "starts_with":
		return content.FilterOpStartsWith
	case "ends_with":
		return content.FilterOpEndsWith
	case "gt":
		return content.FilterOpGreaterThan
	case "lt":
		return content.FilterOpLessThan
	case "gte":
		return content.FilterOpGreaterEqual
	case "lte":
		return content.FilterOpLessEqual
	case "is_empty":
		return content.FilterOpIsEmpty
	case "is_not_empty":
		return content.FilterOpIsNotEmpty
	default:
		return content.FilterOpEquals
	}
}

// ErrNoManifest is returned when no manifest file is provided.
var ErrNoManifest = errors.New("no manifest file provided")
