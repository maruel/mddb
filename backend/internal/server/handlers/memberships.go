// Handles workspace switching and membership settings.

package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// MembershipHandler handles membership-related requests.
type MembershipHandler struct {
	Svc *Services
	Cfg *Config
}

// SwitchWorkspace switches the user's active workspace and returns a new token.
func (h *MembershipHandler) SwitchWorkspace(ctx context.Context, user *identity.User, req *dto.SwitchWorkspaceRequest) (*dto.SwitchWorkspaceResponse, error) {
	// Get workspace to check org membership
	ws, err := h.Svc.Workspace.Get(req.WsID)
	if err != nil {
		return nil, dto.NotFound("workspace")
	}

	// Verify org membership
	orgMem, err := h.Svc.OrgMembership.Get(user.ID, ws.OrganizationID)
	if err != nil {
		return nil, dto.Forbidden("User is not a member of this organization")
	}

	// Verify workspace membership (or org admin)
	wsMem, err := h.Svc.WSMembership.Get(user.ID, req.WsID)
	if err != nil && orgMem.Role != identity.OrgRoleOwner && orgMem.Role != identity.OrgRoleAdmin {
		return nil, dto.Forbidden("User is not a member of this workspace")
	}

	// Persist active workspace in user settings (LRU: prepend to list, limit to 10)
	if _, err := h.Svc.User.Modify(user.ID, func(u *identity.User) error {
		// Remove this workspace from current list (if present)
		newList := make([]ksid.ID, 0, len(u.Settings.LastActiveWorkspaces)+1)
		newList = append(newList, req.WsID) // Prepend as most recent
		for _, id := range u.Settings.LastActiveWorkspaces {
			if id != req.WsID {
				newList = append(newList, id)
			}
		}
		// Limit to 10 entries
		if len(newList) > 10 {
			newList = newList[:10]
		}
		u.Settings.LastActiveWorkspaces = newList
		return nil
	}); err != nil {
		return nil, dto.InternalWithError("Failed to save workspace preference", err)
	}

	token, err := h.Cfg.GenerateToken(user)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response with memberships
	uwm, err := getUserWithMemberships(h.Svc.User, h.Svc.OrgMembership, h.Svc.WSMembership, h.Svc.Organization, h.Svc.Workspace, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Set the switched workspace as active
	userResp.OrganizationID = ws.OrganizationID
	userResp.OrgRole = dto.OrganizationRole(orgMem.Role)
	userResp.WorkspaceID = req.WsID
	if wsMem != nil {
		userResp.WorkspaceRole = dto.WorkspaceRole(wsMem.Role)
	} else {
		// Org admin gets admin access to workspace
		userResp.WorkspaceRole = dto.WSRoleAdmin
	}

	return &dto.SwitchWorkspaceResponse{
		Token: token,
		User:  userResp,
	}, nil
}

// UpdateWSMembershipSettings updates user preferences within a workspace.
func (h *MembershipHandler) UpdateWSMembershipSettings(ctx context.Context, wsID ksid.ID, user *identity.User, req *dto.UpdateWSMembershipSettingsRequest) (*dto.WSMembershipResponse, error) {
	m, err := h.Svc.WSMembership.Get(user.ID, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace membership", err)
	}
	m, err = h.Svc.WSMembership.Modify(m.ID, func(m *identity.WorkspaceMembership) error {
		m.Settings = wsMembershipSettingsToEntity(req.Settings)
		return nil
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to update workspace membership settings", err)
	}
	return wsMembershipToResponse(m), nil
}
