package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/models"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	version string
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(version string) *HealthHandler {
	return &HealthHandler{
		version: version,
	}
}

// Health handles health check requests.
func (h *HealthHandler) Health(ctx context.Context, req models.HealthRequest) (*models.HealthResponse, error) {
	return &models.HealthResponse{
		Status:  "ok",
		Version: h.version,
	}, nil
}
