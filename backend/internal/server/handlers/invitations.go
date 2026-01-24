package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// InvitationHandler handles invitation requests.
type InvitationHandler struct {
	orgInvService *identity.OrganizationInvitationService
	wsInvService  *identity.WorkspaceInvitationService
	userService   *identity.UserService
	orgService    *identity.OrganizationService
	wsService     *identity.WorkspaceService
	orgMemService *identity.OrganizationMembershipService
	wsMemService  *identity.WorkspaceMembershipService
	authHandler   *AuthHandler
}

// NewInvitationHandler creates a new invitation handler.
func NewInvitationHandler(
	orgInvService *identity.OrganizationInvitationService,
	wsInvService *identity.WorkspaceInvitationService,
	userService *identity.UserService,
	orgService *identity.OrganizationService,
	wsService *identity.WorkspaceService,
	orgMemService *identity.OrganizationMembershipService,
	wsMemService *identity.WorkspaceMembershipService,
	authHandler *AuthHandler,
) *InvitationHandler {
	return &InvitationHandler{
		orgInvService: orgInvService,
		wsInvService:  wsInvService,
		userService:   userService,
		orgService:    orgService,
		wsService:     wsService,
		orgMemService: orgMemService,
		wsMemService:  wsMemService,
		authHandler:   authHandler,
	}
}

// CreateOrgInvitation creates a new organization invitation.
func (h *InvitationHandler) CreateOrgInvitation(ctx context.Context, orgID jsonldb.ID, user *identity.User, req *dto.CreateOrgInvitationRequest) (*dto.OrgInvitationResponse, error) {
	if req.Email == "" || req.Role == "" {
		return nil, dto.MissingField("email or role")
	}
	invitation, err := h.orgInvService.Create(req.Email, orgID, orgRoleToEntity(req.Role), user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create invitation", err)
	}
	return orgInvitationToResponse(invitation), nil
}

// ListOrgInvitations returns all pending invitations for an organization.
func (h *InvitationHandler) ListOrgInvitations(ctx context.Context, orgID jsonldb.ID, _ *identity.User, _ *dto.ListOrgInvitationsRequest) (*dto.ListOrgInvitationsResponse, error) {
	var responses []dto.OrgInvitationResponse //nolint:prealloc // Iterator length unknown
	for inv := range h.orgInvService.IterByOrg(orgID) {
		responses = append(responses, *orgInvitationToResponse(inv))
	}
	return &dto.ListOrgInvitationsResponse{Invitations: responses}, nil
}

// CreateWSInvitation creates a new workspace invitation.
func (h *InvitationHandler) CreateWSInvitation(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.CreateWSInvitationRequest) (*dto.WSInvitationResponse, error) {
	if req.Email == "" || req.Role == "" {
		return nil, dto.MissingField("email or role")
	}
	invitation, err := h.wsInvService.Create(req.Email, wsID, wsRoleToEntity(req.Role), user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create invitation", err)
	}
	return wsInvitationToResponse(invitation), nil
}

// ListWSInvitations returns all pending invitations for a workspace.
func (h *InvitationHandler) ListWSInvitations(ctx context.Context, wsID jsonldb.ID, _ *identity.User, _ *dto.ListWSInvitationsRequest) (*dto.ListWSInvitationsResponse, error) {
	var responses []dto.WSInvitationResponse //nolint:prealloc // Iterator length unknown
	for inv := range h.wsInvService.IterByWorkspace(wsID) {
		responses = append(responses, *wsInvitationToResponse(inv))
	}
	return &dto.ListWSInvitationsResponse{Invitations: responses}, nil
}

// AcceptOrgInvitation handles a user accepting an organization invitation.
// This is a public endpoint (no auth required).
func (h *InvitationHandler) AcceptOrgInvitation(ctx context.Context, req *dto.AcceptInvitationRequest) (*dto.AuthResponse, error) {
	if req.Token == "" {
		return nil, dto.MissingField("token")
	}

	inv, err := h.orgInvService.GetByToken(req.Token)
	if err != nil {
		return nil, dto.NewAPIError(404, dto.ErrorCodeNotFound, "Invitation not found or expired")
	}

	if time.Now().After(inv.ExpiresAt) {
		return nil, dto.Expired("invitation")
	}

	// Create user or link to existing
	user, err := h.userService.GetByEmail(inv.Email)
	if err != nil {
		// Create new user if they don't exist
		if req.Password == "" || req.Name == "" {
			return nil, dto.MissingField("password and name required for new account")
		}
		user, err = h.userService.Create(inv.Email, req.Password, req.Name)
		if err != nil {
			return nil, dto.InternalWithError("Failed to create user", err)
		}
	}

	// Create organization membership
	if _, err = h.orgMemService.Create(user.ID, inv.OrganizationID, inv.Role); err != nil {
		return nil, dto.InternalWithError("Failed to create organization membership", err)
	}

	// Delete invitation (best effort - membership already created)
	if err := h.orgInvService.Delete(inv.ID); err != nil {
		slog.Warn("Failed to delete invitation after acceptance", "invitationID", inv.ID, "error", err)
	}

	// Generate token
	token, err := h.authHandler.GenerateToken(user)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response
	uwm, err := getUserWithMemberships(h.userService, h.orgMemService, h.wsMemService, h.orgService, h.wsService, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Set active context
	h.authHandler.PopulateActiveContext(userResp, uwm)

	return &dto.AuthResponse{
		Token: token,
		User:  userResp,
	}, nil
}

// AcceptWSInvitation handles a user accepting a workspace invitation.
// This is a public endpoint (no auth required).
func (h *InvitationHandler) AcceptWSInvitation(ctx context.Context, req *dto.AcceptInvitationRequest) (*dto.AuthResponse, error) {
	if req.Token == "" {
		return nil, dto.MissingField("token")
	}

	inv, err := h.wsInvService.GetByToken(req.Token)
	if err != nil {
		return nil, dto.NewAPIError(404, dto.ErrorCodeNotFound, "Invitation not found or expired")
	}

	if time.Now().After(inv.ExpiresAt) {
		return nil, dto.Expired("invitation")
	}

	// Get workspace to find org
	ws, err := h.wsService.Get(inv.WorkspaceID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	// Create user or link to existing
	user, err := h.userService.GetByEmail(inv.Email)
	if err != nil {
		// Create new user if they don't exist
		if req.Password == "" || req.Name == "" {
			return nil, dto.MissingField("password and name required for new account")
		}
		user, err = h.userService.Create(inv.Email, req.Password, req.Name)
		if err != nil {
			return nil, dto.InternalWithError("Failed to create user", err)
		}
	}

	// Ensure user has org membership (as member if not already)
	if _, err := h.orgMemService.Get(user.ID, ws.OrganizationID); err != nil {
		if _, err = h.orgMemService.Create(user.ID, ws.OrganizationID, identity.OrgRoleMember); err != nil {
			return nil, dto.InternalWithError("Failed to create organization membership", err)
		}
	}

	// Create workspace membership
	if _, err = h.wsMemService.Create(user.ID, inv.WorkspaceID, inv.Role); err != nil {
		return nil, dto.InternalWithError("Failed to create workspace membership", err)
	}

	// Delete invitation (best effort - membership already created)
	if err := h.wsInvService.Delete(inv.ID); err != nil {
		slog.Warn("Failed to delete invitation after acceptance", "invitationID", inv.ID, "error", err)
	}

	// Generate token
	token, err := h.authHandler.GenerateToken(user)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response
	uwm, err := getUserWithMemberships(h.userService, h.orgMemService, h.wsMemService, h.orgService, h.wsService, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Set active context
	h.authHandler.PopulateActiveContext(userResp, uwm)

	return &dto.AuthResponse{
		Token: token,
		User:  userResp,
	}, nil
}
