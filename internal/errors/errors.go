package errors

// ErrorWithStatus is an error that includes an HTTP status code.
type ErrorWithStatus interface {
	Error() string
	StatusCode() int
}

// APIError is a concrete error type with status code.
type APIError struct {
	statusCode int
	message    string
}

// NewAPIError creates a new APIError with the given status code and message.
func NewAPIError(statusCode int, message string) *APIError {
	return &APIError{
		statusCode: statusCode,
		message:    message,
	}
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return e.message
}

// StatusCode returns the HTTP status code.
func (e *APIError) StatusCode() int {
	return e.statusCode
}
