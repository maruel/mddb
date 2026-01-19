// Package models defines the core data structures used throughout the application.
package models

import (
	"fmt"
	"net/http"
)

// ErrorCode defines specific error types for the API.
type ErrorCode string

const (
	// ErrorCodeValidationFailed is returned when input data fails validation
	ErrorCodeValidationFailed ErrorCode = "VALIDATION_FAILED"
	// ErrorCodeMissingField is returned when a required field is missing
	ErrorCodeMissingField ErrorCode = "MISSING_FIELD"
	// ErrorCodeInvalidFormat is returned when a field has an invalid format
	ErrorCodeInvalidFormat ErrorCode = "INVALID_FORMAT"

	// ErrorCodeNotFound is returned when a resource is not found
	ErrorCodeNotFound ErrorCode = "NOT_FOUND"
	// ErrorCodePageNotFound is returned when a page is not found
	ErrorCodePageNotFound ErrorCode = "PAGE_NOT_FOUND"
	// ErrorCodeDatabaseNotFound is returned when a database is not found
	ErrorCodeDatabaseNotFound ErrorCode = "DATABASE_NOT_FOUND"

	// ErrorCodeFileNotFound is returned when a file is not found
	ErrorCodeFileNotFound ErrorCode = "FILE_NOT_FOUND"
	// ErrorCodeStorageError is returned when a storage operation fails
	ErrorCodeStorageError ErrorCode = "STORAGE_ERROR"

	// ErrorCodeInternal is returned when an unexpected server error occurs
	ErrorCodeInternal ErrorCode = "INTERNAL_ERROR"
	// ErrorCodeNotImplemented is returned when a feature is not implemented
	ErrorCodeNotImplemented ErrorCode = "NOT_IMPLEMENTED"
	// ErrorCodeConflict is returned when there is a resource conflict
	ErrorCodeConflict ErrorCode = "CONFLICT"
	// ErrorCodeUnauthorized is returned when authentication is missing or invalid
	ErrorCodeUnauthorized ErrorCode = "UNAUTHORIZED"
	// ErrorCodeForbidden is returned when a user has insufficient permissions
	ErrorCodeForbidden ErrorCode = "FORBIDDEN"

	// ErrorCodeInvalidProvider is returned when an OAuth provider is unknown
	ErrorCodeInvalidProvider ErrorCode = "INVALID_PROVIDER"
	// ErrorCodeOAuthError is returned when an OAuth operation fails
	ErrorCodeOAuthError ErrorCode = "OAUTH_ERROR"
	// ErrorCodeExpired is returned when a resource has expired
	ErrorCodeExpired ErrorCode = "EXPIRED"
)

// ErrorDetails defines the structured error information in a response.
type ErrorDetails struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

// ErrorResponse is the standard API error response.
type ErrorResponse struct {
	Error   ErrorDetails   `json:"error"`
	Details map[string]any `json:"details,omitempty"`
}

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
	return NewAPIError(http.StatusNotFound, ErrorCodeNotFound, fmt.Sprintf("%s not found", resource))
}

// BadRequest creates a 400 Bad Request error.
func BadRequest(message string) *APIError {
	return NewAPIError(http.StatusBadRequest, ErrorCodeValidationFailed, message)
}

// MissingField creates a 400 Bad Request error for a missing field.
func MissingField(fieldName string) *APIError {
	return NewAPIError(http.StatusBadRequest, ErrorCodeMissingField, fmt.Sprintf("Missing required field: %s", fieldName))
}

// Forbidden returns a 403 Forbidden error.
func Forbidden(message string) error {
	return NewAPIError(403, ErrorCodeForbidden, message)
}

// Unauthorized returns a 401 Unauthorized error.
func Unauthorized() error {
	return NewAPIError(401, ErrorCodeUnauthorized, "Unauthorized")
}

// Internal returns a 500 Internal Server Error.
func Internal(message string) *APIError {
	return NewAPIError(http.StatusInternalServerError, ErrorCodeInternal, message)
}

// InternalWithError creates a 500 error wrapping an underlying error.
func InternalWithError(message string, err error) *APIError {
	return Internal(message).Wrap(err)
}

// NotImplemented creates a 501 Not Implemented error.
func NotImplemented(feature string) *APIError {
	return NewAPIError(http.StatusNotImplemented, ErrorCodeNotImplemented, fmt.Sprintf("%s is not yet implemented", feature))
}

// InvalidProvider creates a 404 error for unknown OAuth providers.
func InvalidProvider() *APIError {
	return NewAPIError(http.StatusNotFound, ErrorCodeInvalidProvider, "unknown provider")
}

// OAuthError creates a 500 error for OAuth operation failures.
func OAuthError(operation string) *APIError {
	return NewAPIError(http.StatusInternalServerError, ErrorCodeOAuthError, operation)
}

// Expired creates a 400 error for expired resources.
func Expired(resource string) *APIError {
	return NewAPIError(http.StatusBadRequest, ErrorCodeExpired, fmt.Sprintf("%s expired", resource))
}
