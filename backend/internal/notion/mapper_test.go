// Tests for the Notion to mddb type mapper.

package notion

import (
	"testing"
	"time"

	"github.com/maruel/mddb/backend/internal/rid"
	"github.com/maruel/mddb/backend/internal/storage/content"
)

func TestMapper(t *testing.T) {
	t.Run("MapDatabase", func(t *testing.T) {
		m := NewMapper()
		now := time.Now()

		db := &Database{
			ID:             "db-123",
			CreatedTime:    now,
			LastEditedTime: now,
			Title:          []RichText{{PlainText: "Test Database"}},
			Properties: map[string]DBProperty{
				"Name": {
					Name: "Name",
					Type: "title",
				},
				"Status": {
					Name: "Status",
					Type: "select",
					Select: &SelectConfig{
						Options: []SelectOption{
							{ID: "opt-1", Name: "Open", Color: "green"},
							{ID: "opt-2", Name: "Closed", Color: "red"},
						},
					},
				},
				"Count": {
					Name: "Count",
					Type: "number",
				},
			},
		}

		node, err := m.MapDatabase(db)
		if err != nil {
			t.Fatalf("MapDatabase failed: %v", err)
		}

		if node.Title != "Test Database" {
			t.Errorf("expected title %q, got %q", "Test Database", node.Title)
		}

		if node.Type != content.NodeTypeTable {
			t.Errorf("expected type %q, got %q", content.NodeTypeTable, node.Type)
		}

		if len(node.Properties) != 3 {
			t.Errorf("expected 3 properties, got %d", len(node.Properties))
		}

		// Verify ID mapping
		if _, ok := m.NotionToMddb[db.ID]; !ok {
			t.Error("expected database ID to be mapped")
		}
	})

	t.Run("MapPage", func(t *testing.T) {
		m := NewMapper()
		now := time.Now()

		page := &Page{
			ID:             "page-123",
			CreatedTime:    now,
			LastEditedTime: now,
			Properties: map[string]PropertyValue{
				"title": {
					Type:  "title",
					Title: []RichText{{PlainText: "Test Page"}},
				},
			},
		}

		node, err := m.MapPage(page)
		if err != nil {
			t.Fatalf("MapPage failed: %v", err)
		}

		if node.Title != "Test Page" {
			t.Errorf("expected title %q, got %q", "Test Page", node.Title)
		}

		if node.Type != content.NodeTypeDocument {
			t.Errorf("expected type %q, got %q", content.NodeTypeDocument, node.Type)
		}

		// Verify ID mapping
		if _, ok := m.NotionToMddb[page.ID]; !ok {
			t.Error("expected page ID to be mapped")
		}
	})

	t.Run("MapDatabasePage", func(t *testing.T) {
		m := NewMapper()
		now := time.Now()

		schema := map[string]DBProperty{
			"Name":   {Name: "Name", Type: "title"},
			"Status": {Name: "Status", Type: "select"},
			"Count":  {Name: "Count", Type: "number"},
			"Done":   {Name: "Done", Type: "checkbox"},
		}

		number := 42.0
		checked := true

		page := &Page{
			ID:             "row-123",
			CreatedTime:    now,
			LastEditedTime: now,
			Properties: map[string]PropertyValue{
				"Name": {
					Type:  "title",
					Title: []RichText{{PlainText: "Test Row"}},
				},
				"Status": {
					Type:   "select",
					Select: &SelectValue{ID: "opt-1", Name: "Open"},
				},
				"Count": {
					Type:   "number",
					Number: &number,
				},
				"Done": {
					Type:     "checkbox",
					Checkbox: &checked,
				},
			},
		}

		record, err := m.MapDatabasePage(page, schema)
		if err != nil {
			t.Fatalf("MapDatabasePage failed: %v", err)
		}

		if record.Data["Name"] != "Test Row" {
			t.Errorf("expected Name %q, got %q", "Test Row", record.Data["Name"])
		}

		if record.Data["Status"] != "opt-1" {
			t.Errorf("expected Status %q, got %q", "opt-1", record.Data["Status"])
		}

		if record.Data["Count"] != 42.0 {
			t.Errorf("expected Count %v, got %v", 42.0, record.Data["Count"])
		}

		if record.Data["Done"] != true {
			t.Errorf("expected Done %v, got %v", true, record.Data["Done"])
		}
	})
}

func TestMapDBProperty(t *testing.T) {
	m := NewMapper()

	tests := []struct {
		name     string
		prop     DBProperty
		wantType content.PropertyType
	}{
		{"title", DBProperty{Type: "title"}, content.PropertyTypeText},
		{"rich_text", DBProperty{Type: "rich_text"}, content.PropertyTypeMarkdown},
		{"number", DBProperty{Type: "number"}, content.PropertyTypeNumber},
		{"checkbox", DBProperty{Type: "checkbox"}, content.PropertyTypeCheckbox},
		{"date", DBProperty{Type: "date"}, content.PropertyTypeDate},
		{"select", DBProperty{Type: "select"}, content.PropertyTypeSelect},
		{"multi_select", DBProperty{Type: "multi_select"}, content.PropertyTypeMultiSelect},
		{"url", DBProperty{Type: "url"}, content.PropertyTypeURL},
		{"email", DBProperty{Type: "email"}, content.PropertyTypeEmail},
		{"phone_number", DBProperty{Type: "phone_number"}, content.PropertyTypePhone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.mapDBProperty(tt.name, &tt.prop)
			if result.Type != tt.wantType {
				t.Errorf("mapDBProperty(%q) = %q, want %q", tt.name, result.Type, tt.wantType)
			}
		})
	}
}

func TestRichTextToPlain(t *testing.T) {
	tests := []struct {
		name string
		rt   []RichText
		want string
	}{
		{"empty", nil, ""},
		{"single", []RichText{{PlainText: "Hello"}}, "Hello"},
		{"multiple", []RichText{{PlainText: "Hello "}, {PlainText: "World"}}, "Hello World"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := richTextToPlain(tt.rt)
			if got != tt.want {
				t.Errorf("richTextToPlain() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMapRollupAggregation(t *testing.T) {
	tests := []struct {
		notionFunc string
		want       content.RollupAggregation
	}{
		{"count", content.RollupCount},
		{"count_values", content.RollupCountValues},
		{"sum", content.RollupSum},
		{"average", content.RollupAverage},
		{"min", content.RollupMin},
		{"max", content.RollupMax},
		{"show_original", content.RollupShowAll},
		{"unknown", content.RollupShowAll},
	}

	for _, tt := range tests {
		t.Run(tt.notionFunc, func(t *testing.T) {
			got := mapRollupAggregation(tt.notionFunc)
			if got != tt.want {
				t.Errorf("mapRollupAggregation(%q) = %q, want %q", tt.notionFunc, got, tt.want)
			}
		})
	}
}

func TestMapDBPropertyRelational(t *testing.T) {
	m := NewMapper()

	t.Run("relation", func(t *testing.T) {
		prop := DBProperty{
			Type: "relation",
			Relation: &RelationConfig{
				DatabaseID: "target-db-123",
				Type:       "dual_property",
				DualProperty: &DualPropertyConfig{
					SyncedPropertyName: "BackRef",
				},
			},
		}
		result := m.mapDBProperty("Related Items", &prop)

		if result.Type != content.PropertyTypeRelation {
			t.Errorf("expected type %q, got %q", content.PropertyTypeRelation, result.Type)
		}
		if result.RelationConfig == nil {
			t.Fatal("expected RelationConfig to be set")
		}
		if !result.RelationConfig.IsDualLink {
			t.Error("expected IsDualLink to be true for dual_property")
		}
		if result.RelationConfig.DualPropertyName != "BackRef" {
			t.Errorf("expected DualPropertyName %q, got %q", "BackRef", result.RelationConfig.DualPropertyName)
		}
		// Check pending relation was stored
		if m.PendingRelations["Related Items"] != "target-db-123" {
			t.Error("expected pending relation to be stored")
		}
	})

	t.Run("rollup", func(t *testing.T) {
		prop := DBProperty{
			Type: "rollup",
			Rollup: &RollupConfig{
				RelationPropertyName: "Related Items",
				RollupPropertyName:   "Price",
				Function:             "sum",
			},
		}
		result := m.mapDBProperty("Total", &prop)

		if result.Type != content.PropertyTypeRollup {
			t.Errorf("expected type %q, got %q", content.PropertyTypeRollup, result.Type)
		}
		if result.RollupConfig == nil {
			t.Fatal("expected RollupConfig to be set")
		}
		if result.RollupConfig.RelationProperty != "Related Items" {
			t.Errorf("expected RelationProperty %q, got %q", "Related Items", result.RollupConfig.RelationProperty)
		}
		if result.RollupConfig.TargetProperty != "Price" {
			t.Errorf("expected TargetProperty %q, got %q", "Price", result.RollupConfig.TargetProperty)
		}
		if result.RollupConfig.Aggregation != content.RollupSum {
			t.Errorf("expected Aggregation %q, got %q", content.RollupSum, result.RollupConfig.Aggregation)
		}
	})

	t.Run("formula", func(t *testing.T) {
		prop := DBProperty{
			Type: "formula",
			Formula: &FormulaConfig{
				Expression: "prop(\"Price\") * prop(\"Quantity\")",
			},
		}
		result := m.mapDBProperty("Total", &prop)

		if result.Type != content.PropertyTypeFormula {
			t.Errorf("expected type %q, got %q", content.PropertyTypeFormula, result.Type)
		}
		if result.FormulaConfig == nil {
			t.Fatal("expected FormulaConfig to be set")
		}
		if result.FormulaConfig.Expression != "prop(\"Price\") * prop(\"Quantity\")" {
			t.Errorf("expected Expression %q, got %q", "prop(\"Price\") * prop(\"Quantity\")", result.FormulaConfig.Expression)
		}
	})
}

func TestResolveRelations(t *testing.T) {
	m := NewMapper()

	// Simulate mapping two databases
	mddbID1 := rid.NewID()
	mddbID2 := rid.NewID()
	m.NotionToMddb["notion-db-1"] = mddbID1
	m.NotionToMddb["notion-db-2"] = mddbID2

	// Store pending relation
	m.PendingRelations["Tasks"] = "notion-db-2"

	// Create a node with a relation property
	node := &content.Node{
		Properties: []content.Property{
			{
				Name: "Tasks",
				Type: content.PropertyTypeRelation,
				RelationConfig: &content.RelationConfig{
					// TargetNodeID not set yet
					IsDualLink: true,
				},
			},
			{
				Name: "Name",
				Type: content.PropertyTypeText,
			},
		},
	}

	m.ResolveRelations(node)

	// Check that the relation was resolved
	var tasksProp *content.Property
	for i := range node.Properties {
		if node.Properties[i].Name == "Tasks" {
			tasksProp = &node.Properties[i]
			break
		}
	}

	if tasksProp == nil {
		t.Fatal("Tasks property not found")
	}
	if tasksProp.RelationConfig.TargetNodeID != mddbID2 {
		t.Errorf("expected TargetNodeID %v, got %v", mddbID2, tasksProp.RelationConfig.TargetNodeID)
	}
}

func TestClearPendingRelations(t *testing.T) {
	m := NewMapper()
	m.PendingRelations["Tasks"] = "db-123"
	m.PendingRelations["Projects"] = "db-456"

	m.ClearPendingRelations()

	if len(m.PendingRelations) != 0 {
		t.Errorf("expected empty PendingRelations, got %d items", len(m.PendingRelations))
	}
}

func TestExtractPageTitle(t *testing.T) {
	tests := []struct {
		name string
		page *Page
		want string
	}{
		{
			"with title",
			&Page{Properties: map[string]PropertyValue{
				"Name": {Type: "title", Title: []RichText{{PlainText: "My Page"}}},
			}},
			"My Page",
		},
		{
			"no title property",
			&Page{Properties: map[string]PropertyValue{
				"Status": {Type: "select"},
			}},
			"Untitled",
		},
		{
			"empty properties",
			&Page{Properties: map[string]PropertyValue{}},
			"Untitled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPageTitle(tt.page)
			if got != tt.want {
				t.Errorf("extractPageTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}
