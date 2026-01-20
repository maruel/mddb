package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// UserHandler handles user management requests.
type UserHandler struct {
	userService *identity.UserService
}

// NewUserHandler creates a new user handler.
func NewUserHandler(userService *identity.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

// ListUsers returns all users in the organization.
func (h *UserHandler) ListUsers(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.ListUsersRequest) (*dto.ListUsersResponse, error) {
	allUsers, err := h.userService.ListUsersWithMemberships()
	if err != nil {
		return nil, dto.InternalWithError("Failed to list users", err)
	}

	// Filter by organization membership and convert to response
	var users []dto.UserResponse
	for i := range allUsers {
		for _, m := range allUsers[i].Memberships {
			if m.OrganizationID == orgID {
				users = append(users, *userWithMembershipsToResponse(&allUsers[i]))
				break
			}
		}
	}

	return &dto.ListUsersResponse{Users: users}, nil
}

// UpdateUserRole updates a user's role.
func (h *UserHandler) UpdateUserRole(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.UpdateRoleRequest) (*dto.UserResponse, error) {
	if req.UserID == "" || req.Role == "" {
		return nil, dto.MissingField("user_id or role")
	}
	userID, err := decodeID(req.UserID, "user_id")
	if err != nil {
		return nil, err
	}
	if err := h.userService.UpdateUserRole(userID, orgID, userRoleToEntity(req.Role)); err != nil {
		return nil, dto.InternalWithError("Failed to update user role", err)
	}
	uwm, err := h.userService.GetUserWithMemberships(userID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user", err)
	}
	return userWithMembershipsToResponse(uwm), nil
}

// UpdateUserSettings updates user global settings.
func (h *UserHandler) UpdateUserSettings(ctx context.Context, _ jsonldb.ID, user *entity.User, req dto.UpdateUserSettingsRequest) (*dto.UserResponse, error) {
	if err := h.userService.UpdateSettings(user.ID, userSettingsToEntity(req.Settings)); err != nil {
		return nil, dto.InternalWithError("Failed to update settings", err)
	}
	uwm, err := h.userService.GetUserWithMemberships(user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user", err)
	}
	return userWithMembershipsToResponse(uwm), nil
}
