package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/models"
)

// GitRemoteService handles git remote configuration and secrets.
type GitRemoteService struct {
	rootDir      string
	remoteTable  *jsonldb.Table[*models.GitRemote]
	secretTable  *jsonldb.Table[*remoteSecret]
	mu           sync.RWMutex
	remotesByOrg map[jsonldb.ID][]*models.GitRemote
}

type remoteSecret struct {
	ID       jsonldb.ID `json:"id"`
	RemoteID jsonldb.ID `json:"remote_id"`
	Token    string     `json:"token"`
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
	remoteTable, err := jsonldb.NewTable[*models.GitRemote](remotePath)
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
		remotesByOrg: make(map[jsonldb.ID][]*models.GitRemote),
	}

	for r := range remoteTable.All() {
		s.remotesByOrg[r.OrganizationID] = append(s.remotesByOrg[r.OrganizationID], r)
	}

	return s, nil
}

// ListRemotes returns all remotes for an organization.
func (s *GitRemoteService) ListRemotes(orgIDStr string) ([]*models.GitRemote, error) {
	orgID, err := jsonldb.DecodeID(orgIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid organization id: %w", err)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.remotesByOrg[orgID], nil
}

// CreateRemote creates a new git remote.
func (s *GitRemoteService) CreateRemote(orgIDStr, name, url, remoteType, authType, token string) (*models.GitRemote, error) {
	orgID, err := jsonldb.DecodeID(orgIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid organization id: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := jsonldb.NewID()
	newRemote := &models.GitRemote{
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
	cachedRemote, _ := s.remoteTable.Last()
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
func (s *GitRemoteService) GetRemote(idStr string) (*models.GitRemote, error) {
	id, err := jsonldb.DecodeID(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid remote id: %w", err)
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
func (s *GitRemoteService) GetToken(remoteIDStr string) (string, error) {
	remoteID, err := jsonldb.DecodeID(remoteIDStr)
	if err != nil {
		return "", fmt.Errorf("invalid remote id: %w", err)
	}
	for sec := range s.secretTable.All() {
		if sec.RemoteID == remoteID {
			return sec.Token, nil
		}
	}
	return "", nil
}

// DeleteRemote deletes a git remote.
func (s *GitRemoteService) DeleteRemote(orgIDStr, remoteIDStr string) error {
	orgID, err := jsonldb.DecodeID(orgIDStr)
	if err != nil {
		return fmt.Errorf("invalid organization id: %w", err)
	}
	remoteID, err := jsonldb.DecodeID(remoteIDStr)
	if err != nil {
		return fmt.Errorf("invalid remote id: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove from table
	var newRows []*models.GitRemote
	found := false
	for r := range s.remoteTable.All() {
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
	allSecrets := slices.Collect(s.secretTable.All())
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
	newCache := make([]*models.GitRemote, 0, len(s.remotesByOrg[orgID]))
	for _, r := range s.remotesByOrg[orgID] {
		if r.ID != remoteID {
			newCache = append(newCache, r)
		}
	}
	s.remotesByOrg[orgID] = newCache

	return nil
}

// UpdateLastSync updates the last sync time for a remote.
func (s *GitRemoteService) UpdateLastSync(remoteIDStr string) error {
	remoteID, err := jsonldb.DecodeID(remoteIDStr)
	if err != nil {
		return fmt.Errorf("invalid remote id: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	allRemotes := slices.Collect(s.remoteTable.All())
	for i := range allRemotes {
		if allRemotes[i].ID == remoteID {
			allRemotes[i].LastSync = time.Now()
			return s.remoteTable.Replace(allRemotes)
		}
	}
	return fmt.Errorf("remote not found")
}
