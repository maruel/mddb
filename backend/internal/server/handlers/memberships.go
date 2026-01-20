package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/models"
	"github.com/maruel/mddb/backend/internal/storage"
)

// MembershipHandler handles membership-related requests.
type MembershipHandler struct {
	memService  *storage.MembershipService
	userService *storage.UserService
	authHandler *AuthHandler
}

// NewMembershipHandler creates a new membership handler.
func NewMembershipHandler(memService *storage.MembershipService, userService *storage.UserService, authHandler *AuthHandler) *MembershipHandler {
	return &MembershipHandler{
		memService:  memService,
		userService: userService,
		authHandler: authHandler,
	}
}

// SwitchOrg switches the user's active organization and returns a new token.
func (h *MembershipHandler) SwitchOrg(ctx context.Context, req models.SwitchOrgRequest) (*models.SwitchOrgResponse, error) {
	currentUser, ok := ctx.Value(models.UserKey).(*models.User)
	if !ok {
		return nil, models.Unauthorized()
	}

	// Verify membership
	_, err := h.memService.GetMembership(currentUser.ID.String(), req.OrgID)
	if err != nil {
		return nil, models.Forbidden("User is not a member of this organization")
	}

	// Re-fetch user for token generation
	user, err := h.userService.GetUser(currentUser.ID.String())
	if err != nil {
		return nil, models.InternalWithError("Failed to fetch user", err)
	}

	token, err := h.authHandler.GenerateToken(user)
	if err != nil {
		return nil, models.InternalWithError("Failed to generate token", err)
	}

	// Build user response with memberships
	userResp, err := h.userService.GetUserResponse(currentUser.ID.String())
	if err != nil {
		return nil, models.InternalWithError("Failed to get user response", err)
	}

	h.authHandler.PopulateActiveContext(userResp, req.OrgID)

	return &models.SwitchOrgResponse{
		Token: token,
		User:  userResp,
	}, nil
}

// UpdateMembershipSettings updates user preferences within an organization.
func (h *MembershipHandler) UpdateMembershipSettings(ctx context.Context, req models.UpdateMembershipSettingsRequest) (*models.MembershipResponse, error) {
	currentUser, ok := ctx.Value(models.UserKey).(*models.User)
	if !ok {
		return nil, models.Unauthorized()
	}

	orgID := models.GetOrgID(ctx)
	if err := h.memService.UpdateSettings(currentUser.ID.String(), orgID.String(), req.Settings); err != nil {
		return nil, models.InternalWithError("Failed to update membership settings", err)
	}

	return h.memService.GetMembershipResponse(currentUser.ID.String(), orgID.String())
}
