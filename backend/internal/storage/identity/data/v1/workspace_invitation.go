// Persisted workspace invitation rows.

package v1

import (
	"github.com/maruel/ksid"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// WorkspaceInvitation represents an invitation for a user to join a workspace.
type WorkspaceInvitation struct {
	ID          ksid.ID        `json:"id" jsonschema:"description=Unique invitation identifier"`
	WorkspaceID ksid.ID        `json:"workspace_id" jsonschema:"description=Workspace the user is invited to"`
	Email       string         `json:"email" jsonschema:"description=Email address of the invitee"`
	Role        WorkspaceRole  `json:"role" jsonschema:"description=Role assigned upon acceptance"`
	Token       string         `json:"token" jsonschema:"description=Secret token for invitation verification"`
	InvitedBy   ksid.ID        `json:"invited_by" jsonschema:"description=User ID who created the invitation"`
	ExpiresAt   storagev1.Time `json:"expires_at" jsonschema:"description=Invitation expiration timestamp"`
	Created     storagev1.Time `json:"created" jsonschema:"description=Invitation creation timestamp"`
}

// Clone returns a copy of the WorkspaceInvitation.
func (i *WorkspaceInvitation) Clone() *WorkspaceInvitation {
	c := *i
	return &c
}

// GetID returns the WorkspaceInvitation's ID.
func (i *WorkspaceInvitation) GetID() ksid.ID {
	return i.ID
}

// Validate checks that the WorkspaceInvitation is valid.
func (i *WorkspaceInvitation) Validate() error {
	if i.ID.IsZero() {
		return ErrIDRequired
	}
	if i.WorkspaceID.IsZero() {
		return ErrWSIDEmpty
	}
	if i.Email == "" {
		return ErrEmailEmpty
	}
	if !i.Role.IsValid() {
		return ErrInvalidWSRole
	}
	if i.Token == "" {
		return ErrTokenRequired
	}
	return nil
}

// IsExpired returns true if the invitation has expired.
func (i *WorkspaceInvitation) IsExpired() bool {
	return storagev1.Now().After(i.ExpiresAt)
}
