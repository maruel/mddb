// Defines shared resource quotas applied at server, organization, and workspace levels.

package storage

import "errors"

// ResourceQuotas defines per-workspace content limits shared by server, org, and workspace layers.
// A zero value means "no limit at this layer" (inherit from other layers).
type ResourceQuotas struct {
	// MaxPages limits the number of pages per workspace.
	MaxPages int `json:"max_pages" jsonschema:"description=Maximum pages per workspace (0=no limit at this layer)"`

	// MaxStorageBytes limits storage per workspace.
	MaxStorageBytes int64 `json:"max_storage_bytes" jsonschema:"description=Maximum storage per workspace in bytes (0=no limit at this layer)"`

	// MaxRecordsPerTable limits records per table.
	MaxRecordsPerTable int `json:"max_records_per_table" jsonschema:"description=Maximum records per table (0=no limit at this layer)"`

	// MaxAssetSizeBytes limits the size of a single uploaded asset file.
	MaxAssetSizeBytes int64 `json:"max_asset_size_bytes" jsonschema:"description=Maximum single asset file size in bytes (0=no limit at this layer)"`

	// MaxTablesPerWorkspace limits tables within a single workspace.
	MaxTablesPerWorkspace int `json:"max_tables_per_workspace" jsonschema:"description=Maximum tables per workspace (0=no limit at this layer)"`

	// MaxColumnsPerTable limits properties/columns per table.
	MaxColumnsPerTable int `json:"max_columns_per_table" jsonschema:"description=Maximum columns per table (0=no limit at this layer)"`
}

// Validate checks that all quota values are non-negative.
func (q *ResourceQuotas) Validate() error {
	if q.MaxPages < 0 {
		return errors.New("max_pages must be non-negative")
	}
	if q.MaxStorageBytes < 0 {
		return errors.New("max_storage_bytes must be non-negative")
	}
	if q.MaxRecordsPerTable < 0 {
		return errors.New("max_records_per_table must be non-negative")
	}
	if q.MaxAssetSizeBytes < 0 {
		return errors.New("max_asset_size_bytes must be non-negative")
	}
	if q.MaxTablesPerWorkspace < 0 {
		return errors.New("max_tables_per_workspace must be non-negative")
	}
	if q.MaxColumnsPerTable < 0 {
		return errors.New("max_columns_per_table must be non-negative")
	}
	return nil
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

// EffectiveQuotas computes the effective quotas by taking the minimum positive
// value across server, org, and workspace layers for each field.
// A zero value at any layer means "no limit at this layer" and is ignored.
// If all layers are zero, the result is zero (unlimited).
func EffectiveQuotas(server, org, ws ResourceQuotas) ResourceQuotas {
	return ResourceQuotas{
		MaxPages:              minPositive(server.MaxPages, org.MaxPages, ws.MaxPages),
		MaxStorageBytes:       minPositiveInt64(server.MaxStorageBytes, org.MaxStorageBytes, ws.MaxStorageBytes),
		MaxRecordsPerTable:    minPositive(server.MaxRecordsPerTable, org.MaxRecordsPerTable, ws.MaxRecordsPerTable),
		MaxAssetSizeBytes:     minPositiveInt64(server.MaxAssetSizeBytes, org.MaxAssetSizeBytes, ws.MaxAssetSizeBytes),
		MaxTablesPerWorkspace: minPositive(server.MaxTablesPerWorkspace, org.MaxTablesPerWorkspace, ws.MaxTablesPerWorkspace),
		MaxColumnsPerTable:    minPositive(server.MaxColumnsPerTable, org.MaxColumnsPerTable, ws.MaxColumnsPerTable),
	}
}

// minPositive returns the minimum positive value among the arguments.
// Zero values are ignored. If all are zero, returns 0.
func minPositive(vals ...int) int {
	result := 0
	for _, v := range vals {
		if v > 0 && (result == 0 || v < result) {
			result = v
		}
	}
	return result
}

// minPositiveInt64 returns the minimum positive value among the arguments.
// Zero values are ignored. If all are zero, returns 0.
func minPositiveInt64(vals ...int64) int64 {
	var result int64
	for _, v := range vals {
		if v > 0 && (result == 0 || v < result) {
			result = v
		}
	}
	return result
}
