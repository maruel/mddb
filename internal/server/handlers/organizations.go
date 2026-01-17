package handlers

import (
	"context"

	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/storage"
)

// OrganizationHandler handles organization management.
type OrganizationHandler struct {
	orgService *storage.OrganizationService
}

// NewOrganizationHandler creates a new organization handler.
func NewOrganizationHandler(orgService *storage.OrganizationService) *OrganizationHandler {
	return &OrganizationHandler{
		orgService: orgService,
	}
}

// UpdateOrgSettingsRequest is a request to update organization settings.
type UpdateOrgSettingsRequest struct {
	OrgID    string                      `path:"orgID"`
	Settings models.OrganizationSettings `json:"settings"`
}

// GetOrganization returns an organization by ID.
func (h *OrganizationHandler) GetOrganization(ctx context.Context, req struct {
	OrgID string `path:"orgID"`
}) (*models.Organization, error) {
	return h.orgService.GetOrganization(req.OrgID)
}

// UpdateSettings updates organization-wide settings.
func (h *OrganizationHandler) UpdateSettings(ctx context.Context, req UpdateOrgSettingsRequest) (*models.Organization, error) {
	currentUser, ok := ctx.Value(models.UserKey).(*models.User)
	if !ok || currentUser.Role != models.RoleAdmin {
		return nil, errors.Forbidden("Only admins can update organization settings")
	}

	if req.OrgID != currentUser.OrganizationID {
		return nil, errors.Forbidden("Organization mismatch")
	}

	if err := h.orgService.UpdateSettings(req.OrgID, req.Settings); err != nil {
		return nil, errors.InternalWithError("Failed to update organization settings", err)
	}

	return h.orgService.GetOrganization(req.OrgID)
}
