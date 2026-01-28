// Handles health check endpoints.

package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/server/dto"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	cfg *Config
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(cfg *Config) *HealthHandler {
	return &HealthHandler{cfg: cfg}
}

// GetHealth handles health check requests.
func (h *HealthHandler) GetHealth(ctx context.Context, req *dto.HealthRequest) (*dto.HealthResponse, error) {
	return &dto.HealthResponse{
		Status:  "ok",
		Version: h.cfg.Version,
	}, nil
}
