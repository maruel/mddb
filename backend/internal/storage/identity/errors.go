package identity

import "errors"

// Shared error constants for identity services.
var (
	errUserIDEmpty  = errors.New("user id cannot be empty")
	errOrgIDEmpty   = errors.New("organization id cannot be empty")
	errEmailEmpty   = errors.New("email is required")
	errUserNotFound = errors.New("user not found")
)
