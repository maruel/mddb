// Defines content errors and the verified revision available after a rejected mutation.

package content

import "errors"

// RevisionConflictError reports the revision observed while rejecting a stale mutation.
// A conflict does not distinguish a lost successful response from another writer.
type RevisionConflictError struct {
	CurrentRevision string
}

// Error describes the rejected mutation without claiming which writer changed it.
func (e *RevisionConflictError) Error() string { return ErrRevisionConflict.Error() }

// Unwrap preserves sentinel matching for revision conflicts.
func (e *RevisionConflictError) Unwrap() error { return ErrRevisionConflict }

var (
	// ErrRevisionConflict means the page changed since the supplied revision.
	ErrRevisionConflict = errors.New("page changed; read the latest revision before writing")
	errWSIDRequired     = errors.New("workspace ID is required")
	errOrgIDRequired    = errors.New("organization ID is required")
	// ErrNodeNotFound identifies missing node content, distinct from I/O failures.
	ErrNodeNotFound  = errors.New("page not found")
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
