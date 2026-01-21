package handlers

import (
	"context"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// OrganizationHandler handles organization management requests.
type OrganizationHandler struct {
	orgService *identity.OrganizationService
}

// NewOrganizationHandler creates a new organization handler.
func NewOrganizationHandler(orgService *identity.OrganizationService) *OrganizationHandler {
	return &OrganizationHandler{
		orgService: orgService,
	}
}

// GetOrganization retrieves current organization details.
func (h *OrganizationHandler) GetOrganization(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req dto.GetOnboardingRequest) (*dto.OrganizationResponse, error) {
	org, err := h.orgService.Get(orgID)
	if err != nil {
		return nil, err
	}
	return organizationToResponse(org), nil
}

// GetOnboarding retrieves organization onboarding status.
func (h *OrganizationHandler) GetOnboarding(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req dto.GetOnboardingRequest) (*dto.OnboardingState, error) {
	org, err := h.orgService.Get(orgID)
	if err != nil {
		return nil, err
	}
	result := onboardingStateToDTO(org.Onboarding)
	return &result, nil
}

// UpdateOnboarding updates organization onboarding status.
func (h *OrganizationHandler) UpdateOnboarding(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req dto.UpdateOnboardingRequest) (*dto.OnboardingState, error) {
	org, err := h.orgService.Modify(orgID, func(org *identity.Organization) error {
		org.Onboarding = onboardingStateToEntity(req.State)
		org.Onboarding.UpdatedAt = time.Now()
		return nil
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to update onboarding state", err)
	}
	result := onboardingStateToDTO(org.Onboarding)
	return &result, nil
}

// UpdateSettings updates organization-wide settings.
func (h *OrganizationHandler) UpdateSettings(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req dto.UpdateOrgSettingsRequest) (*dto.OrganizationResponse, error) {
	org, err := h.orgService.Modify(orgID, func(org *identity.Organization) error {
		org.Settings = organizationSettingsToEntity(req.Settings)
		return nil
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to update organization settings", err)
	}
	return organizationToResponse(org), nil
}

// UpdateOrganization updates the organization's name.
func (h *OrganizationHandler) UpdateOrganization(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req dto.UpdateOrganizationRequest) (*dto.OrganizationResponse, error) {
	if req.Name == "" {
		return nil, dto.BadRequest("Organization name is required")
	}
	org, err := h.orgService.Modify(orgID, func(org *identity.Organization) error {
		org.Name = req.Name
		return nil
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to update organization", err)
	}
	return organizationToResponse(org), nil
}
