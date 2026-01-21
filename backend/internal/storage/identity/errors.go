package identity

import (
	"errors"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// Shared error constants for identity services.
var (
	errUserIDEmpty  = errors.New("user id cannot be empty")
	errOrgIDEmpty   = errors.New("organization id cannot be empty")
	errEmailEmpty   = errors.New("email is required")
	errUserNotFound = errors.New("user not found")
)

// membershipKey returns the composite key for a user-organization membership.
func membershipKey(userID, orgID jsonldb.ID) string {
	return userID.String() + "_" + orgID.String()
}
