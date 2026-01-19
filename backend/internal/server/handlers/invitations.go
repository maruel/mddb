package handlers

import (
	"context"
	"time"

	"github.com/maruel/mddb/backend/internal/models"
	"github.com/maruel/mddb/backend/internal/storage"
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

// CreateInvitation creates a new invitation and sends it (logic to be added).
func (h *InvitationHandler) CreateInvitation(ctx context.Context, req models.CreateInvitationRequest) (*models.Invitation, error) {
	if req.Email == "" || req.Role == "" {
		return nil, models.MissingField("email or role")
	}

	orgID := models.GetOrgID(ctx)
	invitation, err := h.invService.CreateInvitation(req.Email, orgID, req.Role)
	if err != nil {
		return nil, models.InternalWithError("Failed to create invitation", err)
	}

	return invitation, nil
}

// ListInvitations returns all pending invitations for an organization.
func (h *InvitationHandler) ListInvitations(ctx context.Context, req models.ListInvitationsRequest) (*models.ListInvitationsResponse, error) {
	orgID := models.GetOrgID(ctx)
	invitations, err := h.invService.ListByOrganization(orgID)
	if err != nil {
		return nil, models.InternalWithError("Failed to list invitations", err)
	}

	return &models.ListInvitationsResponse{Invitations: invitations}, nil
}

// AcceptInvitation handles a user accepting an invitation.
func (h *InvitationHandler) AcceptInvitation(ctx context.Context, req models.AcceptInvitationRequest) (*models.LoginResponse, error) {
	if req.Token == "" {
		return nil, models.MissingField("token")
	}

	inv, err := h.invService.GetInvitationByToken(req.Token)
	if err != nil {
		return nil, models.NewAPIError(404, models.ErrorCodeNotFound, "Invitation not found or expired")
	}

	if time.Now().After(inv.ExpiresAt) {
		return nil, models.Expired("invitation")
	}

	// Create user or link to existing
	user, err := h.userService.GetUserByEmail(inv.Email)
	if err != nil {
		// Create new user if they don't exist
		if req.Password == "" || req.Name == "" {
			return nil, models.MissingField("password and name required for new account")
		}
		user, err = h.userService.CreateUser(inv.Email, req.Password, req.Name, inv.Role)
		if err != nil {
			return nil, models.InternalWithError("Failed to create user", err)
		}
	}

	// Create membership
	_, err = h.memService.CreateMembership(user.ID, inv.OrganizationID, inv.Role)
	if err != nil {
		return nil, models.InternalWithError("Failed to create membership", err)
	}

	// Delete invitation
	_ = h.invService.DeleteInvitation(inv.ID)

	// Return a new token for the user
	// Note: We need the AuthHandler to generate the token, or move the logic.
	// For now, returning a basic response.
	return &models.LoginResponse{
		User: user,
	}, nil
}
