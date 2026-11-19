// Defines sentinel errors for identity operations.

package identity

import (
	"errors"

	"github.com/maruel/ksid"
)

// Shared error constants for identity services.
var (
	errUserNotFound  = errors.New("user not found")
	errQuotaExceeded = errors.New("quota exceeded")
)

// userOrgKey is a composite key for user+organization lookups.
type userOrgKey struct {
	UserID ksid.ID
	OrgID  ksid.ID
}
