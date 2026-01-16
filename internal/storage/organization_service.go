package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/maruel/mddb/internal/models"
)

// OrganizationService handles organization management.
type OrganizationService struct {
	rootDir    string
	orgsDir    string
	fileStore  *FileStore
	gitService *GitService
}

// NewOrganizationService creates a new organization service.
func NewOrganizationService(rootDir string, fileStore *FileStore, gitService *GitService) (*OrganizationService, error) {
	orgsDir := filepath.Join(rootDir, "db", "organizations")
	if err := os.MkdirAll(orgsDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create organizations directory: %w", err)
	}

	return &OrganizationService{
		rootDir:    rootDir,
		orgsDir:    orgsDir,
		fileStore:  fileStore,
		gitService: gitService,
	}, nil
}

// CreateOrganization creates a new organization.
func (s *OrganizationService) CreateOrganization(ctx context.Context, name string) (*models.Organization, error) {
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
	orgDir := filepath.Join(s.rootDir, id)
	orgPagesDir := filepath.Join(orgDir, "pages")
	if err := os.MkdirAll(orgPagesDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create organization content directory: %w", err)
	}

	// Initialize git repository for the organization
	if s.gitService != nil {
		if err := s.gitService.InitRepository(orgDir); err != nil {
			fmt.Printf("failed to initialize git repo for org %s: %v\n", id, err)
		}
	}

	// Create welcome page
	if s.fileStore != nil {
		welcomeTitle := "Welcome to " + name
		welcomeContent := "# Welcome to mddb\n\nThis is your new workspace. You can create pages, databases, and upload assets here."
		_, _ = s.fileStore.WritePage(id, "1", welcomeTitle, welcomeContent)

		// Commit the welcome page
		if s.gitService != nil {
			// Create a context with the new org ID if not already present
			orgCtx := ctx
			if models.GetOrgID(orgCtx) == "" {
				orgCtx = context.WithValue(ctx, models.UserKey, &models.User{OrganizationID: id})
			}
			_ = s.gitService.CommitChange(orgCtx, "create", "page", "1", "Initial welcome page")
		}
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
