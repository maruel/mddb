// Handles health check endpoints.

package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/server/dto"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	Cfg *Config
}

// GetHealth handles health check requests.
func (h *HealthHandler) GetHealth(ctx context.Context, req *dto.HealthRequest) (*dto.HealthResponse, error) {
	return &dto.HealthResponse{
		Status:    "ok",
		Version:   h.Cfg.Version,
		GoVersion: h.Cfg.GoVersion,
		Revision:  h.Cfg.Revision,
		Dirty:     h.Cfg.Dirty,
	}, nil
}
