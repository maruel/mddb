// Owns saved workspace history outcomes, scoped reconciliation, and recovery lifecycle.

package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"

	gogit "github.com/go-git/go-git/v5"

	"github.com/maruel/ksid"
)

type workspaceRepo struct {
	Repository
	dir        string
	gitPointer string
	pending    atomic.Bool
	mu         contextGate
	retired    bool
}

func (r *workspaceRepo) CommitTx(ctx context.Context, author Author, fn func() (string, []string, error)) error {
	if err := r.mu.LockContext(ctx); err != nil {
		return err
	}
	defer r.mu.Unlock()
	if err := r.refreshLocked(ctx); err != nil {
		if ctx.Err() == nil {
			r.pending.Store(true)
		}
		return err
	}
	saved := false
	called := false
	err := r.Repository.CommitTx(ctx, author, func() (string, []string, error) {
		called = true
		msg, files, err := fn()
		saved = err == nil && len(files) > 0
		return msg, files, err
	})
	if err != nil && !called && ctx.Err() == nil {
		r.pending.Store(true)
	}
	if err != nil && saved {
		r.pending.Store(true)
		slog.ErrorContext(ctx, "Saved workspace changes await history recovery", "err", err)
		return nil
	}
	// A later normal commit need not include an older failed mutation. Recovery
	// alone clears pending after reconciling the complete saved snapshot.
	return err
}

func (r *workspaceRepo) Push(ctx context.Context, remote, branch string) error {
	if err := r.mu.LockContext(ctx); err != nil {
		return err
	}
	defer r.mu.Unlock()
	if err := r.refreshLocked(ctx); err != nil {
		return err
	}
	if err := r.recoverLocked(ctx); err != nil {
		return fmt.Errorf("workspace history pending recovery: %w", err)
	}
	return r.Repository.Push(ctx, remote, branch)
}

// FS returns the saved working tree independently of Git metadata.
func (r *workspaceRepo) FS() fs.FS { return os.DirFS(r.dir) }

func (r *workspaceRepo) FSAtCommit(ctx context.Context, hash string) fs.FS {
	return &workspaceCommitFS{repo: r, ctx: ctx, hash: hash}
}

func (r *workspaceRepo) CommitCount(ctx context.Context) (int, error) {
	if err := r.mu.LockContext(ctx); err != nil {
		return 0, err
	}
	defer r.mu.Unlock()
	if err := r.refreshLocked(ctx); err != nil {
		return 0, err
	}
	return r.Repository.CommitCount(ctx)
}

func (r *workspaceRepo) GetHistory(ctx context.Context, path string, n int) ([]*Commit, error) {
	if err := r.mu.LockContext(ctx); err != nil {
		return nil, err
	}
	defer r.mu.Unlock()
	if err := r.refreshLocked(ctx); err != nil {
		return nil, err
	}
	return r.Repository.GetHistory(ctx, path, n)
}

func (r *workspaceRepo) GetFileAtCommit(ctx context.Context, hash, path string) ([]byte, error) {
	if err := r.mu.LockContext(ctx); err != nil {
		return nil, err
	}
	defer r.mu.Unlock()
	if err := r.refreshLocked(ctx); err != nil {
		return nil, err
	}
	return r.Repository.GetFileAtCommit(ctx, hash, path)
}

func (r *workspaceRepo) SetRemote(ctx context.Context, name, url string) error {
	if err := r.mu.LockContext(ctx); err != nil {
		return err
	}
	defer r.mu.Unlock()
	if err := r.refreshLocked(ctx); err != nil {
		return err
	}
	return r.Repository.SetRemote(ctx, name, url)
}

func (r *workspaceRepo) Fetch(ctx context.Context, name, branch string) error {
	if err := r.mu.LockContext(ctx); err != nil {
		return err
	}
	defer r.mu.Unlock()
	if err := r.refreshLocked(ctx); err != nil {
		return err
	}
	return r.Repository.Fetch(ctx, name, branch)
}

func (r *workspaceRepo) Pull(ctx context.Context, name, branch string) (bool, error) {
	if err := r.mu.LockContext(ctx); err != nil {
		return false, err
	}
	defer r.mu.Unlock()
	if err := r.refreshLocked(ctx); err != nil {
		return false, err
	}
	return r.Repository.Pull(ctx, name, branch)
}

func (r *workspaceRepo) HasUnmergedFiles(ctx context.Context) (bool, error) {
	if err := r.mu.LockContext(ctx); err != nil {
		return false, err
	}
	defer r.mu.Unlock()
	if err := r.refreshLocked(ctx); err != nil {
		return false, err
	}
	return r.Repository.HasUnmergedFiles(ctx)
}

func (r *workspaceRepo) AbortMerge(ctx context.Context) error {
	if err := r.mu.LockContext(ctx); err != nil {
		return err
	}
	defer r.mu.Unlock()
	if err := r.refreshLocked(ctx); err != nil {
		return err
	}
	return r.Repository.AbortMerge(ctx)
}

func (r *workspaceRepo) recoverLocked(ctx context.Context) (err error) {
	defer func() { r.pending.Store(err != nil) }()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.refreshLocked(ctx); err != nil {
		return err
	}
	// Raw commit boundaries retain the worker's cancellation. Saved request
	// CommitTx deliberately detaches, and must never be used for recovery.
	switch repo := r.Repository.(type) {
	case *ExecRepo:
		if !repo.mu.TryLock() {
			return errors.New("workspace history is busy")
		}
		defer repo.mu.Unlock()
		conflicts, err := repo.HasUnmergedFiles(ctx)
		if err != nil {
			return err
		}
		if conflicts {
			return errors.New("workspace has unresolved merge conflicts")
		}
		out, err := repo.gitOutput(ctx, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
		if err != nil {
			return err
		}
		staged, err := repo.gitOutput(ctx, "diff", "--cached", "--no-renames", "--name-only", "-z")
		if err != nil {
			return err
		}
		stagedPaths := strings.Split(strings.TrimSuffix(string(staged), "\x00"), "\x00")
		if len(staged) > 0 && len(workspaceHistoryPaths(stagedPaths)) != len(stagedPaths) {
			return errors.New("workspace index contains files outside saved content")
		}
		files := workspaceHistoryPaths(strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00"))
		if len(files) == 0 && len(staged) == 0 {
			return ctx.Err()
		}
		// These are enumerated filenames, including tracked deletions, rather than
		// caller-supplied pathspecs. Wildcards must not expand into excluded files.
		for i, file := range files {
			files[i] = ":(literal)" + file
		}
		// The safe staged delta can consist entirely of deletions, leaving no
		// filenames in the index or worktree to stage again.
		return repo.commit(ctx, Author{}, "recover workspace history", files)
	case *GoGitRepo:
		if !repo.mu.TryLock() {
			return errors.New("workspace history is busy")
		}
		defer repo.mu.Unlock()
		idx, err := repo.repo.Storer.Index()
		if err != nil {
			return err
		}
		for _, entry := range idx.Entries {
			// Git's on-disk stages 1, 2 and 3 are unresolved merge entries.
			if entry.Stage != 0 {
				return errors.New("workspace has unresolved merge conflicts")
			}
		}
		if _, err := repo.head(ctx); err != nil {
			return err
		}
		w, err := repo.repo.Worktree()
		if err != nil {
			return err
		}
		status, err := w.Status()
		if err != nil {
			return err
		}
		files := make([]string, 0, len(status))
		for path, st := range status {
			if st.Staging != gogit.Unmodified && st.Staging != gogit.Untracked && len(workspaceHistoryPaths([]string{path})) == 0 {
				return errors.New("workspace index contains files outside saved content")
			}
			files = append(files, path)
		}
		files = workspaceHistoryPaths(files)
		if len(files) == 0 {
			return ctx.Err()
		}
		return repo.commit(ctx, Author{}, "recover workspace history", files)
	default:
		return errors.New("unsupported workspace history backend")
	}
}

// refreshLocked retains a stable backend for the duration of each operation.
// Root submodule registration moves .git into the parent's modules directory;
// go-git's existing filesystem handle still points at the old directory.
func (r *workspaceRepo) refreshLocked(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.retired {
		return errors.New("workspace storage was discarded")
	}
	gitPath := filepath.Join(r.dir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return fmt.Errorf("workspace history unavailable: %w", err)
	}
	repo, ok := r.Repository.(*GoGitRepo)
	if !ok {
		return nil
	}
	var pointer []byte
	if info.IsDir() {
		if r.gitPointer == "" {
			return nil
		}
	} else {
		pointer, err = os.ReadFile(gitPath) //nolint:gosec // G304: repository-owned Git indirection, never a request path.
		if err != nil {
			return err
		}
	}
	if string(pointer) == r.gitPointer {
		return nil
	}
	reopened, err := newGoGitRepo(ctx, r.dir, repo.defaultName, repo.defaultEmail)
	if err != nil {
		return err
	}
	r.Repository = reopened
	r.gitPointer = string(pointer)
	return nil
}

type workspaceCommitFS struct {
	repo *workspaceRepo
	ctx  context.Context
	hash string
}

func (c *workspaceCommitFS) Open(name string) (fs.File, error) {
	if err := c.repo.mu.LockContext(c.ctx); err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	defer c.repo.mu.Unlock()
	if err := c.repo.refreshLocked(c.ctx); err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return c.repo.Repository.FSAtCommit(c.ctx, c.hash).Open(name)
}

func workspaceHistoryPaths(paths []string) []string {
	files := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "AGENTS.md" {
			files = append(files, path)
			continue
		}
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			continue
		}
		if _, err := ksid.Parse(parts[0]); err != nil {
			continue
		}
		safe := true
		for i, part := range parts {
			if part == ".git" || part == ".." || part == "" || strings.HasPrefix(part, ".jsonldb-") || (part == "tmp" && i > 0 && strings.HasSuffix(parts[i-1], ".blobs")) {
				safe = false
				break
			}
		}
		if safe {
			files = append(files, path)
		}
	}
	slices.Sort(files)
	return slices.Compact(files)
}
