package handlers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

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
}

// NewGitRemoteHandler creates a new git remote handler.
func NewGitRemoteHandler(orgService *identity.OrganizationService, gitService *infra.Git) *GitRemoteHandler {
	return &GitRemoteHandler{
		orgService: orgService,
		gitService: gitService,
	}
}

// GetRemote returns the git remote for an organization, or null if none exists.
func (h *GitRemoteHandler) GetRemote(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.GetGitRemoteRequest) (*dto.GitRemoteResponse, error) {
	org, err := h.orgService.Get(orgID)
	if err != nil {
		return nil, err
	}
	if org.GitRemote.IsZero() {
		return nil, nil //nolint:nilnil // nil response with nil error indicates "no remote configured" which is a valid state
	}
	return gitRemoteToResponse(orgID, &org.GitRemote), nil
}

// SetRemote creates or updates the git remote for an organization.
func (h *GitRemoteHandler) SetRemote(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req *dto.SetGitRemoteRequest) (*dto.GitRemoteResponse, error) {
	org, err := h.orgService.Get(orgID)
	if err != nil {
		return nil, err
	}

	// Preserve existing timestamps on update
	created := org.GitRemote.Created
	lastSync := org.GitRemote.LastSync
	if org.GitRemote.IsZero() {
		created = time.Now()
	}

	org.GitRemote = entity.GitRemote{
		URL:      req.URL,
		Type:     req.Type,
		AuthType: req.AuthType,
		Token:    req.Token,
		Created:  created,
		LastSync: lastSync,
	}

	if err := h.orgService.Update(org); err != nil {
		return nil, err
	}

	// Actually add it to the local git repo
	orgDir := h.gitService.OrgDir(orgID)
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

	return gitRemoteToResponse(orgID, &org.GitRemote), nil
}

// Push pushes changes to the git remote.
func (h *GitRemoteHandler) Push(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.PushGitRemoteRequest) (*dto.OkResponse, error) {
	org, err := h.orgService.Get(orgID)
	if err != nil {
		return nil, err
	}
	if org.GitRemote.IsZero() {
		return nil, dto.NotFound("remote")
	}

	orgDir := h.gitService.OrgDir(orgID)

	url := org.GitRemote.URL
	if org.GitRemote.AuthType == "token" && org.GitRemote.Token != "" {
		if strings.Contains(url, "github.com") {
			url = strings.Replace(url, "https://github.com", fmt.Sprintf("https://x-access-token:%s@github.com", org.GitRemote.Token), 1)
		} else if strings.Contains(url, "gitlab.com") {
			url = strings.Replace(url, "https://gitlab.com", fmt.Sprintf("https://oauth2:%s@gitlab.com", org.GitRemote.Token), 1)
		}
	}

	_ = h.gitService.AddRemote(orgDir, gitRemoteName, url)

	if err := h.gitService.Push(orgDir, gitRemoteName, ""); err != nil {
		return nil, fmt.Errorf("failed to push to git remote: %w", err)
	}

	// Update last sync time
	org.GitRemote.LastSync = time.Now()
	_ = h.orgService.Update(org)

	return &dto.OkResponse{Ok: true}, nil
}

// DeleteRemote deletes the git remote for an organization.
func (h *GitRemoteHandler) DeleteRemote(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.DeleteGitRemoteRequest) (*dto.OkResponse, error) {
	org, err := h.orgService.Get(orgID)
	if err != nil {
		return nil, err
	}
	if org.GitRemote.IsZero() {
		return nil, dto.NotFound("remote")
	}

	org.GitRemote = entity.GitRemote{}
	if err := h.orgService.Update(org); err != nil {
		return nil, err
	}

	// Also remove from local git repo
	orgDir := h.gitService.OrgDir(orgID)
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
