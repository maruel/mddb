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
	if orgID == "" {
		return nil, models.Forbidden("Organization context missing")
	}

	allUsers, err := h.userService.ListUsers()
	if err != nil {
		return nil, models.InternalWithError("Failed to list users", err)
	}

	// Filter by organization membership
	var users []*models.User
	for _, u := range allUsers {
		for _, m := range u.Memberships {
			if m.OrganizationID == orgID {
				users = append(users, u)
				break
			}
		}
	}

	return &models.ListUsersResponse{Users: users}, nil
}

// UpdateUserRole updates a user's role.
func (h *UserHandler) UpdateUserRole(ctx context.Context, req models.UpdateRoleRequest) (*models.User, error) {
	orgID := models.GetOrgID(ctx)
	if orgID == "" {
		return nil, models.Forbidden("Organization context missing")
	}

	if req.UserID == "" || req.Role == "" {
		return nil, models.MissingField("user_id or role")
	}

	if err := h.userService.UpdateUserRole(req.UserID, orgID, req.Role); err != nil {
		return nil, models.InternalWithError("Failed to update user role", err)
	}

	return h.userService.GetUser(req.UserID)
}

// UpdateUserSettings updates user global settings.
func (h *UserHandler) UpdateUserSettings(ctx context.Context, req models.UpdateUserSettingsRequest) (*models.User, error) {
	currentUser, ok := ctx.Value(models.UserKey).(*models.User)
	if !ok {
		return nil, models.Unauthorized()
	}

	if err := h.userService.UpdateSettings(currentUser.ID, req.Settings); err != nil {
		return nil, models.InternalWithError("Failed to update settings", err)
	}

	return h.userService.GetUser(currentUser.ID)
}
