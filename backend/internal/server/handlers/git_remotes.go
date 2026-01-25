package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

const gitRemoteName = "origin"

// GitRemoteHandler handles git remote operations.
type GitRemoteHandler struct {
	wsService *identity.WorkspaceService
	fileStore *content.FileStoreService
}

// NewGitRemoteHandler creates a new git remote handler.
func NewGitRemoteHandler(wsService *identity.WorkspaceService, fileStore *content.FileStoreService) *GitRemoteHandler {
	return &GitRemoteHandler{
		wsService: wsService,
		fileStore: fileStore,
	}
}

// GetGitRemote returns the git remote for a workspace, or null if none exists.
func (h *GitRemoteHandler) GetGitRemote(ctx context.Context, wsID jsonldb.ID, _ *identity.User, _ *dto.GetGitRemoteRequest) (*dto.GitRemoteResponse, error) {
	ws, err := h.wsService.Get(wsID)
	if err != nil {
		return nil, err
	}
	if ws.GitRemote.IsZero() {
		return nil, nil //nolint:nilnil // nil response with nil error indicates "no remote configured" which is a valid state
	}
	return gitRemoteToResponse(wsID, &ws.GitRemote), nil
}

// UpdateGitRemote creates or updates the git remote for a workspace.
func (h *GitRemoteHandler) UpdateGitRemote(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.UpdateGitRemoteRequest) (*dto.GitRemoteResponse, error) {
	ws, err := h.wsService.Modify(wsID, func(ws *identity.Workspace) error {
		// Preserve existing timestamps on update
		created := ws.GitRemote.Created
		lastSync := ws.GitRemote.LastSync
		if ws.GitRemote.IsZero() {
			created = storage.Now()
		}

		ws.GitRemote = identity.GitRemote{
			URL:      req.URL,
			Type:     req.Type,
			AuthType: req.AuthType,
			Token:    req.Token,
			Created:  created,
			LastSync: lastSync,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Actually add it to the local git repo
	wsStore, err := h.fileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace store: %w", err)
	}
	repo := wsStore.Repo()

	url := req.URL

	// If token-based auth, inject token into URL if it's GitHub/GitLab
	if req.AuthType == "token" && req.Token != "" {
		if strings.Contains(url, "github.com") {
			url = strings.Replace(url, "https://github.com", fmt.Sprintf("https://x-access-token:%s@github.com", req.Token), 1)
		} else if strings.Contains(url, "gitlab.com") {
			url = strings.Replace(url, "https://gitlab.com", fmt.Sprintf("https://oauth2:%s@gitlab.com", req.Token), 1)
		}
	}

	if err := repo.SetRemote(ctx, gitRemoteName, url); err != nil {
		return nil, fmt.Errorf("failed to set git remote: %w", err)
	}

	return gitRemoteToResponse(wsID, &ws.GitRemote), nil
}

// PushGit pushes changes to the git remote.
func (h *GitRemoteHandler) PushGit(ctx context.Context, wsID jsonldb.ID, _ *identity.User, _ *dto.PushGitRequest) (*dto.OkResponse, error) {
	ws, err := h.wsService.Get(wsID)
	if err != nil {
		return nil, err
	}
	if ws.GitRemote.IsZero() {
		return nil, dto.NotFound("remote")
	}

	wsStore, err := h.fileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace store: %w", err)
	}
	repo := wsStore.Repo()

	url := ws.GitRemote.URL
	if ws.GitRemote.AuthType == "token" && ws.GitRemote.Token != "" {
		if strings.Contains(url, "github.com") {
			url = strings.Replace(url, "https://github.com", fmt.Sprintf("https://x-access-token:%s@github.com", ws.GitRemote.Token), 1)
		} else if strings.Contains(url, "gitlab.com") {
			url = strings.Replace(url, "https://gitlab.com", fmt.Sprintf("https://oauth2:%s@gitlab.com", ws.GitRemote.Token), 1)
		}
	}

	if err := repo.SetRemote(ctx, gitRemoteName, url); err != nil {
		return nil, fmt.Errorf("failed to set git remote: %w", err)
	}

	if err := repo.Push(ctx, gitRemoteName, ""); err != nil {
		return nil, fmt.Errorf("failed to push to git remote: %w", err)
	}

	// Update last sync time
	if _, err := h.wsService.Modify(wsID, func(ws *identity.Workspace) error {
		ws.GitRemote.LastSync = storage.Now()
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to update last sync time: %w", err)
	}

	return &dto.OkResponse{Ok: true}, nil
}

// DeleteGitRemote deletes the git remote for a workspace.
func (h *GitRemoteHandler) DeleteGitRemote(ctx context.Context, wsID jsonldb.ID, _ *identity.User, _ *dto.DeleteGitRequest) (*dto.OkResponse, error) {
	_, err := h.wsService.Modify(wsID, func(ws *identity.Workspace) error {
		if ws.GitRemote.IsZero() {
			return dto.NotFound("remote")
		}
		ws.GitRemote = identity.GitRemote{}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Also remove from local git repo
	wsStore, err := h.fileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace store: %w", err)
	}
	repo := wsStore.Repo()

	if err := repo.SetRemote(ctx, gitRemoteName, ""); err != nil {
		return nil, fmt.Errorf("failed to remove git remote: %w", err)
	}

	return &dto.OkResponse{Ok: true}, nil
}
