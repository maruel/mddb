// Package errors defines structured error types for the API.
package errors

import (
	"fmt"
	"net/http"
)

// ErrorCode defines specific error types for the API.
type ErrorCode string

const (
	// Validation errors
	ErrValidationFailed ErrorCode = "VALIDATION_FAILED"
	ErrMissingField     ErrorCode = "MISSING_FIELD"
	ErrInvalidFormat    ErrorCode = "INVALID_FORMAT"

	// Resource errors
	ErrNotFound      ErrorCode = "NOT_FOUND"
	ErrAlreadyExists ErrorCode = "ALREADY_EXISTS"
	ErrConflict      ErrorCode = "CONFLICT"

	// File operation errors
	ErrFileNotFound  ErrorCode = "FILE_NOT_FOUND"
	ErrFileReadFail  ErrorCode = "FILE_READ_FAILED"
	ErrFileWriteFail ErrorCode = "FILE_WRITE_FAILED"
	ErrPathTraversal ErrorCode = "PATH_TRAVERSAL_ATTEMPT"

	// Server errors
	ErrInternal     ErrorCode = "INTERNAL_ERROR"
	ErrNotImplemented ErrorCode = "NOT_IMPLEMENTED"
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
	if details != nil {
		for k, v := range details {
			e.details[k] = v
		}
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

// MissingField creates a 400 error for a missing required field.
func MissingField(fieldName string) *APIError {
	return NewAPIError(http.StatusBadRequest, ErrMissingField, fmt.Sprintf("Missing required field: %s", fieldName)).
		WithDetail("field", fieldName)
}

// InvalidFormat creates a 400 error for invalid input format.
func InvalidFormat(fieldName, expected string) *APIError {
	return NewAPIError(http.StatusBadRequest, ErrInvalidFormat, fmt.Sprintf("Invalid format for %s: expected %s", fieldName, expected)).
		WithDetail("field", fieldName).
		WithDetail("expected", expected)
}

// Conflict creates a 409 Conflict error.
func Conflict(message string) *APIError {
	return NewAPIError(http.StatusConflict, ErrConflict, message)
}

// Internal creates a 500 Internal Server Error.
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
