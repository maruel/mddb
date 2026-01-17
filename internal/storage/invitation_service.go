package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/maruel/mddb/internal/models"
)

// InvitationService handles organization invitations.
type InvitationService struct {
	rootDir        string
	invitationsDir string
}

// NewInvitationService creates a new invitation service.
func NewInvitationService(rootDir string) (*InvitationService, error) {
	invitationsDir := filepath.Join(rootDir, "db", "invitations")
	if err := os.MkdirAll(invitationsDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create invitations directory: %w", err)
	}

	return &InvitationService{
		rootDir:        rootDir,
		invitationsDir: invitationsDir,
	}, nil
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

	invitation := &models.Invitation{
		ID:             id,
		Email:          email,
		OrganizationID: orgID,
		Role:           role,
		Token:          token,
		ExpiresAt:      time.Now().Add(7 * 24 * time.Hour), // 7 days
		Created:        time.Now(),
	}

	if err := s.saveInvitation(invitation); err != nil {
		return nil, err
	}

	return invitation, nil
}

// GetInvitationByToken retrieves an invitation by its token.
func (s *InvitationService) GetInvitationByToken(token string) (*models.Invitation, error) {
	entries, err := os.ReadDir(s.invitationsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		inv, err := s.getInvitationByID(entry.Name()[:len(entry.Name())-5])
		if err == nil && inv.Token == token {
			return inv, nil
		}
	}

	return nil, fmt.Errorf("invitation not found")
}

// DeleteInvitation deletes an invitation.
func (s *InvitationService) DeleteInvitation(id string) error {
	filePath := filepath.Join(s.invitationsDir, id+".json")
	return os.Remove(filePath)
}

func (s *InvitationService) getInvitationByID(id string) (*models.Invitation, error) {
	filePath := filepath.Join(s.invitationsDir, id+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var inv models.Invitation
	if err := json.Unmarshal(data, &inv); err != nil {
		return nil, err
	}

	return &inv, nil
}

func (s *InvitationService) saveInvitation(inv *models.Invitation) error {
	data, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return err
	}

	filePath := filepath.Join(s.invitationsDir, inv.ID+".json")
	return os.WriteFile(filePath, data, 0o600)
}

// ListByOrganization returns all invitations for an organization.
func (s *InvitationService) ListByOrganization(orgID string) ([]*models.Invitation, error) {
	entries, err := os.ReadDir(s.invitationsDir)
	if err != nil {
		return nil, err
	}

	var invitations []*models.Invitation
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		inv, err := s.getInvitationByID(entry.Name()[:len(entry.Name())-5])
		if err == nil && inv.OrganizationID == orgID {
			invitations = append(invitations, inv)
		}
	}
	return invitations, nil
}
