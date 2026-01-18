package handlers

import (
	"context"

	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/storage"
)

// OrganizationHandler handles organization management requests.
type OrganizationHandler struct {
	orgService *storage.OrganizationService
}

// NewOrganizationHandler creates a new organization handler.
func NewOrganizationHandler(orgService *storage.OrganizationService) *OrganizationHandler {
	return &OrganizationHandler{
		orgService: orgService,
	}
}

// GetOrganization retrieves current organization details.
func (h *OrganizationHandler) GetOrganization(ctx context.Context, req any) (*models.Organization, error) {
	orgID := models.GetOrgID(ctx)
	if orgID == "" {
		return nil, models.Forbidden("Organization context missing")
	}

	return h.orgService.GetOrganization(orgID)
}

// UpdateSettings updates organization-wide settings.
func (h *OrganizationHandler) UpdateSettings(ctx context.Context, req models.UpdateOrgSettingsRequest) (*models.Organization, error) {
	orgID := models.GetOrgID(ctx)
	if orgID == "" {
		return nil, models.Forbidden("Organization context missing")
	}

	if err := h.orgService.UpdateSettings(orgID, req.Settings); err != nil {
		return nil, models.InternalWithError("Failed to update organization settings", err)
	}

	return h.orgService.GetOrganization(orgID)
}
