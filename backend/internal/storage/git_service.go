package storage

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/maruel/mddb/backend/internal/models"
)

// GitService handles version control operations using git.
// All changes to pages and databases are automatically committed.
type GitService struct {
	repoDir string // Root directory (contains .git/)
}

// NewGitService initializes git service for the given root directory.
func NewGitService(rootDir string) (*GitService, error) {
	gs := &GitService{repoDir: rootDir}

	// Check if root .git exists, initialize if not
	if err := gs.InitRepository(rootDir); err != nil {
		return nil, err
	}

	return gs, nil
}

// InitRepository initializes a git repository in the target directory if it doesn't exist.
func (gs *GitService) InitRepository(dir string) error {
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		// Initialize repo
		cmd := exec.Command("git", "init")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
		)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to initialize git repo in %s: %w", dir, err)
		}

		// Configure git user
		configCmd := func(args ...string) {
			c := exec.Command("git", args...)
			c.Dir = dir
			c.Env = append(os.Environ(),
				"GIT_CONFIG_GLOBAL=/dev/null",
				"GIT_CONFIG_SYSTEM=/dev/null",
			)
			_ = c.Run()
		}
		configCmd("config", "user.email", "mddb@localhost")
		configCmd("config", "user.name", "mddb")
	}
	return nil
}

// CommitChange stages and commits a change to the repository.
// If orgID is present in context, it commits to the organization's repository.
func (gs *GitService) CommitChange(ctx context.Context, operation, resourceType, resourceID, description string) error {
	orgID := models.GetOrgID(ctx)
	targetDir := gs.repoDir
	relPath := "." // Default to root

	if orgID != "" {
		targetDir = filepath.Join(gs.repoDir, orgID)
		relPath = "pages" // Inside org dir, it's pages/
	}

	// Stage changes in the target directory
	if err := gs.execGitInDir(targetDir, "add", relPath); err != nil {
		// Fallback to adding everything if specific path fails
		_ = gs.execGitInDir(targetDir, "add", ".")
	}

	// Check if there are staged changes
	status, err := gs.gitOutputInDir(targetDir, "status", "--porcelain")
	if err != nil {
		return err
	}

	if strings.TrimSpace(status) == "" {
		return nil
	}

	// Build commit message
	message := fmt.Sprintf("%s: %s %s - %s", operation, resourceType, resourceID, description)

	if err := gs.execGitInDir(targetDir, "commit", "-m", message); err != nil {
		return fmt.Errorf("failed to commit in %s: %w", targetDir, err)
	}

	// If we committed to an organization repo, we should also update the root repo
	// if it's tracking the org as a submodule or directory
	if orgID != "" && targetDir != gs.repoDir {
		if err := gs.execGitInDir(gs.repoDir, "add", orgID); err == nil {
			_ = gs.execGitInDir(gs.repoDir, "commit", "-m", fmt.Sprintf("sync: org %s update", orgID))
		}
	}

	return nil
}

// GetHistory returns commit history for a specific resource.
func (gs *GitService) GetHistory(ctx context.Context, resourceType, resourceID string) ([]*models.Commit, error) {
	orgID := models.GetOrgID(ctx)
	targetDir := gs.repoDir
	path := ""

	if orgID != "" {
		targetDir = filepath.Join(gs.repoDir, orgID)
		path = filepath.Join("pages", resourceID)
	} else {
		// Legacy path or system-wide resource
		path = filepath.Join("pages", resourceID)
		if _, err := os.Stat(filepath.Join(targetDir, path)); err != nil {
			path = resourceID // Try without pages/ prefix
		}
	}

	format := "%H|%an|%ai|%s"
	output, err := gs.gitOutputInDir(targetDir, "log", "--pretty=format:"+format, "--", path)
	if err != nil {
		return []*models.Commit{}, nil
	}

	var commits []*models.Commit
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		hash := parts[0]
		timestampStr := parts[2]
		message := parts[3]

		timestamp, err := time.Parse("2006-01-02 15:04:05 -0700", timestampStr)
		if err != nil {
			timestamp = time.Now()
		}

		commits = append(commits, &models.Commit{
			Hash:      hash,
			Message:   message,
			Timestamp: timestamp,
		})
	}

	return commits, nil
}

// GetCommit retrieves a specific commit with full details.
func (gs *GitService) GetCommit(ctx context.Context, hash string) (*models.CommitDetail, error) {
	orgID := models.GetOrgID(ctx)
	targetDir := gs.repoDir
	if orgID != "" {
		targetDir = filepath.Join(gs.repoDir, orgID)
	}

	output, err := gs.gitOutputInDir(targetDir, "show", "-s", "--format=%H%n%ai%n%an%n%ae%n%s%n%b", hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 5 {
		return nil, fmt.Errorf("invalid commit format")
	}

	timestamp, err := time.Parse("2006-01-02 15:04:05 -0700", lines[1])
	if err != nil {
		timestamp = time.Now()
	}

	body := ""
	if len(lines) > 5 {
		body = strings.Join(lines[5:], "\n")
	}

	return &models.CommitDetail{
		Hash:      lines[0],
		Timestamp: timestamp,
		Author:    lines[2],
		Email:     lines[3],
		Subject:   lines[4],
		Body:      body,
	}, nil
}

// GetFileAtCommit retrieves the content of a file at a specific commit.
func (gs *GitService) GetFileAtCommit(ctx context.Context, hash, filePath string) ([]byte, error) {
	orgID := models.GetOrgID(ctx)
	targetDir := gs.repoDir
	if orgID != "" {
		targetDir = filepath.Join(gs.repoDir, orgID)
		// filePath is already relative to targetDir in most cases
		// if it was passed as {orgID}/pages/... we need to strip it
		if strings.HasPrefix(filePath, orgID+"/") {
			filePath = strings.TrimPrefix(filePath, orgID+"/")
		}
	}

	fullPath := fmt.Sprintf("%s:%s", hash, filePath)
	output, err := gs.gitOutputBytesInDir(targetDir, "show", fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file at commit in %s: %w", targetDir, err)
	}
	return output, nil
}

// execGitInDir executes a git command in a specific directory.
func (gs *GitService) execGitInDir(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	return cmd.Run()
}

// gitOutputInDir executes a git command and returns output.
func (gs *GitService) gitOutputInDir(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// gitOutputBytesInDir executes a git command and returns output as bytes.
func (gs *GitService) gitOutputBytesInDir(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	return cmd.Output()
}

// AddRemote adds a remote to the repository in the target directory.
func (gs *GitService) AddRemote(dir, name, url string) error {
	// Check if remote already exists
	remotes, err := gs.gitOutputInDir(dir, "remote")
	if err == nil {
		for _, r := range strings.Split(remotes, "\n") {
			if strings.TrimSpace(r) == name {
				// Remote exists, update URL
				return gs.execGitInDir(dir, "remote", "set-url", name, url)
			}
		}
	}

	return gs.execGitInDir(dir, "remote", "add", name, url)
}

// Push pushes changes to a remote repository.
func (gs *GitService) Push(dir, remoteName, branch string) error {
	if branch == "" {
		branch = "master" // Default to master
		// Check if current branch is main
		curr, err := gs.gitOutputInDir(dir, "rev-parse", "--abbrev-ref", "HEAD")
		if err == nil {
			branch = strings.TrimSpace(curr)
		}
	}

	return gs.execGitInDir(dir, "push", remoteName, branch)
}
