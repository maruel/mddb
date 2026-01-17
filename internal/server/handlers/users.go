package handlers

import (
	"context"

	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/storage"
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

// ListUsersRequest is a request to list users.
type ListUsersRequest struct {
	OrgID string `path:"orgID"`
}

// UpdateRoleRequest is a request to update a user's role.
type UpdateRoleRequest struct {
	OrgID  string          `path:"orgID"`
	UserID string          `json:"user_id"`
	Role   models.UserRole `json:"role"`
}

// ListUsersResponse is a response containing a list of users.
type ListUsersResponse struct {
	Users []*models.User `json:"users"`
}

// ListUsers returns all users in the organization.
func (h *UserHandler) ListUsers(ctx context.Context, req ListUsersRequest) (*ListUsersResponse, error) {
	// Only admins should reach this if middleware is applied correctly
	currentUser, ok := ctx.Value(models.UserKey).(*models.User)
	if !ok || currentUser.Role != models.RoleAdmin {
		return nil, errors.Forbidden("Only admins can list users")
	}

	if req.OrgID != currentUser.OrganizationID {
		return nil, errors.NewAPIError(403, errors.ErrForbidden, "Organization mismatch")
	}

	allUsers, err := h.userService.ListUsers()
	if err != nil {
		return nil, errors.InternalWithError("Failed to list users", err)
	}

	// Filter by organization
	var users []*models.User
	for _, u := range allUsers {
		if u.OrganizationID == req.OrgID {
			users = append(users, u)
		}
	}

	return &ListUsersResponse{Users: users}, nil
}

// UpdateUserRole updates a user's role.
func (h *UserHandler) UpdateUserRole(ctx context.Context, req UpdateRoleRequest) (*models.User, error) {
	currentUser, ok := ctx.Value(models.UserKey).(*models.User)
	if !ok || currentUser.Role != models.RoleAdmin {
		return nil, errors.Forbidden("Only admins can update roles")
	}

	if req.OrgID != currentUser.OrganizationID {
		return nil, errors.NewAPIError(403, errors.ErrForbidden, "Organization mismatch")
	}

	if req.UserID == "" || req.Role == "" {
		return nil, errors.MissingField("user_id or role")
	}

	// Verify target user belongs to the same organization
	targetUser, err := h.userService.GetUser(req.UserID)
	if err != nil {
		return nil, errors.NotFound("user")
	}
	if targetUser.OrganizationID != req.OrgID {
		return nil, errors.Forbidden("Cannot update user from different organization")
	}

	if err := h.userService.UpdateUserRole(req.UserID, req.Role); err != nil {
		return nil, errors.InternalWithError("Failed to update user role", err)
	}

	return h.userService.GetUser(req.UserID)
}

// UpdateUserSettingsRequest is a request to update user global settings.
type UpdateUserSettingsRequest struct {
	Settings models.UserSettings `json:"settings"`
}

// UpdateUserSettings updates user global settings.
func (h *UserHandler) UpdateUserSettings(ctx context.Context, req UpdateUserSettingsRequest) (*models.User, error) {
	currentUser, ok := ctx.Value(models.UserKey).(*models.User)
	if !ok {
		return nil, errors.Unauthorized()
	}

	if err := h.userService.UpdateSettings(currentUser.ID, req.Settings); err != nil {
		return nil, errors.InternalWithError("Failed to update settings", err)
	}

	return h.userService.GetUser(currentUser.ID)
}
