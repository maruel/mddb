// Validates and publishes persisted server settings for global administrators.

package handlers

import (
	"context"
	"errors"
	"fmt"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/mddb/backend/internal/server/dto/v1"
	"github.com/maruel/mddb/backend/internal/storage"
)

// ServerHandler handles server configuration endpoints.
type ServerHandler struct {
	Settings         *storage.ServerSettings
	ConfigDir        string            // holds settings.json
	BandwidthLimiter BandwidthUpdater  // for hot-reload of bandwidth limit
	RateLimiters     RateLimitsUpdater // for hot-reload of rate limits
}

// GetConfig returns the current server configuration with masked password.
func (h *ServerHandler) GetConfig(ctx context.Context, _ *identityv1.User, _ *dto.ServerConfigRequest) (*dto.ServerConfigResponse, error) {
	cfg := h.Settings.Snapshot()
	rq := cfg.Quotas.ResourceQuotas
	return &dto.ServerConfigResponse{
		SMTP: dto.SMTPConfigResponse{
			Host:     cfg.SMTP.Host,
			Port:     cfg.SMTP.Port,
			Username: cfg.SMTP.Username,
			From:     cfg.SMTP.From,
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
			MaxRequestBodyBytes:   cfg.Quotas.MaxRequestBodyBytes,
			MaxSessionsPerUser:    cfg.Quotas.MaxSessionsPerUser,
			MaxOrganizations:      cfg.Quotas.MaxOrganizations,
			MaxWorkspaces:         cfg.Quotas.MaxWorkspaces,
			MaxUsers:              cfg.Quotas.MaxUsers,
			MaxTotalStorageBytes:  cfg.Quotas.MaxTotalStorageBytes,
			MaxEgressBandwidthBps: cfg.Quotas.MaxEgressBandwidthBps,
		},
		RateLimits: dto.RateLimitsConfigResponse{
			AuthRatePerMin:       cfg.RateLimits.AuthRatePerMin,
			WriteRatePerMin:      cfg.RateLimits.WriteRatePerMin,
			ReadAuthRatePerMin:   cfg.RateLimits.ReadAuthRatePerMin,
			ReadUnauthRatePerMin: cfg.RateLimits.ReadUnauthRatePerMin,
		},
	}, nil
}

// UpdateConfig persists a complete valid candidate before publishing live settings.
func (h *ServerHandler) UpdateConfig(ctx context.Context, _ *identityv1.User, req *dto.UpdateServerConfigRequest) (*dto.OkResponse, error) {
	// TODO: Make saved SMTP settings take effect and allow disabling SMTP.
	// cmd/mddb/main.go builds email.Service and the email verification
	// service once at startup from the SMTP settings, so changes saved here
	// only apply after a restart. Clearing every field cannot disable SMTP
	// either: the empty password below keeps the stored one, so the config is
	// no longer IsZero and Validate fails with "host is required". Rebuild or
	// swap the email services when the published settings change, and treat
	// an all-empty request as "disable". docs.mddb.app's email guide documents
	// the restart.
	err := h.Settings.Update(h.ConfigDir, func(next *storagev1.ServerConfig) error {
		if m := req.SMTP; m != nil {
			smtp := storagev1.SMTPConfig{Host: m.Host, Port: m.Port, Username: m.Username, Password: m.Password, From: m.From}
			// An empty password preserves the stored one.
			if smtp.Password == "" {
				smtp.Password = next.SMTP.Password
			}
			if err := smtp.Validate(); err != nil {
				return dto.InvalidField("smtp", err.Error())
			}
			next.SMTP = smtp
		}
		if q := req.Quotas; q != nil {
			// Fields outside this endpoint's schema, including notification retention,
			// retain their current values.
			next.Quotas.ResourceQuotas = storagev1.ResourceQuotas{MaxPages: q.MaxPages, MaxStorageBytes: q.MaxStorageBytes, MaxRecordsPerTable: q.MaxRecordsPerTable, MaxAssetSizeBytes: q.MaxAssetSizeBytes, MaxTablesPerWorkspace: q.MaxTablesPerWorkspace, MaxColumnsPerTable: q.MaxColumnsPerTable}
			next.Quotas.MaxRequestBodyBytes = q.MaxRequestBodyBytes
			next.Quotas.MaxSessionsPerUser = q.MaxSessionsPerUser
			next.Quotas.MaxOrganizations = q.MaxOrganizations
			next.Quotas.MaxWorkspaces = q.MaxWorkspaces
			next.Quotas.MaxUsers = q.MaxUsers
			next.Quotas.MaxTotalStorageBytes = q.MaxTotalStorageBytes
			next.Quotas.MaxEgressBandwidthBps = q.MaxEgressBandwidthBps
			if err := next.Quotas.Validate(); err != nil {
				return dto.InvalidField("quotas", err.Error())
			}
		}
		if r := req.RateLimits; r != nil {
			next.RateLimits = storagev1.RateLimits{AuthRatePerMin: r.AuthRatePerMin, WriteRatePerMin: r.WriteRatePerMin, ReadAuthRatePerMin: r.ReadAuthRatePerMin, ReadUnauthRatePerMin: r.ReadUnauthRatePerMin}
			if err := next.RateLimits.Validate(); err != nil {
				return dto.InvalidField("rate_limits", err.Error())
			}
		}
		return nil
	}, func(next *storagev1.ServerConfig) {
		if req.Quotas != nil && h.BandwidthLimiter != nil {
			h.BandwidthLimiter.Update(next.Quotas.MaxEgressBandwidthBps)
		}
		if req.RateLimits != nil && h.RateLimiters != nil {
			h.RateLimiters.Update(next.RateLimits)
		}
	})
	if err != nil {
		if _, ok := errors.AsType[*dto.APIError](err); ok {
			return nil, err
		}
		return nil, dto.Internal(fmt.Sprintf("failed to save config: %v", err))
	}
	return &dto.OkResponse{Ok: true}, nil
}
