// Manages recipient-bound and bearer organization invitations with durable single-account claims.

package identity

import (
	"errors"
	"fmt"
	"iter"
	"sync"
	"time"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/utils"
)

// OrganizationInvitationService handles organization invitations.
type OrganizationInvitationService struct {
	mu      sync.Mutex
	table   *jsonldb.Table[*identityv1.OrganizationInvitation]
	byToken *jsonldb.UniqueIndex[string, *identityv1.OrganizationInvitation]
	byOrgID *jsonldb.Index[ksid.ID, *identityv1.OrganizationInvitation]
}

// NewOrganizationInvitationService creates a new organization invitation service.
func NewOrganizationInvitationService(tablePath string) (*OrganizationInvitationService, error) {
	table, err := jsonldb.NewTable[*identityv1.OrganizationInvitation](tablePath)
	if err != nil {
		return nil, err
	}
	byToken := jsonldb.NewUniqueIndex(table, func(i *identityv1.OrganizationInvitation) string { return i.Token })
	byOrgID := jsonldb.NewIndex(table, func(i *identityv1.OrganizationInvitation) ksid.ID { return i.OrganizationID })
	return &OrganizationInvitationService{table: table, byToken: byToken, byOrgID: byOrgID}, nil
}

// Create creates a new organization invitation.
func (s *OrganizationInvitationService) Create(email string, orgID ksid.ID, role identityv1.OrganizationRole, invitedBy ksid.ID) (*identityv1.OrganizationInvitation, error) {
	if orgID.IsZero() {
		return nil, identityv1.ErrOrgIDEmpty
	}
	if !role.IsValid() {
		return nil, identityv1.ErrInvalidOrgRole
	}
	token, err := utils.GenerateToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	invitation := &identityv1.OrganizationInvitation{
		ID:             ksid.NewID(),
		OrganizationID: orgID,
		Email:          email,
		Role:           role,
		Token:          token,
		InvitedBy:      invitedBy,
		ExpiresAt:      storagev1.ToTime(time.Now().Add(7 * 24 * time.Hour)), // 7 days
		Created:        storagev1.Now(),
	}
	if err := s.table.Append(invitation, nil); err != nil {
		return nil, err
	}
	return invitation, nil
}

// Get retrieves an invitation by its stable ID for scoped administration.
func (s *OrganizationInvitationService) Get(id ksid.ID) (*identityv1.OrganizationInvitation, error) {
	inv := s.table.Get(id)
	if inv == nil {
		return nil, ErrInvitationNotFound
	}
	return inv, nil
}

// GetByToken retrieves an invitation by its token. O(1) via index.
func (s *OrganizationInvitationService) GetByToken(token string) (*identityv1.OrganizationInvitation, error) {
	inv := s.byToken.Get(token)
	if inv == nil {
		return nil, ErrInvitationNotFound
	}
	return inv, nil
}

// Accept durably binds the invitation before granting membership.
// A failed grant is retryable only by the same account, including after restart.
// Revocation is serialized with the complete acceptance attempt. Completed
// attempts never execute grant again, even when invitation cleanup failed.
func (s *OrganizationInvitationService) Accept(token string, user *identityv1.User, grant func(*identityv1.OrganizationInvitation) error) (*identityv1.OrganizationInvitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, err := s.GetByToken(token)
	if err != nil {
		return nil, err
	}
	if err := validateInvitationClaim(user, inv.Email, inv.ExpiresAt, inv.ClaimedBy); err != nil {
		return nil, err
	}
	if !inv.AcceptedAt.IsZero() {
		return inv, nil
	}
	// Re-persist before every grant attempt. A matching in-memory claimant is
	// not a substitute for a successful save after an earlier write failure.
	inv, err = s.table.Modify(inv.ID, func(row *identityv1.OrganizationInvitation) error { row.ClaimedBy = user.ID; return nil })
	if err != nil {
		return nil, err
	}
	if err := grant(inv); err != nil {
		return nil, err
	}
	inv, err = s.table.Modify(inv.ID, func(row *identityv1.OrganizationInvitation) error { row.AcceptedAt = storagev1.Now(); return nil })
	if err != nil {
		return nil, err
	}
	if _, err := s.table.Delete(inv.ID); err != nil {
		return nil, fmt.Errorf("invitation accepted; cleanup failed: %w", err)
	}
	return inv, nil
}

// Delete deletes an invitation.
func (s *OrganizationInvitationService) Delete(id ksid.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id.IsZero() {
		return errOrgInvitationIDEmpty
	}
	if s.table.Get(id) == nil {
		return ErrInvitationNotFound
	}
	if _, err := s.table.Delete(id); err != nil {
		return err
	}
	return nil
}

// IterByOrg iterates over all invitations for an organization. O(1) via index.
func (s *OrganizationInvitationService) IterByOrg(orgID ksid.ID) iter.Seq[*identityv1.OrganizationInvitation] {
	return s.byOrgID.Iter(orgID)
}

var (
	errOrgInvitationIDEmpty = errors.New("organization invitation id cannot be empty")
)

// validateInvitationClaim keeps recipient proof separate from the link's grant.
func validateInvitationClaim(user *identityv1.User, email string, expiry storagev1.Time, claimedBy ksid.ID) error {
	if user.ID.IsZero() {
		return identityv1.ErrUserIDEmpty
	}
	if storagev1.Now().After(expiry) {
		return ErrInvitationExpired
	}
	if !claimedBy.IsZero() && claimedBy != user.ID {
		return ErrInvitationClaimed
	}
	if email != "" && (!user.EmailVerified || user.Email != email) {
		return ErrInvitationRecipient
	}
	return nil
}

// Invitation acceptance errors distinguish unavailable grants from recipient proof.
var (
	// ErrInvitationNotFound identifies a revoked, consumed, or unknown token.
	ErrInvitationNotFound = errors.New("invitation not found")
	// ErrInvitationExpired identifies a grant past its deadline.
	ErrInvitationExpired = errors.New("invitation expired")
	// ErrInvitationClaimed prevents a token from granting access to a second account.
	ErrInvitationClaimed = errors.New("invitation already claimed by another account")
	// ErrInvitationRecipient requires independent proof of the selected address.
	ErrInvitationRecipient = errors.New("invitation requires its recipient's verified address")
)
