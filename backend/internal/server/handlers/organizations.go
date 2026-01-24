package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// OrganizationHandler handles organization management requests.
type OrganizationHandler struct {
	orgService    *identity.OrganizationService
	orgMemService *identity.OrganizationMembershipService
	wsService     *identity.WorkspaceService
}

// NewOrganizationHandler creates a new organization handler.
func NewOrganizationHandler(
	orgService *identity.OrganizationService,
	orgMemService *identity.OrganizationMembershipService,
	wsService *identity.WorkspaceService,
) *OrganizationHandler {
	return &OrganizationHandler{
		orgService:    orgService,
		orgMemService: orgMemService,
		wsService:     wsService,
	}
}

// GetOrganization retrieves current organization details.
func (h *OrganizationHandler) GetOrganization(ctx context.Context, orgID jsonldb.ID, _ *identity.User, _ *dto.GetOrganizationRequest) (*dto.OrganizationResponse, error) {
	org, err := h.orgService.Get(orgID)
	if err != nil {
		return nil, err
	}
	memberCount := h.orgMemService.CountOrgMemberships(orgID)
	workspaceCount := h.wsService.CountByOrg(orgID)
	return organizationToResponse(org, memberCount, workspaceCount), nil
}

// UpdateOrgPreferences updates organization-wide preferences/settings.
func (h *OrganizationHandler) UpdateOrgPreferences(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req *dto.UpdateOrgPreferencesRequest) (*dto.OrganizationResponse, error) {
	org, err := h.orgService.Modify(orgID, func(org *identity.Organization) error {
		org.Settings = organizationSettingsToEntity(req.Settings)
		return nil
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to update organization settings", err)
	}
	memberCount := h.orgMemService.CountOrgMemberships(orgID)
	workspaceCount := h.wsService.CountByOrg(orgID)
	return organizationToResponse(org, memberCount, workspaceCount), nil
}

// UpdateOrganization updates organization details (name).
func (h *OrganizationHandler) UpdateOrganization(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req *dto.UpdateOrganizationRequest) (*dto.OrganizationResponse, error) {
	org, err := h.orgService.Modify(orgID, func(org *identity.Organization) error {
		org.Name = req.Name
		return nil
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to update organization", err)
	}
	memberCount := h.orgMemService.CountOrgMemberships(orgID)
	workspaceCount := h.wsService.CountByOrg(orgID)
	return organizationToResponse(org, memberCount, workspaceCount), nil
}
