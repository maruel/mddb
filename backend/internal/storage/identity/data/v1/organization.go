// Persisted organization rows, quotas, and the workspace git remote.

package v1

import (
	"errors"
	"fmt"

	"github.com/maruel/ksid"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// Organization represents a billing and administrative entity.
// It contains workspaces and manages organization-level settings and quotas.
type Organization struct {
	ID           ksid.ID              `json:"id" jsonschema:"description=Unique organization identifier"`
	Name         string               `json:"name" jsonschema:"description=Display name of the organization"`
	BillingEmail string               `json:"billing_email,omitempty" jsonschema:"description=Primary billing contact email"`
	Quotas       OrganizationQuotas   `json:"quotas" jsonschema:"description=Resource limits for the organization"`
	Settings     OrganizationSettings `json:"settings" jsonschema:"description=Organization-wide configuration"`
	Created      storagev1.Time       `json:"created" jsonschema:"description=Organization creation timestamp"`
}

// Clone returns a deep copy of the Organization.
func (o *Organization) Clone() *Organization {
	c := *o
	return &c
}

// GetID returns the Organization's ID.
func (o *Organization) GetID() ksid.ID {
	return o.ID
}

// Validate checks that the Organization is valid.
func (o *Organization) Validate() error {
	if o.ID.IsZero() {
		return ErrIDRequired
	}
	if o.Name == "" {
		return ErrNameRequired
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
	DefaultWorkspaceQuotas storagev1.ResourceQuotas `json:"default_workspace_quotas" jsonschema:"description=Default quotas for new workspaces"`
}

// OrganizationQuotas defines limits for an organization.
// ResourceQuotas fields are content limits applied per-workspace (0 = no org-level restriction).
type OrganizationQuotas struct {
	storagev1.ResourceQuotas

	MaxWorkspacesPerOrg    int   `json:"max_workspaces_per_org" jsonschema:"description=Maximum number of workspaces in this org"`
	MaxMembersPerOrg       int   `json:"max_members_per_org" jsonschema:"description=Maximum members at org level"`
	MaxMembersPerWorkspace int   `json:"max_members_per_workspace" jsonschema:"description=Maximum members per workspace"`
	MaxTotalStorageBytes   int64 `json:"max_total_storage_bytes" jsonschema:"description=Total storage across all workspaces in bytes"`
}

// GitRemote represents the single remote repository configuration for a workspace.
type GitRemote struct {
	URL            string         `json:"url,omitempty" jsonschema:"description=Git repository URL"`
	Type           string         `json:"type,omitempty" jsonschema:"description=Remote type (github/gitlab/custom)"`
	AuthType       string         `json:"auth_type,omitempty" jsonschema:"description=Authentication method (token/ssh/github_app)"`
	Token          string         `json:"token,omitempty" jsonschema:"description=Authentication token"`
	InstallationID int64          `json:"installation_id,omitempty" jsonschema:"description=GitHub App installation ID"`
	RepoOwner      string         `json:"repo_owner,omitempty" jsonschema:"description=Repository owner (org or user)"`
	RepoName       string         `json:"repo_name,omitempty" jsonschema:"description=Repository name"`
	Branch         string         `json:"branch,omitempty" jsonschema:"description=Branch to sync"`
	LastSyncError  string         `json:"last_sync_error,omitempty" jsonschema:"description=Last sync error message"`
	SyncStatus     string         `json:"sync_status,omitempty" jsonschema:"description=Current sync status (idle/syncing/error/conflict)"`
	Created        storagev1.Time `json:"created,omitzero" jsonschema:"description=Remote creation timestamp"`
	LastSync       storagev1.Time `json:"last_sync,omitzero" jsonschema:"description=Last synchronization timestamp"`
}

// IsZero returns true if the GitRemote has no URL configured and no GitHub App installation.
func (g *GitRemote) IsZero() bool {
	return g.URL == "" && g.InstallationID == 0
}
