package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/server/dto"
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

// GetHealth handles health check requests.
func (h *HealthHandler) GetHealth(ctx context.Context, req *dto.HealthRequest) (*dto.HealthResponse, error) {
	return &dto.HealthResponse{
		Status:  "ok",
		Version: h.version,
	}, nil
}
