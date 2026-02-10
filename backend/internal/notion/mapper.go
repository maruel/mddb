// Maps Notion types to mddb types.

package notion

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
)

// Mapper converts Notion types to mddb types.
type Mapper struct {
	// NotionToMddb maps Notion IDs to mddb IDs.
	NotionToMddb map[string]ksid.ID
	// PendingRelations maps property names to Notion database IDs that need resolution.
	PendingRelations map[string]string

	// Asset context for downloading files (set before mapping records)
	assets *AssetDownloader
	nodeID ksid.ID
}

// NewMapper creates a new type mapper.
func NewMapper() *Mapper {
	return &Mapper{
		NotionToMddb:     make(map[string]ksid.ID),
		PendingRelations: make(map[string]string),
	}
}

// NewMapperWithIDs creates a new type mapper with pre-existing ID mappings.
// Use this for incremental imports to reuse existing mddb IDs.
func NewMapperWithIDs(existingIDs map[string]ksid.ID) *Mapper {
	ids := make(map[string]ksid.ID, len(existingIDs))
	maps.Copy(ids, existingIDs)
	return &Mapper{
		NotionToMddb:     ids,
		PendingRelations: make(map[string]string),
	}
}

// SetAssetContext configures the mapper to download assets for the given node.
// Call this before mapping database records to enable file downloading.
func (m *Mapper) SetAssetContext(assets *AssetDownloader, nodeID ksid.ID) {
	m.assets = assets
	m.nodeID = nodeID
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
				// Try exact match first
				if mddbID, ok := m.NotionToMddb[notionDBID]; ok {
					prop.RelationConfig.TargetNodeID = mddbID
				} else {
					// Try normalized ID (Notion sometimes uses UUIDs with/without dashes)
					normalizedID := strings.ReplaceAll(notionDBID, "-", "")
					for notionID, mddbID := range m.NotionToMddb {
						if strings.ReplaceAll(notionID, "-", "") == normalizedID {
							prop.RelationConfig.TargetNodeID = mddbID
							break
						}
					}
				}
			}
		}
	}
}

// ClearPendingRelations clears the pending relations map for the next database.
func (m *Mapper) ClearPendingRelations() {
	m.PendingRelations = make(map[string]string)
}

// AssignNodeID pre-assigns an mddb ID to a Notion page or database ID.
// This allows parent resolution to work when children are processed before parents.
func (m *Mapper) AssignNodeID(notionID string) ksid.ID {
	if existing, ok := m.NotionToMddb[notionID]; ok {
		return existing
	}
	id := ksid.NewID()
	m.NotionToMddb[notionID] = id
	return id
}

// AssignRecordID pre-assigns an mddb ID to a Notion page ID without mapping it.
// This allows relation resolution to work for records in the same database.
func (m *Mapper) AssignRecordID(notionID string) ksid.ID {
	return m.AssignNodeID(notionID) // Same logic, different semantic name
}

// MapDatabase converts a Notion database to an mddb Node.
func (m *Mapper) MapDatabase(db *Database) (*content.Node, error) {
	// Use pre-assigned ID if available, otherwise create new
	nodeID, ok := m.NotionToMddb[db.ID]
	if !ok {
		nodeID = ksid.NewID()
		m.NotionToMddb[db.ID] = nodeID
	}

	node := &content.Node{
		ID:       nodeID,
		ParentID: m.resolveParentID(db.Parent),
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

// MapDatabaseIconCover downloads and sets the icon and cover for a database node.
// Call this after MapDatabase when you have an AssetDownloader available.
func (m *Mapper) MapDatabaseIconCover(node *content.Node, db *Database, assets *AssetDownloader) {
	if assets == nil {
		return
	}

	// Process icon
	if db.Icon != nil {
		if icon, err := assets.ProcessIcon(node.ID, db.Icon); err == nil && icon != "" {
			node.Icon = icon
		}
	}

	// Process cover
	if db.Cover != nil {
		if cover, err := assets.ProcessCover(node.ID, db.Cover); err == nil && cover != "" {
			node.Cover = cover
		}
	}
}

// mapDBProperty converts a Notion database property definition to mddb Property.
func (m *Mapper) mapDBProperty(name string, prop *DBProperty) *content.Property {
	mddbProp := &content.Property{
		Name: name,
	}

	switch prop.Type {
	case "title":
		mddbProp.Type = content.PropertyTypeText
	case "rich_text":
		mddbProp.Type = content.PropertyTypeMarkdown
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
	// Use pre-assigned ID if available, otherwise create new
	nodeID, ok := m.NotionToMddb[page.ID]
	if !ok {
		nodeID = ksid.NewID()
		m.NotionToMddb[page.ID] = nodeID
	}

	title := extractPageTitle(page)

	node := &content.Node{
		ID:       nodeID,
		ParentID: m.resolveParentID(page.Parent),
		Title:    title,
		Type:     content.NodeTypeDocument,
		Created:  storage.ToTime(page.CreatedTime),
		Modified: storage.ToTime(page.LastEditedTime),
	}

	return node, nil
}

// MapPageIconCover downloads and sets the icon and cover for a page node.
// Call this after MapPage when you have an AssetDownloader available.
func (m *Mapper) MapPageIconCover(node *content.Node, page *Page, assets *AssetDownloader) {
	if assets == nil {
		return
	}

	// Process icon
	if page.Icon != nil {
		if icon, err := assets.ProcessIcon(node.ID, page.Icon); err == nil && icon != "" {
			node.Icon = icon
		}
	}

	// Process cover
	if page.Cover != nil {
		if cover, err := assets.ProcessCover(node.ID, page.Cover); err == nil && cover != "" {
			node.Cover = cover
		}
	}
}

// resolveParentID converts a Notion Parent to an mddb node ID.
// Returns zero ID for workspace-level items or unresolved parents.
func (m *Mapper) resolveParentID(parent Parent) ksid.ID {
	var notionID string
	switch parent.Type {
	case "page_id":
		notionID = parent.PageID
	case "database_id":
		notionID = parent.DatabaseID
	case "block_id":
		notionID = parent.BlockID
	case "workspace":
		// Workspace-level items have no parent in mddb
		return 0
	default:
		return 0
	}

	// Try exact match first
	if mddbID, ok := m.NotionToMddb[notionID]; ok {
		return mddbID
	}

	// Try normalized ID (Notion sometimes uses UUIDs with/without dashes)
	normalizedID := strings.ReplaceAll(notionID, "-", "")
	for notionKey, mddbID := range m.NotionToMddb {
		if strings.ReplaceAll(notionKey, "-", "") == normalizedID {
			return mddbID
		}
	}

	// Parent not yet imported - return zero ID
	// This happens for:
	// - Child pages/databases whose parents weren't selected for import
	// - Block-level inline databases
	return 0
}

// MapDatabasePage converts a Notion page (database row) to an mddb DataRecord.
func (m *Mapper) MapDatabasePage(page *Page, schema map[string]DBProperty) (*content.DataRecord, error) {
	// Use pre-assigned ID if available (from AssignRecordID), otherwise create new
	recordID, ok := m.NotionToMddb[page.ID]
	if !ok {
		recordID = ksid.NewID()
		m.NotionToMddb[page.ID] = recordID
	}

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
		return richTextToMarkdown(pv.RichText)
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
		// Return mddb IDs for resolved relations, Notion IDs for unresolved
		var ids []string
		for _, rel := range pv.Relation {
			if mddbID, ok := m.NotionToMddb[rel.ID]; ok {
				ids = append(ids, mddbID.String())
			} else {
				// Keep Notion ID as fallback (cross-database relations may not be resolved yet)
				ids = append(ids, "notion:"+rel.ID)
			}
		}
		return ids
	case "rollup":
		if pv.Rollup != nil {
			return mapRollupValue(pv.Rollup)
		}
		return nil
	case "files":
		// Download Notion-hosted files and return local paths
		var paths []string
		for _, f := range pv.Files {
			var url string
			if f.File != nil {
				url = f.File.URL
			} else if f.External != nil {
				url = f.External.URL
			}
			if url == "" {
				continue
			}
			// Download if asset context is set
			if m.assets != nil && !m.nodeID.IsZero() {
				if localPath, err := m.assets.DownloadAsset(m.nodeID, url); err == nil {
					paths = append(paths, localPath)
					continue
				}
				// Fall through to use original URL on error
			}
			paths = append(paths, url)
		}
		return strings.Join(paths, "\n")
	case "unique_id":
		if pv.UniqueID != nil {
			if pv.UniqueID.Prefix != nil {
				return fmt.Sprintf("%s-%d", *pv.UniqueID.Prefix, pv.UniqueID.Number)
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
