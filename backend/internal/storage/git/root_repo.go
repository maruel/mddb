// Manages the root data directory as a git repo with workspace submodules.

package git

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// RootRepo manages the root data directory as a git repository.
//
// It tracks db/*.jsonl and server_config.json, and manages workspace
// directories as git submodules.
type RootRepo struct {
	repo    *Repo
	dataDir string
}

// NewRootRepo initializes the root data directory as a git repository.
//
// On first run it creates .gitignore, commits existing db/ and
// server_config.json, and registers any pre-existing workspace git repos
// as submodules (migration).
func NewRootRepo(ctx context.Context, dataDir, defaultName, defaultEmail string) (*RootRepo, error) {
	if defaultName == "" {
		defaultName = "mddb"
	}
	if defaultEmail == "" {
		defaultEmail = "mddb@localhost"
	}
	repo, err := newRepo(ctx, dataDir, defaultName, defaultEmail)
	if err != nil {
		return nil, fmt.Errorf("root repo init: %w", err)
	}

	rr := &RootRepo{repo: repo, dataDir: dataDir}

	if err := rr.ensureGitignore(ctx); err != nil {
		return nil, err
	}

	if err := rr.initialCommit(ctx); err != nil {
		return nil, err
	}

	if err := rr.migrateExistingWorkspaces(ctx); err != nil {
		return nil, err
	}

	return rr, nil
}

// CommitDBChanges stages db/ and server_config.json and commits if dirty.
func (rr *RootRepo) CommitDBChanges(ctx context.Context, author Author, msg string) error {
	rr.repo.mu.Lock()
	defer rr.repo.mu.Unlock()

	// Stage db/ and server_config.json
	if out, err := rr.repo.gitCombinedOutput(ctx, "add", "--", "db/", "server_config.json"); err != nil {
		return fmt.Errorf("failed to stage db changes: %w\nOutput: %s", err, string(out))
	}

	// Check if there are staged changes
	out, err := rr.repo.gitCombinedOutput(ctx, "diff", "--cached", "--quiet")
	if err == nil {
		// Exit code 0 means no staged changes
		return nil
	}
	// Exit code 1 means there are staged changes — this is the expected path.
	// Any other error would have a non-nil err but we can't distinguish exit
	// codes portably, so we just proceed to commit.
	_ = out

	name := author.Name
	email := author.Email
	if name == "" {
		name = rr.repo.defaultName
	}
	if email == "" {
		email = rr.repo.defaultEmail
	}

	authorStr := fmt.Sprintf("%s <%s>", name, email)
	if err := rr.repo.gitRun(ctx, "commit", "-m", msg, "--author", authorStr); err != nil {
		return fmt.Errorf("failed to commit db changes: %w", err)
	}
	return nil
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
	_ = rr.repo.gitRun(ctx, "submodule", "absorbgitdirs")
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
	_ = rr.repo.gitRun(ctx, "submodule", "absorbgitdirs")

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

// ensureGitignore creates .gitignore in the data dir if it doesn't exist.
func (rr *RootRepo) ensureGitignore(_ context.Context) error {
	path := filepath.Join(rr.dataDir, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.WriteFile(path, []byte(".env\n"), 0o644); err != nil { //nolint:gosec // G306: data dir gitignore
		return fmt.Errorf("failed to create .gitignore: %w", err)
	}
	return nil
}

// initialCommit commits existing db/ and server_config.json if no commits
// exist yet.
func (rr *RootRepo) initialCommit(ctx context.Context) error {
	// Check if there are any commits
	if _, err := rr.repo.gitCombinedOutput(ctx, "rev-parse", "HEAD"); err == nil {
		return nil // Already has commits
	}

	// Stage everything trackable
	files := []string{".gitignore"}
	if fi, err := os.Stat(filepath.Join(rr.dataDir, "db")); err == nil && fi.IsDir() {
		files = append(files, "db/")
	}
	if _, err := os.Stat(filepath.Join(rr.dataDir, "server_config.json")); err == nil {
		files = append(files, "server_config.json")
	}

	args := append([]string{"add", "--"}, files...)
	if out, err := rr.repo.gitCombinedOutput(ctx, args...); err != nil {
		return fmt.Errorf("failed to stage initial files: %w\nOutput: %s", err, string(out))
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
