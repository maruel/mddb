// Maps Notion types to mddb types.

package notion

import (
	"strings"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
)

// Mapper converts Notion types to mddb types.
type Mapper struct {
	// NotionToMddb maps Notion IDs to mddb IDs.
	NotionToMddb map[string]jsonldb.ID
	// PendingRelations maps property names to Notion database IDs that need resolution.
	PendingRelations map[string]string
}

// NewMapper creates a new type mapper.
func NewMapper() *Mapper {
	return &Mapper{
		NotionToMddb:     make(map[string]jsonldb.ID),
		PendingRelations: make(map[string]string),
	}
}

// parseDateValue converts a Notion date string to epoch seconds (float64).
// Date-only values are parsed as midnight UTC.
// Using float64 ensures clean JSON round-trip; schema indicates it's a date.
func parseDateValue(s string) any {
	if s == "" {
		return nil
	}
	// Try datetime format first: "2025-10-22T12:30:00.000Z"
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return float64(t.Unix())
	}
	// Try datetime without timezone
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return float64(t.Unix())
	}
	// Try date-only format: "2025-10-22" → midnight UTC
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return float64(t.Unix())
	}
	return nil // unparseable
}

// ResolveRelations updates relation properties with resolved mddb node IDs.
// Call this after all databases have been mapped.
func (m *Mapper) ResolveRelations(node *content.Node) {
	for i := range node.Properties {
		prop := &node.Properties[i]
		if prop.Type == content.PropertyTypeRelation && prop.RelationConfig != nil {
			if notionDBID, ok := m.PendingRelations[prop.Name]; ok {
				if mddbID, ok := m.NotionToMddb[notionDBID]; ok {
					prop.RelationConfig.TargetNodeID = mddbID
				}
			}
		}
	}
}

// ClearPendingRelations clears the pending relations map for the next database.
func (m *Mapper) ClearPendingRelations() {
	m.PendingRelations = make(map[string]string)
}

// MapDatabase converts a Notion database to an mddb Node.
func (m *Mapper) MapDatabase(db *Database) (*content.Node, error) {
	nodeID := jsonldb.NewID()
	m.NotionToMddb[db.ID] = nodeID

	node := &content.Node{
		ID:       nodeID,
		Title:    richTextToPlain(db.Title),
		Type:     content.NodeTypeTable,
		Created:  storage.ToTime(db.CreatedTime),
		Modified: storage.ToTime(db.LastEditedTime),
	}

	// Convert properties to mddb schema
	for name := range db.Properties {
		prop := db.Properties[name]
		mddbProp := m.mapDBProperty(name, &prop)
		if mddbProp != nil {
			node.Properties = append(node.Properties, *mddbProp)
		}
	}

	return node, nil
}

// mapDBProperty converts a Notion database property definition to mddb Property.
func (m *Mapper) mapDBProperty(name string, prop *DBProperty) *content.Property {
	mddbProp := &content.Property{
		Name: name,
	}

	switch prop.Type {
	case "title", "rich_text":
		mddbProp.Type = content.PropertyTypeText
	case "number":
		mddbProp.Type = content.PropertyTypeNumber
	case "checkbox":
		mddbProp.Type = content.PropertyTypeCheckbox
	case "date", "created_time", "last_edited_time":
		mddbProp.Type = content.PropertyTypeDate
	case "select", "status":
		mddbProp.Type = content.PropertyTypeSelect
		if prop.Select != nil {
			mddbProp.Options = mapSelectOptions(prop.Select.Options)
		}
		if prop.Status != nil {
			mddbProp.Options = mapStatusOptions(prop.Status.Options)
		}
	case "multi_select":
		mddbProp.Type = content.PropertyTypeMultiSelect
		if prop.MultiSelect != nil {
			mddbProp.Options = mapSelectOptions(prop.MultiSelect.Options)
		}
	case "url":
		mddbProp.Type = content.PropertyTypeURL
	case "email":
		mddbProp.Type = content.PropertyTypeEmail
	case "phone_number":
		mddbProp.Type = content.PropertyTypePhone
	case "people", "created_by", "last_edited_by":
		// Map to text for now (names)
		mddbProp.Type = content.PropertyTypeText
	case "files":
		// Files will be handled separately as assets
		mddbProp.Type = content.PropertyTypeText
	case "formula":
		mddbProp.Type = content.PropertyTypeFormula
		if prop.Formula != nil {
			mddbProp.FormulaConfig = &content.FormulaConfig{
				Expression: prop.Formula.Expression,
			}
		}
	case "relation":
		mddbProp.Type = content.PropertyTypeRelation
		if prop.Relation != nil {
			// Store the Notion database ID; will be resolved to mddb ID after all DBs are mapped
			mddbProp.RelationConfig = &content.RelationConfig{
				// TargetNodeID will be resolved later via ResolveRelations()
				IsDualLink: prop.Relation.Type == "dual_property",
			}
			if prop.Relation.DualProperty != nil {
				mddbProp.RelationConfig.DualPropertyName = prop.Relation.DualProperty.SyncedPropertyName
			}
			// Store Notion DB ID temporarily for resolution
			m.PendingRelations[name] = prop.Relation.DatabaseID
		}
	case "rollup":
		mddbProp.Type = content.PropertyTypeRollup
		if prop.Rollup != nil {
			mddbProp.RollupConfig = &content.RollupConfig{
				RelationProperty: prop.Rollup.RelationPropertyName,
				TargetProperty:   prop.Rollup.RollupPropertyName,
				Aggregation:      mapRollupAggregation(prop.Rollup.Function),
			}
		}
	case "unique_id":
		// Unique IDs map to text
		mddbProp.Type = content.PropertyTypeText
	default:
		// Unknown type, use text as fallback
		mddbProp.Type = content.PropertyTypeText
	}

	return mddbProp
}

// mapSelectOptions converts Notion select options to mddb SelectOptions.
func mapSelectOptions(opts []SelectOption) []content.SelectOption {
	result := make([]content.SelectOption, 0, len(opts))
	for i := range opts {
		result = append(result, content.SelectOption{
			ID:    opts[i].ID,
			Name:  opts[i].Name,
			Color: opts[i].Color,
		})
	}
	return result
}

// mapStatusOptions converts Notion status options to mddb SelectOptions.
func mapStatusOptions(opts []StatusOption) []content.SelectOption {
	result := make([]content.SelectOption, 0, len(opts))
	for i := range opts {
		result = append(result, content.SelectOption{
			ID:    opts[i].ID,
			Name:  opts[i].Name,
			Color: opts[i].Color,
		})
	}
	return result
}

// MapPage converts a Notion page (standalone) to an mddb Node.
func (m *Mapper) MapPage(page *Page) (*content.Node, error) {
	nodeID := jsonldb.NewID()
	m.NotionToMddb[page.ID] = nodeID

	title := extractPageTitle(page)

	node := &content.Node{
		ID:       nodeID,
		Title:    title,
		Type:     content.NodeTypeDocument,
		Created:  storage.ToTime(page.CreatedTime),
		Modified: storage.ToTime(page.LastEditedTime),
	}

	return node, nil
}

// MapDatabasePage converts a Notion page (database row) to an mddb DataRecord.
func (m *Mapper) MapDatabasePage(page *Page, schema map[string]DBProperty) (*content.DataRecord, error) {
	recordID := jsonldb.NewID()
	m.NotionToMddb[page.ID] = recordID

	record := &content.DataRecord{
		ID:       recordID,
		Data:     make(map[string]any),
		Created:  storage.ToTime(page.CreatedTime),
		Modified: storage.ToTime(page.LastEditedTime),
	}

	// Map each property value
	for name := range page.Properties {
		propValue := page.Properties[name]
		value := m.mapPropertyValue(&propValue, schema[name].Type)
		if value != nil {
			record.Data[name] = value
		}
	}

	return record, nil
}

// mapPropertyValue converts a Notion property value to an mddb value.
// schemaType is preserved for future use with relation/rollup mapping.
func (m *Mapper) mapPropertyValue(pv *PropertyValue, _ string) any {
	switch pv.Type {
	case "title":
		return richTextToPlain(pv.Title)
	case "rich_text":
		return richTextToPlain(pv.RichText)
	case "number":
		if pv.Number != nil {
			return *pv.Number
		}
		return nil
	case "checkbox":
		if pv.Checkbox != nil {
			return *pv.Checkbox
		}
		return false
	case "select":
		if pv.Select != nil {
			return pv.Select.ID
		}
		return nil
	case "status":
		if pv.Status != nil {
			return pv.Status.ID
		}
		return nil
	case "multi_select":
		var ids []string
		for _, opt := range pv.MultiSelect {
			ids = append(ids, opt.ID)
		}
		return ids
	case "date":
		if pv.Date != nil {
			return parseDateValue(pv.Date.Start)
		}
		return nil
	case "url":
		if pv.URL != nil {
			return *pv.URL
		}
		return nil
	case "email":
		if pv.Email != nil {
			return *pv.Email
		}
		return nil
	case "phone_number":
		if pv.PhoneNumber != nil {
			return *pv.PhoneNumber
		}
		return nil
	case "people":
		var names []string
		for _, p := range pv.People {
			if p.Name != "" {
				names = append(names, p.Name)
			}
		}
		return strings.Join(names, ", ")
	case "created_by":
		if pv.CreatedBy != nil {
			return pv.CreatedBy.Name
		}
		return nil
	case "last_edited_by":
		if pv.LastEditedBy != nil {
			return pv.LastEditedBy.Name
		}
		return nil
	case "created_time":
		if pv.CreatedTime != nil {
			return float64(pv.CreatedTime.Unix())
		}
		return nil
	case "last_edited_time":
		if pv.LastEditedTime != nil {
			return float64(pv.LastEditedTime.Unix())
		}
		return nil
	case "formula":
		if pv.Formula != nil {
			return mapFormulaValue(pv.Formula)
		}
		return nil
	case "relation":
		// For now, return page IDs as comma-separated string
		// Phase 2 will handle proper relation mapping
		var ids []string
		for _, rel := range pv.Relation {
			ids = append(ids, rel.ID)
		}
		return strings.Join(ids, ",")
	case "rollup":
		if pv.Rollup != nil {
			return mapRollupValue(pv.Rollup)
		}
		return nil
	case "files":
		// Return file names/URLs as text for now
		var urls []string
		for _, f := range pv.Files {
			if f.File != nil {
				urls = append(urls, f.File.URL)
			} else if f.External != nil {
				urls = append(urls, f.External.URL)
			}
		}
		return strings.Join(urls, "\n")
	case "unique_id":
		if pv.UniqueID != nil {
			if pv.UniqueID.Prefix != nil {
				return *pv.UniqueID.Prefix + "-" + string(rune(pv.UniqueID.Number))
			}
			return pv.UniqueID.Number
		}
		return nil
	default:
		return nil
	}
}

// mapFormulaValue extracts the computed value from a formula.
func mapFormulaValue(f *FormulaValue) any {
	switch f.Type {
	case "string":
		if f.String != nil {
			return *f.String
		}
	case "number":
		if f.Number != nil {
			return *f.Number
		}
	case "boolean":
		if f.Boolean != nil {
			return *f.Boolean
		}
	case "date":
		if f.Date != nil {
			return parseDateValue(f.Date.Start)
		}
	}
	return nil
}

// mapRollupValue extracts the computed value from a rollup.
func mapRollupValue(r *RollupValue) any {
	switch r.Type {
	case "number":
		if r.Number != nil {
			return *r.Number
		}
	case "date":
		if r.Date != nil {
			return parseDateValue(r.Date.Start)
		}
	case "array":
		// For arrays, try to extract simple values
		var values []any
		for i := range r.Array {
			v := mapRollupArrayItem(&r.Array[i])
			if v != nil {
				values = append(values, v)
			}
		}
		return values
	}
	return nil
}

// mapRollupArrayItem extracts a value from a rollup array item.
func mapRollupArrayItem(pv *PropertyValue) any {
	switch pv.Type {
	case "title", "rich_text":
		return richTextToPlain(pv.Title)
	case "number":
		if pv.Number != nil {
			return *pv.Number
		}
	case "date":
		if pv.Date != nil {
			return parseDateValue(pv.Date.Start)
		}
	}
	return nil
}

// mapRollupAggregation converts a Notion rollup function to mddb RollupAggregation.
func mapRollupAggregation(notionFunc string) content.RollupAggregation {
	switch notionFunc {
	case "count":
		return content.RollupCount
	case "count_values":
		return content.RollupCountValues
	case "sum":
		return content.RollupSum
	case "average":
		return content.RollupAverage
	case "min":
		return content.RollupMin
	case "max":
		return content.RollupMax
	case "show_original":
		return content.RollupShowAll
	default:
		return content.RollupShowAll
	}
}

// richTextToPlain converts rich text to plain text.
func richTextToPlain(rt []RichText) string {
	parts := make([]string, 0, len(rt))
	for i := range rt {
		parts = append(parts, rt[i].PlainText)
	}
	return strings.Join(parts, "")
}

// extractPageTitle extracts the title from a page's properties.
func extractPageTitle(page *Page) string {
	for name := range page.Properties {
		prop := page.Properties[name]
		if prop.Type == "title" {
			return richTextToPlain(prop.Title)
		}
	}
	return "Untitled"
}
