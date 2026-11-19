// Persisted workspace rows and settings.

package v1

import (
	"github.com/maruel/ksid"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// Workspace represents an isolated content container within an organization.
// Each workspace has its own pages, tables, assets, and git remote.
type Workspace struct {
	ID             ksid.ID                  `json:"id" jsonschema:"description=Unique workspace identifier"`
	OrganizationID ksid.ID                  `json:"organization_id" jsonschema:"description=Parent organization ID"`
	Name           string                   `json:"name" jsonschema:"description=Display name of the workspace"`
	Quotas         storagev1.ResourceQuotas `json:"quotas" jsonschema:"description=Resource limits for the workspace"`
	Settings       WorkspaceSettings        `json:"settings" jsonschema:"description=Workspace-wide configuration"`
	GitRemote      GitRemote                `json:"git_remote,omitzero" jsonschema:"description=Git remote repository configuration"`
	Created        storagev1.Time           `json:"created" jsonschema:"description=Workspace creation timestamp"`
}

// Clone returns a deep copy of the Workspace.
func (w *Workspace) Clone() *Workspace {
	c := *w
	if w.Settings.AllowedDomains != nil {
		c.Settings.AllowedDomains = make([]string, len(w.Settings.AllowedDomains))
		copy(c.Settings.AllowedDomains, w.Settings.AllowedDomains)
	}
	return &c
}

// GetID returns the Workspace's ID.
func (w *Workspace) GetID() ksid.ID {
	return w.ID
}

// Validate checks that the Workspace is valid.
// Quota fields of -1 mean "inherit from server/org layer".
func (w *Workspace) Validate() error {
	if w.ID.IsZero() {
		return ErrIDRequired
	}
	if w.OrganizationID.IsZero() {
		return ErrOrgIDEmpty
	}
	if w.Name == "" {
		return ErrNameRequired
	}
	if err := w.Quotas.Validate(); err != nil {
		return ErrInvalidWorkspaceQuota
	}
	return nil
}

// WorkspaceSettings represents workspace-wide settings.
type WorkspaceSettings struct {
	AllowedDomains []string `json:"allowed_domains,omitempty" jsonschema:"description=Additional email domain restrictions (inherits org)"`
	PublicAccess   bool     `json:"public_access" jsonschema:"description=Whether content is publicly accessible"`
	GitAutoPush    bool     `json:"git_auto_push" jsonschema:"description=Automatically push changes to remote"`
}
