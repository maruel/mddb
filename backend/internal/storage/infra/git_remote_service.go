package infra

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
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
		return fmt.Errorf("id is required")
	}
	if g.OrganizationID.IsZero() {
		return fmt.Errorf("organization_id is required")
	}
	if g.URL == "" {
		return fmt.Errorf("url is required")
	}
	return nil
}

// GitRemoteService handles git remote configuration.
type GitRemoteService struct {
	rootDir      string
	remoteTable  *jsonldb.Table[*GitRemote]
	mu           sync.RWMutex
	remotesByOrg map[jsonldb.ID][]*GitRemote
}

// NewGitRemoteService creates a new git remote service.
func NewGitRemoteService(rootDir string) (*GitRemoteService, error) {
	dbDir := filepath.Join(rootDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	remotePath := filepath.Join(dbDir, "git_remotes.jsonl")
	remoteTable, err := jsonldb.NewTable[*GitRemote](remotePath)
	if err != nil {
		return nil, err
	}

	s := &GitRemoteService{
		rootDir:      rootDir,
		remoteTable:  remoteTable,
		remotesByOrg: make(map[jsonldb.ID][]*GitRemote),
	}

	for r := range remoteTable.Iter(0) {
		s.remotesByOrg[r.OrganizationID] = append(s.remotesByOrg[r.OrganizationID], r)
	}

	return s, nil
}

// ListRemotes returns all remotes for an organization.
func (s *GitRemoteService) ListRemotes(orgID jsonldb.ID) ([]*GitRemote, error) {
	if orgID.IsZero() {
		return nil, fmt.Errorf("organization id cannot be empty")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.remotesByOrg[orgID], nil
}

// CreateRemote creates a new git remote.
func (s *GitRemoteService) CreateRemote(orgID jsonldb.ID, name, url, remoteType, authType, token string) (*GitRemote, error) {
	if orgID.IsZero() {
		return nil, fmt.Errorf("organization id cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	newRemote := &GitRemote{
		ID:             jsonldb.NewID(),
		OrganizationID: orgID,
		Name:           name,
		URL:            url,
		Type:           remoteType,
		AuthType:       authType,
		Token:          token,
		Created:        time.Now(),
	}

	if err := s.remoteTable.Append(newRemote); err != nil {
		return nil, err
	}

	// Update cache
	cachedRemote := s.remoteTable.Last()
	s.remotesByOrg[orgID] = append(s.remotesByOrg[orgID], cachedRemote)

	return cachedRemote, nil
}

// GetRemote retrieves a remote by ID.
func (s *GitRemoteService) GetRemote(id jsonldb.ID) (*GitRemote, error) {
	if id.IsZero() {
		return nil, fmt.Errorf("remote id cannot be empty")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, remotes := range s.remotesByOrg {
		for _, r := range remotes {
			if r.ID == id {
				return r, nil
			}
		}
	}
	return nil, fmt.Errorf("remote not found")
}

// GetToken retrieves the token for a remote.
func (s *GitRemoteService) GetToken(remoteID jsonldb.ID) (string, error) {
	remote, err := s.GetRemote(remoteID)
	if err != nil {
		return "", err
	}
	return remote.Token, nil
}

// DeleteRemote deletes a git remote.
func (s *GitRemoteService) DeleteRemote(orgID, remoteID jsonldb.ID) error {
	if orgID.IsZero() {
		return fmt.Errorf("organization id cannot be empty")
	}
	if remoteID.IsZero() {
		return fmt.Errorf("remote id cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove from table
	var newRows []*GitRemote
	found := false
	for r := range s.remoteTable.Iter(0) {
		if r.ID == remoteID {
			found = true
			continue
		}
		newRows = append(newRows, r)
	}
	if !found {
		return fmt.Errorf("remote not found")
	}

	if err := s.remoteTable.Replace(newRows); err != nil {
		return err
	}

	// Update cache
	newCache := make([]*GitRemote, 0, len(s.remotesByOrg[orgID]))
	for _, r := range s.remotesByOrg[orgID] {
		if r.ID != remoteID {
			newCache = append(newCache, r)
		}
	}
	s.remotesByOrg[orgID] = newCache

	return nil
}

// UpdateLastSync updates the last sync time for a remote.
func (s *GitRemoteService) UpdateLastSync(remoteID jsonldb.ID) error {
	if remoteID.IsZero() {
		return fmt.Errorf("remote id cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var allRemotes []*GitRemote
	for r := range s.remoteTable.Iter(0) {
		allRemotes = append(allRemotes, r)
	}
	for i := range allRemotes {
		if allRemotes[i].ID == remoteID {
			allRemotes[i].LastSync = time.Now()
			return s.remoteTable.Replace(allRemotes)
		}
	}
	return fmt.Errorf("remote not found")
}
