package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// MembershipHandler handles membership-related requests.
type MembershipHandler struct {
	memService  *identity.MembershipService
	userService *identity.UserService
	authHandler *AuthHandler
}

// NewMembershipHandler creates a new membership handler.
func NewMembershipHandler(memService *identity.MembershipService, userService *identity.UserService, authHandler *AuthHandler) *MembershipHandler {
	return &MembershipHandler{
		memService:  memService,
		userService: userService,
		authHandler: authHandler,
	}
}

// SwitchOrg switches the user's active organization and returns a new token.
func (h *MembershipHandler) SwitchOrg(ctx context.Context, req dto.SwitchOrgRequest) (*dto.SwitchOrgResponse, error) {
	currentUser, ok := ctx.Value(entity.UserKey).(*entity.User)
	if !ok {
		return nil, dto.Unauthorized()
	}
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}

	// Verify membership
	if _, err = h.memService.GetMembership(currentUser.ID, orgID); err != nil {
		return nil, dto.Forbidden("User is not a member of this organization")
	}

	// Re-fetch user for token generation
	user, err := h.userService.GetUser(currentUser.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to fetch user", err)
	}

	token, err := h.authHandler.GenerateToken(user)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response with memberships
	uwm, err := h.userService.GetUserWithMemberships(currentUser.ID)
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
func (h *MembershipHandler) UpdateMembershipSettings(ctx context.Context, req dto.UpdateMembershipSettingsRequest) (*dto.MembershipResponse, error) {
	currentUser, ok := ctx.Value(entity.UserKey).(*entity.User)
	if !ok {
		return nil, dto.Unauthorized()
	}
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	if err := h.memService.UpdateSettings(currentUser.ID, orgID, membershipSettingsToEntity(req.Settings)); err != nil {
		return nil, dto.InternalWithError("Failed to update membership settings", err)
	}
	m, err := h.memService.GetMembership(currentUser.ID, orgID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get membership", err)
	}
	return membershipToResponse(m), nil
}
