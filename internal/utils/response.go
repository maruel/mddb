package utils

import (
	"encoding/json"
	"net/http"
)

// SuccessResponse wraps a successful API response
type SuccessResponse struct {
	Data any `json:"data"`
}

// ErrorResponse wraps an error API response
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// RespondJSON sends a JSON response with the given status code.
func RespondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Error is already written to client, log only
		_ = err
	}
}

// RespondSuccess sends a successful JSON response.
func RespondSuccess(w http.ResponseWriter, status int, data any) {
	RespondJSON(w, status, SuccessResponse{Data: data})
}

// RespondError sends an error JSON response.
func RespondError(w http.ResponseWriter, status int, message, code string) {
	RespondJSON(w, status, ErrorResponse{Error: message, Code: code})
}
