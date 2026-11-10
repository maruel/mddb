// Manages the root data directory as a git repo with workspace submodules.

package git

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/maruel/mddb/backend/internal/storage"
)

// RootRepo manages the root data directory as a git repository.
//
// It tracks db/*.jsonl and manages workspace directories as git submodules.
// Server settings live in the config directory, outside the data directory.
type RootRepo struct {
	repo    *ExecRepo
	dataDir string
	pending atomic.Bool
}

// NewRootRepo initializes the root data directory as a git repository.
//
// On first run it writes embedded static files, commits existing db/, and
// registers any pre-existing workspace git repos as submodules (migration).
func NewRootRepo(ctx context.Context, dataDir, defaultName, defaultEmail string) (*RootRepo, error) {
	if defaultName == "" {
		defaultName = "mddb"
	}
	if defaultEmail == "" {
		defaultEmail = "mddb@localhost"
	}
	repo, err := newExecRepo(ctx, dataDir, defaultName, defaultEmail)
	if err != nil {
		return nil, fmt.Errorf("root repo init: %w", err)
	}

	rr := &RootRepo{repo: repo, dataDir: dataDir}

	staticFiles, err := storage.WriteRootStaticFiles(dataDir)
	if err != nil {
		return nil, err
	}

	if err := rr.initialCommit(ctx, staticFiles); err != nil {
		return nil, err
	}

	if err := rr.migrateExistingWorkspaces(ctx); err != nil {
		return nil, err
	}
	// Reconcile changes that survived a restart before reporting healthy.
	// A root with no database is valid, including an empty first-time root.
	if info, err := os.Stat(filepath.Join(dataDir, "db")); err == nil {
		if !info.IsDir() {
			return nil, errors.New("root database is not a directory")
		}
		// Startup belongs to its owner, so unlike a request history commit it
		// must stop when that owner cancels initialization.
		historyCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		rr.repo.mu.Lock()
		err := rr.commitDBChangesLocked(historyCtx, Author{}, "recover database history")
		rr.repo.mu.Unlock()
		cancel()
		if ctx.Err() != nil {
			return nil, fmt.Errorf("reconcile database history: %w", ctx.Err())
		}
		if err != nil {
			slog.ErrorContext(ctx, "Failed to reconcile database history", "err", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect root database: %w", err)
	}

	return rr, nil
}

// CommitDBChanges stages db/ and commits if dirty.
//
// Failures mark history as pending for health reporting and background recovery.
// The database mutation is already persisted and must not be replayed. History
// recording outlives a disconnected request, bounded by a five-minute deadline.
func (rr *RootRepo) CommitDBChanges(ctx context.Context, author Author, msg string) error {
	rr.repo.mu.Lock()
	defer rr.repo.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	return rr.commitDBChangesLocked(ctx, author, msg)
}

// HasPendingDBChanges reports whether the last attempt to record database history
// failed. Reading it does not wait for an active Git command.
func (rr *RootRepo) HasPendingDBChanges() bool {
	return rr.pending.Load()
}

// RunCommitRecovery retries pending database history until ctx is canceled.
//
// Recovery coalesces the current database snapshot into one commit. It does not
// replay application mutations or promise one commit for each original request.
// The caller owns its goroutine and must wait for it at shutdown.
func (rr *RootRepo) RunCommitRecovery(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		// An active request already owns Git; never block shutdown behind it.
		if rr.pending.Load() && rr.repo.mu.TryLock() {
			attemptCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			err := rr.commitDBChangesLocked(attemptCtx, Author{}, "recover database history")
			cancel()
			rr.repo.mu.Unlock()
			if err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "Failed to recover database history", "err", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// AddWorkspaceSubmodule registers an existing workspace git directory as a
// submodule of the root repo and commits.
func (rr *RootRepo) AddWorkspaceSubmodule(ctx context.Context, wsID string) error {
	rr.repo.mu.Lock()
	defer rr.repo.mu.Unlock()

	if out, err := rr.repo.gitCombinedOutput(ctx, "submodule", "add", "--force", "./"+wsID, wsID); err != nil {
		return fmt.Errorf("failed to add submodule %s: %w\nOutput: %s", wsID, err, string(out))
	}
	// Absorb the submodule's .git directory into .git/modules/ so that
	// deinit works correctly on removal (some git versions leave a real
	// .git dir after submodule add --force on an existing repo).
	// Limit migration to this workspace: absorbing every module traverses and
	// can repair unrelated repositories while holding the root history lock.
	if err := rr.repo.gitRun(ctx, "submodule", "absorbgitdirs", wsID); err != nil {
		return fmt.Errorf("absorb workspace git directories: %w", err)
	}
	authorStr := fmt.Sprintf("%s <%s>", rr.repo.defaultName, rr.repo.defaultEmail)
	if err := rr.repo.gitRun(ctx, "commit", "-m", "add workspace "+wsID, "--author", authorStr); err != nil {
		return fmt.Errorf("failed to commit submodule add: %w", err)
	}
	return nil
}

// RemoveWorkspaceSubmodule removes a workspace submodule from the root repo.
func (rr *RootRepo) RemoveWorkspaceSubmodule(ctx context.Context, wsID string) error {
	rr.repo.mu.Lock()
	defer rr.repo.mu.Unlock()

	// Absorb .git dir into parent if needed (handles repos added before
	// absorbgitdirs was called in AddWorkspaceSubmodule).
	if err := rr.repo.gitRun(ctx, "submodule", "absorbgitdirs", wsID); err != nil {
		return fmt.Errorf("absorb workspace git directories: %w", err)
	}

	// Deinit the submodule.
	if out, err := rr.repo.gitCombinedOutput(ctx, "submodule", "deinit", "-f", wsID); err != nil {
		return fmt.Errorf("failed to deinit submodule %s: %w\nOutput: %s", wsID, err, string(out))
	}

	// Remove from .git/modules.
	modulePath := filepath.Join(rr.dataDir, ".git", "modules", wsID)
	if err := os.RemoveAll(modulePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove .git/modules/%s: %w", wsID, err)
	}

	// Remove from index and working tree.
	if out, err := rr.repo.gitCombinedOutput(ctx, "rm", "-f", wsID); err != nil {
		return fmt.Errorf("failed to git rm submodule %s: %w\nOutput: %s", wsID, err, string(out))
	}

	authorStr := fmt.Sprintf("%s <%s>", rr.repo.defaultName, rr.repo.defaultEmail)
	if err := rr.repo.gitRun(ctx, "commit", "-m", "remove workspace "+wsID, "--author", authorStr); err != nil {
		return fmt.Errorf("failed to commit submodule removal: %w", err)
	}
	return nil
}

func (rr *RootRepo) commitDBChangesLocked(ctx context.Context, author Author, msg string) (err error) {
	defer func() { rr.pending.Store(err != nil) }()

	// Stage db/
	// JSONL rewrites publish by rename; never stage their in-progress files.
	if out, err := rr.repo.gitCombinedOutput(ctx, "add", "--", "db/", ":(exclude,glob)db/**/.jsonldb-*"); err != nil {
		return fmt.Errorf("failed to stage db changes: %w\nOutput: %s", err, string(out))
	}

	// Check if there are staged changes
	out, err := rr.repo.gitCombinedOutput(ctx, "diff", "--cached", "--quiet")
	if err == nil {
		// Exit code 0 means no staged changes
		return nil
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 1 {
		return fmt.Errorf("failed to inspect staged db changes: %w\nOutput: %s", err, string(out))
	}

	name := author.Name
	email := author.Email
	if name == "" {
		name = rr.repo.defaultName
	}
	if email == "" {
		email = rr.repo.defaultEmail
	}

	authorStr := fmt.Sprintf("%s <%s>", name, email)
	if out, err := rr.repo.gitCombinedOutput(ctx, "commit", "-m", msg, "--author", authorStr); err != nil {
		return fmt.Errorf("failed to commit db changes: %w\nOutput: %s", err, out)
	}
	return nil
}

// initialCommit commits the embedded root static files and db/ if no commits
// exist yet. staticFiles are the paths returned by WriteRootStaticFiles.
// An empty data directory still gets a root commit.
func (rr *RootRepo) initialCommit(ctx context.Context, staticFiles []string) error {
	// Check if there are any commits
	if _, err := rr.repo.gitCombinedOutput(ctx, "rev-parse", "HEAD"); err == nil {
		return nil
	} else {
		unborn, probeErr := rr.repo.isUnborn(ctx)
		if probeErr != nil || !unborn {
			return fmt.Errorf("inspect root HEAD: %w", errors.Join(err, probeErr))
		}
	}

	// Collect everything trackable
	paths := staticFiles
	if fi, err := os.Stat(filepath.Join(rr.dataDir, "db")); err == nil && fi.IsDir() {
		paths = append(paths, "db/")
	}
	if len(paths) != 0 {
		args := append([]string{"add", "--"}, paths...)
		if out, err := rr.repo.gitCombinedOutput(ctx, args...); err != nil {
			return fmt.Errorf("failed to stage initial files: %w\nOutput: %s", err, string(out))
		}
	}

	authorStr := fmt.Sprintf("%s <%s>", rr.repo.defaultName, rr.repo.defaultEmail)
	if out, err := rr.repo.gitCombinedOutput(ctx, "commit", "-m", "initial commit", "--author", authorStr, "--allow-empty"); err != nil {
		return fmt.Errorf("failed to create initial commit: %w\nOutput: %s", err, string(out))
	}
	return nil
}

// migrateExistingWorkspaces scans the data directory for subdirectories that
// contain a .git directory and registers them as submodules if not already
// tracked in .gitmodules.
func (rr *RootRepo) migrateExistingWorkspaces(ctx context.Context) error {
	entries, err := os.ReadDir(rr.dataDir)
	if err != nil {
		return fmt.Errorf("failed to read data dir: %w", err)
	}

	// Read existing .gitmodules to check what's already registered.
	known := rr.knownSubmodules()

	for _, e := range entries {
		if !e.IsDir() || e.Name() == "db" || e.Name() == ".git" || strings.HasPrefix(e.Name(), ".") {
			continue
		}

		gitDir := filepath.Join(rr.dataDir, e.Name(), ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			continue
		}

		if known[e.Name()] {
			continue
		}

		slog.InfoContext(ctx, "Migrating existing workspace as submodule", "wsID", e.Name())
		// Unlock so AddWorkspaceSubmodule can take the lock.
		// This is safe because migrateExistingWorkspaces runs only during init.
		if err := rr.AddWorkspaceSubmodule(ctx, e.Name()); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.WarnContext(ctx, "Failed to migrate workspace submodule", "wsID", e.Name(), "err", err)
		}
	}
	return nil
}

// knownSubmodules returns the set of submodule paths listed in .gitmodules.
func (rr *RootRepo) knownSubmodules() map[string]bool {
	path := filepath.Join(rr.dataDir, ".gitmodules")
	data, err := os.ReadFile(path) //nolint:gosec // G304: constructed from dataDir
	if err != nil {
		return nil
	}

	known := make(map[string]bool)
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "path = "); ok {
			known[v] = true
		}
	}
	return known
}
