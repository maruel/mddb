package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/models"
	"github.com/maruel/mddb/backend/internal/storage"
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

// GetOnboarding retrieves organization onboarding status.
func (h *OrganizationHandler) GetOnboarding(ctx context.Context, req models.GetOnboardingRequest) (*models.OnboardingState, error) {
	org, err := h.orgService.GetOrganization(req.OrgID)
	if err != nil {
		return nil, err
	}
	return &org.Onboarding, nil
}

// UpdateOnboarding updates organization onboarding status.
func (h *OrganizationHandler) UpdateOnboarding(ctx context.Context, req models.UpdateOnboardingRequest) (*models.OnboardingState, error) {
	if err := h.orgService.UpdateOnboarding(req.OrgID, req.State); err != nil {
		return nil, models.InternalWithError("Failed to update onboarding state", err)
	}
	org, _ := h.orgService.GetOrganization(req.OrgID)
	return &org.Onboarding, nil
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
