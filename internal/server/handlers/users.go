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

// UpdateRoleRequest is a request to update a user's role.
type UpdateRoleRequest struct {
	UserID string          `json:"user_id"`
	Role   models.UserRole `json:"role"`
}

// ListUsersResponse is a response containing a list of users.
type ListUsersResponse struct {
	Users []*models.User `json:"users"`
}

// ListUsers returns all users in the system.
func (h *UserHandler) ListUsers(ctx context.Context, _ struct{}) (*ListUsersResponse, error) {
	// Only admins should reach this if middleware is applied correctly
	// But we can double check here
	currentUser, ok := ctx.Value(models.UserKey).(*models.User)
	if !ok || currentUser.Role != models.RoleAdmin {
		return nil, errors.Forbidden("Only admins can list users")
	}

	users, err := h.userService.ListUsers()
	if err != nil {
		return nil, errors.InternalWithError("Failed to list users", err)
	}

	return &ListUsersResponse{Users: users}, nil
}

// UpdateUserRole updates a user's role.
func (h *UserHandler) UpdateUserRole(ctx context.Context, req UpdateRoleRequest) (*models.User, error) {
	if req.UserID == "" || req.Role == "" {
		return nil, errors.MissingField("user_id or role")
	}

	if err := h.userService.UpdateUserRole(req.UserID, req.Role); err != nil {
		return nil, errors.InternalWithError("Failed to update user role", err)
	}

	return h.userService.GetUser(req.UserID)
}
