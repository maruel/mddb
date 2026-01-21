package handlers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

const gitRemoteName = "origin"

// GitRemoteHandler handles git remote operations.
type GitRemoteHandler struct {
	orgService *identity.OrganizationService
	gitService *infra.Git
	rootDir    string
}

// NewGitRemoteHandler creates a new git remote handler.
func NewGitRemoteHandler(orgService *identity.OrganizationService, gitService *infra.Git, rootDir string) *GitRemoteHandler {
	return &GitRemoteHandler{
		orgService: orgService,
		gitService: gitService,
		rootDir:    rootDir,
	}
}

// GetRemote returns the git remote for an organization, or null if none exists.
func (h *GitRemoteHandler) GetRemote(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.GetGitRemoteRequest) (*dto.GitRemoteResponse, error) {
	remote := h.orgService.GetGitRemote(orgID)
	if remote == nil {
		return nil, nil //nolint:nilnil // nil response with nil error indicates "no remote configured" which is a valid state
	}
	return gitRemoteToResponse(orgID, remote), nil
}

// SetRemote creates or updates the git remote for an organization.
func (h *GitRemoteHandler) SetRemote(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req *dto.SetGitRemoteRequest) (*dto.GitRemoteResponse, error) {
	remote, err := h.orgService.SetGitRemote(orgID, req.URL, req.Type, req.AuthType, req.Token)
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

	if err := h.gitService.AddRemote(orgDir, gitRemoteName, url); err != nil {
		return nil, fmt.Errorf("failed to add git remote: %w", err)
	}

	return gitRemoteToResponse(orgID, remote), nil
}

// Push pushes changes to the git remote.
func (h *GitRemoteHandler) Push(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.PushGitRemoteRequest) (*dto.OkResponse, error) {
	remote := h.orgService.GetGitRemote(orgID)
	if remote == nil {
		return nil, dto.NotFound("remote")
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

	_ = h.gitService.AddRemote(orgDir, gitRemoteName, url)

	if err := h.gitService.Push(orgDir, gitRemoteName, ""); err != nil {
		return nil, fmt.Errorf("failed to push to git remote: %w", err)
	}

	_ = h.orgService.UpdateGitRemoteLastSync(orgID)

	return &dto.OkResponse{Ok: true}, nil
}

// DeleteRemote deletes the git remote for an organization.
func (h *GitRemoteHandler) DeleteRemote(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.DeleteGitRemoteRequest) (*dto.OkResponse, error) {
	if h.orgService.GetGitRemote(orgID) == nil {
		return nil, dto.NotFound("remote")
	}

	if err := h.orgService.DeleteGitRemote(orgID); err != nil {
		return nil, err
	}

	// Also remove from local git repo
	orgDir := filepath.Join(h.rootDir, orgID.String())
	_ = h.execGitInDir(orgDir, "remote", "remove", gitRemoteName)

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

// gitRemoteToResponse converts an entity.GitRemote to a dto.GitRemoteResponse.
func gitRemoteToResponse(orgID jsonldb.ID, r *entity.GitRemote) *dto.GitRemoteResponse {
	resp := &dto.GitRemoteResponse{
		OrganizationID: orgID.String(),
		URL:            r.URL,
		Type:           r.Type,
		AuthType:       r.AuthType,
		Created:        r.Created.Format("2006-01-02T15:04:05Z07:00"),
	}
	if !r.LastSync.IsZero() {
		resp.LastSync = r.LastSync.Format("2006-01-02T15:04:05Z07:00")
	}
	return resp
}
