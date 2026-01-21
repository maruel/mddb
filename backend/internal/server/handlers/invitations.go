package handlers

import (
	"context"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// InvitationHandler handles invitation requests.
type InvitationHandler struct {
	invService  *identity.InvitationService
	userService *identity.UserService
	orgService  *identity.OrganizationService
	memService  *identity.MembershipService
}

// NewInvitationHandler creates a new invitation handler.
func NewInvitationHandler(invService *identity.InvitationService, userService *identity.UserService, orgService *identity.OrganizationService, memService *identity.MembershipService) *InvitationHandler {
	return &InvitationHandler{
		invService:  invService,
		userService: userService,
		orgService:  orgService,
		memService:  memService,
	}
}

// CreateInvitation creates a new invitation and sends it (logic to be added).
func (h *InvitationHandler) CreateInvitation(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.CreateInvitationRequest) (*dto.InvitationResponse, error) {
	if req.Email == "" || req.Role == "" {
		return nil, dto.MissingField("email or role")
	}
	invitation, err := h.invService.Create(req.Email, orgID, userRoleToEntity(req.Role))
	if err != nil {
		return nil, dto.InternalWithError("Failed to create invitation", err)
	}
	return invitationToResponse(invitation), nil
}

// ListInvitations returns all pending invitations for an organization.
func (h *InvitationHandler) ListInvitations(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.ListInvitationsRequest) (*dto.ListInvitationsResponse, error) {
	invitations, err := h.invService.Iter(orgID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list invitations", err)
	}
	var responses []dto.InvitationResponse
	for inv := range invitations {
		responses = append(responses, *invitationToResponse(inv))
	}
	return &dto.ListInvitationsResponse{Invitations: responses}, nil
}

// AcceptInvitation handles a user accepting an invitation.
// This is a public endpoint (no auth required).
func (h *InvitationHandler) AcceptInvitation(ctx context.Context, req dto.AcceptInvitationRequest) (*dto.LoginResponse, error) {
	if req.Token == "" {
		return nil, dto.MissingField("token")
	}

	inv, err := h.invService.GetByToken(req.Token)
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

	// Create membership
	if _, err = h.memService.Create(user.ID, inv.OrganizationID, inv.Role); err != nil {
		return nil, dto.InternalWithError("Failed to create membership", err)
	}

	// Delete invitation
	_ = h.invService.Delete(inv.ID)

	// Build user response
	uwm, err := getUserWithMemberships(h.userService, h.memService, h.orgService, user.ID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get user response", err)
	}
	userResp := userWithMembershipsToResponse(uwm)

	// Return response (Note: Token generation requires AuthHandler)
	return &dto.LoginResponse{
		User: userResp,
	}, nil
}
