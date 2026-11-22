// Creates, previews, revokes, emails, and durably accepts bearer or verified-recipient invitation links.

package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/email"
	"github.com/maruel/mddb/backend/internal/server/dto/v1"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// InvitationHandler handles invitation requests.
type InvitationHandler struct {
	Svc *Services
	Cfg *Config
}

// CreateOrgInvitation creates a new organization invitation.
func (h *InvitationHandler) CreateOrgInvitation(ctx context.Context, orgID ksid.ID, user *identityv1.User, req *dto.CreateOrgInvitationRequest) (*dto.OrgInvitationResponse, error) {
	if req.Role == "" {
		return nil, dto.MissingField("role")
	}
	invitation, err := h.Svc.OrgInvitation.Create(req.Email, orgID, orgRoleToEntity(req.Role), user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create invitation", err)
	}
	resp := orgInvitationToResponse(invitation, h.Cfg.BaseURL)

	// Email recipient-bound invitations when SMTP is configured; bearer links are only copied.
	if req.Email != "" && h.Svc.Email != nil {
		org, err := h.Svc.Organization.Get(orgID)
		if err != nil {
			slog.WarnContext(ctx, "Failed to get org for invitation email", "err", err, "org_id", orgID)
		} else {
			locale := invitationLocale(req.Locale, user)
			if err := h.Svc.Email.SendOrgInvitation(ctx, req.Email, org.Name, user.Name, string(req.Role), resp.InviteURL, locale); err != nil {
				slog.WarnContext(ctx, "Failed to send org invitation email", "err", err, "email", req.Email)
			} else {
				slog.InfoContext(ctx, "Org invitation email sent", "email", req.Email, "org_id", orgID, "locale", locale)
			}
		}
	}

	// Notify the invitee (if they already have an account).
	if invitee, err := h.Svc.User.GetByEmail(req.Email); req.Email != "" && err == nil && invitee.EmailVerified {
		org, _ := h.Svc.Organization.Get(orgID)
		orgName := ""
		if org != nil {
			orgName = org.Name
		}
		h.Svc.Emit(ctx, h.Cfg.VAPIDKeys(), invitee.ID, identityv1.NotifOrgInvite,
			user.Name+" invited you to "+orgName, "", orgID.String(), user.ID)
	}

	return resp, nil
}

// ListOrgInvitations returns all pending invitations for an organization.
func (h *InvitationHandler) ListOrgInvitations(ctx context.Context, orgID ksid.ID, _ *identityv1.User, _ *dto.ListOrgInvitationsRequest) (*dto.ListOrgInvitationsResponse, error) {
	var responses []dto.OrgInvitationResponse
	for inv := range h.Svc.OrgInvitation.IterByOrg(orgID) {
		if inv.IsExpired() || !inv.AcceptedAt.IsZero() {
			continue
		}
		responses = append(responses, *orgInvitationToResponse(inv, h.Cfg.BaseURL))
	}
	return &dto.ListOrgInvitationsResponse{Invitations: responses}, nil
}

// CreateWSInvitation creates a new workspace invitation.
func (h *InvitationHandler) CreateWSInvitation(ctx context.Context, wsID ksid.ID, user *identityv1.User, req *dto.CreateWSInvitationRequest) (*dto.WSInvitationResponse, error) {
	if req.Role == "" {
		return nil, dto.MissingField("role")
	}
	invitation, err := h.Svc.WSInvitation.Create(req.Email, wsID, wsRoleToEntity(req.Role), user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create invitation", err)
	}
	resp := wsInvitationToResponse(invitation, h.Cfg.BaseURL)

	// Email recipient-bound invitations when SMTP is configured; bearer links are only copied.
	if req.Email != "" && h.Svc.Email != nil {
		ws, err := h.Svc.Workspace.Get(wsID)
		if err != nil {
			slog.WarnContext(ctx, "Failed to get workspace for invitation email", "err", err, "ws_id", wsID)
		} else if org, err := h.Svc.Organization.Get(ws.OrganizationID); err != nil {
			slog.WarnContext(ctx, "Failed to get org for invitation email", "err", err, "org_id", ws.OrganizationID)
		} else {
			locale := invitationLocale(req.Locale, user)
			if err := h.Svc.Email.SendWSInvitation(ctx, req.Email, ws.Name, org.Name, user.Name, string(req.Role), resp.InviteURL, locale); err != nil {
				slog.WarnContext(ctx, "Failed to send ws invitation email", "err", err, "email", req.Email)
			} else {
				slog.InfoContext(ctx, "Workspace invitation email sent", "email", req.Email, "ws_id", wsID, "locale", locale)
			}
		}
	}

	// Notify the invitee (if they already have an account).
	if invitee, err := h.Svc.User.GetByEmail(req.Email); req.Email != "" && err == nil && invitee.EmailVerified {
		wsObj, _ := h.Svc.Workspace.Get(wsID)
		wsName := ""
		if wsObj != nil {
			wsName = wsObj.Name
		}
		h.Svc.Emit(ctx, h.Cfg.VAPIDKeys(), invitee.ID, identityv1.NotifWSInvite,
			user.Name+" invited you to "+wsName, "", wsID.String(), user.ID)
	}

	return resp, nil
}

// invitationLocale selects the request locale, then the inviter's language, then the default.
func invitationLocale(requested string, inviter *identityv1.User) email.Locale {
	if requested == "" && inviter.Settings.Language != "" {
		return email.ParseLocale(inviter.Settings.Language)
	}
	return email.ParseLocale(requested)
}

// ListWSInvitations returns all pending invitations for a workspace.
func (h *InvitationHandler) ListWSInvitations(ctx context.Context, wsID ksid.ID, _ *identityv1.User, _ *dto.ListWSInvitationsRequest) (*dto.ListWSInvitationsResponse, error) {
	var responses []dto.WSInvitationResponse
	for inv := range h.Svc.WSInvitation.IterByWorkspace(wsID) {
		if inv.IsExpired() || !inv.AcceptedAt.IsZero() {
			continue
		}
		responses = append(responses, *wsInvitationToResponse(inv, h.Cfg.BaseURL))
	}
	return &dto.ListWSInvitationsResponse{Invitations: responses}, nil
}

// AcceptOrgInvitation handles a user accepting an organization invitation.
// The invitation token selects a grant; it is not an authentication or email proof.
func (h *InvitationHandler) AcceptOrgInvitation(ctx context.Context, user *identityv1.User, req *dto.AcceptInvitationRequest) (*dto.UserResponse, error) {
	if req.Token == "" {
		return nil, dto.MissingField("token")
	}

	user, err := h.Svc.User.Get(user.ID)
	if err != nil {
		return nil, dto.Unauthorized()
	}
	inv, err := h.Svc.OrgInvitation.Accept(req.Token, user, func(inv *identityv1.OrganizationInvitation) error {
		if _, err := h.Svc.OrgMembership.Get(user.ID, inv.OrganizationID); err == nil {
			return nil
		}
		_, err := h.Svc.OrgMembership.Create(user.ID, inv.OrganizationID, inv.Role)
		return err
	})
	if err != nil {
		return nil, invitationAcceptanceError(err)
	}

	// Build user response
	uwm, err := getUserWithMemberships(h.Svc.User, h.Svc.OrgMembership, h.Svc.WSMembership, h.Svc.Organization, h.Svc.Workspace, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)
	userResp.SessionID = reqctx.SessionID(ctx)

	// Set active context
	uwm.populateActiveContext(userResp)

	// Notify org admins that a new member joined.
	org, _ := h.Svc.Organization.Get(inv.OrganizationID)
	orgName := ""
	if org != nil {
		orgName = org.Name
	}
	for m := range h.Svc.OrgMembership.IterByOrg(inv.OrganizationID) {
		if m.UserID != user.ID && (m.Role == identityv1.OrgRoleAdmin || m.Role == identityv1.OrgRoleOwner) {
			h.Svc.Emit(ctx, h.Cfg.VAPIDKeys(), m.UserID, identityv1.NotifMemberJoined,
				user.Name+" joined "+orgName, "", inv.OrganizationID.String(), user.ID)
		}
	}

	return userResp, nil
}

// AcceptWSInvitation handles a user accepting a workspace invitation.
// The invitation token selects a grant; it is not an authentication or email proof.
func (h *InvitationHandler) AcceptWSInvitation(ctx context.Context, user *identityv1.User, req *dto.AcceptInvitationRequest) (*dto.UserResponse, error) {
	if req.Token == "" {
		return nil, dto.MissingField("token")
	}

	user, err := h.Svc.User.Get(user.ID)
	if err != nil {
		return nil, dto.Unauthorized()
	}
	// Membership tables publish separately. On failure report the live portions
	// to this claimant; persistence errors can leave their durability uncertain.
	var orgID, wsID ksid.ID
	membershipWriteFailed := false

	inv, err := h.Svc.WSInvitation.Accept(req.Token, user, func(inv *identityv1.WorkspaceInvitation) error {
		ws, err := h.Svc.Workspace.Get(inv.WorkspaceID)
		if err != nil {
			return err
		}
		orgID, wsID = ws.OrganizationID, ws.ID
		if _, err := h.Svc.OrgMembership.Get(user.ID, ws.OrganizationID); err != nil {
			if _, err := h.Svc.OrgMembership.Create(user.ID, ws.OrganizationID, identityv1.OrgRoleMember); err != nil {
				membershipWriteFailed = true
				return err
			}
		}
		if _, err := h.Svc.WSMembership.Get(user.ID, inv.WorkspaceID); err == nil {
			return nil
		}
		_, err = h.Svc.WSMembership.Create(user.ID, inv.WorkspaceID, inv.Role)
		membershipWriteFailed = err != nil
		return err
	})
	if err != nil {
		if !wsID.IsZero() {
			_, orgErr := h.Svc.OrgMembership.Get(user.ID, orgID)
			_, wsErr := h.Svc.WSMembership.Get(user.ID, wsID)
			if orgErr == nil || wsErr == nil {
				return nil, dto.InternalWithError("Invitation grant is incomplete; retry with the same account", err).
					WithDetail("organization_id", orgID).
					WithDetail("workspace_id", wsID).
					WithDetail("organization_membership_present", orgErr == nil).
					WithDetail("workspace_membership_present", wsErr == nil).
					WithDetail("retryable", true).
					WithDetail("persistence_uncertain", membershipWriteFailed)
			}
		}
		return nil, invitationAcceptanceError(err)
	}
	ws, err := h.Svc.Workspace.Get(inv.WorkspaceID)
	if err != nil {
		return nil, dto.NotFound("workspace")
	}

	// Build user response
	uwm, err := getUserWithMemberships(h.Svc.User, h.Svc.OrgMembership, h.Svc.WSMembership, h.Svc.Organization, h.Svc.Workspace, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)
	userResp.SessionID = reqctx.SessionID(ctx)

	// Set active context
	uwm.populateActiveContext(userResp)

	// Notify workspace admins that a new member joined.
	for m := range h.Svc.WSMembership.IterByWorkspace(inv.WorkspaceID) {
		if m.UserID != user.ID && m.Role == identityv1.WSRoleAdmin {
			h.Svc.Emit(ctx, h.Cfg.VAPIDKeys(), m.UserID, identityv1.NotifMemberJoined,
				user.Name+" joined "+ws.Name, "", inv.WorkspaceID.String(), user.ID)
		}
	}

	return userResp, nil
}

// GetOrgInvitationInfo describes a grant selected by its secret without claiming it.
func (h *InvitationHandler) GetOrgInvitationInfo(_ context.Context, req *dto.OrgInvitationInfoRequest) (*dto.OrgInvitationInfoResponse, error) {
	inv, err := h.Svc.OrgInvitation.GetByToken(req.Token)
	if err != nil || inv.IsExpired() || !inv.AcceptedAt.IsZero() {
		return nil, dto.NotFound("invitation")
	}
	org, err := h.Svc.Organization.Get(inv.OrganizationID)
	if err != nil {
		return nil, dto.NotFound("organization")
	}
	return &dto.OrgInvitationInfoResponse{Email: inv.Email, Role: dto.OrganizationRole(inv.Role), OrganizationID: org.ID, OrganizationName: org.Name, ExpiresAt: inv.ExpiresAt}, nil
}

// GetWSInvitationInfo describes a grant selected by its secret without claiming it.
func (h *InvitationHandler) GetWSInvitationInfo(_ context.Context, req *dto.WSInvitationInfoRequest) (*dto.WSInvitationInfoResponse, error) {
	inv, err := h.Svc.WSInvitation.GetByToken(req.Token)
	if err != nil || inv.IsExpired() || !inv.AcceptedAt.IsZero() {
		return nil, dto.NotFound("invitation")
	}
	ws, err := h.Svc.Workspace.Get(inv.WorkspaceID)
	if err != nil {
		return nil, dto.NotFound("workspace")
	}
	org, err := h.Svc.Organization.Get(ws.OrganizationID)
	if err != nil {
		return nil, dto.NotFound("organization")
	}
	return &dto.WSInvitationInfoResponse{Email: inv.Email, Role: dto.WorkspaceRole(inv.Role), OrganizationID: org.ID, OrganizationName: org.Name, WorkspaceID: ws.ID, WorkspaceName: ws.Name, ExpiresAt: inv.ExpiresAt}, nil
}

// DeleteOrgInvitation revokes an invitation only within its owning organization.
func (h *InvitationHandler) DeleteOrgInvitation(_ context.Context, orgID ksid.ID, _ *identityv1.User, req *dto.DeleteOrgInvitationRequest) (*dto.OkResponse, error) {
	inv, err := h.Svc.OrgInvitation.Get(req.InvitationID)
	if err != nil || inv.OrganizationID != orgID {
		return nil, dto.NotFound("invitation")
	}
	if err := h.Svc.OrgInvitation.Delete(inv.ID); err != nil {
		return nil, dto.InternalWithError("Failed to revoke invitation", err)
	}
	return &dto.OkResponse{Ok: true}, nil
}

// DeleteWSInvitation revokes an invitation only within its owning workspace.
func (h *InvitationHandler) DeleteWSInvitation(_ context.Context, wsID ksid.ID, _ *identityv1.User, req *dto.DeleteWSInvitationRequest) (*dto.OkResponse, error) {
	inv, err := h.Svc.WSInvitation.Get(req.InvitationID)
	if err != nil || inv.WorkspaceID != wsID {
		return nil, dto.NotFound("invitation")
	}
	if err := h.Svc.WSInvitation.Delete(inv.ID); err != nil {
		return nil, dto.InternalWithError("Failed to revoke invitation", err)
	}
	return &dto.OkResponse{Ok: true}, nil
}
func invitationAcceptanceError(err error) error {
	switch {
	case errors.Is(err, identity.ErrInvitationRecipient):
		return dto.NewAPIError(http.StatusForbidden, dto.ErrorCodeEmailNotVerified, "Sign in with the verified email address named by this invitation")
	case errors.Is(err, identity.ErrInvitationClaimed):
		return dto.NewAPIError(http.StatusConflict, dto.ErrorCodeConflict, "This invitation was claimed by another account")
	case errors.Is(err, identity.ErrInvitationExpired):
		return dto.Expired("invitation")
	case errors.Is(err, identity.ErrInvitationNotFound):
		return dto.NotFound("invitation")
	default:
		return dto.InternalWithError("Failed to accept invitation", err)
	}
}
