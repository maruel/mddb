package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maruel/mddb/backend/internal/server/dto"
)

func TestWriteErrorResponse_APIError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   dto.ErrorCode
		expectedMsg    string
	}{
		{
			name:           "not found error",
			err:            dto.NotFound("page"),
			expectedStatus: http.StatusNotFound,
			expectedCode:   dto.ErrorCodeNotFound,
			expectedMsg:    "page not found",
		},
		{
			name:           "bad request error",
			err:            dto.BadRequest("invalid input"),
			expectedStatus: http.StatusBadRequest,
			expectedCode:   dto.ErrorCodeValidationFailed,
			expectedMsg:    "invalid input",
		},
		{
			name:           "unauthorized error",
			err:            dto.Unauthorized(),
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   dto.ErrorCodeUnauthorized,
			expectedMsg:    "Unauthorized",
		},
		{
			name:           "forbidden error",
			err:            dto.Forbidden("access denied"),
			expectedStatus: http.StatusForbidden,
			expectedCode:   dto.ErrorCodeForbidden,
			expectedMsg:    "access denied",
		},
		{
			name:           "internal error",
			err:            dto.Internal("server error"),
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   dto.ErrorCodeInternal,
			expectedMsg:    "server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeErrorResponse(w, tt.err)

			if w.Code != tt.expectedStatus {
				t.Errorf("status code = %d, want %d", w.Code, tt.expectedStatus)
			}

			if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
			}

			var resp dto.ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if resp.Error.Code != tt.expectedCode {
				t.Errorf("error code = %q, want %q", resp.Error.Code, tt.expectedCode)
			}
			if resp.Error.Message != tt.expectedMsg {
				t.Errorf("error message = %q, want %q", resp.Error.Message, tt.expectedMsg)
			}
		})
	}
}

func TestWriteErrorResponse_GenericError(t *testing.T) {
	w := httptest.NewRecorder()
	err := errors.New("some random error")
	writeErrorResponse(w, err)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status code = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var resp dto.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error.Code != dto.ErrorCodeInternal {
		t.Errorf("error code = %q, want %q", resp.Error.Code, dto.ErrorCodeInternal)
	}
	if resp.Error.Message != "internal error" {
		t.Errorf("error message = %q, want %q", resp.Error.Message, "internal error")
	}
}

func TestWriteErrorResponse_WithDetails(t *testing.T) {
	w := httptest.NewRecorder()
	err := dto.BadRequest("validation failed").
		WithDetails(map[string]any{"field": "email", "reason": "invalid format"})
	writeErrorResponse(w, err)

	var resp dto.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Details == nil {
		t.Fatal("expected details to be non-nil")
	}
	if resp.Details["field"] != "email" {
		t.Errorf("details[field] = %v, want %q", resp.Details["field"], "email")
	}
	if resp.Details["reason"] != "invalid format" {
		t.Errorf("details[reason] = %v, want %q", resp.Details["reason"], "invalid format")
	}
}
