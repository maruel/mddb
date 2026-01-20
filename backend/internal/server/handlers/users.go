package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/models"
	"github.com/maruel/mddb/backend/internal/storage"
)

// UserHandler handles user management requests.
type UserHandler struct {
	userService *storage.UserService
}

// NewUserHandler creates a new user handler.
func NewUserHandler(userService *storage.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

// ListUsers returns all users in the organization.
func (h *UserHandler) ListUsers(ctx context.Context, req models.ListUsersRequest) (*models.ListUsersResponse, error) {
	// Active org ID is verified by middleware and injected into context
	orgID := models.GetOrgID(ctx)
	if orgID.IsZero() {
		return nil, models.Forbidden("Organization context missing")
	}

	allUsers, err := h.userService.ListUserResponses()
	if err != nil {
		return nil, models.InternalWithError("Failed to list users", err)
	}

	// Filter by organization membership
	var users []models.UserResponse
	for i := range allUsers {
		for _, m := range allUsers[i].Memberships {
			if m.OrganizationID == orgID.String() {
				users = append(users, allUsers[i])
				break
			}
		}
	}

	return &models.ListUsersResponse{Users: users}, nil
}

// UpdateUserRole updates a user's role.
func (h *UserHandler) UpdateUserRole(ctx context.Context, req models.UpdateRoleRequest) (*models.UserResponse, error) {
	orgID := models.GetOrgID(ctx)
	if orgID.IsZero() {
		return nil, models.Forbidden("Organization context missing")
	}

	if req.UserID == "" || req.Role == "" {
		return nil, models.MissingField("user_id or role")
	}

	if err := h.userService.UpdateUserRole(req.UserID, orgID.String(), req.Role); err != nil {
		return nil, models.InternalWithError("Failed to update user role", err)
	}

	return h.userService.GetUserResponse(req.UserID)
}

// UpdateUserSettings updates user global settings.
func (h *UserHandler) UpdateUserSettings(ctx context.Context, req models.UpdateUserSettingsRequest) (*models.UserResponse, error) {
	currentUser, ok := ctx.Value(models.UserKey).(*models.User)
	if !ok {
		return nil, models.Unauthorized()
	}

	if err := h.userService.UpdateSettings(currentUser.ID.String(), req.Settings); err != nil {
		return nil, models.InternalWithError("Failed to update settings", err)
	}

	return h.userService.GetUserResponse(currentUser.ID.String())
}
