package storage

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/models"
)

// InvitationService handles organization invitations.
type InvitationService struct {
	rootDir string
	table   *jsonldb.Table[models.Invitation]
	mu      sync.RWMutex
	byID    map[string]*models.Invitation
	byToken map[string]*models.Invitation
}

// NewInvitationService creates a new invitation service.
func NewInvitationService(rootDir string) (*InvitationService, error) {
	tablePath := filepath.Join(rootDir, "db", "invitations.jsonl")
	table, err := jsonldb.NewTable[models.Invitation](tablePath)
	if err != nil {
		return nil, err
	}

	s := &InvitationService{
		rootDir: rootDir,
		table:   table,
		byID:    make(map[string]*models.Invitation),
		byToken: make(map[string]*models.Invitation),
	}

	for i := range table.Rows {
		inv := &table.Rows[i]
		s.byID[inv.ID] = inv
		s.byToken[inv.Token] = inv
	}

	return s, nil
}

// CreateInvitation creates a new invitation.
func (s *InvitationService) CreateInvitation(email, orgID string, role models.UserRole) (*models.Invitation, error) {
	if email == "" || orgID == "" {
		return nil, fmt.Errorf("email and organization ID are required")
	}

	token, err := GenerateToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ID: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	invitation := &models.Invitation{
		ID:             id,
		Email:          email,
		OrganizationID: orgID,
		Role:           role,
		Token:          token,
		ExpiresAt:      time.Now().Add(7 * 24 * time.Hour), // 7 days
		Created:        time.Now(),
	}

	if err := s.table.Append(*invitation); err != nil {
		return nil, err
	}

	// Update local cache
	s.table.Mu.RLock()
	newInv := &s.table.Rows[len(s.table.Rows)-1]
	s.table.Mu.RUnlock()
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
func (s *InvitationService) DeleteInvitation(id string) error {
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
func (s *InvitationService) ListByOrganization(orgID string) ([]*models.Invitation, error) {
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

func (s *InvitationService) getAllFromCache() []models.Invitation {
	rows := make([]models.Invitation, 0, len(s.byID))
	for _, v := range s.byID {
		rows = append(rows, *v)
	}
	return rows
}
