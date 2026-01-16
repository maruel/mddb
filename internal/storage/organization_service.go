package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/maruel/mddb/internal/models"
)

// OrganizationService handles organization management.
type OrganizationService struct {
	rootDir string
	orgsDir string
}

// NewOrganizationService creates a new organization service.
func NewOrganizationService(rootDir string) (*OrganizationService, error) {
	orgsDir := filepath.Join(rootDir, "organizations")
	if err := os.MkdirAll(orgsDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create organizations directory: %w", err)
	}

	return &OrganizationService{
		rootDir: rootDir,
		orgsDir: orgsDir,
	}, nil
}

// CreateOrganization creates a new organization.
func (s *OrganizationService) CreateOrganization(name string) (*models.Organization, error) {
	if name == "" {
		return nil, fmt.Errorf("organization name is required")
	}

	id := generateShortID()
	now := time.Now()
	org := &models.Organization{
		ID:      id,
		Name:    name,
		Created: now,
	}

	if err := s.saveOrganization(org); err != nil {
		return nil, err
	}

	// Create organization content directory
	orgContentDir := filepath.Join(s.rootDir, "orgs", id, "pages")
	if err := os.MkdirAll(orgContentDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create organization content directory: %w", err)
	}

	return org, nil
}

// GetOrganization retrieves an organization by ID.
func (s *OrganizationService) GetOrganization(id string) (*models.Organization, error) {
	filePath := filepath.Join(s.orgsDir, id+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("organization not found")
	}

	var org models.Organization
	if err := json.Unmarshal(data, &org); err != nil {
		return nil, fmt.Errorf("failed to parse organization: %w", err)
	}

	return &org, nil
}

// ListOrganizations returns all organizations.
func (s *OrganizationService) ListOrganizations() ([]*models.Organization, error) {
	entries, err := os.ReadDir(s.orgsDir)
	if err != nil {
		return nil, err
	}

	var orgs []*models.Organization
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		org, err := s.GetOrganization(entry.Name()[:len(entry.Name())-5])
		if err == nil {
			orgs = append(orgs, org)
		}
	}
	return orgs, nil
}

func (s *OrganizationService) saveOrganization(org *models.Organization) error {
	data, err := json.MarshalIndent(org, "", "  ")
	if err != nil {
		return err
	}

	filePath := filepath.Join(s.orgsDir, org.ID+".json")
	return os.WriteFile(filePath, data, 0o644)
}
