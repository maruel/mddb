package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/models"
)

// OrganizationService handles organization management.
type OrganizationService struct {
	rootDir    string
	table      *jsonldb.Table[models.Organization]
	fileStore  *FileStore
	gitService *GitService
	mu         sync.RWMutex
	byID       map[string]*models.Organization
}

// NewOrganizationService creates a new organization service.
func NewOrganizationService(rootDir string, fileStore *FileStore, gitService *GitService) (*OrganizationService, error) {
	tablePath := filepath.Join(rootDir, "db", "organizations.jsonl")
	table, err := jsonldb.NewTable[models.Organization](tablePath)
	if err != nil {
		return nil, err
	}

	s := &OrganizationService{
		rootDir:    rootDir,
		table:      table,
		fileStore:  fileStore,
		gitService: gitService,
		byID:       make(map[string]*models.Organization),
	}

	for i := range table.Rows {
		s.byID[table.Rows[i].ID] = &table.Rows[i]
	}

	return s, nil
}

// CreateOrganization creates a new organization.
func (s *OrganizationService) CreateOrganization(ctx context.Context, name string) (*models.Organization, error) {
	if name == "" {
		return nil, fmt.Errorf("organization name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID()
	now := time.Now()
	org := &models.Organization{
		ID:      id,
		Name:    name,
		Created: now,
		Onboarding: models.OnboardingState{
			Completed: false,
			Step:      "name",
			UpdatedAt: now,
		},
	}

	if err := s.table.Append(*org); err != nil {
		return nil, err
	}

	// Update local cache
	s.table.Mu.RLock()
	newOrg := &s.table.Rows[len(s.table.Rows)-1]
	s.table.Mu.RUnlock()
	s.byID[id] = newOrg

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
			// Create a context with the new org ID
			orgCtx := context.WithValue(ctx, models.OrgKey, id)
			_ = s.gitService.CommitChange(orgCtx, "create", "page", "1", "Initial welcome page")
		}
	}

	return org, nil
}

// GetOrganization retrieves an organization by ID.
func (s *OrganizationService) GetOrganization(id string) (*models.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	org, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("organization not found")
	}

	return org, nil
}

// ListOrganizations returns all organizations.
func (s *OrganizationService) ListOrganizations() ([]*models.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	orgs := make([]*models.Organization, 0, len(s.byID))
	for _, org := range s.byID {
		orgs = append(orgs, org)
	}
	return orgs, nil
}

// UpdateSettings updates organization-wide settings.
func (s *OrganizationService) UpdateSettings(id string, settings models.OrganizationSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	org, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("organization not found")
	}

	org.Settings = settings
	return s.table.Replace(s.getAllFromCache())
}

// UpdateOnboarding updates the onboarding state of an organization.
func (s *OrganizationService) UpdateOnboarding(id string, state models.OnboardingState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	org, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("organization not found")
	}

	org.Onboarding = state
	org.Onboarding.UpdatedAt = time.Now()
	return s.table.Replace(s.getAllFromCache())
}

func (s *OrganizationService) getAllFromCache() []models.Organization {
	rows := make([]models.Organization, 0, len(s.byID))
	for _, v := range s.byID {
		rows = append(rows, *v)
	}
	return rows
}

func (s *OrganizationService) nextID() string {
	var max uint64
	for id := range s.byID {
		if n, err := DecodeID(id); err == nil && n > max {
			max = n
		}
	}
	return EncodeID(max + 1)
}

// RootDir returns the root directory of the organization service.
func (s *OrganizationService) RootDir() string {
	return s.rootDir
}
