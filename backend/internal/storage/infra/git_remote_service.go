package infra

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)

// GitRemoteService handles git remote configuration and secrets.
type GitRemoteService struct {
	rootDir      string
	remoteTable  *jsonldb.Table[*entity.GitRemote]
	secretTable  *jsonldb.Table[*remoteSecret]
	mu           sync.RWMutex
	remotesByOrg map[jsonldb.ID][]*entity.GitRemote
}

type remoteSecret struct {
	ID       jsonldb.ID `json:"id" jsonschema:"description=Unique secret identifier"`
	RemoteID jsonldb.ID `json:"remote_id" jsonschema:"description=Git remote this secret belongs to"`
	Token    string     `json:"token" jsonschema:"description=Authentication token value"`
}

func (r *remoteSecret) Clone() *remoteSecret {
	c := *r
	return &c
}

// GetID returns the remoteSecret's ID.
func (r *remoteSecret) GetID() jsonldb.ID {
	return r.ID
}

// Validate checks that the remoteSecret is valid.
func (r *remoteSecret) Validate() error {
	if r.ID.IsZero() {
		return fmt.Errorf("id is required")
	}
	if r.RemoteID.IsZero() {
		return fmt.Errorf("remote_id is required")
	}
	return nil
}

// NewGitRemoteService creates a new git remote service.
func NewGitRemoteService(rootDir string) (*GitRemoteService, error) {
	dbDir := filepath.Join(rootDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	remotePath := filepath.Join(dbDir, "git_remotes.jsonl")
	remoteTable, err := jsonldb.NewTable[*entity.GitRemote](remotePath)
	if err != nil {
		return nil, err
	}

	secretPath := filepath.Join(dbDir, "git_remote_secrets.jsonl")
	secretTable, err := jsonldb.NewTable[*remoteSecret](secretPath)
	if err != nil {
		return nil, err
	}

	s := &GitRemoteService{
		rootDir:      rootDir,
		remoteTable:  remoteTable,
		secretTable:  secretTable,
		remotesByOrg: make(map[jsonldb.ID][]*entity.GitRemote),
	}

	for r := range remoteTable.Iter(0) {
		s.remotesByOrg[r.OrganizationID] = append(s.remotesByOrg[r.OrganizationID], r)
	}

	return s, nil
}

// ListRemotes returns all remotes for an organization.
func (s *GitRemoteService) ListRemotes(orgID jsonldb.ID) ([]*entity.GitRemote, error) {
	if orgID.IsZero() {
		return nil, fmt.Errorf("organization id cannot be empty")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.remotesByOrg[orgID], nil
}

// CreateRemote creates a new git remote.
func (s *GitRemoteService) CreateRemote(orgID jsonldb.ID, name, url, remoteType, authType, token string) (*entity.GitRemote, error) {
	if orgID.IsZero() {
		return nil, fmt.Errorf("organization id cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := jsonldb.NewID()
	newRemote := &entity.GitRemote{
		ID:             id,
		OrganizationID: orgID,
		Name:           name,
		URL:            url,
		Type:           remoteType,
		AuthType:       authType,
		Created:        time.Now(),
	}

	if err := s.remoteTable.Append(newRemote); err != nil {
		return nil, err
	}

	// Update cache
	cachedRemote := s.remoteTable.Last()
	s.remotesByOrg[orgID] = append(s.remotesByOrg[orgID], cachedRemote)

	// Save secret if provided
	if token != "" {
		if err := s.secretTable.Append(&remoteSecret{ID: jsonldb.NewID(), RemoteID: id, Token: token}); err != nil {
			return nil, err
		}
	}

	return cachedRemote, nil
}

// GetRemote retrieves a remote by ID.
func (s *GitRemoteService) GetRemote(id jsonldb.ID) (*entity.GitRemote, error) {
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
	if remoteID.IsZero() {
		return "", fmt.Errorf("remote id cannot be empty")
	}
	for sec := range s.secretTable.Iter(0) {
		if sec.RemoteID == remoteID {
			return sec.Token, nil
		}
	}
	return "", nil
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
	var newRows []*entity.GitRemote
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

	// Remove secret
	allSecrets := slices.Collect(s.secretTable.Iter(0))
	var newSecrets []*remoteSecret
	for _, sec := range allSecrets {
		if sec.RemoteID != remoteID {
			newSecrets = append(newSecrets, sec)
		}
	}
	if len(newSecrets) != len(allSecrets) {
		_ = s.secretTable.Replace(newSecrets)
	}

	// Update cache
	newCache := make([]*entity.GitRemote, 0, len(s.remotesByOrg[orgID]))
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

	allRemotes := slices.Collect(s.remoteTable.Iter(0))
	for i := range allRemotes {
		if allRemotes[i].ID == remoteID {
			allRemotes[i].LastSync = time.Now()
			return s.remoteTable.Replace(allRemotes)
		}
	}
	return fmt.Errorf("remote not found")
}
