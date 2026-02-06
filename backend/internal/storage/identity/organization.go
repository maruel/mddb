// Manages organization entities and their settings.

package identity

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
)

// Organization represents a billing and administrative entity.
// It contains workspaces and manages organization-level settings and quotas.
type Organization struct {
	ID           jsonldb.ID           `json:"id" jsonschema:"description=Unique organization identifier"`
	Name         string               `json:"name" jsonschema:"description=Display name of the organization"`
	BillingEmail string               `json:"billing_email,omitempty" jsonschema:"description=Primary billing contact email"`
	Quotas       OrganizationQuotas   `json:"quotas" jsonschema:"description=Resource limits for the organization"`
	Settings     OrganizationSettings `json:"settings" jsonschema:"description=Organization-wide configuration"`
	Created      storage.Time         `json:"created" jsonschema:"description=Organization creation timestamp"`
}

// Clone returns a deep copy of the Organization.
func (o *Organization) Clone() *Organization {
	c := *o
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
	if err := o.Quotas.Validate(); err != nil {
		return fmt.Errorf("invalid organization quota: %w", err)
	}
	if o.Quotas.MaxWorkspacesPerOrg <= 0 {
		return errors.New("invalid organization quota: max_workspaces_per_org must be positive")
	}
	if o.Quotas.MaxMembersPerOrg <= 0 {
		return errors.New("invalid organization quota: max_members_per_org must be positive")
	}
	if o.Quotas.MaxMembersPerWorkspace <= 0 {
		return errors.New("invalid organization quota: max_members_per_workspace must be positive")
	}
	if o.Quotas.MaxTotalStorageBytes <= 0 {
		return errors.New("invalid organization quota: max_total_storage_bytes must be positive")
	}
	return nil
}

// OrganizationSettings represents organization-wide settings.
type OrganizationSettings struct {
	// Defaults for new workspaces
	DefaultWorkspaceQuotas WorkspaceQuotas `json:"default_workspace_quotas" jsonschema:"description=Default quotas for new workspaces"`
}

// OrganizationQuotas defines limits for an organization.
// ResourceQuotas fields are content limits applied per-workspace (0 = no org-level restriction).
type OrganizationQuotas struct {
	storage.ResourceQuotas

	MaxWorkspacesPerOrg    int   `json:"max_workspaces_per_org" jsonschema:"description=Maximum number of workspaces in this org"`
	MaxMembersPerOrg       int   `json:"max_members_per_org" jsonschema:"description=Maximum members at org level"`
	MaxMembersPerWorkspace int   `json:"max_members_per_workspace" jsonschema:"description=Maximum members per workspace"`
	MaxTotalStorageBytes   int64 `json:"max_total_storage_bytes" jsonschema:"description=Total storage across all workspaces in bytes"`
}

// DefaultOrganizationQuotas returns the default quotas for a new organization.
// ResourceQuotas fields default to zero (no org-level content restriction).
func DefaultOrganizationQuotas() OrganizationQuotas {
	return OrganizationQuotas{
		MaxWorkspacesPerOrg:    3,
		MaxMembersPerOrg:       10,
		MaxMembersPerWorkspace: 10,
		MaxTotalStorageBytes:   5 * 1024 * 1024 * 1024, // 5 GiB
	}
}

// OrganizationService handles organization management.
type OrganizationService struct {
	table *jsonldb.Table[*Organization]
}

// NewOrganizationService creates a new organization service.
func NewOrganizationService(tablePath string) (*OrganizationService, error) {
	table, err := jsonldb.NewTable[*Organization](tablePath)
	if err != nil {
		return nil, err
	}
	return &OrganizationService{
		table: table,
	}, nil
}

// Create creates a new organization.
func (s *OrganizationService) Create(_ context.Context, name, billingEmail string) (*Organization, error) {
	if name == "" {
		return nil, errOrgNameRequired
	}
	org := &Organization{
		ID:           jsonldb.NewID(),
		Name:         name,
		BillingEmail: billingEmail,
		Quotas:       DefaultOrganizationQuotas(),
		Settings: OrganizationSettings{
			DefaultWorkspaceQuotas: DefaultWorkspaceQuotas(),
		},
		Created: storage.Now(),
	}
	if err := s.table.Append(org); err != nil {
		return nil, err
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

// Iter iterates over organizations with ID greater than startID.
func (s *OrganizationService) Iter(startID jsonldb.ID) iter.Seq[*Organization] {
	return s.table.Iter(startID)
}

// Delete deletes an organization.
func (s *OrganizationService) Delete(id jsonldb.ID) error {
	if id.IsZero() {
		return errOrgNotFound
	}
	if s.table.Get(id) == nil {
		return errOrgNotFound
	}
	if _, err := s.table.Delete(id); err != nil {
		return err
	}
	return nil
}

// Count returns the total number of organizations.
func (s *OrganizationService) Count() int {
	return s.table.Len()
}

//

var (
	errOrgNameRequired = errors.New("organization name is required")
	errOrgNotFound     = errors.New("organization not found")
)

// GitRemote represents the single remote repository configuration for a workspace.
type GitRemote struct {
	URL      string       `json:"url,omitempty" jsonschema:"description=Git repository URL"`
	Type     string       `json:"type,omitempty" jsonschema:"description=Remote type (github/gitlab/custom)"`
	AuthType string       `json:"auth_type,omitempty" jsonschema:"description=Authentication method (token/ssh)"`
	Token    string       `json:"token,omitempty" jsonschema:"description=Authentication token"`
	Created  storage.Time `json:"created,omitzero" jsonschema:"description=Remote creation timestamp"`
	LastSync storage.Time `json:"last_sync,omitzero" jsonschema:"description=Last synchronization timestamp"`
}

// IsZero returns true if the GitRemote has no URL configured.
func (g *GitRemote) IsZero() bool {
	return g.URL == ""
}
