package handlers

import (
	"context"
	"time"

	"github.com/maruel/mddb/backend/internal/dto"
	"github.com/maruel/mddb/backend/internal/entity"
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
func (h *InvitationHandler) CreateInvitation(ctx context.Context, req dto.CreateInvitationRequest) (*dto.InvitationResponse, error) {
	if req.Email == "" || req.Role == "" {
		return nil, dto.MissingField("email or role")
	}

	orgID := entity.GetOrgID(ctx)
	invitation, err := h.invService.CreateInvitation(req.Email, orgID.String(), userRoleToEntity(req.Role))
	if err != nil {
		return nil, dto.InternalWithError("Failed to create invitation", err)
	}

	return invitationToResponse(invitation), nil
}

// ListInvitations returns all pending invitations for an organization.
func (h *InvitationHandler) ListInvitations(ctx context.Context, req dto.ListInvitationsRequest) (*dto.ListInvitationsResponse, error) {
	orgID := entity.GetOrgID(ctx)
	invitations, err := h.invService.ListByOrganization(orgID.String())
	if err != nil {
		return nil, dto.InternalWithError("Failed to list invitations", err)
	}

	// Convert to response types (excludes Token)
	responses := make([]dto.InvitationResponse, 0, len(invitations))
	for _, inv := range invitations {
		responses = append(responses, *invitationToResponse(inv))
	}

	return &dto.ListInvitationsResponse{Invitations: responses}, nil
}

// AcceptInvitation handles a user accepting an invitation.
func (h *InvitationHandler) AcceptInvitation(ctx context.Context, req dto.AcceptInvitationRequest) (*dto.LoginResponse, error) {
	if req.Token == "" {
		return nil, dto.MissingField("token")
	}

	inv, err := h.invService.GetInvitationByToken(req.Token)
	if err != nil {
		return nil, dto.NewAPIError(404, dto.ErrorCodeNotFound, "Invitation not found or expired")
	}

	if time.Now().After(inv.ExpiresAt) {
		return nil, dto.Expired("invitation")
	}

	// Create user or link to existing
	user, err := h.userService.GetUserByEmail(inv.Email)
	if err != nil {
		// Create new user if they don't exist
		if req.Password == "" || req.Name == "" {
			return nil, dto.MissingField("password and name required for new account")
		}
		user, err = h.userService.CreateUser(inv.Email, req.Password, req.Name, inv.Role)
		if err != nil {
			return nil, dto.InternalWithError("Failed to create user", err)
		}
	}

	// Create membership
	_, err = h.memService.CreateMembership(user.ID.String(), inv.OrganizationID.String(), inv.Role)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create membership", err)
	}

	// Delete invitation
	_ = h.invService.DeleteInvitation(inv.ID.String())

	// Build user response
	uwm, err := h.userService.GetUserWithMemberships(user.ID.String())
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Return response (Note: Token generation requires AuthHandler)
	return &dto.LoginResponse{
		User: userResp,
	}, nil
}
