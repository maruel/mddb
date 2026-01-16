package storage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GitService handles version control operations using git.
// All changes to pages and databases are automatically committed.
type GitService struct {
	repoDir string // Root directory (contains .git/)
}

// NewGitService initializes git repository in the given directory.
// If repo doesn't exist, it will be initialized.
func NewGitService(rootDir string) (*GitService, error) {
	gs := &GitService{repoDir: rootDir}

	// Check if .git exists
	gitDir := filepath.Join(rootDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		// Initialize repo
		if err := gs.execGit("init"); err != nil {
			return nil, fmt.Errorf("failed to initialize git repo: %w", err)
		}

		// Configure git user (required for commits)
		_ = gs.execGit("config", "user.email", "mddb@localhost")
		_ = gs.execGit("config", "user.name", "mddb")
	}

	return gs, nil
}

// CommitChange stages and commits a change to the repository.
// Pattern: "operation: resource_type resource_id - description"
// Example: "create: page 1 - Getting Started"
func (gs *GitService) CommitChange(operation, resourceType, resourceID, description string) error {
	// Stage all changes in pages directory
	if err := gs.execGit("add", "pages"); err != nil {
		return fmt.Errorf("failed to stage changes: %w", err)
	}

	// Check if there are staged changes
	status, err := gs.gitOutput("status", "--porcelain")
	if err != nil {
		return err
	}

	if strings.TrimSpace(status) == "" {
		// No changes to commit
		return nil
	}

	// Build commit message
	message := fmt.Sprintf("%s: %s %s - %s", operation, resourceType, resourceID, description)

	if err := gs.execGit("commit", "-m", message); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// GetHistory returns commit history for a specific resource.
// Returns list of commits (hash, author, timestamp, message).
func (gs *GitService) GetHistory(resourceType, resourceID string) ([]*Commit, error) {
	// Get commits that mention this resource
	pattern := fmt.Sprintf("%s %s", resourceType, resourceID)

	// Use git log with grep to find relevant commits
	output, err := gs.gitOutput("log", "--oneline", "--grep", pattern)
	if err != nil {
		// git log returns error if no matches, treat as empty history
		return []*Commit{}, nil
	}

	var commits []*Commit
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}

		hash := parts[0]
		message := parts[1]

		// Get full commit details
		details, err := gs.gitOutput("show", "-s", "--format=%ai", hash)
		if err != nil {
			continue
		}

		timestamp, err := time.Parse("2006-01-02 15:04:05 -0700", strings.TrimSpace(details))
		if err != nil {
			timestamp = time.Now()
		}

		commits = append(commits, &Commit{
			Hash:      hash,
			Message:   message,
			Timestamp: timestamp,
		})
	}

	return commits, nil
}

// GetCommit retrieves a specific commit with full details.
func (gs *GitService) GetCommit(hash string) (*CommitDetail, error) {
	// Get commit details
	output, err := gs.gitOutput("show", "-s", "--format=%H%n%ai%n%an%n%ae%n%s%n%b", hash)
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

	return &CommitDetail{
		Hash:      lines[0],
		Timestamp: timestamp,
		Author:    lines[2],
		Email:     lines[3],
		Subject:   lines[4],
		Body:      body,
	}, nil
}

// GetFileAtCommit retrieves the content of a file at a specific commit.
// Returns the file content as bytes.
func (gs *GitService) GetFileAtCommit(hash, filePath string) ([]byte, error) {
	// Use git show to get file at commit
	fullPath := fmt.Sprintf("%s:%s", hash, filePath)
	content, err := gs.gitOutputBytes("show", fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file at commit: %w", err)
	}
	return content, nil
}

// Commit represents a commit in git history.
type Commit struct {
	Hash      string    `json:"hash"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// CommitDetail contains full commit information.
type CommitDetail struct {
	Hash      string    `json:"hash"`
	Timestamp time.Time `json:"timestamp"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
}

// execGit executes a git command in the repo directory.
func (gs *GitService) execGit(args ...string) error {
	cmd := gs.newGitCmd(args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// gitOutput executes a git command and returns the output as string.
func (gs *GitService) gitOutput(args ...string) (string, error) {
	cmd := gs.newGitCmd(args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Some git commands exit with error even on success (e.g., grep with no matches)
		// Return output anyway for the caller to handle
	}
	return string(output), nil
}

// gitOutputBytes executes a git command and returns the output as bytes.
func (gs *GitService) gitOutputBytes(args ...string) ([]byte, error) {
	cmd := gs.newGitCmd(args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}

// newGitCmd creates a git command with isolated environment.
func (gs *GitService) newGitCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = gs.repoDir
	// Ignore system and global git config to ensure reproducibility and isolation
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	return cmd
}
