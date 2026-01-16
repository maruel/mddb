// Package errors defines structured error types for the API.
package errors

import (
	"fmt"
	"net/http"
)

// ErrorCode defines specific error types for the API.
type ErrorCode string

const (
	// ErrValidationFailed is returned when input data fails validation
	ErrValidationFailed ErrorCode = "VALIDATION_FAILED"
	// ErrMissingField is returned when a required field is missing
	ErrMissingField ErrorCode = "MISSING_FIELD"
	// ErrInvalidFormat is returned when a field has an invalid format
	ErrInvalidFormat ErrorCode = "INVALID_FORMAT"

	// ErrNotFound is returned when a resource is not found
	ErrNotFound ErrorCode = "NOT_FOUND"
	// ErrPageNotFound is returned when a page is not found
	ErrPageNotFound ErrorCode = "PAGE_NOT_FOUND"
	// ErrDatabaseNotFound is returned when a database is not found
	ErrDatabaseNotFound ErrorCode = "DATABASE_NOT_FOUND"

	// ErrFileNotFound is returned when a file is not found
	ErrFileNotFound ErrorCode = "FILE_NOT_FOUND"
	// ErrStorageError is returned when a storage operation fails
	ErrStorageError ErrorCode = "STORAGE_ERROR"

	// ErrInternal is returned when an unexpected server error occurs
	ErrInternal ErrorCode = "INTERNAL_ERROR"
	// ErrNotImplemented is returned when a feature is not implemented
	ErrNotImplemented ErrorCode = "NOT_IMPLEMENTED"
	// ErrConflict is returned when there is a resource conflict
	ErrConflict ErrorCode = "CONFLICT"
	// ErrUnauthorized is returned when authentication is missing or invalid
	ErrUnauthorized ErrorCode = "UNAUTHORIZED"
	// ErrForbidden is returned when a user has insufficient permissions
	ErrForbidden ErrorCode = "FORBIDDEN"
)

// ErrorWithStatus is an error that includes an HTTP status code and error code.
type ErrorWithStatus interface {
	Error() string
	StatusCode() int
	Code() ErrorCode
	Details() map[string]any
}

// APIError is a concrete error type with status code, code, and optional details.
type APIError struct {
	statusCode int
	code       ErrorCode
	message    string
	details    map[string]any
	wrappedErr error
}

// NewAPIError creates a new APIError with the given status code and message.
func NewAPIError(statusCode int, code ErrorCode, message string) *APIError {
	return &APIError{
		statusCode: statusCode,
		code:       code,
		message:    message,
		details:    make(map[string]any),
	}
}

// WithDetails adds details to the error.
func (e *APIError) WithDetails(details map[string]any) *APIError {
	if e.details == nil {
		e.details = make(map[string]any)
	}
	for k, v := range details {
		e.details[k] = v
	}
	return e
}

// WithDetail adds a single detail to the error.
func (e *APIError) WithDetail(key string, value any) *APIError {
	if e.details == nil {
		e.details = make(map[string]any)
	}
	e.details[key] = value
	return e
}

// Wrap wraps an underlying error.
func (e *APIError) Wrap(err error) *APIError {
	e.wrappedErr = err
	return e
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.wrappedErr != nil {
		return fmt.Sprintf("%s: %v", e.message, e.wrappedErr)
	}
	return e.message
}

// StatusCode returns the HTTP status code.
func (e *APIError) StatusCode() int {
	return e.statusCode
}

// Code returns the error code.
func (e *APIError) Code() ErrorCode {
	return e.code
}

// Details returns additional error details.
func (e *APIError) Details() map[string]any {
	return e.details
}

// Unwrap returns the wrapped error if any.
func (e *APIError) Unwrap() error {
	return e.wrappedErr
}

// Predefined error constructors for common cases

// NotFound creates a 404 Not Found error.
func NotFound(resource string) *APIError {
	return NewAPIError(http.StatusNotFound, ErrNotFound, fmt.Sprintf("%s not found", resource))
}

// BadRequest creates a 400 Bad Request error.
func BadRequest(message string) *APIError {
	return NewAPIError(http.StatusBadRequest, ErrValidationFailed, message)
}

// MissingField creates a 400 Bad Request error for a missing field.
func MissingField(fieldName string) *APIError {
	return NewAPIError(http.StatusBadRequest, ErrMissingField, fmt.Sprintf("Missing required field: %s", fieldName))
}

// Forbidden returns a 403 Forbidden error.
func Forbidden(message string) error {
	return NewAPIError(403, ErrForbidden, message)
}

// Unauthorized returns a 401 Unauthorized error.
func Unauthorized() error {
	return NewAPIError(401, ErrUnauthorized, "Unauthorized")
}

// Internal returns a 500 Internal Server Error.
func Internal(message string) *APIError {
	return NewAPIError(http.StatusInternalServerError, ErrInternal, message)
}

// InternalWithError creates a 500 error wrapping an underlying error.
func InternalWithError(message string, err error) *APIError {
	return Internal(message).Wrap(err)
}

// NotImplemented creates a 501 Not Implemented error.
func NotImplemented(feature string) *APIError {
	return NewAPIError(http.StatusNotImplemented, ErrNotImplemented, fmt.Sprintf("%s is not yet implemented", feature))
}
