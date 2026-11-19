// Resource quota defaults and the effective quota across server, organization, and workspace levels.

package storage

import (
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// AllInheritResourceQuotas returns a ResourceQuotas where every field is -1 (inherit).
// Use this as a placeholder when a layer should be excluded from EffectiveQuotas.
func AllInheritResourceQuotas() storagev1.ResourceQuotas {
	return storagev1.ResourceQuotas{
		MaxPages:              -1,
		MaxStorageBytes:       -1,
		MaxRecordsPerTable:    -1,
		MaxAssetSizeBytes:     -1,
		MaxTablesPerWorkspace: -1,
		MaxColumnsPerTable:    -1,
	}
}

// DefaultResourceQuotas returns the default server-level resource quotas.
func DefaultResourceQuotas() storagev1.ResourceQuotas {
	return storagev1.ResourceQuotas{
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
func EffectiveQuotas(server, org, ws storagev1.ResourceQuotas) storagev1.ResourceQuotas {
	return storagev1.ResourceQuotas{
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
