package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Client handles version control operations using git.
// All changes to pages and databases are automatically committed.
type Client struct {
	repoDir string // Root directory (contains .git/)
}

// Commit represents a commit in git history.
type Commit struct {
	Hash           string    `json:"hash"`
	Message        string    `json:"message"` // Subject line.
	Author         string    `json:"author"`
	AuthorEmail    string    `json:"author_email"`
	AuthorDate     time.Time `json:"author_date"`
	Committer      string    `json:"committer"`
	CommitterEmail string    `json:"committer_email"`
	CommitDate     time.Time `json:"commit_date"`
}

// New initializes git service for the given root directory.
func New(rootDir string) (*Client, error) {
	gs := &Client{repoDir: rootDir}

	// Check if root .git exists, initialize if not
	if err := gs.InitRepository(""); err != nil {
		return nil, err
	}

	return gs, nil
}

// InitRepository initializes a git repository in the target subdirectory if it doesn't exist.
// If subdir is empty, initializes the root repository.
func (gs *Client) InitRepository(subdir string) error {
	dir := gs.repoDir
	if subdir != "" {
		dir = filepath.Join(gs.repoDir, subdir)
	}
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
// If subdir is non-empty, it commits to that subdirectory's repository.
func (gs *Client) CommitChange(ctx context.Context, subdir, operation, resourceType, resourceID, description string) error {
	targetDir := gs.repoDir
	relPath := "." // Default to root

	if subdir != "" {
		targetDir = filepath.Join(gs.repoDir, subdir)
		relPath = "pages" // Inside subdir, it's pages/
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

	// If we committed to a subdirectory repo, we should also update the root repo
	// if it's tracking the subdir as a submodule or directory
	if subdir != "" && targetDir != gs.repoDir {
		if err := gs.execGitInDir(gs.repoDir, "add", subdir); err == nil {
			_ = gs.execGitInDir(gs.repoDir, "commit", "-m", fmt.Sprintf("sync: %s update", subdir))
		}
	}

	return nil
}

// GetHistory returns commit history for a specific resource.
func (gs *Client) GetHistory(ctx context.Context, subdir, resourceType, resourceID string) ([]*Commit, error) {
	targetDir := gs.repoDir
	path := ""

	if subdir != "" {
		targetDir = filepath.Join(gs.repoDir, subdir)
		path = filepath.Join("pages", resourceID)
	} else {
		// Legacy path or system-wide resource
		path = filepath.Join("pages", resourceID)
		if _, err := os.Stat(filepath.Join(targetDir, path)); err != nil {
			path = resourceID // Try without pages/ prefix
		}
	}

	format := "%H%x00%an%x00%ae%x00%ai%x00%cn%x00%ce%x00%ci%x00%s"
	output, err := gs.gitOutputInDir(targetDir, "log", "--pretty=format:"+format, "--", path)
	if err != nil {
		return nil, nil //nolint:nilerr // git log returns error for paths with no history, which is not an error condition
	}

	var commits []*Commit
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "\x00")
		if len(parts) < 8 {
			continue
		}

		authorDate, _ := time.Parse("2006-01-02 15:04:05 -0700", parts[3])
		commitDate, _ := time.Parse("2006-01-02 15:04:05 -0700", parts[6])

		commits = append(commits, &Commit{
			Hash:           parts[0],
			Author:         parts[1],
			AuthorEmail:    parts[2],
			AuthorDate:     authorDate,
			Committer:      parts[4],
			CommitterEmail: parts[5],
			CommitDate:     commitDate,
			Message:        parts[7],
		})
	}

	return commits, nil
}

// GetFileAtCommit retrieves the content of a file at a specific commit.
func (gs *Client) GetFileAtCommit(ctx context.Context, subdir, hash, filePath string) ([]byte, error) {
	targetDir := gs.repoDir
	if subdir != "" {
		targetDir = filepath.Join(gs.repoDir, subdir)
		// filePath is already relative to targetDir in most cases
		// if it was passed as {subdir}/pages/... we need to strip it
		if strings.HasPrefix(filePath, subdir+"/") {
			filePath = strings.TrimPrefix(filePath, subdir+"/")
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
func (gs *Client) execGitInDir(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	return cmd.Run()
}

// gitOutputInDir executes a git command and returns output.
func (gs *Client) gitOutputInDir(dir string, args ...string) (string, error) {
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
func (gs *Client) gitOutputBytesInDir(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	return cmd.Output()
}

// AddRemote adds a remote to the repository for the given subdirectory.
func (gs *Client) AddRemote(subdir, name, url string) error {
	dir := gs.repoDir
	if subdir != "" {
		dir = filepath.Join(gs.repoDir, subdir)
	}
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

// Push pushes changes to a remote repository for the given subdirectory.
func (gs *Client) Push(subdir, remoteName, branch string) error {
	dir := gs.repoDir
	if subdir != "" {
		dir = filepath.Join(gs.repoDir, subdir)
	}
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

// RemoveRemote removes a remote from the repository for the given subdirectory.
func (gs *Client) RemoveRemote(subdir, name string) error {
	dir := gs.repoDir
	if subdir != "" {
		dir = filepath.Join(gs.repoDir, subdir)
	}
	return gs.execGitInDir(dir, "remote", "remove", name)
}
