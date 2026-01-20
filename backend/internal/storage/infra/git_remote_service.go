package infra

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

var (
	errRemoteIDRequired  = errors.New("id is required")
	errRemoteOrgRequired = errors.New("organization_id is required")
	errRemoteURLRequired = errors.New("url is required")
	errOrgIDEmptyRemote  = errors.New("organization id cannot be empty")
	errRemoteIDEmpty     = errors.New("remote id cannot be empty")
	errRemoteNotFound    = errors.New("remote not found")
)

// GitRemote represents a remote repository for an organization.
type GitRemote struct {
	ID             jsonldb.ID `json:"id" jsonschema:"description=Unique git remote identifier"`
	OrganizationID jsonldb.ID `json:"organization_id" jsonschema:"description=Organization this remote belongs to"`
	Name           string     `json:"name" jsonschema:"description=Remote name (e.g. origin)"`
	URL            string     `json:"url" jsonschema:"description=Git repository URL"`
	Type           string     `json:"type" jsonschema:"description=Remote type (github/gitlab/custom)"`
	AuthType       string     `json:"auth_type" jsonschema:"description=Authentication method (token/ssh)"`
	Token          string     `json:"token,omitempty" jsonschema:"description=Authentication token"`
	Created        time.Time  `json:"created" jsonschema:"description=Remote creation timestamp"`
	LastSync       time.Time  `json:"last_sync,omitempty" jsonschema:"description=Last synchronization timestamp"`
}

// Clone returns a copy of the GitRemote.
func (g *GitRemote) Clone() *GitRemote {
	c := *g
	return &c
}

// GetID returns the GitRemote's ID.
func (g *GitRemote) GetID() jsonldb.ID {
	return g.ID
}

// Validate checks that the GitRemote is valid.
func (g *GitRemote) Validate() error {
	if g.ID.IsZero() {
		return errRemoteIDRequired
	}
	if g.OrganizationID.IsZero() {
		return errRemoteOrgRequired
	}
	if g.URL == "" {
		return errRemoteURLRequired
	}
	return nil
}

// GitRemoteService handles git remote configuration.
type GitRemoteService struct {
	table *jsonldb.Table[*GitRemote]
}

// NewGitRemoteService creates a new git remote service.
func NewGitRemoteService(rootDir string) (*GitRemoteService, error) {
	dbDir := filepath.Join(rootDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	table, err := jsonldb.NewTable[*GitRemote](filepath.Join(dbDir, "git_remotes.jsonl"))
	if err != nil {
		return nil, err
	}

	return &GitRemoteService{table: table}, nil
}

// List returns all remotes for an organization.
func (s *GitRemoteService) List(orgID jsonldb.ID) ([]*GitRemote, error) {
	if orgID.IsZero() {
		return nil, errOrgIDEmptyRemote
	}
	var result []*GitRemote
	for r := range s.table.Iter(0) {
		if r.OrganizationID == orgID {
			result = append(result, r)
		}
	}
	return result, nil
}

// Create creates a new git remote.
func (s *GitRemoteService) Create(orgID jsonldb.ID, name, url, remoteType, authType, token string) (*GitRemote, error) {
	if orgID.IsZero() {
		return nil, errOrgIDEmptyRemote
	}
	r := &GitRemote{
		ID:             jsonldb.NewID(),
		OrganizationID: orgID,
		Name:           name,
		URL:            url,
		Type:           remoteType,
		AuthType:       authType,
		Token:          token,
		Created:        time.Now(),
	}
	if err := s.table.Append(r); err != nil {
		return nil, err
	}
	return r, nil
}

// Get retrieves a remote by ID.
func (s *GitRemoteService) Get(id jsonldb.ID) (*GitRemote, error) {
	if id.IsZero() {
		return nil, errRemoteIDEmpty
	}
	for r := range s.table.Iter(0) {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, errRemoteNotFound
}

// Delete deletes a git remote.
func (s *GitRemoteService) Delete(orgID, remoteID jsonldb.ID) error {
	if orgID.IsZero() {
		return errOrgIDEmptyRemote
	}
	if remoteID.IsZero() {
		return errRemoteIDEmpty
	}
	found, err := s.table.Delete(remoteID)
	if err != nil {
		return err
	}
	if !found {
		return errRemoteNotFound
	}
	return nil
}

// UpdateLastSync updates the last sync time for a remote.
func (s *GitRemoteService) UpdateLastSync(remoteID jsonldb.ID) error {
	if remoteID.IsZero() {
		return errRemoteIDEmpty
	}

	var all []*GitRemote
	for r := range s.table.Iter(0) {
		all = append(all, r)
	}
	for i := range all {
		if all[i].ID == remoteID {
			all[i].LastSync = time.Now()
			return s.table.Replace(all)
		}
	}
	return errRemoteNotFound
}
