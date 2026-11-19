// Manages invitations for users to join workspaces.

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

// WorkspaceInvitationService handles workspace invitations.
type WorkspaceInvitationService struct {
	table   *jsonldb.Table[*identityv1.WorkspaceInvitation]
	byToken *jsonldb.UniqueIndex[string, *identityv1.WorkspaceInvitation]
	byWSID  *jsonldb.Index[ksid.ID, *identityv1.WorkspaceInvitation]
}

// NewWorkspaceInvitationService creates a new workspace invitation service.
func NewWorkspaceInvitationService(tablePath string) (*WorkspaceInvitationService, error) {
	table, err := jsonldb.NewTable[*identityv1.WorkspaceInvitation](tablePath)
	if err != nil {
		return nil, err
	}
	byToken := jsonldb.NewUniqueIndex(table, func(i *identityv1.WorkspaceInvitation) string { return i.Token })
	byWSID := jsonldb.NewIndex(table, func(i *identityv1.WorkspaceInvitation) ksid.ID { return i.WorkspaceID })
	return &WorkspaceInvitationService{table: table, byToken: byToken, byWSID: byWSID}, nil
}

// Create creates a new workspace invitation.
func (s *WorkspaceInvitationService) Create(email string, wsID ksid.ID, role identityv1.WorkspaceRole, invitedBy ksid.ID) (*identityv1.WorkspaceInvitation, error) {
	if email == "" {
		return nil, identityv1.ErrEmailEmpty
	}
	if wsID.IsZero() {
		return nil, identityv1.ErrWSIDEmpty
	}
	if !role.IsValid() {
		return nil, identityv1.ErrInvalidWSRole
	}
	token, err := utils.GenerateToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	invitation := &identityv1.WorkspaceInvitation{
		ID:          ksid.NewID(),
		WorkspaceID: wsID,
		Email:       email,
		Role:        role,
		Token:       token,
		InvitedBy:   invitedBy,
		ExpiresAt:   storagev1.ToTime(time.Now().Add(7 * 24 * time.Hour)), // 7 days
		Created:     storagev1.Now(),
	}
	if err := s.table.Append(invitation); err != nil {
		return nil, err
	}
	return invitation, nil
}

// GetByToken retrieves an invitation by its token. O(1) via index.
func (s *WorkspaceInvitationService) GetByToken(token string) (*identityv1.WorkspaceInvitation, error) {
	inv := s.byToken.Get(token)
	if inv == nil {
		return nil, errWSInvitationNotFound
	}
	return inv, nil
}

// Delete deletes an invitation.
func (s *WorkspaceInvitationService) Delete(id ksid.ID) error {
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
func (s *WorkspaceInvitationService) IterByWorkspace(wsID ksid.ID) iter.Seq[*identityv1.WorkspaceInvitation] {
	return s.byWSID.Iter(wsID)
}

// DeleteAllByWorkspace removes all invitations for a workspace.
func (s *WorkspaceInvitationService) DeleteAllByWorkspace(wsID ksid.ID) error {
	var toDelete []ksid.ID
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
