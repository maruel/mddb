// Persisted organization invitation rows.

package v1

import (
	"github.com/maruel/ksid"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// OrganizationInvitation represents an invitation for a user to join an organization.
type OrganizationInvitation struct {
	ID             ksid.ID          `json:"id" jsonschema:"description=Unique invitation identifier"`
	OrganizationID ksid.ID          `json:"organization_id" jsonschema:"description=Organization the user is invited to"`
	Email          string           `json:"email" jsonschema:"description=Email address of the invitee"`
	Role           OrganizationRole `json:"role" jsonschema:"description=Role assigned upon acceptance"`
	Token          string           `json:"token" jsonschema:"description=Secret token for invitation verification"`
	InvitedBy      ksid.ID          `json:"invited_by" jsonschema:"description=User ID who created the invitation"`
	ExpiresAt      storagev1.Time   `json:"expires_at" jsonschema:"description=Invitation expiration timestamp"`
	Created        storagev1.Time   `json:"created" jsonschema:"description=Invitation creation timestamp"`
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
		return ErrIDRequired
	}
	if i.OrganizationID.IsZero() {
		return ErrOrgIDEmpty
	}
	if i.Email == "" {
		return ErrEmailEmpty
	}
	if !i.Role.IsValid() {
		return ErrInvalidOrgRole
	}
	if i.Token == "" {
		return ErrTokenRequired
	}
	return nil
}

// IsExpired returns true if the invitation has expired.
func (i *OrganizationInvitation) IsExpired() bool {
	return storagev1.Now().After(i.ExpiresAt)
}
