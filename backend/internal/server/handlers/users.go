package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/dto"
	"github.com/maruel/mddb/backend/internal/entity"
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
func (h *UserHandler) ListUsers(ctx context.Context, req dto.ListUsersRequest) (*dto.ListUsersResponse, error) {
	// Active org ID is verified by middleware and injected into context
	orgID := entity.GetOrgID(ctx)
	if orgID.IsZero() {
		return nil, dto.Forbidden("Organization context missing")
	}

	allUsers, err := h.userService.ListUsersWithMemberships()
	if err != nil {
		return nil, dto.InternalWithError("Failed to list users", err)
	}

	// Filter by organization membership and convert to response
	var users []dto.UserResponse
	for i := range allUsers {
		for _, m := range allUsers[i].Memberships {
			if m.OrganizationID.String() == orgID.String() {
				users = append(users, *userWithMembershipsToResponse(&allUsers[i]))
				break
			}
		}
	}

	return &dto.ListUsersResponse{Users: users}, nil
}

// UpdateUserRole updates a user's role.
func (h *UserHandler) UpdateUserRole(ctx context.Context, req dto.UpdateRoleRequest) (*dto.UserResponse, error) {
	orgID := entity.GetOrgID(ctx)
	if orgID.IsZero() {
		return nil, dto.Forbidden("Organization context missing")
	}

	if req.UserID == "" || req.Role == "" {
		return nil, dto.MissingField("user_id or role")
	}

	if err := h.userService.UpdateUserRole(req.UserID, orgID.String(), userRoleToEntity(req.Role)); err != nil {
		return nil, dto.InternalWithError("Failed to update user role", err)
	}

	uwm, err := h.userService.GetUserWithMemberships(req.UserID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user", err)
	}
	return userWithMembershipsToResponse(uwm), nil
}

// UpdateUserSettings updates user global settings.
func (h *UserHandler) UpdateUserSettings(ctx context.Context, req dto.UpdateUserSettingsRequest) (*dto.UserResponse, error) {
	currentUser, ok := ctx.Value(entity.UserKey).(*entity.User)
	if !ok {
		return nil, dto.Unauthorized()
	}

	if err := h.userService.UpdateSettings(currentUser.ID.String(), userSettingsToEntity(req.Settings)); err != nil {
		return nil, dto.InternalWithError("Failed to update settings", err)
	}

	uwm, err := h.userService.GetUserWithMemberships(currentUser.ID.String())
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user", err)
	}
	return userWithMembershipsToResponse(uwm), nil
}
