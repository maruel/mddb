// Manages invitations for users to join organizations.

package identity

import (
	"errors"
	"fmt"
	"iter"
	"time"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/utils"
)

// OrganizationInvitationService handles organization invitations.
type OrganizationInvitationService struct {
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
	if email == "" {
		return nil, identityv1.ErrEmailEmpty
	}
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
	if err := s.table.Append(invitation); err != nil {
		return nil, err
	}
	return invitation, nil
}

// GetByToken retrieves an invitation by its token. O(1) via index.
func (s *OrganizationInvitationService) GetByToken(token string) (*identityv1.OrganizationInvitation, error) {
	inv := s.byToken.Get(token)
	if inv == nil {
		return nil, errOrgInvitationNotFound
	}
	return inv, nil
}

// Delete deletes an invitation.
func (s *OrganizationInvitationService) Delete(id ksid.ID) error {
	if id.IsZero() {
		return errOrgInvitationIDEmpty
	}
	if s.table.Get(id) == nil {
		return errOrgInvitationNotFound
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

//

var (
	errOrgInvitationNotFound = errors.New("organization invitation not found")
	errOrgInvitationIDEmpty  = errors.New("organization invitation id cannot be empty")
)
