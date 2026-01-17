package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/storage"
)

// InvitationHandler handles invitation requests.
type InvitationHandler struct {
	invService  *storage.InvitationService
	userService *storage.UserService
	orgService  *storage.OrganizationService
	memService  *storage.MembershipService
}

// NewInvitationHandler creates a new invitation handler.
func NewInvitationHandler(invService *storage.InvitationService, userService *storage.UserService, orgService *storage.OrganizationService, memService *storage.MembershipService) *InvitationHandler {
	return &InvitationHandler{
		invService:  invService,
		userService: userService,
		orgService:  orgService,
		memService:  memService,
	}
}

// CreateInvitationRequest is a request to create a new invitation.
type CreateInvitationRequest struct {
	OrgID string          `path:"orgID"`
	Email string          `json:"email"`
	Role  models.UserRole `json:"role"`
}

// ListInvitationsRequest is a request to list invitations.
type ListInvitationsRequest struct {
	OrgID string `path:"orgID"`
}

// ListInvitationsResponse is a response containing a list of invitations.
type ListInvitationsResponse struct {
	Invitations []*models.Invitation `json:"invitations"`
}

// AcceptInvitationRequest is a request to accept an invitation.
type AcceptInvitationRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// CreateInvitation creates a new invitation.
func (h *InvitationHandler) CreateInvitation(ctx context.Context, req CreateInvitationRequest) (*models.Invitation, error) {
	if req.Email == "" || req.Role == "" {
		return nil, errors.MissingField("email or role")
	}

	// Permission check (only Admin can invite)
	currentUser, ok := ctx.Value(models.UserKey).(*models.User)
	if !ok || currentUser.Role != models.RoleAdmin {
		return nil, errors.Forbidden("Only admins can create invitations")
	}

	// Check Quota
	org, err := h.orgService.GetOrganization(req.OrgID)
	if err == nil && org.Quotas.MaxUsers > 0 {
		members, _ := h.memService.ListByOrganization(req.OrgID)
		pending, _ := h.invService.ListByOrganization(req.OrgID)
		if len(members)+len(pending) >= org.Quotas.MaxUsers {
			return nil, errors.NewAPIError(403, "quota_exceeded", fmt.Sprintf("User quota exceeded (%d/%d)", len(members)+len(pending), org.Quotas.MaxUsers))
		}
	}

	return h.invService.CreateInvitation(req.Email, req.OrgID, req.Role)
}

// ListInvitations returns all active invitations for the organization.
func (h *InvitationHandler) ListInvitations(ctx context.Context, req ListInvitationsRequest) (*ListInvitationsResponse, error) {
	invitations, err := h.invService.ListByOrganization(req.OrgID)
	if err != nil {
		return nil, errors.InternalWithError("Failed to list invitations", err)
	}
	return &ListInvitationsResponse{Invitations: invitations}, nil
}

// AcceptInvitation creates a new user based on an invitation.
func (h *InvitationHandler) AcceptInvitation(ctx context.Context, req AcceptInvitationRequest) (*models.User, error) {
	if req.Token == "" || req.Password == "" || req.Name == "" {
		return nil, errors.MissingField("token, password, or name")
	}

	inv, err := h.invService.GetInvitationByToken(req.Token)
	if err != nil {
		return nil, errors.NotFound("Invitation")
	}

	if time.Now().After(inv.ExpiresAt) {
		_ = h.invService.DeleteInvitation(inv.ID)
		return nil, errors.NewAPIError(400, "invitation_expired", "Invitation has expired")
	}

	// Create user
	user, err := h.userService.CreateUser(inv.Email, req.Password, req.Name, inv.Role)
	if err != nil {
		return nil, errors.InternalWithError("Failed to create user", err)
	}

	// Set organization
	if err := h.userService.UpdateUserOrg(user.ID, inv.OrganizationID); err != nil {
		return nil, errors.InternalWithError("Failed to set user organization", err)
	}

	// Delete invitation
	_ = h.invService.DeleteInvitation(inv.ID)

	return h.userService.GetUser(user.ID)
}
