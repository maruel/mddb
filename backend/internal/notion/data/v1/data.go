// Persisted Notion import state: the ID mapping, node manifest, and table views.

// Package v1 defines version 1 of the files the Notion importer writes into a workspace:
// notion_id_mapping.json, nodes.jsonl, and views-only metadata.json.
package v1

import (
	"github.com/maruel/ksid"
	contentv1 "github.com/maruel/mddb/backend/internal/storage/content/data/v1"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// IDMapping stores the Notion ID to mddb ID mapping for incremental imports.
// It lives in notion_id_mapping.json in the workspace directory. A rerun
// reuses the mapped IDs, so references between nodes stay valid, and rewrites
// nodes.jsonl and each table's data.jsonl.
type IDMapping struct {
	Version int                `json:"version"`
	IDs     map[string]ksid.ID `json:"ids"` // Notion ID -> mddb ID
}

// IDMappingVersion is the only IDMapping version; every release wrote it.
const IDMappingVersion = 1

// TableMetadata is the views-only metadata.json the importer writes; the store reads it
// as content/data/v1 Table.
type TableMetadata struct {
	Views []contentv1.View `json:"views,omitempty"`
}

// NodeEntry is a manifest entry for a node (stored in nodes.jsonl).
type NodeEntry struct {
	ID       ksid.ID        `json:"id"`
	ParentID ksid.ID        `json:"parent_id,omitempty"`
	Title    string         `json:"title"`
	Type     string         `json:"type"`
	Icon     string         `json:"icon,omitempty"`
	Cover    string         `json:"cover,omitempty"`
	Created  storagev1.Time `json:"created"`
	Modified storagev1.Time `json:"modified"`
}
