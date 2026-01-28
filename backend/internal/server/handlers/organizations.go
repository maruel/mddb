// Handles organization management endpoints.

package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// OrganizationHandler handles organization management requests.
type OrganizationHandler struct {
	Svc *Services
	Cfg *Config
}

// GetOrganization retrieves current organization details.
func (h *OrganizationHandler) GetOrganization(ctx context.Context, orgID jsonldb.ID, _ *identity.User, _ *dto.GetOrganizationRequest) (*dto.OrganizationResponse, error) {
	org, err := h.Svc.Organization.Get(orgID)
	if err != nil {
		return nil, err
	}
	memberCount := h.Svc.OrgMembership.CountOrgMemberships(orgID)
	workspaceCount := h.Svc.Workspace.CountByOrg(orgID)
	return organizationToResponse(org, memberCount, workspaceCount), nil
}

// UpdateOrgPreferences updates organization-wide preferences/settings.
func (h *OrganizationHandler) UpdateOrgPreferences(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req *dto.UpdateOrgPreferencesRequest) (*dto.OrganizationResponse, error) {
	org, err := h.Svc.Organization.Modify(orgID, func(org *identity.Organization) error {
		org.Settings = organizationSettingsToEntity(req.Settings)
		return nil
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to update organization settings", err)
	}
	memberCount := h.Svc.OrgMembership.CountOrgMemberships(orgID)
	workspaceCount := h.Svc.Workspace.CountByOrg(orgID)
	return organizationToResponse(org, memberCount, workspaceCount), nil
}

// UpdateOrganization updates organization details (name).
func (h *OrganizationHandler) UpdateOrganization(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req *dto.UpdateOrganizationRequest) (*dto.OrganizationResponse, error) {
	org, err := h.Svc.Organization.Modify(orgID, func(org *identity.Organization) error {
		org.Name = req.Name
		return nil
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to update organization", err)
	}
	memberCount := h.Svc.OrgMembership.CountOrgMemberships(orgID)
	workspaceCount := h.Svc.Workspace.CountByOrg(orgID)
	return organizationToResponse(org, memberCount, workspaceCount), nil
}

// CreateWorkspace creates a new workspace within an organization.
func (h *OrganizationHandler) CreateWorkspace(ctx context.Context, orgID jsonldb.ID, user *identity.User, req *dto.CreateWorkspaceRequest) (*dto.WorkspaceResponse, error) {
	if req.Name == "" {
		return nil, dto.MissingField("name")
	}

	// Check server-wide workspace quota
	if h.Cfg.ServerQuotas.MaxWorkspaces > 0 && h.Svc.Workspace.Count() >= h.Cfg.ServerQuotas.MaxWorkspaces {
		return nil, dto.QuotaExceeded("workspaces", h.Cfg.ServerQuotas.MaxWorkspaces)
	}

	// Create workspace
	ws, err := h.Svc.Workspace.Create(ctx, orgID, req.Name)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create workspace", err)
	}

	// Create workspace membership (user becomes admin of new workspace)
	if _, err := h.Svc.WSMembership.Create(user.ID, ws.ID, identity.WSRoleAdmin); err != nil {
		return nil, dto.InternalWithError("Failed to create workspace membership", err)
	}

	// Initialize workspace storage
	if err := h.Svc.FileStore.InitWorkspace(ctx, ws.ID); err != nil {
		return nil, dto.InternalWithError("Failed to initialize workspace storage", err)
	}

	memberCount := h.Svc.WSMembership.CountWSMemberships(ws.ID)
	return workspaceToResponse(ws, memberCount), nil
}

// GetWorkspace retrieves workspace details.
func (h *OrganizationHandler) GetWorkspace(_ context.Context, wsID jsonldb.ID, _ *identity.User, _ *dto.GetWorkspaceRequest) (*dto.WorkspaceResponse, error) {
	ws, err := h.Svc.Workspace.Get(wsID)
	if err != nil {
		return nil, err
	}
	memberCount := h.Svc.WSMembership.CountWSMemberships(wsID)
	return workspaceToResponse(ws, memberCount), nil
}

// UpdateWorkspace updates workspace details (name).
func (h *OrganizationHandler) UpdateWorkspace(_ context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.UpdateWorkspaceRequest) (*dto.WorkspaceResponse, error) {
	ws, err := h.Svc.Workspace.Modify(wsID, func(ws *identity.Workspace) error {
		ws.Name = req.Name
		return nil
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to update workspace", err)
	}
	memberCount := h.Svc.WSMembership.CountWSMemberships(wsID)
	return workspaceToResponse(ws, memberCount), nil
}
