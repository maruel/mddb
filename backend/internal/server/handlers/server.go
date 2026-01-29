// Handles server configuration endpoints for global admins.

package handlers

import (
	"context"
	"fmt"

	"github.com/maruel/mddb/backend/internal/email"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// ServerHandler handles server configuration endpoints.
type ServerHandler struct {
	Cfg     *storage.ServerConfig
	DataDir string
}

// GetConfig returns the current server configuration with masked password.
func (h *ServerHandler) GetConfig(ctx context.Context, _ *identity.User, _ *dto.ServerConfigRequest) (*dto.ServerConfigResponse, error) {
	return &dto.ServerConfigResponse{
		SMTP: dto.SMTPConfigResponse{
			Host:     h.Cfg.SMTP.Host,
			Port:     h.Cfg.SMTP.Port,
			Username: h.Cfg.SMTP.Username,
			From:     h.Cfg.SMTP.From,
			// Password intentionally omitted (masked)
		},
		Quotas: dto.QuotasConfigResponse{
			MaxRequestBodyBytes:   h.Cfg.Quotas.MaxRequestBodyBytes,
			MaxSessionsPerUser:    h.Cfg.Quotas.MaxSessionsPerUser,
			MaxTablesPerWorkspace: h.Cfg.Quotas.MaxTablesPerWorkspace,
			MaxColumnsPerTable:    h.Cfg.Quotas.MaxColumnsPerTable,
			MaxRowsPerTable:       h.Cfg.Quotas.MaxRowsPerTable,
			MaxOrganizations:      h.Cfg.Quotas.MaxOrganizations,
			MaxWorkspaces:         h.Cfg.Quotas.MaxWorkspaces,
			MaxUsers:              h.Cfg.Quotas.MaxUsers,
			MaxTotalStorageBytes:  h.Cfg.Quotas.MaxTotalStorageBytes,
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
			MaxRequestBodyBytes:   req.Quotas.MaxRequestBodyBytes,
			MaxSessionsPerUser:    req.Quotas.MaxSessionsPerUser,
			MaxTablesPerWorkspace: req.Quotas.MaxTablesPerWorkspace,
			MaxColumnsPerTable:    req.Quotas.MaxColumnsPerTable,
			MaxRowsPerTable:       req.Quotas.MaxRowsPerTable,
			MaxOrganizations:      req.Quotas.MaxOrganizations,
			MaxWorkspaces:         req.Quotas.MaxWorkspaces,
			MaxUsers:              req.Quotas.MaxUsers,
			MaxTotalStorageBytes:  req.Quotas.MaxTotalStorageBytes,
		}
		// Validate the new quotas
		if err := newQuotas.Validate(); err != nil {
			return nil, dto.InvalidField("quotas", err.Error())
		}
		h.Cfg.Quotas = newQuotas
	}

	// Save to disk
	if err := h.Cfg.Save(h.DataDir); err != nil {
		return nil, dto.Internal(fmt.Sprintf("failed to save config: %v", err))
	}

	return &dto.UpdateServerConfigResponse{Ok: true}, nil
}
