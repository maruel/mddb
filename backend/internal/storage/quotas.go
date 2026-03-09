// Defines shared resource quotas applied at server, organization, and workspace levels.

package storage

import "errors"

// ResourceQuotas defines per-workspace content limits shared by server, org, and workspace layers.
// Semantics by layer:
//   - Server: all fields must be strictly positive (server is the ultimate fallback).
//   - Org and workspace: -1 means "inherit from parent" (skip this layer), 0 means "disabled"
//     (zero allowed), and a positive value overrides the parent with a stricter limit.
type ResourceQuotas struct {
	// MaxPages limits the number of pages per workspace.
	MaxPages int `json:"max_pages" jsonschema:"description=Maximum pages per workspace (-1=inherit, 0=disabled, positive=limit)"`

	// MaxStorageBytes limits storage per workspace.
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

// AllInheritResourceQuotas returns a ResourceQuotas where every field is -1 (inherit).
// Use this as a placeholder when a layer should be excluded from EffectiveQuotas.
func AllInheritResourceQuotas() ResourceQuotas {
	return ResourceQuotas{
		MaxPages:              -1,
		MaxStorageBytes:       -1,
		MaxRecordsPerTable:    -1,
		MaxAssetSizeBytes:     -1,
		MaxTablesPerWorkspace: -1,
		MaxColumnsPerTable:    -1,
	}
}

// DefaultResourceQuotas returns the default server-level resource quotas.
func DefaultResourceQuotas() ResourceQuotas {
	return ResourceQuotas{
		MaxPages:              1000,
		MaxStorageBytes:       1024 * 1024 * 1024, // 1 GiB
		MaxRecordsPerTable:    10000,
		MaxAssetSizeBytes:     50 * 1024 * 1024, // 50 MiB
		MaxTablesPerWorkspace: 100,
		MaxColumnsPerTable:    50,
	}
}

// EffectiveQuotas computes the effective quotas by taking the minimum non-inherit
// value across server, org, and workspace layers for each field.
// A -1 value means "inherit from parent" and is skipped.
// A 0 value means "disabled" and is treated as 0 (blocks all usage).
// The server layer must always have positive values, so the result is always ≥ 0.
func EffectiveQuotas(server, org, ws ResourceQuotas) ResourceQuotas {
	return ResourceQuotas{
		MaxPages:              minEffective(server.MaxPages, org.MaxPages, ws.MaxPages),
		MaxStorageBytes:       minEffectiveInt64(server.MaxStorageBytes, org.MaxStorageBytes, ws.MaxStorageBytes),
		MaxRecordsPerTable:    minEffective(server.MaxRecordsPerTable, org.MaxRecordsPerTable, ws.MaxRecordsPerTable),
		MaxAssetSizeBytes:     minEffectiveInt64(server.MaxAssetSizeBytes, org.MaxAssetSizeBytes, ws.MaxAssetSizeBytes),
		MaxTablesPerWorkspace: minEffective(server.MaxTablesPerWorkspace, org.MaxTablesPerWorkspace, ws.MaxTablesPerWorkspace),
		MaxColumnsPerTable:    minEffective(server.MaxColumnsPerTable, org.MaxColumnsPerTable, ws.MaxColumnsPerTable),
	}
}

// minEffective returns the minimum non-inherit value among the arguments.
// -1 values (inherit) are skipped. 0 and positive values are included.
// If all values are -1, returns -1 (fully inherited, caller must handle).
func minEffective(vals ...int) int {
	result := -1
	for _, v := range vals {
		if v != -1 && (result == -1 || v < result) {
			result = v
		}
	}
	return result
}

// minEffectiveInt64 returns the minimum non-inherit value among the arguments.
// -1 values (inherit) are skipped. 0 and positive values are included.
// If all values are -1, returns -1 (fully inherited, caller must handle).
func minEffectiveInt64(vals ...int64) int64 {
	result := int64(-1)
	for _, v := range vals {
		if v != -1 && (result == -1 || v < result) {
			result = v
		}
	}
	return result
}
