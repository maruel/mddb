package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// AdminHandler handles global admin endpoints.
type AdminHandler struct {
	userService *identity.UserService
	orgService  *identity.OrganizationService
	memService  *identity.MembershipService
}

// NewAdminHandler creates a new admin handler.
func NewAdminHandler(userService *identity.UserService, orgService *identity.OrganizationService, memService *identity.MembershipService) *AdminHandler {
	return &AdminHandler{
		userService: userService,
		orgService:  orgService,
		memService:  memService,
	}
}

// Stats returns server-wide statistics.
func (h *AdminHandler) Stats(ctx context.Context, _ *identity.User, _ *dto.AdminStatsRequest) (*dto.AdminStatsResponse, error) {
	var userCount, orgCount int
	for range h.userService.Iter(0) {
		userCount++
	}
	for range h.orgService.Iter(0) {
		orgCount++
	}
	return &dto.AdminStatsResponse{
		UserCount: userCount,
		OrgCount:  orgCount,
	}, nil
}

// ListAllUsers returns all users in the system.
func (h *AdminHandler) ListAllUsers(ctx context.Context, _ *identity.User, _ *dto.AdminUsersRequest) (*dto.AdminUsersResponse, error) {
	users := make([]dto.UserResponse, 0) //nolint:prealloc // size unknown from iterator
	for user := range h.userService.Iter(0) {
		users = append(users, *userToResponse(user))
	}
	return &dto.AdminUsersResponse{Users: users}, nil
}

// ListAllOrgs returns all organizations in the system.
func (h *AdminHandler) ListAllOrgs(ctx context.Context, _ *identity.User, _ *dto.AdminOrgsRequest) (*dto.AdminOrgsResponse, error) {
	orgs := make([]dto.OrganizationResponse, 0) //nolint:prealloc // size unknown from iterator
	for org := range h.orgService.Iter(0) {
		orgs = append(orgs, dto.OrganizationResponse{
			ID:         org.ID.String(),
			Name:       org.Name,
			Quotas:     organizationQuotaToDTO(org.Quotas),
			Settings:   organizationSettingsToDTO(org.Settings),
			Onboarding: onboardingStateToDTO(org.Onboarding),
			Created:    formatTime(org.Created),
		})
	}
	return &dto.AdminOrgsResponse{Organizations: orgs}, nil
}
