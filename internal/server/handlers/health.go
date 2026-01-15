package handlers

import (
	"net/http"

	"github.com/maruel/mddb/internal/utils"
)

// HealthHandler handles health check endpoints
type HealthHandler struct{}

// NewHealthHandler creates a new health handler
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Health returns the health status of the server
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	utils.RespondSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}
