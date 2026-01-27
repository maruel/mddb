// Tests for view manifest parsing.

package notion

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/storage/content"
)

func TestParseManifestBytes(t *testing.T) {
	yaml := `
version: 1
databases:
  - notion_id: "db-123"
    views:
      - name: "All Tasks"
        type: table
        default: true
        columns:
          - property: "Title"
            width: 300
          - property: "Status"
        sorts:
          - property: "Due Date"
            direction: asc
      - name: "By Status"
        type: board
        group_by: "Status"
        hidden_groups:
          - "Archived"
        filters:
          - property: "Archived"
            operator: not_equals
            value: true
`

	manifest, err := ParseManifestBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseManifestBytes failed: %v", err)
	}

	if manifest.Version != 1 {
		t.Errorf("expected version 1, got %d", manifest.Version)
	}

	if len(manifest.Databases) != 1 {
		t.Fatalf("expected 1 database, got %d", len(manifest.Databases))
	}

	db := manifest.Databases[0]
	if db.NotionID != "db-123" {
		t.Errorf("expected notion_id %q, got %q", "db-123", db.NotionID)
	}

	if len(db.Views) != 2 {
		t.Fatalf("expected 2 views, got %d", len(db.Views))
	}

	// Check first view
	view1 := db.Views[0]
	if view1.Name != "All Tasks" {
		t.Errorf("expected name %q, got %q", "All Tasks", view1.Name)
	}
	if view1.Type != "table" {
		t.Errorf("expected type %q, got %q", "table", view1.Type)
	}
	if !view1.Default {
		t.Error("expected default to be true")
	}
	if len(view1.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(view1.Columns))
	}
	if view1.Columns[0].Width != 300 {
		t.Errorf("expected width 300, got %d", view1.Columns[0].Width)
	}
	if len(view1.Sorts) != 1 {
		t.Errorf("expected 1 sort, got %d", len(view1.Sorts))
	}

	// Check second view
	view2 := db.Views[1]
	if view2.Name != "By Status" {
		t.Errorf("expected name %q, got %q", "By Status", view2.Name)
	}
	if view2.Type != "board" {
		t.Errorf("expected type %q, got %q", "board", view2.Type)
	}
	if view2.GroupBy != "Status" {
		t.Errorf("expected group_by %q, got %q", "Status", view2.GroupBy)
	}
	if len(view2.HiddenGroups) != 1 || view2.HiddenGroups[0] != "Archived" {
		t.Errorf("expected hidden_groups [Archived], got %v", view2.HiddenGroups)
	}
	if len(view2.Filters) != 1 {
		t.Errorf("expected 1 filter, got %d", len(view2.Filters))
	}
}

func TestParseManifestBytes_Invalid(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			"invalid version",
			`version: 99
databases: []`,
		},
		{
			"missing notion_id",
			`version: 1
databases:
  - views:
      - name: "Test"
        type: table`,
		},
		{
			"missing view name",
			`version: 1
databases:
  - notion_id: "db-123"
    views:
      - type: table`,
		},
		{
			"missing view type",
			`version: 1
databases:
  - notion_id: "db-123"
    views:
      - name: "Test"`,
		},
		{
			"invalid view type",
			`version: 1
databases:
  - notion_id: "db-123"
    views:
      - name: "Test"
        type: invalid`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseManifestBytes([]byte(tt.yaml))
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestViewsForDatabase(t *testing.T) {
	yaml := `
version: 1
databases:
  - notion_id: "db-1"
    views:
      - name: "View 1"
        type: table
  - notion_id: "db-2"
    views:
      - name: "View 2"
        type: board
      - name: "View 3"
        type: gallery
`

	manifest, err := ParseManifestBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseManifestBytes failed: %v", err)
	}

	t.Run("existing database", func(t *testing.T) {
		views := manifest.ViewsForDatabase("db-2")
		if len(views) != 2 {
			t.Fatalf("expected 2 views, got %d", len(views))
		}
		if views[0].Name != "View 2" {
			t.Errorf("expected %q, got %q", "View 2", views[0].Name)
		}
	})

	t.Run("non-existing database", func(t *testing.T) {
		views := manifest.ViewsForDatabase("db-999")
		if views != nil {
			t.Errorf("expected nil, got %v", views)
		}
	})
}

func TestToContentViews(t *testing.T) {
	yaml := `
version: 1
databases:
  - notion_id: "db-123"
    views:
      - name: "All Tasks"
        type: table
        default: true
        columns:
          - property: "Title"
            width: 300
          - property: "Status"
            visible: false
        sorts:
          - property: "Due Date"
            direction: desc
        filters:
          - property: "Active"
            operator: equals
            value: true
      - name: "By Status"
        type: board
        group_by: "Status"
        hidden_groups:
          - "Archived"
`

	manifest, err := ParseManifestBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseManifestBytes failed: %v", err)
	}

	views := manifest.ToContentViews("db-123")
	if len(views) != 2 {
		t.Fatalf("expected 2 views, got %d", len(views))
	}

	// Check first view conversion
	v1 := views[0]
	if v1.Name != "All Tasks" {
		t.Errorf("expected name %q, got %q", "All Tasks", v1.Name)
	}
	if v1.Type != content.ViewTypeTable {
		t.Errorf("expected type %q, got %q", content.ViewTypeTable, v1.Type)
	}
	if !v1.Default {
		t.Error("expected default to be true")
	}
	if v1.ID.IsZero() {
		t.Error("expected ID to be set")
	}

	// Check columns
	if len(v1.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(v1.Columns))
	}
	if v1.Columns[0].Property != "Title" || v1.Columns[0].Width != 300 || !v1.Columns[0].Visible {
		t.Errorf("column 0 mismatch: %+v", v1.Columns[0])
	}
	if v1.Columns[1].Visible {
		t.Error("expected column 1 to be hidden")
	}

	// Check sorts
	if len(v1.Sorts) != 1 {
		t.Fatalf("expected 1 sort, got %d", len(v1.Sorts))
	}
	if v1.Sorts[0].Direction != content.SortDesc {
		t.Errorf("expected desc, got %s", v1.Sorts[0].Direction)
	}

	// Check filters
	if len(v1.Filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(v1.Filters))
	}
	if v1.Filters[0].Operator != content.FilterOpEquals {
		t.Errorf("expected equals, got %s", v1.Filters[0].Operator)
	}

	// Check second view (board)
	v2 := views[1]
	if v2.Type != content.ViewTypeBoard {
		t.Errorf("expected type %q, got %q", content.ViewTypeBoard, v2.Type)
	}
	if len(v2.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(v2.Groups))
	}
	if v2.Groups[0].Property != "Status" {
		t.Errorf("expected group property %q, got %q", "Status", v2.Groups[0].Property)
	}
	if len(v2.Groups[0].Hidden) != 1 {
		t.Errorf("expected 1 hidden group, got %d", len(v2.Groups[0].Hidden))
	}
}

func TestParseManifestBytes_CompoundFilters(t *testing.T) {
	yaml := `
version: 1
databases:
  - notion_id: "db-123"
    views:
      - name: "Test"
        type: table
        filters:
          - and:
              - property: "Status"
                operator: equals
                value: "Active"
              - property: "Priority"
                operator: gt
                value: 5
          - or:
              - property: "Type"
                operator: equals
                value: "Bug"
              - property: "Type"
                operator: equals
                value: "Feature"
`

	manifest, err := ParseManifestBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseManifestBytes failed: %v", err)
	}

	views := manifest.ToContentViews("db-123")
	if len(views) != 1 {
		t.Fatalf("expected 1 view, got %d", len(views))
	}

	filters := views[0].Filters
	if len(filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(filters))
	}

	// Check AND filter
	andFilter := filters[0]
	if len(andFilter.And) != 2 {
		t.Errorf("expected 2 AND conditions, got %d", len(andFilter.And))
	}

	// Check OR filter
	orFilter := filters[1]
	if len(orFilter.Or) != 2 {
		t.Errorf("expected 2 OR conditions, got %d", len(orFilter.Or))
	}
}
