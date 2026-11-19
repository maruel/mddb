// Validation errors returned by persisted table rows and views.

package v1

import (
	"errors"
)

// Validation errors returned by the Validate methods of version 1 rows and views.
var (
	// ErrIDRequired is returned for a missing ID.
	ErrIDRequired = errors.New("ID is required")
	// ErrNameRequired is returned for a missing view name.
	ErrNameRequired = errors.New("name is required")
)
