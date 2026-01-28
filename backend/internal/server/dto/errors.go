// Defines structured error types and codes for the API.

// Package dto defines API request/response types and error handling.
//
// This package contains all types used for HTTP API communication via goty:
//   - Request types with path/query/json struct tags for parameter binding
//   - Response types with string IDs and RFC3339 timestamps for JSON serialization
//   - Structured error types with HTTP status codes and error codes
//   - API-specific types (Property, UserRole, NodeType, Settings, etc.)
//
// The dto package is the API contract layer, fully self-contained with no
// dependency on the entity package. This ensures that changes to internal
// domain models (entity) do not accidentally affect the API contract.
//
// Conversion between dto and entity types is handled by the handlers package
// (in convert.go), which imports both packages.
//
// Error handling follows a structured pattern:
//   - ErrorCode provides machine-readable error classification
//   - APIError wraps errors with HTTP status codes and details
//   - Constructor functions (NotFound, BadRequest, etc.) create common errors
package dto

import (
	"fmt"
	"maps"
	"net/http"
)

// ErrorCode defines specific error types for the API.
type ErrorCode string

const (
	// ErrorCodeValidationFailed is returned when input data fails validation.
	ErrorCodeValidationFailed ErrorCode = "VALIDATION_FAILED"
	// ErrorCodeMissingField is returned when a required field is missing.
	ErrorCodeMissingField ErrorCode = "MISSING_FIELD"
	// ErrorCodeInvalidFormat is returned when a field has an invalid format.
	ErrorCodeInvalidFormat ErrorCode = "INVALID_FORMAT"

	// ErrorCodeNotFound is returned when a resource is not found.
	ErrorCodeNotFound ErrorCode = "NOT_FOUND"
	// ErrorCodeNodeNotFound is returned when a node is not found.
	ErrorCodeNodeNotFound ErrorCode = "NODE_NOT_FOUND"
	// ErrorCodeTableNotFound is returned when a table is not found.
	ErrorCodeTableNotFound ErrorCode = "TABLE_NOT_FOUND"

	// ErrorCodeFileNotFound is returned when a file is not found.
	ErrorCodeFileNotFound ErrorCode = "FILE_NOT_FOUND"
	// ErrorCodeStorageError is returned when a storage operation fails.
	ErrorCodeStorageError ErrorCode = "STORAGE_ERROR"

	// ErrorCodeInternal is returned when an unexpected server error occurs.
	ErrorCodeInternal ErrorCode = "INTERNAL_ERROR"
	// ErrorCodeNotImplemented is returned when a feature is not implemented.
	ErrorCodeNotImplemented ErrorCode = "NOT_IMPLEMENTED"
	// ErrorCodeConflict is returned when there is a resource conflict.
	ErrorCodeConflict ErrorCode = "CONFLICT"
	// ErrorCodeUnauthorized is returned when authentication is missing or invalid.
	ErrorCodeUnauthorized ErrorCode = "UNAUTHORIZED"
	// ErrorCodeForbidden is returned when a user has insufficient permissions.
	ErrorCodeForbidden ErrorCode = "FORBIDDEN"

	// ErrorCodeInvalidProvider is returned when an OAuth provider is unknown.
	ErrorCodeInvalidProvider ErrorCode = "INVALID_PROVIDER"
	// ErrorCodeOAuthError is returned when an OAuth operation fails.
	ErrorCodeOAuthError ErrorCode = "OAUTH_ERROR"
	// ErrorCodeExpired is returned when a resource has expired.
	ErrorCodeExpired ErrorCode = "EXPIRED"
	// ErrorCodeRateLimitExceeded is returned when rate limit is exceeded.
	ErrorCodeRateLimitExceeded ErrorCode = "RATE_LIMIT_EXCEEDED"

	// ErrorCodeCannotUnlinkOnlyAuth is returned when trying to unlink the only authentication method.
	ErrorCodeCannotUnlinkOnlyAuth ErrorCode = "CANNOT_UNLINK_ONLY_AUTH"
	// ErrorCodeProviderAlreadyLinked is returned when trying to link an already linked provider.
	ErrorCodeProviderAlreadyLinked ErrorCode = "PROVIDER_ALREADY_LINKED"
	// ErrorCodeProviderNotLinked is returned when trying to unlink a provider that is not linked.
	ErrorCodeProviderNotLinked ErrorCode = "PROVIDER_NOT_LINKED"
	// ErrorCodeEmailInUse is returned when an email is already in use by another account.
	ErrorCodeEmailInUse ErrorCode = "EMAIL_IN_USE"

	// ErrorCodeQuotaExceeded is returned when a server-wide quota is exceeded.
	ErrorCodeQuotaExceeded ErrorCode = "QUOTA_EXCEEDED"
	// ErrorCodePayloadTooLarge is returned when the request body exceeds the size limit.
	ErrorCodePayloadTooLarge ErrorCode = "PAYLOAD_TOO_LARGE"
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

// APIError is a concrete error type with status code and optional details.
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
	maps.Copy(e.details, details)
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
	return NewAPIError(http.StatusNotFound, ErrorCodeNotFound, resource+" not found")
}

// BadRequest creates a 400 Bad Request error.
func BadRequest(message string) *APIError {
	return NewAPIError(http.StatusBadRequest, ErrorCodeValidationFailed, message)
}

// MissingField creates a 400 Bad Request error for a missing field.
func MissingField(fieldName string) *APIError {
	return NewAPIError(http.StatusBadRequest, ErrorCodeMissingField, "Missing required field: "+fieldName)
}

// InvalidField creates a 400 Bad Request error for an invalid field value.
func InvalidField(fieldName, reason string) *APIError {
	return NewAPIError(http.StatusBadRequest, ErrorCodeInvalidFormat, fieldName+": "+reason)
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
	return NewAPIError(http.StatusNotImplemented, ErrorCodeNotImplemented, feature+" is not yet implemented")
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
	return NewAPIError(http.StatusBadRequest, ErrorCodeExpired, resource+" expired")
}

// RateLimitExceeded creates a 429 error for rate limit violations.
func RateLimitExceeded(retryAfterSeconds int) *APIError {
	return NewAPIError(http.StatusTooManyRequests, ErrorCodeRateLimitExceeded,
		fmt.Sprintf("Too many requests. Please retry after %d seconds.", retryAfterSeconds)).
		WithDetail("retry_after_seconds", retryAfterSeconds)
}

// CannotUnlinkOnlyAuth creates a 400 error when trying to unlink the only auth method.
func CannotUnlinkOnlyAuth() *APIError {
	return NewAPIError(http.StatusBadRequest, ErrorCodeCannotUnlinkOnlyAuth,
		"Cannot unlink the only authentication method. Add a password or link another provider first.")
}

// ProviderAlreadyLinked creates a 409 error when provider is already linked.
func ProviderAlreadyLinked(provider string) *APIError {
	return NewAPIError(http.StatusConflict, ErrorCodeProviderAlreadyLinked,
		provider+" is already linked to your account")
}

// ProviderNotLinked creates a 404 error when provider is not linked.
func ProviderNotLinked(provider string) *APIError {
	return NewAPIError(http.StatusNotFound, ErrorCodeProviderNotLinked,
		provider+" is not linked to your account")
}

// EmailInUse creates a 409 error when email is already in use.
func EmailInUse() *APIError {
	return NewAPIError(http.StatusConflict, ErrorCodeEmailInUse,
		"This email address is already in use by another account")
}

// QuotaExceeded creates a 429 error for server-wide quota violations.
func QuotaExceeded(resource string, limit int) *APIError {
	return NewAPIError(http.StatusTooManyRequests, ErrorCodeQuotaExceeded,
		fmt.Sprintf("Quota exceeded: maximum %d %s allowed", limit, resource)).
		WithDetail("resource", resource).
		WithDetail("limit", limit)
}

// QuotaExceededInt64 creates a 429 error for server-wide quota violations with int64 limit.
func QuotaExceededInt64(resource string, limit int64) *APIError {
	return NewAPIError(http.StatusTooManyRequests, ErrorCodeQuotaExceeded,
		fmt.Sprintf("Quota exceeded: maximum %d %s allowed", limit, resource)).
		WithDetail("resource", resource).
		WithDetail("limit", limit)
}

// PayloadTooLarge creates a 413 error for oversized request bodies.
func PayloadTooLarge(maxBytes int64) *APIError {
	return NewAPIError(http.StatusRequestEntityTooLarge, ErrorCodePayloadTooLarge,
		fmt.Sprintf("Request body too large: maximum %d bytes allowed", maxBytes)).
		WithDetail("max_bytes", maxBytes)
}
