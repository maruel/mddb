// Handles user context switching (org/workspace) and membership settings.

package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// MembershipHandler handles membership-related requests.
type MembershipHandler struct {
	orgMemService *identity.OrganizationMembershipService
	wsMemService  *identity.WorkspaceMembershipService
	userService   *identity.UserService
	orgService    *identity.OrganizationService
	wsService     *identity.WorkspaceService
	authHandler   *AuthHandler
}

// NewMembershipHandler creates a new membership handler.
func NewMembershipHandler(
	orgMemService *identity.OrganizationMembershipService,
	wsMemService *identity.WorkspaceMembershipService,
	userService *identity.UserService,
	orgService *identity.OrganizationService,
	wsService *identity.WorkspaceService,
	authHandler *AuthHandler,
) *MembershipHandler {
	return &MembershipHandler{
		orgMemService: orgMemService,
		wsMemService:  wsMemService,
		userService:   userService,
		orgService:    orgService,
		wsService:     wsService,
		authHandler:   authHandler,
	}
}

// SwitchOrg switches the user's active organization and returns a new token.
func (h *MembershipHandler) SwitchOrg(ctx context.Context, _ jsonldb.ID, user *identity.User, req *dto.SwitchOrgRequest) (*dto.SwitchOrgResponse, error) {
	// Verify membership
	orgMem, err := h.orgMemService.Get(user.ID, req.OrgID)
	if err != nil {
		return nil, dto.Forbidden("User is not a member of this organization")
	}

	token, err := h.authHandler.GenerateToken(user)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response with memberships
	uwm, err := getUserWithMemberships(h.userService, h.orgMemService, h.wsMemService, h.orgService, h.wsService, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Set the switched org as active
	userResp.OrganizationID = req.OrgID
	userResp.OrgRole = dto.OrganizationRole(orgMem.Role)

	// Find first workspace in this org
	for _, ws := range uwm.WSMemberships {
		if ws.OrganizationID == req.OrgID {
			userResp.WorkspaceID = ws.WorkspaceID
			userResp.WorkspaceRole = dto.WorkspaceRole(ws.Role)
			break
		}
	}

	return &dto.SwitchOrgResponse{
		Token: token,
		User:  userResp,
	}, nil
}

// SwitchWorkspace switches the user's active workspace and returns a new token.
func (h *MembershipHandler) SwitchWorkspace(ctx context.Context, _ jsonldb.ID, user *identity.User, req *dto.SwitchWorkspaceRequest) (*dto.SwitchWorkspaceResponse, error) {
	// Get workspace to check org membership
	ws, err := h.wsService.Get(req.WsID)
	if err != nil {
		return nil, dto.NotFound("workspace")
	}

	// Verify org membership
	orgMem, err := h.orgMemService.Get(user.ID, ws.OrganizationID)
	if err != nil {
		return nil, dto.Forbidden("User is not a member of this organization")
	}

	// Verify workspace membership (or org admin)
	wsMem, err := h.wsMemService.Get(user.ID, req.WsID)
	if err != nil && orgMem.Role != identity.OrgRoleOwner && orgMem.Role != identity.OrgRoleAdmin {
		return nil, dto.Forbidden("User is not a member of this workspace")
	}

	token, err := h.authHandler.GenerateToken(user)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response with memberships
	uwm, err := getUserWithMemberships(h.userService, h.orgMemService, h.wsMemService, h.orgService, h.wsService, user.ID)
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
func (h *MembershipHandler) UpdateWSMembershipSettings(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.UpdateWSMembershipSettingsRequest) (*dto.WSMembershipResponse, error) {
	m, err := h.wsMemService.Get(user.ID, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace membership", err)
	}
	m, err = h.wsMemService.Modify(m.ID, func(m *identity.WorkspaceMembership) error {
		m.Settings = wsMembershipSettingsToEntity(req.Settings)
		return nil
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to update workspace membership settings", err)
	}
	return wsMembershipToResponse(m), nil
}
