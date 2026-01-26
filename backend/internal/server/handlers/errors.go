// Provides helper functions for writing error responses.

package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/maruel/mddb/backend/internal/server/dto"
)

// writeErrorResponse writes an APIError as a JSON response.
// Use this in raw http.HandlerFunc handlers that don't use server.Wrap.
func writeErrorResponse(w http.ResponseWriter, err error) {
	statusCode := http.StatusInternalServerError
	errorCode := dto.ErrorCodeInternal
	message := "internal error"
	var details map[string]any

	var ewsErr dto.ErrorWithStatus
	if errors.As(err, &ewsErr) {
		statusCode = ewsErr.StatusCode()
		errorCode = ewsErr.Code()
		message = ewsErr.Error()
		details = ewsErr.Details()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := dto.ErrorResponse{
		Error: dto.ErrorDetails{
			Code:    errorCode,
			Message: message,
		},
		Details: details,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("Failed to encode error response", "error", err)
	}
}
