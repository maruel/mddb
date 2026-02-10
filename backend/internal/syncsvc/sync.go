// Provides background sync operations (push/pull) between workspace git repos and remotes.

package syncsvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/githubapp"
	"github.com/maruel/mddb/backend/internal/rid"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// Service manages push/pull sync operations for workspaces.
type Service struct {
	wsSvc     *identity.WorkspaceService
	fileStore *content.FileStoreService
	githubApp *githubapp.Client // nil if not configured
	rootRepo  *git.RootRepo

	mu      sync.Mutex
	active  map[rid.ID]struct{}
	timers  map[rid.ID]*time.Timer
	cancels map[rid.ID]context.CancelFunc
}

// New creates a new sync service.
func New(wsSvc *identity.WorkspaceService, fileStore *content.FileStoreService, githubApp *githubapp.Client, rootRepo *git.RootRepo) *Service {
	return &Service{
		wsSvc:     wsSvc,
		fileStore: fileStore,
		githubApp: githubApp,
		rootRepo:  rootRepo,
		active:    make(map[rid.ID]struct{}),
		timers:    make(map[rid.ID]*time.Timer),
		cancels:   make(map[rid.ID]context.CancelFunc),
	}
}

const autoPushDebounce = 5 * time.Second

// TriggerPush starts an async debounced push for a workspace if GitAutoPush is enabled.
func (s *Service) TriggerPush(wsID rid.ID) {
	ws, err := s.wsSvc.Get(wsID)
	if err != nil || ws.GitRemote.IsZero() || !ws.Settings.GitAutoPush {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Cancel any existing timer.
	if cancel, ok := s.cancels[wsID]; ok {
		cancel()
	}
	if t, ok := s.timers[wsID]; ok {
		t.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	s.cancels[wsID] = cancel
	s.timers[wsID] = time.AfterFunc(autoPushDebounce, func() {
		defer cancel()
		if err := s.Push(ctx, wsID); err != nil {
			slog.Error("Auto-push failed", "wsID", wsID, "err", err)
		}
	})
}

// Push pushes workspace changes to the configured remote.
func (s *Service) Push(ctx context.Context, wsID rid.ID) error {
	if !s.tryAcquire(wsID) {
		return nil // another sync in progress
	}
	defer s.release(wsID)

	ws, err := s.wsSvc.Get(wsID)
	if err != nil {
		return err
	}
	if ws.GitRemote.IsZero() {
		return errors.New("no remote configured")
	}

	s.setSyncStatus(wsID, "syncing", "")

	wsStore, err := s.fileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		s.setSyncStatus(wsID, "error", err.Error())
		return fmt.Errorf("get workspace store: %w", err)
	}
	repo := wsStore.Repo()

	url, err := s.getAuthURL(ctx, ws)
	if err != nil {
		s.setSyncStatus(wsID, "error", err.Error())
		return err
	}

	if err := repo.SetRemote(ctx, "origin", url); err != nil {
		s.setSyncStatus(wsID, "error", err.Error())
		return fmt.Errorf("set remote: %w", err)
	}

	branch := ws.GitRemote.Branch
	if err := repo.Push(ctx, "origin", branch); err != nil {
		errStr := err.Error()
		// "everything up-to-date" is not an error.
		if errStr == "exit status 0" || strings.Contains(errStr, "up-to-date") || strings.Contains(errStr, "Everything up-to-date") {
			s.updateLastSync(wsID)
			return nil
		}
		s.setSyncStatus(wsID, "error", errStr)
		return fmt.Errorf("push: %w", err)
	}

	s.updateLastSync(wsID)
	return nil
}

// Pull fetches and merges from the remote into the workspace.
func (s *Service) Pull(ctx context.Context, wsID rid.ID) error {
	if !s.tryAcquire(wsID) {
		return nil
	}
	defer s.release(wsID)

	ws, err := s.wsSvc.Get(wsID)
	if err != nil {
		return err
	}
	if ws.GitRemote.IsZero() {
		return errors.New("no remote configured")
	}

	s.setSyncStatus(wsID, "syncing", "")

	wsStore, err := s.fileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		s.setSyncStatus(wsID, "error", err.Error())
		return fmt.Errorf("get workspace store: %w", err)
	}
	repo := wsStore.Repo()

	url, err := s.getAuthURL(ctx, ws)
	if err != nil {
		s.setSyncStatus(wsID, "error", err.Error())
		return err
	}

	if err := repo.SetRemote(ctx, "origin", url); err != nil {
		s.setSyncStatus(wsID, "error", err.Error())
		return fmt.Errorf("set remote: %w", err)
	}

	branch := ws.GitRemote.Branch
	_, err = repo.Pull(ctx, "origin", branch)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "conflict") {
			s.setSyncStatus(wsID, "conflict", errStr)
		} else {
			s.setSyncStatus(wsID, "error", errStr)
		}
		return fmt.Errorf("pull: %w", err)
	}

	s.updateLastSync(wsID)
	return nil
}

// getAuthURL returns the remote URL with authentication injected.
func (s *Service) getAuthURL(ctx context.Context, ws *identity.Workspace) (string, error) {
	remote := &ws.GitRemote

	if remote.AuthType == "github_app" && s.githubApp != nil {
		token, _, err := s.githubApp.GetInstallationToken(ctx, remote.InstallationID)
		if err != nil {
			return "", fmt.Errorf("get installation token: %w", err)
		}
		url := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, remote.RepoOwner, remote.RepoName)
		return url, nil
	}

	url := remote.URL
	if remote.AuthType == "token" && remote.Token != "" {
		url = git.InjectTokenInURL(url, remote.Token, remote.Type)
	}
	return url, nil
}

func (s *Service) tryAcquire(wsID rid.ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.active[wsID]; ok {
		return false
	}
	s.active[wsID] = struct{}{}
	return true
}

func (s *Service) release(wsID rid.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, wsID)
}

func (s *Service) setSyncStatus(wsID rid.ID, status, lastError string) {
	if _, err := s.wsSvc.Modify(wsID, func(ws *identity.Workspace) error {
		ws.GitRemote.SyncStatus = status
		ws.GitRemote.LastSyncError = lastError
		return nil
	}); err != nil {
		slog.Error("Failed to update sync status", "wsID", wsID, "err", err)
	}
}

func (s *Service) updateLastSync(wsID rid.ID) {
	if _, err := s.wsSvc.Modify(wsID, func(ws *identity.Workspace) error {
		ws.GitRemote.SyncStatus = "idle"
		ws.GitRemote.LastSyncError = ""
		ws.GitRemote.LastSync = storage.Now()
		return nil
	}); err != nil {
		slog.Error("Failed to update last sync", "wsID", wsID, "err", err)
	}
}
