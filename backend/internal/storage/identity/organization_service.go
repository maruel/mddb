package identity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

var (
	errOrgNameRequired = errors.New("organization name is required")
	errOrgNotFound     = errors.New("organization not found")
)

// OrganizationService handles organization management.
//
// An Organization owns a file storage that is managed by git. Users can be member of this organization via a
// Membership.
type OrganizationService struct {
	rootDir    string
	table      *jsonldb.Table[*Organization]
	fileStore  *infra.FileStore
	gitService *infra.Git
}

// NewOrganizationService creates a new organization service.
func NewOrganizationService(rootDir string, fileStore *infra.FileStore, gitService *infra.Git) (*OrganizationService, error) {
	dbDir := filepath.Join(rootDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	tablePath := filepath.Join(dbDir, "organizations.jsonl")
	table, err := jsonldb.NewTable[*Organization](tablePath)
	if err != nil {
		return nil, err
	}

	return &OrganizationService{
		rootDir:    rootDir,
		table:      table,
		fileStore:  fileStore,
		gitService: gitService,
	}, nil
}

// Create creates a new organization.
func (s *OrganizationService) Create(ctx context.Context, name string) (*Organization, error) {
	if name == "" {
		return nil, errOrgNameRequired
	}

	id := jsonldb.NewID()
	now := time.Now()
	org := &Organization{
		ID:      id,
		Name:    name,
		Created: now,
		Onboarding: OnboardingState{
			Completed: false,
			Step:      "name",
			UpdatedAt: now,
		},
	}

	if err := s.table.Append(org); err != nil {
		return nil, err
	}

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

// Get retrieves an organization by ID.
func (s *OrganizationService) Get(id jsonldb.ID) (*Organization, error) {
	org := s.table.Get(id)
	if org == nil {
		return nil, errOrgNotFound
	}
	return org, nil
}

// Modify atomically modifies an organization.
func (s *OrganizationService) Modify(id jsonldb.ID, fn func(org *Organization) error) (*Organization, error) {
	if id.IsZero() {
		return nil, errOrgNotFound
	}
	return s.table.Modify(id, fn)
}
