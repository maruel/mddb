package identity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
//
// An Organization owns a file storage that is managed by git. Users can be member of this organization via a
// Membership.
type OrganizationService struct {
	rootDir    string
	table      *jsonldb.Table[*entity.Organization]
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
	table, err := jsonldb.NewTable[*entity.Organization](tablePath)
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

// CreateOrganization creates a new organization.
func (s *OrganizationService) CreateOrganization(ctx context.Context, name string) (*entity.Organization, error) {
	if name == "" {
		return nil, errOrgNameRequired
	}

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
	org := s.table.Get(id)
	if org == nil {
		return nil, errOrgNotFound
	}
	return org, nil
}

// UpdateSettings updates organization-wide settings.
func (s *OrganizationService) UpdateSettings(id jsonldb.ID, settings entity.OrganizationSettings) error {
	org := s.table.Get(id)
	if org == nil {
		return errOrgNotFound
	}
	org.Settings = settings
	_, err := s.table.Update(org)
	return err
}

// UpdateOnboarding updates the onboarding state of an organization.
func (s *OrganizationService) UpdateOnboarding(id jsonldb.ID, state entity.OnboardingState) error {
	org := s.table.Get(id)
	if org == nil {
		return errOrgNotFound
	}
	org.Onboarding = state
	org.Onboarding.UpdatedAt = time.Now()
	_, err := s.table.Update(org)
	return err
}

// RootDir returns the root directory of the organization service.
func (s *OrganizationService) RootDir() string {
	return s.rootDir
}

// GetGitRemote returns the git remote for an organization, or nil if none is configured.
func (s *OrganizationService) GetGitRemote(id jsonldb.ID) *entity.GitRemote {
	org := s.table.Get(id)
	if org == nil || org.GitRemote.IsZero() {
		return nil
	}
	remote := org.GitRemote
	return &remote
}

// SetGitRemote creates or updates the git remote for an organization.
func (s *OrganizationService) SetGitRemote(id jsonldb.ID, url, remoteType, authType, token string) (*entity.GitRemote, error) {
	org := s.table.Get(id)
	if org == nil {
		return nil, errOrgNotFound
	}

	// Preserve existing timestamps on update
	created := org.GitRemote.Created
	lastSync := org.GitRemote.LastSync
	if org.GitRemote.IsZero() {
		created = time.Now()
	}

	org.GitRemote = entity.GitRemote{
		URL:      url,
		Type:     remoteType,
		AuthType: authType,
		Token:    token,
		Created:  created,
		LastSync: lastSync,
	}

	if _, err := s.table.Update(org); err != nil {
		return nil, err
	}
	remote := org.GitRemote
	return &remote, nil
}

// DeleteGitRemote removes the git remote configuration from an organization.
func (s *OrganizationService) DeleteGitRemote(id jsonldb.ID) error {
	org := s.table.Get(id)
	if org == nil {
		return errOrgNotFound
	}
	if org.GitRemote.IsZero() {
		return errOrgNotFound // No remote to delete
	}

	org.GitRemote = entity.GitRemote{}
	_, err := s.table.Update(org)
	return err
}

// UpdateGitRemoteLastSync updates the last sync time for an organization's git remote.
func (s *OrganizationService) UpdateGitRemoteLastSync(id jsonldb.ID) error {
	org := s.table.Get(id)
	if org == nil {
		return errOrgNotFound
	}
	if org.GitRemote.IsZero() {
		return errOrgNotFound // No remote to update
	}

	org.GitRemote.LastSync = time.Now()
	_, err := s.table.Update(org)
	return err
}
