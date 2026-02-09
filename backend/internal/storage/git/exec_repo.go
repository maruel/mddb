// Implements Repository using os/exec git commands.

package git

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ExecRepo implements Repository using os/exec git commands.
type ExecRepo struct {
	dir          string
	defaultName  string
	defaultEmail string
	mu           sync.Mutex
}

func newExecRepo(ctx context.Context, dir, defaultName, defaultEmail string) (*ExecRepo, error) {
	r := &ExecRepo{
		dir:          dir,
		defaultName:  defaultName,
		defaultEmail: defaultEmail,
	}
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *ExecRepo) init(ctx context.Context) error {
	gitDir := filepath.Join(r.dir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := os.MkdirAll(r.dir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
			return fmt.Errorf("failed to create repo directory: %w", err)
		}
		if err := r.gitRun(ctx, "init"); err != nil {
			return fmt.Errorf("failed to initialize git repo: %w", err)
		}
		if err := r.gitRun(ctx, "config", "user.email", r.defaultEmail); err != nil {
			return fmt.Errorf("failed to configure git user.email: %w", err)
		}
		if err := r.gitRun(ctx, "config", "user.name", r.defaultName); err != nil {
			return fmt.Errorf("failed to configure git user.name: %w", err)
		}
	}
	return nil
}

// FS returns a read-only filesystem view of the repository's working directory.
func (r *ExecRepo) FS() fs.FS {
	return os.DirFS(r.dir)
}

// FSAtCommit returns a read-only filesystem view at a specific commit.
func (r *ExecRepo) FSAtCommit(ctx context.Context, hash string) fs.FS {
	return &commitFS{repo: r, ctx: ctx, hash: hash}
}

// CommitTx executes fn while holding a lock and commits the returned files atomically.
// If fn returns an error or no files, no commit is made.
func (r *ExecRepo) CommitTx(ctx context.Context, author Author, fn func() (msg string, files []string, err error)) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	msg, files, err := fn()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	return r.commit(ctx, author, msg, files)
}

func (r *ExecRepo) commit(ctx context.Context, author Author, message string, files []string) error {
	// Stage specified files
	args := append([]string{"add", "--"}, files...)
	if out, err := r.gitCombinedOutput(ctx, args...); err != nil {
		return fmt.Errorf("failed to stage files: %w\nOutput: %s", err, string(out))
	}

	// Check if there are staged changes
	out, err := r.gitCombinedOutput(ctx, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) == "" {
		return nil
	}

	// Use defaults if not provided
	name := author.Name
	email := author.Email
	if name == "" {
		name = r.defaultName
	}
	if email == "" {
		email = r.defaultEmail
	}

	authorStr := fmt.Sprintf("%s <%s>", name, email)
	if err := r.gitRun(ctx, "commit", "-m", message, "--author", authorStr); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// CommitCount returns the total number of commits in the repository.
func (r *ExecRepo) CommitCount(ctx context.Context) (int, error) {
	out, err := r.gitOutput(ctx, "rev-list", "--count", "HEAD")
	if err != nil {
		return 0, nil //nolint:nilerr // no commits yet is not an error
	}
	n := 0
	for _, b := range out {
		if b >= '0' && b <= '9' {
			n = n*10 + int(b-'0')
		}
	}
	return n, nil
}

// GetHistory returns commit history for a specific path, limited to n commits.
// n is capped at 1000. If n <= 0, defaults to 1000.
func (r *ExecRepo) GetHistory(ctx context.Context, path string, n int) ([]*Commit, error) {
	if n <= 0 || n > 1000 {
		n = 1000
	}

	// Use record separator (%x1e) between commits since body can contain newlines
	format := "%H%x00%an%x00%ae%x00%ai%x00%cn%x00%ce%x00%ci%x00%s%x00%b%x1e"
	args := []string{"log", "--pretty=format:" + format, fmt.Sprintf("-n%d", n), "--", path}
	out, err := r.gitCombinedOutput(ctx, args...)
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
func (r *ExecRepo) GetFileAtCommit(ctx context.Context, hash, filePath string) ([]byte, error) {
	fullPath := fmt.Sprintf("%s:%s", hash, filePath)
	out, err := r.gitOutput(ctx, "show", fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file at commit: %w", err)
	}
	return out, nil
}

// SetRemote adds or updates a remote in the repository.
// If url is empty, the remote is removed.
func (r *ExecRepo) SetRemote(ctx context.Context, name, url string) error {
	// Check if remote already exists
	out, err := r.gitCombinedOutput(ctx, "remote")
	exists := false
	if err == nil {
		for rem := range strings.SplitSeq(string(out), "\n") {
			if strings.TrimSpace(rem) == name {
				exists = true
				break
			}
		}
	}

	if url == "" {
		if exists {
			return r.gitRun(ctx, "remote", "remove", name)
		}
		return nil
	}

	if exists {
		return r.gitRun(ctx, "remote", "set-url", name, url)
	}
	return r.gitRun(ctx, "remote", "add", name, url)
}

// Push pushes changes to a remote repository.
func (r *ExecRepo) Push(ctx context.Context, remoteName, branch string) error {
	if branch == "" {
		branch = "master"
		// Check if current branch is different
		out, err := r.gitCombinedOutput(ctx, "rev-parse", "--abbrev-ref", "HEAD")
		if err == nil {
			branch = strings.TrimSpace(string(out))
		}
	}

	return r.gitRun(ctx, "push", remoteName, branch)
}

// gitCmd creates an exec.Cmd for git with standard environment settings.
func (r *ExecRepo) gitCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	return cmd
}

// gitRun executes a git command using a detached context with timeout.
//
// The command is NOT tied to the HTTP request's cancellation, allowing git
// operations to complete even if the client disconnects.
func (r *ExecRepo) gitRun(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	return r.gitCmd(ctx, args...).Run()
}

// gitOutput executes a git command and returns its stdout.
//
// The command is NOT tied to the HTTP request's cancellation, allowing git
// operations to complete even if the client disconnects.
func (r *ExecRepo) gitOutput(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	return r.gitCmd(ctx, args...).Output()
}

// gitCombinedOutput executes a git command and returns combined stdout/stderr.
//
// The command is NOT tied to the HTTP request's cancellation, allowing git
// operations to complete even if the client disconnects.
func (r *ExecRepo) gitCombinedOutput(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	return r.gitCmd(ctx, args...).CombinedOutput()
}

// commitFS implements fs.FS for a specific commit using os/exec git.
type commitFS struct {
	repo *ExecRepo
	ctx  context.Context
	hash string
}

func (c *commitFS) Open(name string) (fs.File, error) {
	// Clean the path
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		name = "."
	}

	// Check if it's a directory using git ls-tree
	out, err := c.repo.gitCombinedOutput(c.ctx, "ls-tree", c.hash, name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	line := strings.TrimSpace(string(out))
	if line == "" {
		// Could be a directory - check with trailing slash
		out, err = c.repo.gitCombinedOutput(c.ctx, "ls-tree", c.hash, name+"/")
		if err != nil || strings.TrimSpace(string(out)) == "" {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		// It's a directory
		return &commitDir{fs: nil, name: name}, nil
	}

	// Parse ls-tree output: "mode type hash\tname"
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	if parts[1] == "tree" {
		return &commitDir{fs: nil, name: name}, nil
	}

	// It's a file - get the content
	data, err := c.repo.GetFileAtCommit(c.ctx, c.hash, name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	return &memFile{name: filepath.Base(name), data: data}, nil
}
