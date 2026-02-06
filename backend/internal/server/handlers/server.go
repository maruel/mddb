// Handles server configuration endpoints for global admins.

package handlers

import (
	"context"
	"fmt"

	"github.com/maruel/mddb/backend/internal/email"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// ServerHandler handles server configuration endpoints.
type ServerHandler struct {
	Cfg              *storage.ServerConfig
	DataDir          string
	FileStore        *content.FileStoreService // for cache invalidation on quota changes
	BandwidthLimiter BandwidthUpdater          // for hot-reload of bandwidth limit
	RateLimiters     RateLimitsUpdater         // for hot-reload of rate limits
}

// GetConfig returns the current server configuration with masked password.
func (h *ServerHandler) GetConfig(ctx context.Context, _ *identity.User, _ *dto.ServerConfigRequest) (*dto.ServerConfigResponse, error) {
	rq := h.Cfg.Quotas.ResourceQuotas
	return &dto.ServerConfigResponse{
		SMTP: dto.SMTPConfigResponse{
			Host:     h.Cfg.SMTP.Host,
			Port:     h.Cfg.SMTP.Port,
			Username: h.Cfg.SMTP.Username,
			From:     h.Cfg.SMTP.From,
			// Password intentionally omitted (masked)
		},
		Quotas: dto.QuotasConfigResponse{
			ResourceQuotas: dto.ResourceQuotas{
				MaxPages:              rq.MaxPages,
				MaxStorageBytes:       rq.MaxStorageBytes,
				MaxRecordsPerTable:    rq.MaxRecordsPerTable,
				MaxAssetSizeBytes:     rq.MaxAssetSizeBytes,
				MaxTablesPerWorkspace: rq.MaxTablesPerWorkspace,
				MaxColumnsPerTable:    rq.MaxColumnsPerTable,
			},
			MaxRequestBodyBytes:   h.Cfg.Quotas.MaxRequestBodyBytes,
			MaxSessionsPerUser:    h.Cfg.Quotas.MaxSessionsPerUser,
			MaxOrganizations:      h.Cfg.Quotas.MaxOrganizations,
			MaxWorkspaces:         h.Cfg.Quotas.MaxWorkspaces,
			MaxUsers:              h.Cfg.Quotas.MaxUsers,
			MaxTotalStorageBytes:  h.Cfg.Quotas.MaxTotalStorageBytes,
			MaxEgressBandwidthBps: h.Cfg.Quotas.MaxEgressBandwidthBps,
		},
		RateLimits: dto.RateLimitsConfigResponse{
			AuthRatePerMin:       h.Cfg.RateLimits.AuthRatePerMin,
			WriteRatePerMin:      h.Cfg.RateLimits.WriteRatePerMin,
			ReadAuthRatePerMin:   h.Cfg.RateLimits.ReadAuthRatePerMin,
			ReadUnauthRatePerMin: h.Cfg.RateLimits.ReadUnauthRatePerMin,
		},
	}, nil
}

// UpdateConfig updates the server configuration and saves to disk.
func (h *ServerHandler) UpdateConfig(ctx context.Context, _ *identity.User, req *dto.UpdateServerConfigRequest) (*dto.UpdateServerConfigResponse, error) {
	// Update SMTP if provided
	if req.SMTP != nil {
		newSMTP := email.Config{
			Host:     req.SMTP.Host,
			Port:     req.SMTP.Port,
			Username: req.SMTP.Username,
			Password: req.SMTP.Password,
			From:     req.SMTP.From,
		}
		// If password is empty, preserve existing password
		if newSMTP.Password == "" {
			newSMTP.Password = h.Cfg.SMTP.Password
		}
		// Validate the new SMTP config
		if err := newSMTP.Validate(); err != nil {
			return nil, dto.InvalidField("smtp", err.Error())
		}
		h.Cfg.SMTP = newSMTP
	}

	// Update quotas if provided
	if req.Quotas != nil {
		newQuotas := storage.ServerQuotas{
			ResourceQuotas: storage.ResourceQuotas{
				MaxPages:              req.Quotas.MaxPages,
				MaxStorageBytes:       req.Quotas.MaxStorageBytes,
				MaxRecordsPerTable:    req.Quotas.MaxRecordsPerTable,
				MaxAssetSizeBytes:     req.Quotas.MaxAssetSizeBytes,
				MaxTablesPerWorkspace: req.Quotas.MaxTablesPerWorkspace,
				MaxColumnsPerTable:    req.Quotas.MaxColumnsPerTable,
			},
			MaxRequestBodyBytes:   req.Quotas.MaxRequestBodyBytes,
			MaxSessionsPerUser:    req.Quotas.MaxSessionsPerUser,
			MaxOrganizations:      req.Quotas.MaxOrganizations,
			MaxWorkspaces:         req.Quotas.MaxWorkspaces,
			MaxUsers:              req.Quotas.MaxUsers,
			MaxTotalStorageBytes:  req.Quotas.MaxTotalStorageBytes,
			MaxEgressBandwidthBps: req.Quotas.MaxEgressBandwidthBps,
		}
		// Validate the new quotas
		if err := newQuotas.Validate(); err != nil {
			return nil, dto.InvalidField("quotas", err.Error())
		}
		h.Cfg.Quotas = newQuotas

		// Invalidate all cached workspace stores so effective quotas are recomputed.
		if h.FileStore != nil {
			h.FileStore.InvalidateAllStores()
		}
	}

	// Update rate limits if provided
	if req.RateLimits != nil {
		newRateLimits := storage.RateLimits{
			AuthRatePerMin:       req.RateLimits.AuthRatePerMin,
			WriteRatePerMin:      req.RateLimits.WriteRatePerMin,
			ReadAuthRatePerMin:   req.RateLimits.ReadAuthRatePerMin,
			ReadUnauthRatePerMin: req.RateLimits.ReadUnauthRatePerMin,
		}
		// Validate the new rate limits
		if err := newRateLimits.Validate(); err != nil {
			return nil, dto.InvalidField("rate_limits", err.Error())
		}
		h.Cfg.RateLimits = newRateLimits
	}

	// Save to disk
	if err := h.Cfg.Save(h.DataDir); err != nil {
		return nil, dto.Internal(fmt.Sprintf("failed to save config: %v", err))
	}

	// Hot-reload bandwidth limiter if quotas were updated
	if req.Quotas != nil && h.BandwidthLimiter != nil {
		h.BandwidthLimiter.Update(h.Cfg.Quotas.MaxEgressBandwidthBps)
	}

	// Hot-reload rate limiters if rate limits were updated
	if req.RateLimits != nil && h.RateLimiters != nil {
		h.RateLimiters.Update(
			h.Cfg.RateLimits.AuthRatePerMin,
			h.Cfg.RateLimits.WriteRatePerMin,
			h.Cfg.RateLimits.ReadAuthRatePerMin,
			h.Cfg.RateLimits.ReadUnauthRatePerMin,
		)
	}

	return &dto.UpdateServerConfigResponse{Ok: true}, nil
}
