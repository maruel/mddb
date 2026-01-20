package dto

import (
	"errors"
	"net/http"
	"testing"
)

func TestNewAPIError(t *testing.T) {
	err := NewAPIError(http.StatusNotFound, ErrorCodeNotFound, "resource not found")

	if err.StatusCode() != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, err.StatusCode())
	}
	if err.Code() != ErrorCodeNotFound {
		t.Errorf("Expected code %s, got %s", ErrorCodeNotFound, err.Code())
	}
	if err.Error() != "resource not found" {
		t.Errorf("Expected message 'resource not found', got '%s'", err.Error())
	}
	if err.Details() == nil {
		t.Error("Expected Details() to return non-nil map")
	}
}

func TestAPIError_WithDetails(t *testing.T) {
	err := NewAPIError(http.StatusBadRequest, ErrorCodeValidationFailed, "validation failed")
	err = err.WithDetails(map[string]any{"field": "email", "reason": "invalid format"})

	details := err.Details()
	if details["field"] != "email" {
		t.Errorf("Expected field 'email', got %v", details["field"])
	}
	if details["reason"] != "invalid format" {
		t.Errorf("Expected reason 'invalid format', got %v", details["reason"])
	}
}

func TestAPIError_WithDetails_NilMap(t *testing.T) {
	err := &APIError{
		statusCode: http.StatusBadRequest,
		code:       ErrorCodeValidationFailed,
		message:    "test",
		details:    nil, // explicitly nil
	}
	err = err.WithDetails(map[string]any{"key": "value"})

	if err.Details()["key"] != "value" {
		t.Error("Expected WithDetails to initialize nil map")
	}
}

func TestAPIError_WithDetail(t *testing.T) {
	err := NewAPIError(http.StatusBadRequest, ErrorCodeValidationFailed, "validation failed")
	err = err.WithDetail("field", "username")

	if err.Details()["field"] != "username" {
		t.Errorf("Expected field 'username', got %v", err.Details()["field"])
	}
}

func TestAPIError_WithDetail_NilMap(t *testing.T) {
	err := &APIError{
		statusCode: http.StatusBadRequest,
		code:       ErrorCodeValidationFailed,
		message:    "test",
		details:    nil,
	}
	err = err.WithDetail("key", "value")

	if err.Details()["key"] != "value" {
		t.Error("Expected WithDetail to initialize nil map")
	}
}

func TestAPIError_Wrap(t *testing.T) {
	origErr := errors.New("original error")
	err := NewAPIError(http.StatusInternalServerError, ErrorCodeInternal, "wrapped error")
	err = err.Wrap(origErr)

	if err.Unwrap() != origErr {
		t.Error("Expected Unwrap() to return the original error")
	}
	expected := "wrapped error: original error"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

func TestNotFound(t *testing.T) {
	err := NotFound("page")

	if err.StatusCode() != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, err.StatusCode())
	}
	if err.Code() != ErrorCodeNotFound {
		t.Errorf("Expected code %s, got %s", ErrorCodeNotFound, err.Code())
	}
	if err.Error() != "page not found" {
		t.Errorf("Expected message 'page not found', got '%s'", err.Error())
	}
}

func TestBadRequest(t *testing.T) {
	err := BadRequest("invalid input")

	if err.StatusCode() != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, err.StatusCode())
	}
	if err.Code() != ErrorCodeValidationFailed {
		t.Errorf("Expected code %s, got %s", ErrorCodeValidationFailed, err.Code())
	}
	if err.Error() != "invalid input" {
		t.Errorf("Expected message 'invalid input', got '%s'", err.Error())
	}
}

func TestMissingField(t *testing.T) {
	err := MissingField("email")

	if err.StatusCode() != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, err.StatusCode())
	}
	if err.Code() != ErrorCodeMissingField {
		t.Errorf("Expected code %s, got %s", ErrorCodeMissingField, err.Code())
	}
	expected := "Missing required field: email"
	if err.Error() != expected {
		t.Errorf("Expected message '%s', got '%s'", expected, err.Error())
	}
}

func TestForbidden(t *testing.T) {
	err := Forbidden("access denied")

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatal("Expected Forbidden to return *APIError")
	}
	if apiErr.StatusCode() != http.StatusForbidden {
		t.Errorf("Expected status code %d, got %d", http.StatusForbidden, apiErr.StatusCode())
	}
	if apiErr.Code() != ErrorCodeForbidden {
		t.Errorf("Expected code %s, got %s", ErrorCodeForbidden, apiErr.Code())
	}
}

func TestUnauthorized(t *testing.T) {
	err := Unauthorized()

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatal("Expected Unauthorized to return *APIError")
	}
	if apiErr.StatusCode() != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, apiErr.StatusCode())
	}
	if apiErr.Code() != ErrorCodeUnauthorized {
		t.Errorf("Expected code %s, got %s", ErrorCodeUnauthorized, apiErr.Code())
	}
}

func TestInternal(t *testing.T) {
	err := Internal("server error")

	if err.StatusCode() != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, err.StatusCode())
	}
	if err.Code() != ErrorCodeInternal {
		t.Errorf("Expected code %s, got %s", ErrorCodeInternal, err.Code())
	}
}

func TestInternalWithError(t *testing.T) {
	origErr := errors.New("database connection failed")
	err := InternalWithError("failed to fetch data", origErr)

	if err.StatusCode() != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, err.StatusCode())
	}
	if err.Unwrap() != origErr {
		t.Error("Expected InternalWithError to wrap the original error")
	}
}

func TestNotImplemented(t *testing.T) {
	err := NotImplemented("feature X")

	if err.StatusCode() != http.StatusNotImplemented {
		t.Errorf("Expected status code %d, got %d", http.StatusNotImplemented, err.StatusCode())
	}
	if err.Code() != ErrorCodeNotImplemented {
		t.Errorf("Expected code %s, got %s", ErrorCodeNotImplemented, err.Code())
	}
	expected := "feature X is not yet implemented"
	if err.Error() != expected {
		t.Errorf("Expected message '%s', got '%s'", expected, err.Error())
	}
}

func TestInvalidProvider(t *testing.T) {
	err := InvalidProvider()

	if err.StatusCode() != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, err.StatusCode())
	}
	if err.Code() != ErrorCodeInvalidProvider {
		t.Errorf("Expected code %s, got %s", ErrorCodeInvalidProvider, err.Code())
	}
}

func TestOAuthError(t *testing.T) {
	err := OAuthError("token exchange failed")

	if err.StatusCode() != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, err.StatusCode())
	}
	if err.Code() != ErrorCodeOAuthError {
		t.Errorf("Expected code %s, got %s", ErrorCodeOAuthError, err.Code())
	}
}

func TestExpired(t *testing.T) {
	err := Expired("invitation")

	if err.StatusCode() != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, err.StatusCode())
	}
	if err.Code() != ErrorCodeExpired {
		t.Errorf("Expected code %s, got %s", ErrorCodeExpired, err.Code())
	}
	expected := "invitation expired"
	if err.Error() != expected {
		t.Errorf("Expected message '%s', got '%s'", expected, err.Error())
	}
}
