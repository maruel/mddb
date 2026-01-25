package identity

import (
	"context"
	"errors"
	"iter"
	"regexp"
	"strings"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
)

// Workspace represents an isolated content container within an organization.
// Each workspace has its own pages, tables, assets, and git remote.
type Workspace struct {
	ID             jsonldb.ID        `json:"id" jsonschema:"description=Unique workspace identifier"`
	OrganizationID jsonldb.ID        `json:"organization_id" jsonschema:"description=Parent organization ID"`
	Name           string            `json:"name" jsonschema:"description=Display name of the workspace"`
	Slug           string            `json:"slug" jsonschema:"description=URL-friendly identifier"`
	Quotas         WorkspaceQuotas   `json:"quotas" jsonschema:"description=Resource limits for the workspace"`
	Settings       WorkspaceSettings `json:"settings" jsonschema:"description=Workspace-wide configuration"`
	GitRemote      GitRemote         `json:"git_remote,omitzero" jsonschema:"description=Git remote repository configuration"`
	Created        storage.Time      `json:"created" jsonschema:"description=Workspace creation timestamp"`
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
func (w *Workspace) GetID() jsonldb.ID {
	return w.ID
}

// Validate checks that the Workspace is valid.
func (w *Workspace) Validate() error {
	if w.ID.IsZero() {
		return errIDRequired
	}
	if w.OrganizationID.IsZero() {
		return errOrgIDEmpty
	}
	if w.Name == "" {
		return errNameRequired
	}
	if w.Slug == "" {
		return errSlugRequired
	}
	if !isValidSlug(w.Slug) {
		return errInvalidSlug
	}
	if w.Quotas.MaxPages <= 0 || w.Quotas.MaxStorageMB <= 0 || w.Quotas.MaxRecordsPerTable <= 0 || w.Quotas.MaxAssetSizeMB <= 0 {
		return errInvalidWorkspaceQuota
	}
	return nil
}

// WorkspaceSettings represents workspace-wide settings.
type WorkspaceSettings struct {
	AllowedDomains []string `json:"allowed_domains,omitempty" jsonschema:"description=Additional email domain restrictions (inherits org)"`
	PublicAccess   bool     `json:"public_access" jsonschema:"description=Whether content is publicly accessible"`
	GitAutoPush    bool     `json:"git_auto_push" jsonschema:"description=Automatically push changes to remote"`
}

// WorkspaceQuotas defines limits for a workspace.
type WorkspaceQuotas struct {
	MaxPages           int `json:"max_pages" jsonschema:"description=Maximum number of pages allowed"`
	MaxStorageMB       int `json:"max_storage_mb" jsonschema:"description=Maximum storage in megabytes"`
	MaxRecordsPerTable int `json:"max_records_per_table" jsonschema:"description=Maximum records per table"`
	MaxAssetSizeMB     int `json:"max_asset_size_mb" jsonschema:"description=Maximum size of a single asset in megabytes"`
}

// DefaultWorkspaceQuotas returns the default quotas for a new workspace.
func DefaultWorkspaceQuotas() WorkspaceQuotas {
	return WorkspaceQuotas{
		MaxPages:           1000,
		MaxStorageMB:       1024, // 1 GiB
		MaxRecordsPerTable: 10000,
		MaxAssetSizeMB:     50, // 50 MiB
	}
}

// WorkspaceService handles workspace management.
type WorkspaceService struct {
	table  *jsonldb.Table[*Workspace]
	byOrg  *jsonldb.Index[jsonldb.ID, *Workspace]
	bySlug *workspaceSlugIndex
}

// NewWorkspaceService creates a new workspace service.
func NewWorkspaceService(tablePath string) (*WorkspaceService, error) {
	table, err := jsonldb.NewTable[*Workspace](tablePath)
	if err != nil {
		return nil, err
	}
	byOrg := jsonldb.NewIndex(table, func(w *Workspace) jsonldb.ID { return w.OrganizationID })
	bySlug := newWorkspaceSlugIndex(table)
	return &WorkspaceService{
		table:  table,
		byOrg:  byOrg,
		bySlug: bySlug,
	}, nil
}

// Create creates a new workspace in an organization.
func (s *WorkspaceService) Create(_ context.Context, orgID jsonldb.ID, name, slug string) (*Workspace, error) {
	if orgID.IsZero() {
		return nil, errOrgIDEmpty
	}
	if name == "" {
		return nil, errWorkspaceNameRequired
	}
	if slug == "" {
		slug = generateSlug(name)
	}
	if !isValidSlug(slug) {
		return nil, errInvalidSlug
	}

	// Check if slug is unique within the org
	if s.bySlug.Get(orgID, slug) != nil {
		return nil, errSlugExists
	}

	ws := &Workspace{
		ID:             jsonldb.NewID(),
		OrganizationID: orgID,
		Name:           name,
		Slug:           slug,
		Quotas:         DefaultWorkspaceQuotas(),
		Created:        storage.Now(),
	}
	if err := s.table.Append(ws); err != nil {
		return nil, err
	}
	return ws, nil
}

// Get retrieves a workspace by ID.
func (s *WorkspaceService) Get(id jsonldb.ID) (*Workspace, error) {
	ws := s.table.Get(id)
	if ws == nil {
		return nil, errWorkspaceNotFound
	}
	return ws, nil
}

// GetBySlug retrieves a workspace by org ID and slug. O(1) via index.
func (s *WorkspaceService) GetBySlug(orgID jsonldb.ID, slug string) (*Workspace, error) {
	ws := s.bySlug.Get(orgID, slug)
	if ws == nil {
		return nil, errWorkspaceNotFound
	}
	return ws, nil
}

// Modify atomically modifies a workspace.
func (s *WorkspaceService) Modify(id jsonldb.ID, fn func(ws *Workspace) error) (*Workspace, error) {
	if id.IsZero() {
		return nil, errWorkspaceNotFound
	}
	return s.table.Modify(id, fn)
}

// IterByOrg iterates over workspaces in an organization. O(1) via index.
func (s *WorkspaceService) IterByOrg(orgID jsonldb.ID) iter.Seq[*Workspace] {
	return s.byOrg.Iter(orgID)
}

// Iter iterates over all workspaces with ID greater than startID.
func (s *WorkspaceService) Iter(startID jsonldb.ID) iter.Seq[*Workspace] {
	return s.table.Iter(startID)
}

// CountByOrg returns the number of workspaces in an organization.
func (s *WorkspaceService) CountByOrg(orgID jsonldb.ID) int {
	count := 0
	for range s.byOrg.Iter(orgID) {
		count++
	}
	return count
}

// Delete deletes a workspace.
func (s *WorkspaceService) Delete(id jsonldb.ID) error {
	if id.IsZero() {
		return errWorkspaceNotFound
	}
	if s.table.Get(id) == nil {
		return errWorkspaceNotFound
	}
	if _, err := s.table.Delete(id); err != nil {
		return err
	}
	return nil
}

//

var (
	errWorkspaceNameRequired = errors.New("workspace name is required")
	errWorkspaceNotFound     = errors.New("workspace not found")
	errInvalidWorkspaceQuota = errors.New("invalid workspace quota")
	errSlugRequired          = errors.New("slug is required")
	errInvalidSlug           = errors.New("invalid slug: must be lowercase alphanumeric with hyphens")
	errSlugExists            = errors.New("slug already exists in this organization")
)

// slugRegex validates URL-friendly slugs.
var slugRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func isValidSlug(slug string) bool {
	return len(slug) >= 1 && len(slug) <= 100 && slugRegex.MatchString(slug)
}

func generateSlug(name string) string {
	slug := strings.ToLower(name)
	slug = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, slug)
	// Collapse multiple hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "workspace"
	}
	return slug
}

// workspaceSlugIndex indexes workspaces by (orgID, slug) for uniqueness within an org.
type workspaceSlugIndex struct {
	table *jsonldb.Table[*Workspace]
	byKey map[orgSlugKey]jsonldb.ID
}

type orgSlugKey struct {
	OrgID jsonldb.ID
	Slug  string
}

func newWorkspaceSlugIndex(table *jsonldb.Table[*Workspace]) *workspaceSlugIndex {
	idx := &workspaceSlugIndex{table: table, byKey: make(map[orgSlugKey]jsonldb.ID)}
	table.AddObserver(idx)
	return idx
}

func (idx *workspaceSlugIndex) Get(orgID jsonldb.ID, slug string) *Workspace {
	id, ok := idx.byKey[orgSlugKey{OrgID: orgID, Slug: slug}]
	if !ok {
		return nil
	}
	return idx.table.Get(id)
}

func (idx *workspaceSlugIndex) OnAppend(row *Workspace) {
	idx.byKey[orgSlugKey{OrgID: row.OrganizationID, Slug: row.Slug}] = row.ID
}

func (idx *workspaceSlugIndex) OnUpdate(prev, curr *Workspace) {
	delete(idx.byKey, orgSlugKey{OrgID: prev.OrganizationID, Slug: prev.Slug})
	idx.byKey[orgSlugKey{OrgID: curr.OrganizationID, Slug: curr.Slug}] = curr.ID
}

func (idx *workspaceSlugIndex) OnDelete(row *Workspace) {
	delete(idx.byKey, orgSlugKey{OrgID: row.OrganizationID, Slug: row.Slug})
}
