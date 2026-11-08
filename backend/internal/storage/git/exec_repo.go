// Implements Repository using os/exec git commands.

package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	saved        SavedFiles
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
	if err := ctx.Err(); err != nil {
		return err
	}
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
	} else if err != nil {
		return fmt.Errorf("inspect git directory: %w", err)
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

// ReadTx reads saved files while excluding repository-owned mutations, independently of Git metadata.
func (r *ExecRepo) ReadTx(ctx context.Context, fn func(*SavedFiles) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(&r.saved)
}

// CommitTx executes a serialized mutation and records its changed files.
func (r *ExecRepo) CommitTx(ctx context.Context, author Author, fn func() (msg string, files []string, err error)) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	msg, files, err := fn()
	r.saved.invalidate(files, err != nil)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	// The callback has saved its changes; finish recording them even if its
	// request disconnects, within one deadline for the complete commit.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	return r.commit(ctx, author, msg, files)
}

func (r *ExecRepo) commit(ctx context.Context, author Author, message string, files []string) error {
	// Recovery can have only already-staged deletions and no remaining paths.
	if len(files) > 0 {
		args := append([]string{"add", "--"}, files...)
		if out, err := r.gitCombinedOutput(ctx, args...); err != nil {
			return fmt.Errorf("failed to stage files: %w\nOutput: %s", err, string(out))
		}
	}

	// Check if there are staged changes
	out, err := r.gitCombinedOutput(ctx, "diff", "--cached", "--quiet")
	if err == nil {
		return nil
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 1 {
		return fmt.Errorf("inspect staged changes: %w: %s", err, out)
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
		if unborn, probeErr := r.isUnborn(ctx); probeErr == nil && unborn {
			return 0, nil
		}
		return 0, fmt.Errorf("count commits: %w", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("invalid git commit count %q: %w", out, err)
	}
	if n < 0 {
		return 0, errors.New("git returned a negative commit count")
	}
	return n, nil
}

// isUnborn distinguishes an absent current branch from broken or unavailable
// repository metadata. A detached HEAD is never an unborn branch.
func (r *ExecRepo) isUnborn(ctx context.Context) (bool, error) {
	out, err := r.gitOutput(ctx, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok && exit.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	_, err = r.gitOutput(ctx, "show-ref", "--verify", "--quiet", strings.TrimSpace(string(out)))
	if exit, ok := errors.AsType[*exec.ExitError](err); ok && exit.ExitCode() == 1 {
		return true, nil
	}
	return false, err
}

// GetHistory returns commit history for a specific path, limited to n commits.
// n is capped at 1000. If n <= 0, defaults to 1000.
func (r *ExecRepo) GetHistory(ctx context.Context, path string, n int) ([]*Commit, error) {
	if n <= 0 || n > 1000 {
		n = 1000
	}

	// Use record separator (%x1e) between commits since body can contain newlines
	format := "%H%x00%an%x00%ae%x00%aI%x00%cn%x00%ce%x00%cI%x00%s%x00%b%x00"
	args := []string{"log", "--pretty=format:" + format, fmt.Sprintf("-n%d", n), "--", path}
	out, err := r.gitCombinedOutput(ctx, args...)
	if err != nil {
		if unborn, probeErr := r.isUnborn(ctx); probeErr == nil && unborn {
			return nil, nil
		}
		return nil, fmt.Errorf("read commit history: %w", err)
	}

	var commits []*Commit
	fields := strings.Split(string(out), "\x00")
	// NUL terminates each field, including the commit body. Commit messages may
	// contain record separators, so their text cannot serve as a record delimiter.
	if strings.TrimSpace(fields[len(fields)-1]) != "" {
		return nil, errors.New("unterminated git history record")
	}
	fields = fields[:len(fields)-1]
	if len(fields)%9 != 0 {
		return nil, errors.New("invalid git history field count")
	}
	for i := 0; i < len(fields); i += 9 {
		p := fields[i : i+9]
		authorDate, err := time.Parse(time.RFC3339, p[3])
		if err != nil {
			return nil, fmt.Errorf("parse commit author date: %w", err)
		}
		commitDate, err := time.Parse(time.RFC3339, p[6])
		if err != nil {
			return nil, fmt.Errorf("parse commit committer date: %w", err)
		}
		commits = append(commits, &Commit{Hash: strings.TrimSpace(p[0]), Author: p[1], AuthorEmail: p[2], AuthorDate: authorDate, Committer: p[4], CommitterEmail: p[5], CommitDate: commitDate, Message: p[7], Body: strings.TrimSpace(p[8])})
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
	if err != nil {
		return fmt.Errorf("list remotes: %w", err)
	}
	exists := false
	for rem := range strings.SplitSeq(string(out), "\n") {
		if strings.TrimSpace(rem) == name {
			exists = true
			break
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
		// Check if current branch is different
		out, err := r.gitCombinedOutput(ctx, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return fmt.Errorf("resolve current branch: %w", err)
		}
		branch = strings.TrimSpace(string(out))
	}

	// Use HEAD:<branch> so the current local branch is pushed to the
	// configured remote branch even when their names differ (e.g. local
	// "master" → remote "main").
	refspec := "HEAD:refs/heads/" + branch
	out, err := r.gitCombinedOutput(ctx, "push", remoteName, refspec)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Fetch fetches from a remote repository.
func (r *ExecRepo) Fetch(ctx context.Context, remoteName, branch string) error {
	args := []string{"fetch", remoteName}
	if branch != "" {
		args = append(args, branch)
	}
	out, err := r.gitCombinedOutput(ctx, args...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Pull fetches and merges from a remote. Returns true if files changed.
func (r *ExecRepo) Pull(ctx context.Context, remoteName, branch string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.saved.invalidate(nil, true)
	if branch == "" {
		out, err := r.gitCombinedOutput(ctx, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return false, fmt.Errorf("resolve current branch: %w", err)
		}
		branch = strings.TrimSpace(string(out))
	}

	// Get HEAD before merge.
	headBefore, err := r.gitOutput(ctx, "rev-parse", "HEAD")
	if err != nil {
		return false, fmt.Errorf("resolve HEAD before pull: %w", err)
	}

	// Fetch first.
	if err := r.Fetch(ctx, remoteName, branch); err != nil {
		return false, fmt.Errorf("fetch failed: %w", err)
	}

	// Merge.
	ref := remoteName + "/" + branch
	out, err := r.gitCombinedOutput(ctx, "merge", ref, "--no-edit")
	if err != nil {
		// Check for conflicts.
		hasConflicts, inspectErr := r.HasUnmergedFiles(ctx)
		if inspectErr != nil {
			return false, errors.Join(fmt.Errorf("merge failed: %w", err), inspectErr)
		}
		if hasConflicts {
			return false, errors.Join(fmt.Errorf("merge conflicts detected: %w: %s", err, strings.TrimSpace(string(out))), r.abortMergeLocked(ctx))
		}
		return false, fmt.Errorf("merge failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	// Check if HEAD changed.
	headAfter, err := r.gitOutput(ctx, "rev-parse", "HEAD")
	if err != nil {
		return false, fmt.Errorf("resolve HEAD after pull: %w", err)
	}
	changed := !bytes.Equal(headBefore, headAfter)
	return changed, nil
}

// HasUnmergedFiles returns true if there are unmerged files (merge conflicts).
func (r *ExecRepo) HasUnmergedFiles(ctx context.Context) (bool, error) {
	out, err := r.gitCombinedOutput(ctx, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// AbortMerge aborts an in-progress merge.
func (r *ExecRepo) AbortMerge(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.abortMergeLocked(ctx)
}

func (r *ExecRepo) abortMergeLocked(ctx context.Context) error {
	defer r.saved.invalidate(nil, true)
	return r.gitRun(ctx, "merge", "--abort")
}

// gitCmd creates an exec.Cmd for git with standard environment settings.
func (r *ExecRepo) gitCmd(ctx context.Context, args ...string) *exec.Cmd {
	// Keep automatic maintenance inside the command's lifetime. Detached Git
	// processes can otherwise keep writing after a commit returns or race with
	// workspace deletion. Command-line settings also cover existing repositories.
	// Explicit paths prevent an unavailable child repository from silently
	// resolving its parent's metadata. Git also resolves absorbed .git files here.
	args = append([]string{"--git-dir=.git", "--work-tree=.", "-c", "gc.autoDetach=false", "-c", "maintenance.autoDetach=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: args are internal, not user input
	cmd.WaitDelay = time.Second
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	return cmd
}

// gitRun executes a command with the owner's cancellation and a bounded timeout.
func (r *ExecRepo) gitRun(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	err := r.gitCmd(ctx, args...).Run()
	return errors.Join(err, ctx.Err())
}

// gitOutput reads stdout with the owner's cancellation and a bounded timeout.
func (r *ExecRepo) gitOutput(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	out, err := r.gitCmd(ctx, args...).Output()
	return out, errors.Join(err, ctx.Err())
}

// gitCombinedOutput reads output with the owner's cancellation and a bounded timeout.
func (r *ExecRepo) gitCombinedOutput(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	out, err := r.gitCmd(ctx, args...).CombinedOutput()
	return out, errors.Join(err, ctx.Err())
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
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	line := strings.TrimSpace(string(out))
	if line == "" {
		// Could be a directory - check with trailing slash
		out, err = c.repo.gitCombinedOutput(c.ctx, "ls-tree", c.hash, name+"/")
		if err != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: err}
		}
		if strings.TrimSpace(string(out)) == "" {
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
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	return &memFile{name: filepath.Base(name), data: data}, nil
}
