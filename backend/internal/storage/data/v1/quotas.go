// Defines resource quotas persisted at server, organization, and workspace levels.

package v1

import (
	"errors"
)

// ResourceQuotas defines per-workspace content limits shared by server, org, and workspace layers.
// Semantics by layer:
//   - Server: all fields must be strictly positive (server is the ultimate fallback).
//   - Org and workspace: -1 means "inherit from parent" (skip this layer), 0 means "disabled"
//     (zero allowed), and a positive value overrides the parent with a stricter limit.
type ResourceQuotas struct {
	// MaxPages limits the number of pages per workspace.
	MaxPages int `json:"max_pages" jsonschema:"description=Maximum pages per workspace (-1=inherit, 0=disabled, positive=limit)"`

	// MaxStorageBytes limits current logical content bytes per workspace, including
	// serialized headers, schemas, records, and retained assets. Git history,
	// administrative settings, and temporary staging are not logical content.
	MaxStorageBytes int64 `json:"max_storage_bytes" jsonschema:"description=Maximum storage per workspace in bytes (-1=inherit, 0=disabled, positive=limit)"`

	// MaxRecordsPerTable limits records per table.
	MaxRecordsPerTable int `json:"max_records_per_table" jsonschema:"description=Maximum records per table (-1=inherit, 0=disabled, positive=limit)"`

	// MaxAssetSizeBytes limits the size of a single uploaded asset file.
	MaxAssetSizeBytes int64 `json:"max_asset_size_bytes" jsonschema:"description=Maximum single asset file size in bytes (-1=inherit, 0=disabled, positive=limit)"`

	// MaxTablesPerWorkspace limits tables within a single workspace.
	MaxTablesPerWorkspace int `json:"max_tables_per_workspace" jsonschema:"description=Maximum tables per workspace (-1=inherit, 0=disabled, positive=limit)"`

	// MaxColumnsPerTable limits properties/columns per table.
	MaxColumnsPerTable int `json:"max_columns_per_table" jsonschema:"description=Maximum columns per table (-1=inherit, 0=disabled, positive=limit)"`
}

// Validate checks that all quota values are valid for org/workspace layers.
// -1 means "inherit from parent", 0 means "disabled", and positive means a limit.
// Values below -1 are invalid.
func (q *ResourceQuotas) Validate() error {
	if q.MaxPages < -1 {
		return errors.New("max_pages must be -1 (inherit), 0 (disabled), or positive")
	}
	if q.MaxStorageBytes < -1 {
		return errors.New("max_storage_bytes must be -1 (inherit), 0 (disabled), or positive")
	}
	if q.MaxRecordsPerTable < -1 {
		return errors.New("max_records_per_table must be -1 (inherit), 0 (disabled), or positive")
	}
	if q.MaxAssetSizeBytes < -1 {
		return errors.New("max_asset_size_bytes must be -1 (inherit), 0 (disabled), or positive")
	}
	if q.MaxTablesPerWorkspace < -1 {
		return errors.New("max_tables_per_workspace must be -1 (inherit), 0 (disabled), or positive")
	}
	if q.MaxColumnsPerTable < -1 {
		return errors.New("max_columns_per_table must be -1 (inherit), 0 (disabled), or positive")
	}
	return nil
}

// ValidatePositive checks that all quota values are strictly positive.
// Used for server-level quotas where -1 (inherit) and 0 (disabled) are not allowed.
func (q *ResourceQuotas) ValidatePositive() error {
	if q.MaxPages <= 0 {
		return errors.New("max_pages must be positive")
	}
	if q.MaxStorageBytes <= 0 {
		return errors.New("max_storage_bytes must be positive")
	}
	if q.MaxRecordsPerTable <= 0 {
		return errors.New("max_records_per_table must be positive")
	}
	if q.MaxAssetSizeBytes <= 0 {
		return errors.New("max_asset_size_bytes must be positive")
	}
	if q.MaxTablesPerWorkspace <= 0 {
		return errors.New("max_tables_per_workspace must be positive")
	}
	if q.MaxColumnsPerTable <= 0 {
		return errors.New("max_columns_per_table must be positive")
	}
	return nil
}
