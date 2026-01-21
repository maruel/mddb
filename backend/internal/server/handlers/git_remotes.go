package handlers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

// GitRemoteHandler handles git remote operations.
type GitRemoteHandler struct {
	remoteService *infra.GitRemoteService
	gitService    *infra.Git
	rootDir       string
}

// NewGitRemoteHandler creates a new git remote handler.
func NewGitRemoteHandler(remoteService *infra.GitRemoteService, gitService *infra.Git, rootDir string) *GitRemoteHandler {
	return &GitRemoteHandler{
		remoteService: remoteService,
		gitService:    gitService,
		rootDir:       rootDir,
	}
}

// ListRemotes lists all git remotes for an organization.
func (h *GitRemoteHandler) ListRemotes(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.ListGitRemotesRequest) (*dto.ListGitRemotesResponse, error) {
	it, err := h.remoteService.Iter(orgID)
	if err != nil {
		return nil, err
	}
	remotes := slices.Collect(it)
	responses := make([]dto.GitRemoteResponse, 0, len(remotes))
	for _, r := range remotes {
		responses = append(responses, *gitRemoteToResponse(r))
	}
	return &dto.ListGitRemotesResponse{Remotes: responses}, nil
}

// CreateRemote creates a new git remote.
func (h *GitRemoteHandler) CreateRemote(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req *dto.CreateGitRemoteRequest) (*dto.GitRemoteResponse, error) {
	remote, err := h.remoteService.Create(orgID, req.Name, req.URL, req.Type, req.AuthType, req.Token)
	if err != nil {
		return nil, err
	}

	// Actually add it to the local git repo
	orgDir := filepath.Join(h.rootDir, orgID.String())
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

	return gitRemoteToResponse(remote), nil
}

// Push pushes changes to a git remote.
func (h *GitRemoteHandler) Push(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.PushGitRemoteRequest) (*dto.OkResponse, error) {
	remoteID, err := decodeID(req.RemoteID, "remote_id")
	if err != nil {
		return nil, err
	}

	remote, err := h.remoteService.Get(remoteID)
	if err != nil {
		return nil, err
	}

	orgDir := filepath.Join(h.rootDir, orgID.String())

	url := remote.URL
	if remote.AuthType == "token" && remote.Token != "" {
		if strings.Contains(url, "github.com") {
			url = strings.Replace(url, "https://github.com", fmt.Sprintf("https://x-access-token:%s@github.com", remote.Token), 1)
		} else if strings.Contains(url, "gitlab.com") {
			url = strings.Replace(url, "https://gitlab.com", fmt.Sprintf("https://oauth2:%s@gitlab.com", remote.Token), 1)
		}
	}

	_ = h.gitService.AddRemote(orgDir, remote.Name, url)

	if err := h.gitService.Push(orgDir, remote.Name, ""); err != nil {
		return nil, fmt.Errorf("failed to push to git remote: %w", err)
	}

	_ = h.remoteService.UpdateLastSync(remoteID)

	return &dto.OkResponse{Ok: true}, nil
}

// DeleteRemote deletes a git remote.
func (h *GitRemoteHandler) DeleteRemote(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.DeleteGitRemoteRequest) (*dto.OkResponse, error) {
	remoteID, err := decodeID(req.RemoteID, "remote_id")
	if err != nil {
		return nil, err
	}

	remote, err := h.remoteService.Get(remoteID)
	if err != nil {
		return nil, err
	}

	if err := h.remoteService.Delete(orgID, remoteID); err != nil {
		return nil, err
	}

	// Also remove from local git repo
	orgDir := filepath.Join(h.rootDir, orgID.String())
	_ = h.execGitInDir(orgDir, "remote", "remove", remote.Name)

	return &dto.OkResponse{Ok: true}, nil
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
