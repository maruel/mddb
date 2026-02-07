// Defines sentinel errors for content operations.

package content

import "errors"

var (
	errWSIDRequired  = errors.New("workspace ID is required")
	errOrgIDRequired = errors.New("organization ID is required")
	errPageNotFound  = errors.New("page not found")
	errTableNotFound = errors.New("table not found")
	errAssetNotFound = errors.New("asset not found")
	errIDRequired    = errors.New("ID is required")
	errNameRequired  = errors.New("name is required")
	errQuotaExceeded = errors.New("quota exceeded")
	// ErrTableQuotaExceeded is returned when the table limit for a workspace is reached.
	ErrTableQuotaExceeded = errors.New("maximum number of tables per workspace exceeded")
	errCycleDetected      = errors.New("move would create a cycle")
	// ErrServerStorageQuotaExceeded is returned when the server-wide storage limit is reached.
	ErrServerStorageQuotaExceeded = errors.New("server storage quota exceeded")
)
