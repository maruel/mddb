// Handles organization and workspace invitations.

package handlers

import (
	"context"
	"log/slog"

	"github.com/maruel/mddb/backend/internal/email"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// InvitationHandler handles invitation requests.
type InvitationHandler struct {
	Svc *Services
	Cfg *Config
}

// CreateOrgInvitation creates a new organization invitation.
func (h *InvitationHandler) CreateOrgInvitation(ctx context.Context, orgID jsonldb.ID, user *identity.User, req *dto.CreateOrgInvitationRequest) (*dto.OrgInvitationResponse, error) {
	if req.Email == "" || req.Role == "" {
		return nil, dto.MissingField("email or role")
	}
	invitation, err := h.Svc.OrgInvitation.Create(req.Email, orgID, orgRoleToEntity(req.Role), user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create invitation", err)
	}

	// Send invitation email if configured
	if h.Svc.Email != nil {
		org, err := h.Svc.Organization.Get(orgID)
		if err != nil {
			slog.WarnContext(ctx, "Failed to get org for invitation email", "err", err, "org_id", orgID)
		} else {
			// Determine locale: use request locale, fall back to inviter's settings, then default
			locale := email.ParseLocale(req.Locale)
			if req.Locale == "" && user.Settings.Language != "" {
				locale = email.ParseLocale(user.Settings.Language)
			}
			acceptURL := h.Cfg.BaseURL + "/accept-invitation/org?token=" + invitation.Token
			if err := h.Svc.Email.SendOrgInvitation(ctx, req.Email, org.Name, user.Name, string(req.Role), acceptURL, locale); err != nil {
				slog.WarnContext(ctx, "Failed to send org invitation email", "err", err, "email", req.Email)
			} else {
				slog.InfoContext(ctx, "Org invitation email sent", "email", req.Email, "org_id", orgID, "locale", locale)
			}
		}
	}

	return orgInvitationToResponse(invitation), nil
}

// ListOrgInvitations returns all pending invitations for an organization.
func (h *InvitationHandler) ListOrgInvitations(ctx context.Context, orgID jsonldb.ID, _ *identity.User, _ *dto.ListOrgInvitationsRequest) (*dto.ListOrgInvitationsResponse, error) {
	var responses []dto.OrgInvitationResponse //nolint:prealloc // Iterator length unknown
	for inv := range h.Svc.OrgInvitation.IterByOrg(orgID) {
		responses = append(responses, *orgInvitationToResponse(inv))
	}
	return &dto.ListOrgInvitationsResponse{Invitations: responses}, nil
}

// CreateWSInvitation creates a new workspace invitation.
func (h *InvitationHandler) CreateWSInvitation(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.CreateWSInvitationRequest) (*dto.WSInvitationResponse, error) {
	if req.Email == "" || req.Role == "" {
		return nil, dto.MissingField("email or role")
	}
	invitation, err := h.Svc.WSInvitation.Create(req.Email, wsID, wsRoleToEntity(req.Role), user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create invitation", err)
	}

	// Send invitation email if configured
	if h.Svc.Email != nil {
		ws, err := h.Svc.Workspace.Get(wsID)
		if err != nil {
			slog.WarnContext(ctx, "Failed to get workspace for invitation email", "err", err, "ws_id", wsID)
		} else {
			org, err := h.Svc.Organization.Get(ws.OrganizationID)
			if err != nil {
				slog.WarnContext(ctx, "Failed to get org for invitation email", "err", err, "org_id", ws.OrganizationID)
			} else {
				// Determine locale: use request locale, fall back to inviter's settings, then default
				locale := email.ParseLocale(req.Locale)
				if req.Locale == "" && user.Settings.Language != "" {
					locale = email.ParseLocale(user.Settings.Language)
				}
				acceptURL := h.Cfg.BaseURL + "/accept-invitation/ws?token=" + invitation.Token
				if err := h.Svc.Email.SendWSInvitation(ctx, req.Email, ws.Name, org.Name, user.Name, string(req.Role), acceptURL, locale); err != nil {
					slog.WarnContext(ctx, "Failed to send ws invitation email", "err", err, "email", req.Email)
				} else {
					slog.InfoContext(ctx, "Workspace invitation email sent", "email", req.Email, "ws_id", wsID, "locale", locale)
				}
			}
		}
	}

	return wsInvitationToResponse(invitation), nil
}

// ListWSInvitations returns all pending invitations for a workspace.
func (h *InvitationHandler) ListWSInvitations(ctx context.Context, wsID jsonldb.ID, _ *identity.User, _ *dto.ListWSInvitationsRequest) (*dto.ListWSInvitationsResponse, error) {
	var responses []dto.WSInvitationResponse //nolint:prealloc // Iterator length unknown
	for inv := range h.Svc.WSInvitation.IterByWorkspace(wsID) {
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

	inv, err := h.Svc.OrgInvitation.GetByToken(req.Token)
	if err != nil {
		return nil, dto.NewAPIError(404, dto.ErrorCodeNotFound, "Invitation not found or expired")
	}

	if storage.Now().After(inv.ExpiresAt) {
		return nil, dto.Expired("invitation")
	}

	// Create user or link to existing
	user, err := h.Svc.User.GetByEmail(inv.Email)
	if err != nil {
		// Create new user if they don't exist
		if req.Password == "" || req.Name == "" {
			return nil, dto.MissingField("password and name required for new account")
		}
		user, err = h.Svc.User.Create(inv.Email, req.Password, req.Name)
		if err != nil {
			return nil, dto.InternalWithError("Failed to create user", err)
		}
	}

	// Create organization membership
	if _, err = h.Svc.OrgMembership.Create(user.ID, inv.OrganizationID, inv.Role); err != nil {
		return nil, dto.InternalWithError("Failed to create organization membership", err)
	}

	// Delete invitation (best effort - membership already created)
	if err := h.Svc.OrgInvitation.Delete(inv.ID); err != nil {
		slog.Warn("Failed to delete invitation after acceptance", "invitationID", inv.ID, "error", err)
	}

	// Generate token
	token, err := h.Cfg.GenerateToken(user)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response
	uwm, err := getUserWithMemberships(h.Svc.User, h.Svc.OrgMembership, h.Svc.WSMembership, h.Svc.Organization, h.Svc.Workspace, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Set active context
	uwm.populateActiveContext(userResp)

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

	inv, err := h.Svc.WSInvitation.GetByToken(req.Token)
	if err != nil {
		return nil, dto.NewAPIError(404, dto.ErrorCodeNotFound, "Invitation not found or expired")
	}

	if storage.Now().After(inv.ExpiresAt) {
		return nil, dto.Expired("invitation")
	}

	// Get workspace to find org
	ws, err := h.Svc.Workspace.Get(inv.WorkspaceID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}

	// Create user or link to existing
	user, err := h.Svc.User.GetByEmail(inv.Email)
	if err != nil {
		// Create new user if they don't exist
		if req.Password == "" || req.Name == "" {
			return nil, dto.MissingField("password and name required for new account")
		}
		user, err = h.Svc.User.Create(inv.Email, req.Password, req.Name)
		if err != nil {
			return nil, dto.InternalWithError("Failed to create user", err)
		}
	}

	// Ensure user has org membership (as member if not already)
	if _, err := h.Svc.OrgMembership.Get(user.ID, ws.OrganizationID); err != nil {
		if _, err = h.Svc.OrgMembership.Create(user.ID, ws.OrganizationID, identity.OrgRoleMember); err != nil {
			return nil, dto.InternalWithError("Failed to create organization membership", err)
		}
	}

	// Create workspace membership
	if _, err = h.Svc.WSMembership.Create(user.ID, inv.WorkspaceID, inv.Role); err != nil {
		return nil, dto.InternalWithError("Failed to create workspace membership", err)
	}

	// Delete invitation (best effort - membership already created)
	if err := h.Svc.WSInvitation.Delete(inv.ID); err != nil {
		slog.Warn("Failed to delete invitation after acceptance", "invitationID", inv.ID, "error", err)
	}

	// Generate token
	token, err := h.Cfg.GenerateToken(user)
	if err != nil {
		return nil, dto.InternalWithError("Failed to generate token", err)
	}

	// Build user response
	uwm, err := getUserWithMemberships(h.Svc.User, h.Svc.OrgMembership, h.Svc.WSMembership, h.Svc.Organization, h.Svc.Workspace, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Set active context
	uwm.populateActiveContext(userResp)

	return &dto.AuthResponse{
		Token: token,
		User:  userResp,
	}, nil
}
