// Handles health check endpoints.

package handlers

import (
	"context"
	"net/http"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/git"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	Cfg      *Config
	RootRepo *git.RootRepo
}

// GetHealth handles health check requests.
func (h *HealthHandler) GetHealth(ctx context.Context, req *dto.HealthRequest) (*dto.HealthResponse, error) {
	if h.RootRepo.HasPendingDBChanges() {
		return nil, dto.NewAPIError(http.StatusServiceUnavailable, dto.ErrorCodeStorageError, "Database history is pending recovery")
	}
	return &dto.HealthResponse{
		Status:    "ok",
		Version:   h.Cfg.Version,
		GoVersion: h.Cfg.GoVersion,
		Revision:  h.Cfg.Revision,
		Dirty:     h.Cfg.Dirty,
	}, nil
}
