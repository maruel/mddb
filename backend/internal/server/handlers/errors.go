package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/maruel/mddb/backend/internal/models"
)

// writeErrorResponse writes an APIError as a JSON response.
// Use this in raw http.HandlerFunc handlers that don't use server.Wrap.
func writeErrorResponse(w http.ResponseWriter, err error) {
	statusCode := http.StatusInternalServerError
	errorCode := models.ErrorCodeInternal
	message := "internal error"
	var details map[string]any

	var ewsErr models.ErrorWithStatus
	if errors.As(err, &ewsErr) {
		statusCode = ewsErr.StatusCode()
		errorCode = ewsErr.Code()
		message = ewsErr.Error()
		details = ewsErr.Details()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := models.ErrorResponse{
		Error: models.ErrorDetails{
			Code:    errorCode,
			Message: message,
		},
		Details: details,
	}

	_ = json.NewEncoder(w).Encode(response)
}
