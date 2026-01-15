package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// MembershipHandler handles membership-related requests.
type MembershipHandler struct {
	memService  *identity.MembershipService
	userService *identity.UserService
	orgService  *identity.OrganizationService
	authHandler *AuthHandler
}

// NewMembershipHandler creates a new membership handler.
func NewMembershipHandler(memService *identity.MembershipService, userService *identity.UserService, orgService *identity.OrganizationService, authHandler *AuthHandler) *MembershipHandler {
	return &MembershipHandler{
		memService:  memService,
		userService: userService,
		orgService:  orgService,
		authHandler: authHandler,
	}
}

// SwitchOrg switches the user's active organization and returns a new token.
func (h *MembershipHandler) SwitchOrg(ctx context.Context, _ jsonldb.ID, user *identity.User, req dto.SwitchOrgRequest) (*dto.SwitchOrgResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}

	// Verify membership
	if _, err = h.memService.Get(user.ID, orgID); err != nil {
		return nil, dto.Forbidden("User is not a member of this organization")
	}

	token, err := h.authHandler.GenerateToken(user)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response with memberships
	uwm, err := getUserWithMemberships(h.userService, h.memService, h.orgService, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	h.authHandler.PopulateActiveContext(userResp, req.OrgID)

	return &dto.SwitchOrgResponse{
		Token: token,
		User:  userResp,
	}, nil
}

// UpdateMembershipSettings updates user preferences within an organization.
func (h *MembershipHandler) UpdateMembershipSettings(ctx context.Context, orgID jsonldb.ID, user *identity.User, req dto.UpdateMembershipSettingsRequest) (*dto.MembershipResponse, error) {
	m, err := h.memService.Get(user.ID, orgID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get membership", err)
	}
	m, err = h.memService.Modify(m.ID, func(m *identity.Membership) error {
		m.Settings = membershipSettingsToEntity(req.Settings)
		return nil
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to update membership settings", err)
	}
	return membershipToResponse(m), nil
}
