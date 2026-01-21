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
	memService  *identity.MembershipService
	orgService  *identity.OrganizationService
}

// NewUserHandler creates a new user handler.
func NewUserHandler(userService *identity.UserService, memService *identity.MembershipService, orgService *identity.OrganizationService) *UserHandler {
	return &UserHandler{
		userService: userService,
		memService:  memService,
		orgService:  orgService,
	}
}

// ListUsers returns all users in the organization.
func (h *UserHandler) ListUsers(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.ListUsersRequest) (*dto.ListUsersResponse, error) {
	// Filter by organization membership and convert to response
	var users []dto.UserResponse
	for user := range h.userService.Iter() {
		uwm, err := getUserWithMemberships(h.userService, h.memService, h.orgService, user.ID)
		if err != nil {
			continue
		}
		for _, m := range uwm.Memberships {
			if m.OrganizationID == orgID {
				users = append(users, *userWithMembershipsToResponse(uwm))
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

	// Update or create membership
	m, err := h.memService.Get(userID, orgID)
	if err != nil {
		if _, err = h.memService.Create(userID, orgID, userRoleToEntity(req.Role)); err != nil {
			return nil, dto.InternalWithError("Failed to create membership", err)
		}
	} else {
		newRole := userRoleToEntity(req.Role)
		if _, err = h.memService.Modify(m.ID, func(m *entity.Membership) error {
			m.Role = newRole
			return nil
		}); err != nil {
			return nil, dto.InternalWithError("Failed to update user role", err)
		}
	}

	uwm, err := getUserWithMemberships(h.userService, h.memService, h.orgService, userID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user", err)
	}
	return userWithMembershipsToResponse(uwm), nil
}

// UpdateUserSettings updates user global settings.
func (h *UserHandler) UpdateUserSettings(ctx context.Context, _ jsonldb.ID, user *entity.User, req dto.UpdateUserSettingsRequest) (*dto.UserResponse, error) {
	_, err := h.userService.Modify(user.ID, func(u *entity.User) error {
		u.Settings = userSettingsToEntity(req.Settings)
		return nil
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to update settings", err)
	}
	uwm, err := getUserWithMemberships(h.userService, h.memService, h.orgService, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user", err)
	}
	return userWithMembershipsToResponse(uwm), nil
}
