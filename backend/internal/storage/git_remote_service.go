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
	remotesByOrg map[jsonldb.ID][]*models.GitRemote
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
		remotesByOrg: make(map[jsonldb.ID][]*models.GitRemote),
	}

	for i := range remoteTable.Len() {
		r := remoteTable.At(i)
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

	if err := s.remoteTable.Append(*newRemote); err != nil {
		return nil, err
	}

	// Update cache
	s.remoteTable.RLock()
	cachedRemote := s.remoteTable.At(s.remoteTable.Len() - 1)
	s.remoteTable.RUnlock()
	s.remotesByOrg[orgID] = append(s.remotesByOrg[orgID], cachedRemote)

	// Save secret if provided
	if token != "" {
		if err := s.secretTable.Append(remoteSecret{RemoteID: id.String(), Token: token}); err != nil {
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
func (s *GitRemoteService) GetToken(remoteID string) (string, error) {
	s.secretTable.RLock()
	defer s.secretTable.RUnlock()

	for i := range s.secretTable.Len() {
		if s.secretTable.At(i).RemoteID == remoteID {
			return s.secretTable.At(i).Token, nil
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
	allRemotes := s.remoteTable.All()
	var newRows []models.GitRemote
	found := false
	for i := range allRemotes {
		if allRemotes[i].ID == remoteID {
			found = true
			continue
		}
		newRows = append(newRows, allRemotes[i])
	}
	if !found {
		return fmt.Errorf("remote not found")
	}

	if err := s.remoteTable.Replace(newRows); err != nil {
		return err
	}

	// Remove secret
	allSecrets := s.secretTable.All()
	var newSecrets []remoteSecret
	for _, sec := range allSecrets {
		if sec.RemoteID != remoteIDStr {
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

	allRemotes := s.remoteTable.All()
	for i := range allRemotes {
		if allRemotes[i].ID == remoteID {
			allRemotes[i].LastSync = time.Now()
			return s.remoteTable.Replace(allRemotes)
		}
	}
	return fmt.Errorf("remote not found")
}
