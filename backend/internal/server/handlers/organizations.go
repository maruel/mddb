package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/dto"
	"github.com/maruel/mddb/backend/internal/entity"
	"github.com/maruel/mddb/backend/internal/jsonldb"
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
func (h *OrganizationHandler) GetOrganization(ctx context.Context, req any) (*dto.OrganizationResponse, error) {
	orgID := entity.GetOrgID(ctx)
	if orgID.IsZero() {
		return nil, dto.Forbidden("Organization context missing")
	}

	org, err := h.orgService.GetOrganization(orgID)
	if err != nil {
		return nil, err
	}

	return organizationToResponse(org), nil
}

// GetOnboarding retrieves organization onboarding status.
func (h *OrganizationHandler) GetOnboarding(ctx context.Context, req dto.GetOnboardingRequest) (*dto.OnboardingState, error) {
	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
	}
	org, err := h.orgService.GetOrganization(orgID)
	if err != nil {
		return nil, err
	}
	result := onboardingStateToDTO(org.Onboarding)
	return &result, nil
}

// UpdateOnboarding updates organization onboarding status.
func (h *OrganizationHandler) UpdateOnboarding(ctx context.Context, req dto.UpdateOnboardingRequest) (*dto.OnboardingState, error) {
	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
	}
	if err := h.orgService.UpdateOnboarding(orgID, onboardingStateToEntity(req.State)); err != nil {
		return nil, dto.InternalWithError("Failed to update onboarding state", err)
	}
	org, _ := h.orgService.GetOrganization(orgID)
	result := onboardingStateToDTO(org.Onboarding)
	return &result, nil
}

// UpdateSettings updates organization-wide settings.
func (h *OrganizationHandler) UpdateSettings(ctx context.Context, req dto.UpdateOrgSettingsRequest) (*dto.OrganizationResponse, error) {
	orgID := entity.GetOrgID(ctx)
	if orgID.IsZero() {
		return nil, dto.Forbidden("Organization context missing")
	}

	if err := h.orgService.UpdateSettings(orgID, organizationSettingsToEntity(req.Settings)); err != nil {
		return nil, dto.InternalWithError("Failed to update organization settings", err)
	}

	org, err := h.orgService.GetOrganization(orgID)
	if err != nil {
		return nil, err
	}

	return organizationToResponse(org), nil
}
