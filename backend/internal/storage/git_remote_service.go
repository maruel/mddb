package storage

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/models"
)

// GitRemoteService handles git remote configuration and secrets.
type GitRemoteService struct {
	rootDir      string
	remoteTable  *jsonldb.Table[models.GitRemote]
	secretTable  *jsonldb.Table[remoteSecret]
	mu           sync.RWMutex
	remotesByOrg map[string][]*models.GitRemote
}

type remoteSecret struct {
	RemoteID string `json:"remote_id"`
	Token    string `json:"token"`
}

// NewGitRemoteService creates a new git remote service.
func NewGitRemoteService(rootDir string) (*GitRemoteService, error) {
	remotePath := filepath.Join(rootDir, "db", "git_remotes.jsonl")
	remoteTable, err := jsonldb.NewTable[models.GitRemote](remotePath)
	if err != nil {
		return nil, err
	}

	secretPath := filepath.Join(rootDir, "db", "git_remote_secrets.jsonl")
	secretTable, err := jsonldb.NewTable[remoteSecret](secretPath)
	if err != nil {
		return nil, err
	}

	s := &GitRemoteService{
		rootDir:      rootDir,
		remoteTable:  remoteTable,
		secretTable:  secretTable,
		remotesByOrg: make(map[string][]*models.GitRemote),
	}

	for i := range remoteTable.Rows {
		r := &remoteTable.Rows[i]
		s.remotesByOrg[r.OrganizationID] = append(s.remotesByOrg[r.OrganizationID], r)
	}

	return s, nil
}

// ListRemotes returns all remotes for an organization.
func (s *GitRemoteService) ListRemotes(orgID string) ([]*models.GitRemote, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.remotesByOrg[orgID], nil
}

// CreateRemote creates a new git remote.
func (s *GitRemoteService) CreateRemote(orgID, name, url, remoteType, authType, token string) (*models.GitRemote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := generateShortID()
	remote := &models.GitRemote{
		ID:             id,
		OrganizationID: orgID,
		Name:           name,
		URL:            url,
		Type:           remoteType,
		AuthType:       authType,
		Created:        time.Now(),
	}

	if err := s.remoteTable.Append(*remote); err != nil {
		return nil, err
	}

	// Update cache
	s.remoteTable.Mu.RLock()
	newRemote := &s.remoteTable.Rows[len(s.remoteTable.Rows)-1]
	s.remoteTable.Mu.RUnlock()
	s.remotesByOrg[orgID] = append(s.remotesByOrg[orgID], newRemote)

	// Save secret if provided
	if token != "" {
		if err := s.secretTable.Append(remoteSecret{RemoteID: id, Token: token}); err != nil {
			return nil, err
		}
	}

	return newRemote, nil
}

// GetRemote retrieves a remote by ID.
func (s *GitRemoteService) GetRemote(id string) (*models.GitRemote, error) {
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
func (s *GitRemoteService) GetToken(remoteID string) (string, error) {
	s.secretTable.Mu.RLock()
	defer s.secretTable.Mu.RUnlock()

	for i := range s.secretTable.Rows {
		if s.secretTable.Rows[i].RemoteID == remoteID {
			return s.secretTable.Rows[i].Token, nil
		}
	}
	return "", nil
}

// DeleteRemote deletes a git remote.
func (s *GitRemoteService) DeleteRemote(orgID, remoteID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove from table
	var newRows []models.GitRemote
	found := false
	for i := range s.remoteTable.Rows {
		if s.remoteTable.Rows[i].ID == remoteID {
			found = true
			continue
		}
		newRows = append(newRows, s.remoteTable.Rows[i])
	}
	if !found {
		return fmt.Errorf("remote not found")
	}

	if err := s.remoteTable.Replace(newRows); err != nil {
		return err
	}

	// Remove secret
	var newSecrets []remoteSecret
	for _, sec := range s.secretTable.Rows {
		if sec.RemoteID == remoteID {
			continue
		}
		newSecrets = append(newSecrets, sec)
	}
	_ = s.secretTable.Replace(newSecrets)

	// Update cache
	var newCache []*models.GitRemote
	for _, r := range s.remotesByOrg[orgID] {
		if r.ID == remoteID {
			continue
		}
		newCache = append(newCache, r)
	}
	s.remotesByOrg[orgID] = newCache

	return nil
}

// UpdateLastSync updates the last sync time for a remote.
func (s *GitRemoteService) UpdateLastSync(remoteID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.remoteTable.Rows {
		if s.remoteTable.Rows[i].ID == remoteID {
			s.remoteTable.Rows[i].LastSync = time.Now()
			return s.remoteTable.Replace(s.remoteTable.Rows)
		}
	}
	return fmt.Errorf("remote not found")
}
