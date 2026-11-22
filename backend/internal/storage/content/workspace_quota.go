// Admits exact serialized workspace growth under the shared repository mutation gate.

package content

import (
	"fmt"
	"os"
)

// QuotaError identifies the workspace ceiling that rejected new growth.
// Existing over-limit resources may stay equal or shrink; other growth still
// needs to fit every applicable ceiling. Administrators use the same checks.
type QuotaError struct {
	Resource string
	Limit    int64
}

// Error identifies the rejected resource and ceiling.
func (e *QuotaError) Error() string {
	return fmt.Sprintf("workspace %s quota exceeded (limit %d)", e.Resource, e.Limit)
}

// Unwrap preserves quota sentinel matching for storage callers.
func (e *QuotaError) Unwrap() error { return errQuotaExceeded }

type workspaceUsage struct {
	pages, tables int
	bytes         int64
}

func quotaGrowth(resource string, before, after, limit int64) error {
	if after > before && after > limit {
		return &QuotaError{Resource: resource, Limit: limit}
	}
	return nil
}

func fileBytes(path string) (int64, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
