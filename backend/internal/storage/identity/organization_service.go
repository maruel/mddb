package identity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

var (
	errOrgNameRequired = errors.New("organization name is required")
	errOrgNotFound     = errors.New("organization not found")
)

// OrganizationService handles organization management.
type OrganizationService struct {
	rootDir    string
	table      *jsonldb.Table[*entity.Organization]
	fileStore  *infra.FileStore
	gitService *infra.Git
	mu         sync.RWMutex
	byID       map[jsonldb.ID]*entity.Organization
}

// NewOrganizationService creates a new organization service.
func NewOrganizationService(rootDir string, fileStore *infra.FileStore, gitService *infra.Git) (*OrganizationService, error) {
	dbDir := filepath.Join(rootDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	tablePath := filepath.Join(dbDir, "organizations.jsonl")
	table, err := jsonldb.NewTable[*entity.Organization](tablePath)
	if err != nil {
		return nil, err
	}

	s := &OrganizationService{
		rootDir:    rootDir,
		table:      table,
		fileStore:  fileStore,
		gitService: gitService,
		byID:       make(map[jsonldb.ID]*entity.Organization),
	}

	for org := range table.Iter(0) {
		s.byID[org.ID] = org
	}

	return s, nil
}

// CreateOrganization creates a new organization.
func (s *OrganizationService) CreateOrganization(ctx context.Context, name string) (*entity.Organization, error) {
	if name == "" {
		return nil, errOrgNameRequired
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := jsonldb.NewID()
	now := time.Now()
	org := &entity.Organization{
		ID:      id,
		Name:    name,
		Created: now,
		Onboarding: entity.OnboardingState{
			Completed: false,
			Step:      "name",
			UpdatedAt: now,
		},
	}

	if err := s.table.Append(org); err != nil {
		return nil, err
	}

	// Update local cache
	newOrg := s.table.Last()
	s.byID[id] = newOrg

	// Create organization content directory
	orgDir := filepath.Join(s.rootDir, id.String())
	orgPagesDir := filepath.Join(orgDir, "pages")
	if err := os.MkdirAll(orgPagesDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
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
		welcomeID := jsonldb.NewID()
		_, _ = s.fileStore.WritePage(id, welcomeID, welcomeTitle, welcomeContent)

		// Commit the welcome page
		if s.gitService != nil {
			_ = s.gitService.CommitChange(ctx, id, "create", "page", welcomeID.String(), "Initial welcome page")
		}
	}

	return org, nil
}

// GetOrganization retrieves an organization by ID.
func (s *OrganizationService) GetOrganization(id jsonldb.ID) (*entity.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	org, ok := s.byID[id]
	if !ok {
		return nil, errOrgNotFound
	}

	return org, nil
}

// GetOrganizationByID retrieves an organization by string ID.
func (s *OrganizationService) GetOrganizationByID(idStr string) (*entity.Organization, error) {
	id, err := jsonldb.DecodeID(idStr)
	if err != nil {
		return nil, err
	}
	return s.GetOrganization(id)
}

// ListOrganizations returns all organizations.
func (s *OrganizationService) ListOrganizations() ([]*entity.Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	orgs := make([]*entity.Organization, 0, len(s.byID))
	for _, org := range s.byID {
		orgs = append(orgs, org)
	}
	return orgs, nil
}

// UpdateSettings updates organization-wide settings.
func (s *OrganizationService) UpdateSettings(id jsonldb.ID, settings entity.OrganizationSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	org, ok := s.byID[id]
	if !ok {
		return errOrgNotFound
	}

	org.Settings = settings
	return s.table.Replace(s.getAllFromCache())
}

// UpdateOnboarding updates the onboarding state of an organization.
func (s *OrganizationService) UpdateOnboarding(id jsonldb.ID, state entity.OnboardingState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	org, ok := s.byID[id]
	if !ok {
		return errOrgNotFound
	}

	org.Onboarding = state
	org.Onboarding.UpdatedAt = time.Now()
	return s.table.Replace(s.getAllFromCache())
}

func (s *OrganizationService) getAllFromCache() []*entity.Organization {
	rows := make([]*entity.Organization, 0, len(s.byID))
	for _, v := range s.byID {
		rows = append(rows, v)
	}
	return rows
}

// RootDir returns the root directory of the organization service.
func (s *OrganizationService) RootDir() string {
	return s.rootDir
}
