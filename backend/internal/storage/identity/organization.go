package identity

import (
	"context"
	"errors"
	"iter"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// Organization represents a workspace or group of users.
type Organization struct {
	ID        jsonldb.ID           `json:"id" jsonschema:"description=Unique organization identifier"`
	Name      string               `json:"name" jsonschema:"description=Display name of the organization"`
	Quotas    OrganizationQuota    `json:"quotas" jsonschema:"description=Resource limits for the organization"`
	Settings  OrganizationSettings `json:"settings" jsonschema:"description=Organization-wide configuration"`
	GitRemote GitRemote            `json:"git_remote,omitzero" jsonschema:"description=Git remote repository configuration"`
	Created   time.Time            `json:"created" jsonschema:"description=Organization creation timestamp"`
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
	if o.Quotas.MaxPages <= 0 || o.Quotas.MaxStorage <= 0 || o.Quotas.MaxUsers <= 0 || o.Quotas.MaxRecordsPerTable <= 0 || o.Quotas.MaxAssetSize <= 0 {
		return errInvalidOrgQuota
	}
	return nil
}

// OrganizationSettings represents organization-wide settings.
type OrganizationSettings struct {
	AllowedDomains []string `json:"allowed_domains,omitempty" jsonschema:"description=Email domains allowed for membership"`
	PublicAccess   bool     `json:"public_access" jsonschema:"description=Whether content is publicly accessible"`
	GitAutoPush    bool     `json:"git_auto_push" jsonschema:"description=Automatically push changes to remote"`
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

// OrganizationQuota defines limits for an organization.
type OrganizationQuota struct {
	MaxPages           int   `json:"max_pages" jsonschema:"description=Maximum number of pages allowed"`
	MaxStorage         int64 `json:"max_storage" jsonschema:"description=Maximum storage in bytes"`
	MaxUsers           int   `json:"max_users" jsonschema:"description=Maximum number of users allowed"`
	MaxRecordsPerTable int   `json:"max_records_per_table" jsonschema:"description=Maximum number of records allowed per table"`
	MaxAssetSize       int64 `json:"max_asset_size" jsonschema:"description=Maximum size of a single asset in bytes"`
}

// OrganizationService handles organization management.
//
// An Organization owns a file storage that is managed by git. Users can be member of this organization via a
// Membership.
type OrganizationService struct {
	table *jsonldb.Table[*Organization]
}

// NewOrganizationService creates a new organization service.
// tablePath is the path to the organizations.jsonl file.
func NewOrganizationService(tablePath string) (*OrganizationService, error) {
	table, err := jsonldb.NewTable[*Organization](tablePath)
	if err != nil {
		return nil, err
	}
	return &OrganizationService{
		table: table,
	}, nil
}

// Create creates a new organization record.
// Note: After calling Create, callers should also call FileStore.InitOrg to initialize storage.
func (s *OrganizationService) Create(_ context.Context, name string) (*Organization, error) {
	if name == "" {
		return nil, errOrgNameRequired
	}
	id := jsonldb.NewID()
	now := time.Now()
	org := &Organization{
		ID:      id,
		Name:    name,
		Created: now,
		Quotas: OrganizationQuota{
			MaxPages:           1000,
			MaxStorage:         1024 * 1024 * 1024, // 1 GiB
			MaxUsers:           3,
			MaxRecordsPerTable: 10000,
			MaxAssetSize:       50 * 1024 * 1024, // 50 MiB
		},
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

// Iter iterates over organizations with ID greater than startID. Pass 0 to iterate from the beginning.
func (s *OrganizationService) Iter(startID jsonldb.ID) iter.Seq[*Organization] {
	return s.table.Iter(startID)
}

//

var (
	errOrgNameRequired = errors.New("organization name is required")
	errOrgNotFound     = errors.New("organization not found")
	errInvalidOrgQuota = errors.New("invalid organization quota")
)
