// Handles git remote configuration and synchronization.

package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/maruel/mddb/backend/internal/githubapp"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

const gitRemoteName = "origin"

// GitRemoteHandler handles git remote operations.
type GitRemoteHandler struct {
	Svc       *Services
	GitHubApp *githubapp.Client // may be nil
}

// GetGitRemote returns the git remote for a workspace, or null if none exists.
func (h *GitRemoteHandler) GetGitRemote(ctx context.Context, wsID jsonldb.ID, _ *identity.User, _ *dto.GetGitRemoteRequest) (*dto.GitRemoteResponse, error) {
	ws, err := h.Svc.Workspace.Get(wsID)
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
	ws, err := h.Svc.Workspace.Modify(wsID, func(ws *identity.Workspace) error {
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

	wsStore, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace store: %w", err)
	}
	repo := wsStore.Repo()

	url := git.InjectTokenInURL(req.URL, req.Token, req.Type)
	if err := repo.SetRemote(ctx, gitRemoteName, url); err != nil {
		return nil, fmt.Errorf("failed to set git remote: %w", err)
	}

	return gitRemoteToResponse(wsID, &ws.GitRemote), nil
}

// getAuthURL returns the remote URL with authentication injected.
// For github_app remotes, it fetches a fresh installation token.
func (h *GitRemoteHandler) getAuthURL(ctx context.Context, remote *identity.GitRemote) (string, error) {
	if remote.AuthType == "github_app" && h.GitHubApp != nil {
		token, _, err := h.GitHubApp.GetInstallationToken(ctx, remote.InstallationID)
		if err != nil {
			return "", fmt.Errorf("get installation token: %w", err)
		}
		return fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, remote.RepoOwner, remote.RepoName), nil
	}
	return git.InjectTokenInURL(remote.URL, remote.Token, remote.Type), nil
}

// PushGit pushes changes to the git remote.
func (h *GitRemoteHandler) PushGit(ctx context.Context, wsID jsonldb.ID, _ *identity.User, _ *dto.PushGitRequest) (*dto.OkResponse, error) {
	ws, err := h.Svc.Workspace.Get(wsID)
	if err != nil {
		return nil, err
	}
	if ws.GitRemote.IsZero() {
		return nil, dto.NotFound("remote")
	}

	wsStore, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace store: %w", err)
	}
	repo := wsStore.Repo()

	url, err := h.getAuthURL(ctx, &ws.GitRemote)
	if err != nil {
		return nil, err
	}
	if err := repo.SetRemote(ctx, gitRemoteName, url); err != nil {
		return nil, fmt.Errorf("failed to set git remote: %w", err)
	}
	if err := repo.Push(ctx, gitRemoteName, ws.GitRemote.Branch); err != nil {
		return nil, fmt.Errorf("failed to push to git remote: %w", err)
	}

	if _, err := h.Svc.Workspace.Modify(wsID, func(ws *identity.Workspace) error {
		ws.GitRemote.LastSync = storage.Now()
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to update last sync time: %w", err)
	}

	return &dto.OkResponse{Ok: true}, nil
}

// PullGit pulls changes from the git remote.
func (h *GitRemoteHandler) PullGit(ctx context.Context, wsID jsonldb.ID, _ *identity.User, _ *dto.PullGitRequest) (*dto.OkResponse, error) {
	if h.Svc.SyncService == nil {
		return nil, errors.New("sync service not available")
	}
	if err := h.Svc.SyncService.Pull(ctx, wsID); err != nil {
		return nil, err
	}
	return &dto.OkResponse{Ok: true}, nil
}

// GetSyncStatus returns the current sync status for a workspace.
func (h *GitRemoteHandler) GetSyncStatus(ctx context.Context, wsID jsonldb.ID, _ *identity.User, _ *dto.GetSyncStatusRequest) (*dto.GitSyncStatusResponse, error) {
	ws, err := h.Svc.Workspace.Get(wsID)
	if err != nil {
		return nil, err
	}
	return &dto.GitSyncStatusResponse{
		SyncStatus:    ws.GitRemote.SyncStatus,
		LastSync:      ws.GitRemote.LastSync,
		LastSyncError: ws.GitRemote.LastSyncError,
	}, nil
}

// SetupGitHubAppRemote configures a workspace to use a GitHub App installation for sync.
func (h *GitRemoteHandler) SetupGitHubAppRemote(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.SetupGitHubAppRemoteRequest) (*dto.GitRemoteResponse, error) {
	if h.GitHubApp == nil {
		return nil, errors.New("GitHub App not configured")
	}

	// Validate installation by fetching a token.
	_, _, err := h.GitHubApp.GetInstallationToken(ctx, req.InstallationID)
	if err != nil {
		return nil, fmt.Errorf("invalid installation: %w", err)
	}

	ws, err := h.Svc.Workspace.Modify(wsID, func(ws *identity.Workspace) error {
		created := ws.GitRemote.Created
		if ws.GitRemote.IsZero() {
			created = storage.Now()
		}
		ws.GitRemote = identity.GitRemote{
			URL:            fmt.Sprintf("https://github.com/%s/%s.git", req.RepoOwner, req.RepoName),
			Type:           "github",
			AuthType:       "github_app",
			InstallationID: req.InstallationID,
			RepoOwner:      req.RepoOwner,
			RepoName:       req.RepoName,
			Branch:         req.Branch,
			SyncStatus:     "idle",
			Created:        created,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Set up the git remote with an authenticated URL.
	wsStore, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace store: %w", err)
	}
	repo := wsStore.Repo()

	token, _, err := h.GitHubApp.GetInstallationToken(ctx, req.InstallationID)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}
	url := fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, req.RepoOwner, req.RepoName)
	if err := repo.SetRemote(ctx, gitRemoteName, url); err != nil {
		return nil, fmt.Errorf("failed to set git remote: %w", err)
	}

	return gitRemoteToResponse(wsID, &ws.GitRemote), nil
}

// ListGitHubAppRepos lists repositories accessible to a GitHub App installation.
func (h *GitRemoteHandler) ListGitHubAppRepos(ctx context.Context, _ *identity.User, req *dto.ListGitHubAppReposRequest) (*dto.ListGitHubAppReposResponse, error) {
	if h.GitHubApp == nil {
		return nil, errors.New("GitHub App not configured")
	}

	repos, err := h.GitHubApp.ListInstallationRepos(ctx, req.InstallationID)
	if err != nil {
		return nil, err
	}

	dtoRepos := make([]dto.GitHubAppRepoResponse, len(repos))
	for i, r := range repos {
		dtoRepos[i] = dto.GitHubAppRepoResponse{
			FullName: r.FullName,
			Owner:    r.Owner,
			Name:     r.Name,
			Private:  r.Private,
			HTMLURL:  r.HTMLURL,
		}
	}
	return &dto.ListGitHubAppReposResponse{Repos: dtoRepos}, nil
}

// ListGitHubAppInstallations lists all installations of the GitHub App.
func (h *GitRemoteHandler) ListGitHubAppInstallations(ctx context.Context, _ *identity.User, _ *dto.ListGitHubAppInstallationsRequest) (*dto.ListGitHubAppInstallationsResponse, error) {
	if h.GitHubApp == nil {
		return nil, errors.New("GitHub App not configured")
	}

	installations, err := h.GitHubApp.ListInstallations(ctx)
	if err != nil {
		return nil, err
	}

	dtoInstallations := make([]dto.GitHubAppInstallationResponse, len(installations))
	for i, inst := range installations {
		dtoInstallations[i] = dto.GitHubAppInstallationResponse{
			ID:      inst.ID,
			Account: inst.Account,
		}
	}
	return &dto.ListGitHubAppInstallationsResponse{Installations: dtoInstallations}, nil
}

// IsGitHubAppAvailable returns whether the GitHub App is configured.
func (h *GitRemoteHandler) IsGitHubAppAvailable(_ context.Context, _ *dto.GitHubAppAvailableRequest) (*dto.GitHubAppAvailableResponse, error) {
	return &dto.GitHubAppAvailableResponse{Available: h.GitHubApp != nil}, nil
}

// DeleteGitRemote deletes the git remote for a workspace.
func (h *GitRemoteHandler) DeleteGitRemote(ctx context.Context, wsID jsonldb.ID, _ *identity.User, _ *dto.DeleteGitRequest) (*dto.OkResponse, error) {
	_, err := h.Svc.Workspace.Modify(wsID, func(ws *identity.Workspace) error {
		if ws.GitRemote.IsZero() {
			return dto.NotFound("remote")
		}
		ws.GitRemote = identity.GitRemote{}
		return nil
	})
	if err != nil {
		return nil, err
	}

	wsStore, err := h.Svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace store: %w", err)
	}
	repo := wsStore.Repo()

	if err := repo.SetRemote(ctx, gitRemoteName, ""); err != nil {
		return nil, fmt.Errorf("failed to remove git remote: %w", err)
	}

	return &dto.OkResponse{Ok: true}, nil
}
