// Handles user management endpoints.

package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// UserHandler handles user management requests.
type UserHandler struct {
	svc *Services
}

// NewUserHandler creates a new user handler.
func NewUserHandler(svc *Services) *UserHandler {
	return &UserHandler{svc: svc}
}

// ListUsers returns all users in the organization.
func (h *UserHandler) ListUsers(ctx context.Context, orgID jsonldb.ID, _ *identity.User, _ *dto.ListUsersRequest) (*dto.ListUsersResponse, error) {
	// Filter by organization membership and convert to response
	var users []dto.UserResponse
	for user := range h.svc.User.Iter(0) {
		uwm, err := getUserWithMemberships(h.svc.User, h.svc.OrgMembership, h.svc.WSMembership, h.svc.Organization, h.svc.Workspace, user.ID)
		if err != nil {
			continue
		}
		for _, m := range uwm.OrgMemberships {
			if m.OrganizationID == orgID {
				users = append(users, *userWithMembershipsToResponse(uwm))
				break
			}
		}
	}

	return &dto.ListUsersResponse{Users: users}, nil
}

// UpdateOrgMemberRole updates a user's organization role.
func (h *UserHandler) UpdateOrgMemberRole(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req *dto.UpdateOrgMemberRoleRequest) (*dto.UserResponse, error) {
	if req.UserID.IsZero() || req.Role == "" {
		return nil, dto.MissingField("user_id or role")
	}

	// Update or create org membership
	m, err := h.svc.OrgMembership.Get(req.UserID, orgID)
	if err != nil {
		if _, err = h.svc.OrgMembership.Create(req.UserID, orgID, orgRoleToEntity(req.Role)); err != nil {
			return nil, dto.InternalWithError("Failed to create org membership", err)
		}
	} else {
		newRole := orgRoleToEntity(req.Role)
		if _, err = h.svc.OrgMembership.Modify(m.ID, func(m *identity.OrganizationMembership) error {
			m.Role = newRole
			return nil
		}); err != nil {
			return nil, dto.InternalWithError("Failed to update org member role", err)
		}
	}

	uwm, err := getUserWithMemberships(h.svc.User, h.svc.OrgMembership, h.svc.WSMembership, h.svc.Organization, h.svc.Workspace, req.UserID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user", err)
	}
	return userWithMembershipsToResponse(uwm), nil
}

// UpdateWSMemberRole updates a user's workspace role.
func (h *UserHandler) UpdateWSMemberRole(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.UpdateWSMemberRoleRequest) (*dto.UserResponse, error) {
	if req.UserID.IsZero() || req.Role == "" {
		return nil, dto.MissingField("user_id or role")
	}

	// Update or create workspace membership
	m, err := h.svc.WSMembership.Get(req.UserID, wsID)
	if err != nil {
		if _, err = h.svc.WSMembership.Create(req.UserID, wsID, wsRoleToEntity(req.Role)); err != nil {
			return nil, dto.InternalWithError("Failed to create workspace membership", err)
		}
	} else {
		newRole := wsRoleToEntity(req.Role)
		if _, err = h.svc.WSMembership.Modify(m.ID, func(m *identity.WorkspaceMembership) error {
			m.Role = newRole
			return nil
		}); err != nil {
			return nil, dto.InternalWithError("Failed to update workspace member role", err)
		}
	}

	uwm, err := getUserWithMemberships(h.svc.User, h.svc.OrgMembership, h.svc.WSMembership, h.svc.Organization, h.svc.Workspace, req.UserID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user", err)
	}
	return userWithMembershipsToResponse(uwm), nil
}

// UpdateUserSettings updates user global settings.
func (h *UserHandler) UpdateUserSettings(ctx context.Context, user *identity.User, req *dto.UpdateUserSettingsRequest) (*dto.UserResponse, error) {
	_, err := h.svc.User.Modify(user.ID, func(u *identity.User) error {
		u.Settings = userSettingsToEntity(req.Settings)
		return nil
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to update settings", err)
	}
	uwm, err := getUserWithMemberships(h.svc.User, h.svc.OrgMembership, h.svc.WSMembership, h.svc.Organization, h.svc.Workspace, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user", err)
	}
	return userWithMembershipsToResponse(uwm), nil
}
