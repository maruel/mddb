package identity

import (
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/utils"
)

var (
	errInvitationNotFound = errors.New("invitation not found")
	errInvitationIDEmpty  = errors.New("invitation id cannot be empty")
)

// InvitationService handles organization invitations.
type InvitationService struct {
	table *jsonldb.Table[*Invitation]
}

// NewInvitationService creates a new invitation service.
func NewInvitationService(rootDir string) (*InvitationService, error) {
	dbDir := filepath.Join(rootDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	tablePath := filepath.Join(dbDir, "invitations.jsonl")
	table, err := jsonldb.NewTable[*Invitation](tablePath)
	if err != nil {
		return nil, err
	}

	return &InvitationService{table: table}, nil
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

// GetByToken retrieves an invitation by its token.
func (s *InvitationService) GetByToken(token string) (*Invitation, error) {
	for inv := range s.table.Iter(0) {
		if inv.Token == token {
			return inv, nil
		}
	}
	return nil, errInvitationNotFound
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

// Iter iterates over all invitations for an organization.
func (s *InvitationService) Iter(orgID jsonldb.ID) (iter.Seq[*Invitation], error) {
	if orgID.IsZero() {
		return nil, errOrgIDEmpty
	}
	return func(yield func(*Invitation) bool) {
		for inv := range s.table.Iter(0) {
			if inv.OrganizationID == orgID && !yield(inv) {
				return
			}
		}
	}, nil
}
