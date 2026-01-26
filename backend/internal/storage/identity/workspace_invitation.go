// Manages invitations for users to join workspaces.

package identity

import (
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/utils"
)

// WorkspaceInvitation represents an invitation for a user to join a workspace.
type WorkspaceInvitation struct {
	ID          jsonldb.ID    `json:"id" jsonschema:"description=Unique invitation identifier"`
	WorkspaceID jsonldb.ID    `json:"workspace_id" jsonschema:"description=Workspace the user is invited to"`
	Email       string        `json:"email" jsonschema:"description=Email address of the invitee"`
	Role        WorkspaceRole `json:"role" jsonschema:"description=Role assigned upon acceptance"`
	Token       string        `json:"token" jsonschema:"description=Secret token for invitation verification"`
	InvitedBy   jsonldb.ID    `json:"invited_by" jsonschema:"description=User ID who created the invitation"`
	ExpiresAt   storage.Time  `json:"expires_at" jsonschema:"description=Invitation expiration timestamp"`
	Created     storage.Time  `json:"created" jsonschema:"description=Invitation creation timestamp"`
}

// Clone returns a copy of the WorkspaceInvitation.
func (i *WorkspaceInvitation) Clone() *WorkspaceInvitation {
	c := *i
	return &c
}

// GetID returns the WorkspaceInvitation's ID.
func (i *WorkspaceInvitation) GetID() jsonldb.ID {
	return i.ID
}

// Validate checks that the WorkspaceInvitation is valid.
func (i *WorkspaceInvitation) Validate() error {
	if i.ID.IsZero() {
		return errIDRequired
	}
	if i.WorkspaceID.IsZero() {
		return errWSIDEmpty
	}
	if i.Email == "" {
		return errEmailEmpty
	}
	if !i.Role.IsValid() {
		return errInvalidWSRole
	}
	if i.Token == "" {
		return errTokenRequired
	}
	return nil
}

// IsExpired returns true if the invitation has expired.
func (i *WorkspaceInvitation) IsExpired() bool {
	return storage.Now().After(i.ExpiresAt)
}

// WorkspaceInvitationService handles workspace invitations.
type WorkspaceInvitationService struct {
	table   *jsonldb.Table[*WorkspaceInvitation]
	byToken *jsonldb.UniqueIndex[string, *WorkspaceInvitation]
	byWSID  *jsonldb.Index[jsonldb.ID, *WorkspaceInvitation]
}

// NewWorkspaceInvitationService creates a new workspace invitation service.
func NewWorkspaceInvitationService(tablePath string) (*WorkspaceInvitationService, error) {
	table, err := jsonldb.NewTable[*WorkspaceInvitation](tablePath)
	if err != nil {
		return nil, err
	}
	byToken := jsonldb.NewUniqueIndex(table, func(i *WorkspaceInvitation) string { return i.Token })
	byWSID := jsonldb.NewIndex(table, func(i *WorkspaceInvitation) jsonldb.ID { return i.WorkspaceID })
	return &WorkspaceInvitationService{table: table, byToken: byToken, byWSID: byWSID}, nil
}

// Create creates a new workspace invitation.
func (s *WorkspaceInvitationService) Create(email string, wsID jsonldb.ID, role WorkspaceRole, invitedBy jsonldb.ID) (*WorkspaceInvitation, error) {
	if email == "" {
		return nil, errEmailEmpty
	}
	if wsID.IsZero() {
		return nil, errWSIDEmpty
	}
	if !role.IsValid() {
		return nil, errInvalidWSRole
	}
	token, err := utils.GenerateToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	invitation := &WorkspaceInvitation{
		ID:          jsonldb.NewID(),
		WorkspaceID: wsID,
		Email:       email,
		Role:        role,
		Token:       token,
		InvitedBy:   invitedBy,
		ExpiresAt:   storage.ToTime(time.Now().Add(7 * 24 * time.Hour)), // 7 days
		Created:     storage.Now(),
	}
	if err := s.table.Append(invitation); err != nil {
		return nil, err
	}
	return invitation, nil
}

// GetByToken retrieves an invitation by its token. O(1) via index.
func (s *WorkspaceInvitationService) GetByToken(token string) (*WorkspaceInvitation, error) {
	inv := s.byToken.Get(token)
	if inv == nil {
		return nil, errWSInvitationNotFound
	}
	return inv, nil
}

// Delete deletes an invitation.
func (s *WorkspaceInvitationService) Delete(id jsonldb.ID) error {
	if id.IsZero() {
		return errWSInvitationIDEmpty
	}
	if s.table.Get(id) == nil {
		return errWSInvitationNotFound
	}
	if _, err := s.table.Delete(id); err != nil {
		return err
	}
	return nil
}

// IterByWorkspace iterates over all invitations for a workspace. O(1) via index.
func (s *WorkspaceInvitationService) IterByWorkspace(wsID jsonldb.ID) iter.Seq[*WorkspaceInvitation] {
	return s.byWSID.Iter(wsID)
}

// DeleteAllByWorkspace removes all invitations for a workspace.
func (s *WorkspaceInvitationService) DeleteAllByWorkspace(wsID jsonldb.ID) error {
	var toDelete []jsonldb.ID //nolint:prealloc // Iterator length unknown
	for inv := range s.byWSID.Iter(wsID) {
		toDelete = append(toDelete, inv.ID)
	}
	for _, id := range toDelete {
		if _, err := s.table.Delete(id); err != nil {
			return err
		}
	}
	return nil
}

//

var (
	errWSInvitationNotFound = errors.New("workspace invitation not found")
	errWSInvitationIDEmpty  = errors.New("workspace invitation id cannot be empty")
)
