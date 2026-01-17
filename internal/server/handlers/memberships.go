package handlers

import (
	"context"

	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/storage"
)

// MembershipHandler handles membership-related requests.
type MembershipHandler struct {
	memService  *storage.MembershipService
	userService *storage.UserService
	authHandler *AuthHandler // To generate new tokens when switching orgs
}

// NewMembershipHandler creates a new membership handler.
func NewMembershipHandler(memService *storage.MembershipService, userService *storage.UserService, authHandler *AuthHandler) *MembershipHandler {
	return &MembershipHandler{
		memService:  memService,
		userService: userService,
		authHandler: authHandler,
	}
}

// SwitchOrgRequest is a request to switch the active organization.
type SwitchOrgRequest struct {
	OrgID string `json:"org_id"`
}

// SwitchOrgResponse is the response after switching organizations.
type SwitchOrgResponse struct {
	Token string       `json:"token"`
	User  *models.User `json:"user"`
}

// SwitchOrg switches the user's active organization and returns a new token.
func (h *MembershipHandler) SwitchOrg(ctx context.Context, req SwitchOrgRequest) (*SwitchOrgResponse, error) {
	currentUser, ok := ctx.Value(models.UserKey).(*models.User)
	if !ok {
		return nil, errors.Unauthorized()
	}

	if req.OrgID == "" {
		return nil, errors.MissingField("org_id")
	}

	// Verify membership
	m, err := h.memService.GetMembership(currentUser.ID, req.OrgID)
	if err != nil {
		return nil, errors.Forbidden("You are not a member of this organization")
	}

	// Update active organization and role in user profile
	if err := h.userService.UpdateUserOrg(currentUser.ID, req.OrgID); err != nil {
		return nil, errors.InternalWithError("Failed to switch organization", err)
	}

	// Get updated user
	user, err := h.userService.GetUser(currentUser.ID)
	if err != nil {
		return nil, errors.InternalWithError("Failed to retrieve updated user", err)
	}
	user.Role = m.Role // Ensure role from membership is used

	// Generate new token
	token, err := h.authHandler.GenerateToken(user)
	if err != nil {
		return nil, errors.InternalWithError("Failed to generate new token", err)
	}

	return &SwitchOrgResponse{
		Token: token,
		User:  user,
	}, nil
}

// UpdateMembershipSettingsRequest is a request to update membership settings.
type UpdateMembershipSettingsRequest struct {
	OrgID    string                    `path:"orgID"`
	Settings models.MembershipSettings `json:"settings"`
}

// UpdateMembershipSettings updates user preferences within a specific organization.
func (h *MembershipHandler) UpdateMembershipSettings(ctx context.Context, req UpdateMembershipSettingsRequest) (*models.Membership, error) {
	currentUser, ok := ctx.Value(models.UserKey).(*models.User)
	if !ok {
		return nil, errors.Unauthorized()
	}

	if err := h.memService.UpdateSettings(currentUser.ID, req.OrgID, req.Settings); err != nil {
		return nil, errors.InternalWithError("Failed to update membership settings", err)
	}

	return h.memService.GetMembership(currentUser.ID, req.OrgID)
}
