package handlers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/maruel/mddb/backend/internal/models"
	"github.com/maruel/mddb/backend/internal/storage"
)

// GitRemoteHandler handles git remote operations.
type GitRemoteHandler struct {
	remoteService *storage.GitRemoteService
	gitService    *storage.GitService
	rootDir       string
}

// NewGitRemoteHandler creates a new git remote handler.
func NewGitRemoteHandler(remoteService *storage.GitRemoteService, gitService *storage.GitService, rootDir string) *GitRemoteHandler {
	return &GitRemoteHandler{
		remoteService: remoteService,
		gitService:    gitService,
		rootDir:       rootDir,
	}
}

// ListRemotes lists all git remotes for an organization.
func (h *GitRemoteHandler) ListRemotes(ctx context.Context, req models.ListGitRemotesRequest) (*models.ListGitRemotesResponse, error) {
	remotes, err := h.remoteService.ListRemotes(req.OrgID)
	if err != nil {
		return nil, err
	}
	return &models.ListGitRemotesResponse{Remotes: remotes}, nil
}

// CreateRemote creates a new git remote.
func (h *GitRemoteHandler) CreateRemote(ctx context.Context, req *models.CreateGitRemoteRequest) (*models.GitRemote, error) {
	remote, err := h.remoteService.CreateRemote(req.OrgID, req.Name, req.URL, req.Type, req.AuthType, req.Token)
	if err != nil {
		return nil, err
	}

	// Actually add it to the local git repo
	orgDir := filepath.Join(h.rootDir, req.OrgID)
	url := req.URL

	// If token-based auth, inject token into URL if it's GitHub/GitLab
	if req.AuthType == "token" && req.Token != "" {
		if strings.Contains(url, "github.com") {
			url = strings.Replace(url, "https://github.com", fmt.Sprintf("https://x-access-token:%s@github.com", req.Token), 1)
		} else if strings.Contains(url, "gitlab.com") {
			url = strings.Replace(url, "https://gitlab.com", fmt.Sprintf("https://oauth2:%s@gitlab.com", req.Token), 1)
		}
	}

	if err := h.gitService.AddRemote(orgDir, req.Name, url); err != nil {
		return nil, fmt.Errorf("failed to add git remote: %w", err)
	}

	return remote, nil
}

// Push pushes changes to a git remote.
func (h *GitRemoteHandler) Push(ctx context.Context, req models.PushGitRemoteRequest) (*any, error) {
	remote, err := h.remoteService.GetRemote(req.RemoteID)
	if err != nil {
		return nil, err
	}

	orgDir := filepath.Join(h.rootDir, req.OrgID)

	token, _ := h.remoteService.GetToken(req.RemoteID)
	url := remote.URL
	if remote.AuthType == "token" && token != "" {
		if strings.Contains(url, "github.com") {
			url = strings.Replace(url, "https://github.com", fmt.Sprintf("https://x-access-token:%s@github.com", token), 1)
		} else if strings.Contains(url, "gitlab.com") {
			url = strings.Replace(url, "https://gitlab.com", fmt.Sprintf("https://oauth2:%s@gitlab.com", token), 1)
		}
	}

	_ = h.gitService.AddRemote(orgDir, remote.Name, url)

	if err := h.gitService.Push(orgDir, remote.Name, ""); err != nil {
		return nil, fmt.Errorf("failed to push to git remote: %w", err)
	}

	_ = h.remoteService.UpdateLastSync(req.RemoteID)

	return nil, nil
}

// DeleteRemote deletes a git remote.
func (h *GitRemoteHandler) DeleteRemote(ctx context.Context, req models.DeleteGitRemoteRequest) (*any, error) {
	remote, err := h.remoteService.GetRemote(req.RemoteID)
	if err != nil {
		return nil, err
	}

	if err := h.remoteService.DeleteRemote(req.OrgID, req.RemoteID); err != nil {
		return nil, err
	}

	// Also remove from local git repo
	orgDir := filepath.Join(h.rootDir, req.OrgID)
	_ = h.execGitInDir(orgDir, "remote", "remove", remote.Name)

	return nil, nil
}

func (h *GitRemoteHandler) execGitInDir(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	return cmd.Run()
}
