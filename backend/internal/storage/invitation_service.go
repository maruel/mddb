package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/models"
)

// InvitationService handles organization invitations.
type InvitationService struct {
	rootDir string
	table   *jsonldb.Table[*models.Invitation]
	mu      sync.RWMutex
	byID    map[jsonldb.ID]*models.Invitation
	byToken map[string]*models.Invitation
}

// NewInvitationService creates a new invitation service.
func NewInvitationService(rootDir string) (*InvitationService, error) {
	dbDir := filepath.Join(rootDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	tablePath := filepath.Join(dbDir, "invitations.jsonl")
	table, err := jsonldb.NewTable[*models.Invitation](tablePath)
	if err != nil {
		return nil, err
	}

	s := &InvitationService{
		rootDir: rootDir,
		table:   table,
		byID:    make(map[jsonldb.ID]*models.Invitation),
		byToken: make(map[string]*models.Invitation),
	}

	for inv := range table.Iter(0) {
		s.byID[inv.ID] = inv
		s.byToken[inv.Token] = inv
	}

	return s, nil
}

// CreateInvitation creates a new invitation.
func (s *InvitationService) CreateInvitation(email, orgIDStr string, role models.UserRole) (*models.Invitation, error) {
	if email == "" || orgIDStr == "" {
		return nil, fmt.Errorf("email and organization ID are required")
	}

	orgID, err := jsonldb.DecodeID(orgIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid organization id: %w", err)
	}

	token, err := GenerateToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := jsonldb.NewID()

	invitation := &models.Invitation{
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
func (s *InvitationService) GetInvitationByToken(token string) (*models.Invitation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	inv, ok := s.byToken[token]
	if !ok {
		return nil, fmt.Errorf("invitation not found")
	}

	return inv, nil
}

// DeleteInvitation deletes an invitation.
func (s *InvitationService) DeleteInvitation(idStr string) error {
	id, err := jsonldb.DecodeID(idStr)
	if err != nil {
		return fmt.Errorf("invalid invitation id: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	inv, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("invitation not found")
	}

	delete(s.byToken, inv.Token)
	delete(s.byID, id)

	return s.table.Replace(s.getAllFromCache())
}

// ListByOrganization returns all invitations for an organization.
func (s *InvitationService) ListByOrganization(orgIDStr string) ([]*models.Invitation, error) {
	orgID, err := jsonldb.DecodeID(orgIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid organization id: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var invitations []*models.Invitation
	for _, inv := range s.byID {
		if inv.OrganizationID == orgID {
			invitations = append(invitations, inv)
		}
	}
	return invitations, nil
}

func (s *InvitationService) getAllFromCache() []*models.Invitation {
	rows := make([]*models.Invitation, 0, len(s.byID))
	for _, v := range s.byID {
		rows = append(rows, v)
	}
	return rows
}
