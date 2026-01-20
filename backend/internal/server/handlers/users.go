package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/entity"
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
	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
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
	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
	}

	if req.UserID == "" || req.Role == "" {
		return nil, dto.MissingField("user_id or role")
	}

	userID, err := jsonldb.DecodeID(req.UserID)
	if err != nil {
		return nil, dto.BadRequest("invalid_user_id")
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
func (h *UserHandler) UpdateUserSettings(ctx context.Context, req dto.UpdateUserSettingsRequest) (*dto.UserResponse, error) {
	currentUser, ok := ctx.Value(entity.UserKey).(*entity.User)
	if !ok {
		return nil, dto.Unauthorized()
	}

	if err := h.userService.UpdateSettings(currentUser.ID, userSettingsToEntity(req.Settings)); err != nil {
		return nil, dto.InternalWithError("Failed to update settings", err)
	}

	uwm, err := h.userService.GetUserWithMemberships(currentUser.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user", err)
	}
	return userWithMembershipsToResponse(uwm), nil
}
