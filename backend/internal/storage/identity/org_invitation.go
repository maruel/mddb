// Manages invitations for users to join organizations.

package identity

import (
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/ksid"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/utils"
)

// OrganizationInvitation represents an invitation for a user to join an organization.
type OrganizationInvitation struct {
	ID             ksid.ID          `json:"id" jsonschema:"description=Unique invitation identifier"`
	OrganizationID ksid.ID          `json:"organization_id" jsonschema:"description=Organization the user is invited to"`
	Email          string           `json:"email" jsonschema:"description=Email address of the invitee"`
	Role           OrganizationRole `json:"role" jsonschema:"description=Role assigned upon acceptance"`
	Token          string           `json:"token" jsonschema:"description=Secret token for invitation verification"`
	InvitedBy      ksid.ID          `json:"invited_by" jsonschema:"description=User ID who created the invitation"`
	ExpiresAt      storage.Time     `json:"expires_at" jsonschema:"description=Invitation expiration timestamp"`
	Created        storage.Time     `json:"created" jsonschema:"description=Invitation creation timestamp"`
}

// Clone returns a copy of the OrganizationInvitation.
func (i *OrganizationInvitation) Clone() *OrganizationInvitation {
	c := *i
	return &c
}

// GetID returns the OrganizationInvitation's ID.
func (i *OrganizationInvitation) GetID() ksid.ID {
	return i.ID
}

// Validate checks that the OrganizationInvitation is valid.
func (i *OrganizationInvitation) Validate() error {
	if i.ID.IsZero() {
		return errIDRequired
	}
	if i.OrganizationID.IsZero() {
		return errOrgIDEmpty
	}
	if i.Email == "" {
		return errEmailEmpty
	}
	if !i.Role.IsValid() {
		return errInvalidOrgRole
	}
	if i.Token == "" {
		return errTokenRequired
	}
	return nil
}

// IsExpired returns true if the invitation has expired.
func (i *OrganizationInvitation) IsExpired() bool {
	return storage.Now().After(i.ExpiresAt)
}

// OrganizationInvitationService handles organization invitations.
type OrganizationInvitationService struct {
	table   *jsonldb.Table[*OrganizationInvitation]
	byToken *jsonldb.UniqueIndex[string, *OrganizationInvitation]
	byOrgID *jsonldb.Index[ksid.ID, *OrganizationInvitation]
}

// NewOrganizationInvitationService creates a new organization invitation service.
func NewOrganizationInvitationService(tablePath string) (*OrganizationInvitationService, error) {
	table, err := jsonldb.NewTable[*OrganizationInvitation](tablePath)
	if err != nil {
		return nil, err
	}
	byToken := jsonldb.NewUniqueIndex(table, func(i *OrganizationInvitation) string { return i.Token })
	byOrgID := jsonldb.NewIndex(table, func(i *OrganizationInvitation) ksid.ID { return i.OrganizationID })
	return &OrganizationInvitationService{table: table, byToken: byToken, byOrgID: byOrgID}, nil
}

// Create creates a new organization invitation.
func (s *OrganizationInvitationService) Create(email string, orgID ksid.ID, role OrganizationRole, invitedBy ksid.ID) (*OrganizationInvitation, error) {
	if email == "" {
		return nil, errEmailEmpty
	}
	if orgID.IsZero() {
		return nil, errOrgIDEmpty
	}
	if !role.IsValid() {
		return nil, errInvalidOrgRole
	}
	token, err := utils.GenerateToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	invitation := &OrganizationInvitation{
		ID:             ksid.NewID(),
		OrganizationID: orgID,
		Email:          email,
		Role:           role,
		Token:          token,
		InvitedBy:      invitedBy,
		ExpiresAt:      storage.ToTime(time.Now().Add(7 * 24 * time.Hour)), // 7 days
		Created:        storage.Now(),
	}
	if err := s.table.Append(invitation); err != nil {
		return nil, err
	}
	return invitation, nil
}

// GetByToken retrieves an invitation by its token. O(1) via index.
func (s *OrganizationInvitationService) GetByToken(token string) (*OrganizationInvitation, error) {
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
func (s *OrganizationInvitationService) IterByOrg(orgID ksid.ID) iter.Seq[*OrganizationInvitation] {
	return s.byOrgID.Iter(orgID)
}

//

var (
	errOrgInvitationNotFound = errors.New("organization invitation not found")
	errOrgInvitationIDEmpty  = errors.New("organization invitation id cannot be empty")
)
