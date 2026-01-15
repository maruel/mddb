package identity

import (
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/utils"
)

// Invitation represents a request for a user to join an organization.
type Invitation struct {
	ID             jsonldb.ID `json:"id" jsonschema:"description=Unique invitation identifier"`
	Email          string     `json:"email" jsonschema:"description=Email address of the invitee"`
	OrganizationID jsonldb.ID `json:"organization_id" jsonschema:"description=Organization the user is invited to"`
	Role           UserRole   `json:"role" jsonschema:"description=Role assigned upon acceptance"`
	Token          string     `json:"token" jsonschema:"description=Secret token for invitation verification"`
	ExpiresAt      time.Time  `json:"expires_at" jsonschema:"description=Invitation expiration timestamp"`
	Created        time.Time  `json:"created" jsonschema:"description=Invitation creation timestamp"`
}

// Clone returns a copy of the Invitation.
func (i *Invitation) Clone() *Invitation {
	c := *i
	return &c
}

// GetID returns the Invitation's ID.
func (i *Invitation) GetID() jsonldb.ID {
	return i.ID
}

// Validate checks that the Invitation is valid.
func (i *Invitation) Validate() error {
	if i.ID.IsZero() {
		return errIDRequired
	}
	if i.Email == "" {
		return errEmailEmpty
	}
	if i.OrganizationID.IsZero() {
		return errOrgIDEmpty
	}
	if i.Token == "" {
		return errTokenRequired
	}
	return nil
}

// InvitationService handles organization invitations.
type InvitationService struct {
	table   *jsonldb.Table[*Invitation]
	byToken *jsonldb.UniqueIndex[string, *Invitation]
	byOrgID *jsonldb.Index[jsonldb.ID, *Invitation]
}

// NewInvitationService creates a new invitation service.
func NewInvitationService(tablePath string) (*InvitationService, error) {
	table, err := jsonldb.NewTable[*Invitation](tablePath)
	if err != nil {
		return nil, err
	}
	byToken := jsonldb.NewUniqueIndex(table, func(i *Invitation) string { return i.Token })
	byOrgID := jsonldb.NewIndex(table, func(i *Invitation) jsonldb.ID { return i.OrganizationID })
	return &InvitationService{table: table, byToken: byToken, byOrgID: byOrgID}, nil
}

// Create creates a new invitation.
func (s *InvitationService) Create(email string, orgID jsonldb.ID, role UserRole) (*Invitation, error) {
	if email == "" {
		return nil, errEmailEmpty
	}
	if orgID.IsZero() {
		return nil, errOrgIDEmpty
	}
	token, err := utils.GenerateToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	invitation := &Invitation{
		ID:             jsonldb.NewID(),
		Email:          email,
		OrganizationID: orgID,
		Role:           role,
		Token:          token,
		ExpiresAt:      time.Now().Add(7 * 24 * time.Hour), // 7 days
		Created:        time.Now(),
	}
	if err := s.table.Append(invitation); err != nil {
		return nil, err
	}
	return invitation, nil
}

// GetByToken retrieves an invitation by its token. O(1) via index.
func (s *InvitationService) GetByToken(token string) (*Invitation, error) {
	inv := s.byToken.Get(token)
	if inv == nil {
		return nil, errInvitationNotFound
	}
	return inv, nil
}

// Delete deletes an invitation.
func (s *InvitationService) Delete(id jsonldb.ID) error {
	if id.IsZero() {
		return errInvitationIDEmpty
	}
	if s.table.Get(id) == nil {
		return errInvitationNotFound
	}
	if _, err := s.table.Delete(id); err != nil {
		return err
	}
	return nil
}

// Iter iterates over all invitations for an organization. O(1) via index.
func (s *InvitationService) Iter(orgID jsonldb.ID) (iter.Seq[*Invitation], error) {
	if orgID.IsZero() {
		return nil, errOrgIDEmpty
	}
	return s.byOrgID.Iter(orgID), nil
}

//

var (
	errInvitationNotFound = errors.New("invitation not found")
	errInvitationIDEmpty  = errors.New("invitation id cannot be empty")
)
