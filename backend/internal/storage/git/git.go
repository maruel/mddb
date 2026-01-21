package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Client handles version control operations using git.
type Client struct {
	root         string // Root directory (contains .git/ and subdirectories with their own repos).
	defaultName  string // Default author/committer name.
	defaultEmail string // Default author/committer email.
}

var errInvalidSubdir = errors.New("subdir path escapes root directory")

// Commit represents a commit in git history.
type Commit struct {
	Hash           string    `json:"hash"`
	Message        string    `json:"message"` // Subject line.
	Body           string    `json:"body"`    // Commit body (may be empty).
	Author         string    `json:"author"`
	AuthorEmail    string    `json:"author_email"`
	AuthorDate     time.Time `json:"author_date"`
	Committer      string    `json:"committer"`
	CommitterEmail string    `json:"committer_email"`
	CommitDate     time.Time `json:"commit_date"`
}

// New initializes git service for the given root directory.
func New(ctx context.Context, root, defaultName, defaultEmail string) (*Client, error) {
	if defaultName == "" {
		defaultName = "mddb"
	}
	if defaultEmail == "" {
		defaultEmail = "mddb@localhost"
	}
	c := &Client{root: root, defaultName: defaultName, defaultEmail: defaultEmail}

	// Check if root .git exists, initialize if not
	if err := c.Init(ctx, ""); err != nil {
		return nil, err
	}

	return c, nil
}

// Init initializes a git repository in the target subdirectory if it doesn't exist.
// If subdir is empty, initializes the root repository.
func (c *Client) Init(ctx context.Context, subdir string) error {
	dir, err := c.dir(subdir)
	if err != nil {
		return err
	}
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := git(ctx, dir, "init").Run(); err != nil {
			return fmt.Errorf("failed to initialize git repo in %s: %w", dir, err)
		}
		if err := git(ctx, dir, "config", "user.email", c.defaultEmail).Run(); err != nil {
			return fmt.Errorf("failed to configure git user.email in %s: %w", dir, err)
		}
		if err := git(ctx, dir, "config", "user.name", c.defaultName).Run(); err != nil {
			return fmt.Errorf("failed to configure git user.name in %s: %w", dir, err)
		}
	}
	return nil
}

// Commit stages the specified files and commits them to the repository.
// If subdir is non-empty, it commits to that subdirectory's repository.
// Files are paths relative to the subdir (or root if subdir is empty).
// If authorName or authorEmail are empty, defaults are used.
func (c *Client) Commit(ctx context.Context, subdir, authorName, authorEmail, message string, files []string) error {
	dir, err := c.dir(subdir)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		return nil
	}

	// Stage specified files
	args := append([]string{"add", "--"}, files...)
	if err := git(ctx, dir, args...).Run(); err != nil {
		return fmt.Errorf("failed to stage files in %s: %w", dir, err)
	}

	// Check if there are staged changes
	out, err := git(ctx, dir, "status", "--porcelain").CombinedOutput()
	if err != nil {
		return err
	}

	if strings.TrimSpace(string(out)) == "" {
		return nil
	}

	// Use defaults if not provided
	if authorName == "" {
		authorName = c.defaultName
	}
	if authorEmail == "" {
		authorEmail = c.defaultEmail
	}

	author := fmt.Sprintf("%s <%s>", authorName, authorEmail)

	if err := git(ctx, dir, "commit", "-m", message, "--author", author).Run(); err != nil {
		return fmt.Errorf("failed to commit in %s: %w", dir, err)
	}

	// If we committed to a subdirectory repo, we should also update the root repo
	// if it's tracking the subdir as a submodule or directory
	if subdir != "" {
		if err := git(ctx, c.root, "add", subdir).Run(); err != nil {
			return fmt.Errorf("failed to stage subdir %s in root: %w", subdir, err)
		}
		if err := git(ctx, c.root, "commit", "-m", fmt.Sprintf("sync: %s update", subdir), "--author", author).Run(); err != nil {
			return fmt.Errorf("failed to commit sync in root: %w", err)
		}
	}

	return nil
}

// GetHistory returns commit history for a specific path, limited to n commits.
// n is capped at 1000. If n <= 0, defaults to 1000.
func (c *Client) GetHistory(ctx context.Context, subdir, path string, n int) ([]*Commit, error) {
	dir, err := c.dir(subdir)
	if err != nil {
		return nil, err
	}

	if n <= 0 || n > 1000 {
		n = 1000
	}

	// Use record separator (%x1e) between commits since body can contain newlines
	format := "%H%x00%an%x00%ae%x00%ai%x00%cn%x00%ce%x00%ci%x00%s%x00%b%x1e"
	args := []string{"log", "--pretty=format:" + format, fmt.Sprintf("-n%d", n), "--", path}
	out, err := git(ctx, dir, args...).CombinedOutput()
	if err != nil {
		return nil, nil //nolint:nilerr // git log returns error for paths with no history, which is not an error condition
	}

	var commits []*Commit
	for record := range strings.SplitSeq(string(out), "\x1e") {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		parts := strings.Split(record, "\x00")
		if len(parts) < 9 {
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
			Body:           strings.TrimSpace(parts[8]),
		})
	}

	return commits, nil
}

// GetFileAtCommit retrieves the content of a file at a specific commit.
func (c *Client) GetFileAtCommit(ctx context.Context, subdir, hash, filePath string) ([]byte, error) {
	dir, err := c.dir(subdir)
	if err != nil {
		return nil, err
	}

	// Strip subdir prefix if present
	if subdir != "" {
		if after, found := strings.CutPrefix(filePath, subdir+"/"); found {
			filePath = after
		}
	}

	fullPath := fmt.Sprintf("%s:%s", hash, filePath)
	out, err := git(ctx, dir, "show", fullPath).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get file at commit in %s: %w", dir, err)
	}
	return out, nil
}

// SetRemote adds or updates a remote in the repository for the given subdirectory.
// If url is empty, the remote is removed.
func (c *Client) SetRemote(ctx context.Context, subdir, name, url string) error {
	dir, err := c.dir(subdir)
	if err != nil {
		return err
	}

	// Check if remote already exists
	out, err := git(ctx, dir, "remote").CombinedOutput()
	exists := false
	if err == nil {
		for r := range strings.SplitSeq(string(out), "\n") {
			if strings.TrimSpace(r) == name {
				exists = true
				break
			}
		}
	}

	if url == "" {
		if exists {
			return git(ctx, dir, "remote", "remove", name).Run()
		}
		return nil
	}

	if exists {
		return git(ctx, dir, "remote", "set-url", name, url).Run()
	}
	return git(ctx, dir, "remote", "add", name, url).Run()
}

// Push pushes changes to a remote repository for the given subdirectory.
func (c *Client) Push(ctx context.Context, subdir, remoteName, branch string) error {
	dir, err := c.dir(subdir)
	if err != nil {
		return err
	}

	if branch == "" {
		branch = "master" // Default to master
		// Check if current branch is main
		out, err := git(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
		if err == nil {
			branch = strings.TrimSpace(string(out))
		}
	}

	return git(ctx, dir, "push", remoteName, branch).Run()
}

// dir returns the absolute directory path for a subdir, validating it doesn't escape root.
func (c *Client) dir(subdir string) (string, error) {
	if subdir == "" {
		return c.root, nil
	}
	// Clean the path to resolve any .. or . components
	cleaned := filepath.Clean(subdir)
	// Reject absolute paths or paths that try to escape
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
		return "", errInvalidSubdir
	}
	return filepath.Join(c.root, cleaned), nil
}

// git returns a configured exec.Cmd for running git in the specified directory.
func git(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	return cmd
}
