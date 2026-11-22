// Manages recipient-bound and bearer workspace invitations with durable single-account claims.

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

// WorkspaceInvitationService handles workspace invitations.
type WorkspaceInvitationService struct {
	mu      sync.Mutex
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

// Get retrieves an invitation by its stable ID for scoped administration.
func (s *WorkspaceInvitationService) Get(id ksid.ID) (*identityv1.WorkspaceInvitation, error) {
	inv := s.table.Get(id)
	if inv == nil {
		return nil, ErrInvitationNotFound
	}
	return inv, nil
}

// GetByToken retrieves an invitation by its token. O(1) via index.
func (s *WorkspaceInvitationService) GetByToken(token string) (*identityv1.WorkspaceInvitation, error) {
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
func (s *WorkspaceInvitationService) Accept(token string, user *identityv1.User, grant func(*identityv1.WorkspaceInvitation) error) (*identityv1.WorkspaceInvitation, error) {
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
	inv, err = s.table.Modify(inv.ID, func(row *identityv1.WorkspaceInvitation) error { row.ClaimedBy = user.ID; return nil })
	if err != nil {
		return nil, err
	}
	if err := grant(inv); err != nil {
		return nil, err
	}
	inv, err = s.table.Modify(inv.ID, func(row *identityv1.WorkspaceInvitation) error { row.AcceptedAt = storagev1.Now(); return nil })
	if err != nil {
		return nil, err
	}
	if _, err := s.table.Delete(inv.ID); err != nil {
		return nil, fmt.Errorf("invitation accepted; cleanup failed: %w", err)
	}
	return inv, nil
}

// Delete deletes an invitation.
func (s *WorkspaceInvitationService) Delete(id ksid.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id.IsZero() {
		return errWSInvitationIDEmpty
	}
	if s.table.Get(id) == nil {
		return ErrInvitationNotFound
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
	s.mu.Lock()
	defer s.mu.Unlock()
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

var (
	errWSInvitationIDEmpty = errors.New("workspace invitation id cannot be empty")
)
