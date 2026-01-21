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

// Organization represents a workspace or group of users.
type Organization struct {
	ID         jsonldb.ID           `json:"id" jsonschema:"description=Unique organization identifier"`
	Name       string               `json:"name" jsonschema:"description=Display name of the organization"`
	Quotas     Quota                `json:"quotas" jsonschema:"description=Resource limits for the organization"`
	Settings   OrganizationSettings `json:"settings" jsonschema:"description=Organization-wide configuration"`
	Onboarding OnboardingState      `json:"onboarding" jsonschema:"description=Initial setup progress tracking"`
	GitRemote  GitRemote            `json:"git_remote,omitzero" jsonschema:"description=Git remote repository configuration"`
	Created    time.Time            `json:"created" jsonschema:"description=Organization creation timestamp"`
}

// Clone returns a deep copy of the Organization.
func (o *Organization) Clone() *Organization {
	c := *o
	if o.Settings.AllowedDomains != nil {
		c.Settings.AllowedDomains = make([]string, len(o.Settings.AllowedDomains))
		copy(c.Settings.AllowedDomains, o.Settings.AllowedDomains)
	}
	return &c
}

// GetID returns the Organization's ID.
func (o *Organization) GetID() jsonldb.ID {
	return o.ID
}

// Validate checks that the Organization is valid.
func (o *Organization) Validate() error {
	if o.ID.IsZero() {
		return errIDRequired
	}
	if o.Name == "" {
		return errNameRequired
	}
	return nil
}

// OnboardingState tracks the progress of an organization's initial setup.
type OnboardingState struct {
	Completed bool      `json:"completed" jsonschema:"description=Whether onboarding is complete"`
	Step      string    `json:"step" jsonschema:"description=Current onboarding step (name/members/git/done)"`
	UpdatedAt time.Time `json:"updated_at" jsonschema:"description=Last progress update timestamp"`
}

// OrganizationSettings represents organization-wide settings.
type OrganizationSettings struct {
	AllowedDomains []string    `json:"allowed_domains,omitempty" jsonschema:"description=Email domains allowed for membership"`
	PublicAccess   bool        `json:"public_access" jsonschema:"description=Whether content is publicly accessible"`
	Git            GitSettings `json:"git" jsonschema:"description=Git synchronization configuration"`
}

// GitSettings contains configuration for Git remotes and synchronization.
type GitSettings struct {
	AutoPush bool `json:"auto_push" jsonschema:"description=Automatically push changes to remote"`
}

// GitRemote represents the single remote repository configuration for an organization.
type GitRemote struct {
	URL      string    `json:"url,omitempty" jsonschema:"description=Git repository URL"`
	Type     string    `json:"type,omitempty" jsonschema:"description=Remote type (github/gitlab/custom)"`
	AuthType string    `json:"auth_type,omitempty" jsonschema:"description=Authentication method (token/ssh)"`
	Token    string    `json:"token,omitempty" jsonschema:"description=Authentication token"`
	Created  time.Time `json:"created,omitzero" jsonschema:"description=Remote creation timestamp"`
	LastSync time.Time `json:"last_sync,omitzero" jsonschema:"description=Last synchronization timestamp"`
}

// IsZero returns true if the GitRemote has no URL configured.
func (g *GitRemote) IsZero() bool {
	return g.URL == ""
}

// Quota defines limits for an organization.
type Quota struct {
	MaxPages   int   `json:"max_pages" jsonschema:"description=Maximum number of pages allowed"`
	MaxStorage int64 `json:"max_storage" jsonschema:"description=Maximum storage in bytes"`
	MaxUsers   int   `json:"max_users" jsonschema:"description=Maximum number of users allowed"`
}

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
// tablePath is the path to the organizations.jsonl file.
// rootDir is the root directory for organization content (each org gets a subdirectory).
func NewOrganizationService(tablePath, rootDir string, fileStore *infra.FileStore, gitService *infra.Git) (*OrganizationService, error) {
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

//

var (
	errOrgNameRequired = errors.New("organization name is required")
	errOrgNotFound     = errors.New("organization not found")
)
