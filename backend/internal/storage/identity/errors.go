package identity

import (
	"errors"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// Shared error constants for identity services.
var (
	errUserIDEmpty   = errors.New("user id cannot be empty")
	errOrgIDEmpty    = errors.New("organization id cannot be empty")
	errEmailEmpty    = errors.New("email is required")
	errUserNotFound  = errors.New("user not found")
	errIDRequired    = errors.New("id is required")
	errNameRequired  = errors.New("name is required")
	errTokenRequired = errors.New("token is required")
	errQuotaExceeded = errors.New("quota exceeded")
)

// userOrgKey is a composite key for user+organization lookups.
type userOrgKey struct {
	UserID jsonldb.ID
	OrgID  jsonldb.ID
}
