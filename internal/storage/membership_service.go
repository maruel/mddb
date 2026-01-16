package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/maruel/mddb/internal/models"
)

// MembershipService handles user-organization relationships.
type MembershipService struct {
	rootDir        string
	membershipsDir string
}

// NewMembershipService creates a new membership service.
func NewMembershipService(rootDir string) (*MembershipService, error) {
	membershipsDir := filepath.Join(rootDir, "db", "memberships")
	if err := os.MkdirAll(membershipsDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create memberships directory: %w", err)
	}

	return &MembershipService{
		rootDir:        rootDir,
		membershipsDir: membershipsDir,
	}, nil
}

// CreateMembership adds a user to an organization.
func (s *MembershipService) CreateMembership(userID, orgID string, role models.UserRole) (*models.Membership, error) {
	membership := &models.Membership{
		UserID:         userID,
		OrganizationID: orgID,
		Role:           role,
		Created:        time.Now(),
	}

	if err := s.saveMembership(membership); err != nil {
		return nil, err
	}

	return membership, nil
}

// GetMembership retrieves a specific user-org relationship.
func (s *MembershipService) GetMembership(userID, orgID string) (*models.Membership, error) {
	filePath := filepath.Join(s.membershipsDir, fmt.Sprintf("%s_%s.json", userID, orgID))
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("membership not found")
	}

	var m models.Membership
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	return &m, nil
}

// ListByUser returns all organizations a user belongs to.
func (s *MembershipService) ListByUser(userID string) ([]models.Membership, error) {
	entries, err := os.ReadDir(s.membershipsDir)
	if err != nil {
		return nil, err
	}

	var memberships []models.Membership
	prefix := userID + "_"
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" && len(entry.Name()) > len(prefix) && entry.Name()[:len(prefix)] == prefix {
			data, err := os.ReadFile(filepath.Join(s.membershipsDir, entry.Name()))
			if err == nil {
				var m models.Membership
				if err := json.Unmarshal(data, &m); err == nil {
					memberships = append(memberships, m)
				}
			}
		}
	}
	return memberships, nil
}

// ListByOrganization returns all users in an organization.
func (s *MembershipService) ListByOrganization(orgID string) ([]models.Membership, error) {
	entries, err := os.ReadDir(s.membershipsDir)
	if err != nil {
		return nil, err
	}

	var memberships []models.Membership
	suffix := "_" + orgID + ".json"
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" && len(entry.Name()) > len(suffix) && entry.Name()[len(entry.Name())-len(suffix):] == suffix {
			data, err := os.ReadFile(filepath.Join(s.membershipsDir, entry.Name()))
			if err == nil {
				var m models.Membership
				if err := json.Unmarshal(data, &m); err == nil {
					memberships = append(memberships, m)
				}
			}
		}
	}
	return memberships, nil
}

// UpdateRole updates a user's role in an organization.
func (s *MembershipService) UpdateRole(userID, orgID string, role models.UserRole) error {
	m, err := s.GetMembership(userID, orgID)
	if err != nil {
		return err
	}

	m.Role = role
	return s.saveMembership(m)
}

// DeleteMembership removes a user from an organization.
func (s *MembershipService) DeleteMembership(userID, orgID string) error {
	filePath := filepath.Join(s.membershipsDir, fmt.Sprintf("%s_%s.json", userID, orgID))
	return os.Remove(filePath)
}

func (s *MembershipService) saveMembership(m *models.Membership) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	filePath := filepath.Join(s.membershipsDir, fmt.Sprintf("%s_%s.json", m.UserID, m.OrganizationID))
	return os.WriteFile(filePath, data, 0o600)
}
