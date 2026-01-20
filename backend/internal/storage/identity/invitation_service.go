package identity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/utils"
)

var (
	errEmailRequired      = errors.New("email is required")
	errOrgIDEmpty         = errors.New("organization id cannot be empty")
	errInvitationNotFound = errors.New("invitation not found")
	errInvitationIDEmpty  = errors.New("invitation id cannot be empty")
)

// InvitationService handles organization invitations.
type InvitationService struct {
	rootDir string
	table   *jsonldb.Table[*entity.Invitation]
	mu      sync.RWMutex
	byID    map[jsonldb.ID]*entity.Invitation
	byToken map[string]*entity.Invitation
}

// NewInvitationService creates a new invitation service.
func NewInvitationService(rootDir string) (*InvitationService, error) {
	dbDir := filepath.Join(rootDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	tablePath := filepath.Join(dbDir, "invitations.jsonl")
	table, err := jsonldb.NewTable[*entity.Invitation](tablePath)
	if err != nil {
		return nil, err
	}

	s := &InvitationService{
		rootDir: rootDir,
		table:   table,
		byID:    make(map[jsonldb.ID]*entity.Invitation),
		byToken: make(map[string]*entity.Invitation),
	}

	for inv := range table.Iter(0) {
		s.byID[inv.ID] = inv
		s.byToken[inv.Token] = inv
	}

	return s, nil
}

// CreateInvitation creates a new invitation.
func (s *InvitationService) CreateInvitation(email string, orgID jsonldb.ID, role entity.UserRole) (*entity.Invitation, error) {
	if email == "" {
		return nil, errEmailRequired
	}
	if orgID.IsZero() {
		return nil, errOrgIDEmpty
	}

	token, err := utils.GenerateToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := jsonldb.NewID()

	invitation := &entity.Invitation{
		ID:             id,
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

	// Update local cache
	newInv := s.table.Last()
	s.byID[id] = newInv
	s.byToken[token] = newInv

	return invitation, nil
}

// GetInvitationByToken retrieves an invitation by its token.
func (s *InvitationService) GetInvitationByToken(token string) (*entity.Invitation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	inv, ok := s.byToken[token]
	if !ok {
		return nil, errInvitationNotFound
	}

	return inv, nil
}

// DeleteInvitation deletes an invitation.
func (s *InvitationService) DeleteInvitation(id jsonldb.ID) error {
	if id.IsZero() {
		return errInvitationIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	inv, ok := s.byID[id]
	if !ok {
		return errInvitationNotFound
	}

	if _, err := s.table.Delete(id); err != nil {
		return err
	}

	delete(s.byToken, inv.Token)
	delete(s.byID, id)
	return nil
}

// ListByOrganization returns all invitations for an organization.
func (s *InvitationService) ListByOrganization(orgID jsonldb.ID) ([]*entity.Invitation, error) {
	if orgID.IsZero() {
		return nil, errOrgIDEmpty
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var invitations []*entity.Invitation
	for _, inv := range s.byID {
		if inv.OrganizationID == orgID {
			invitations = append(invitations, inv)
		}
	}
	return invitations, nil
}
