// Implements Repository using go-git (pure Go, no git binary dependency).

package git

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// GoGitRepo implements Repository using go-git (pure Go).
type GoGitRepo struct {
	dir          string
	defaultName  string
	defaultEmail string
	repo         *gogit.Repository
	mu           sync.Mutex
}

func newGoGitRepo(_ context.Context, dir, defaultName, defaultEmail string) (*GoGitRepo, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return nil, fmt.Errorf("failed to create repo directory: %w", err)
	}

	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		// Not a repo yet — initialize.
		repo, err = gogit.PlainInit(dir, false)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize git repo: %w", err)
		}
		// Set user.name and user.email in the repo config.
		cfg, err := repo.Config()
		if err != nil {
			return nil, fmt.Errorf("failed to read git config: %w", err)
		}
		cfg.User.Name = defaultName
		cfg.User.Email = defaultEmail
		if err := repo.SetConfig(cfg); err != nil {
			return nil, fmt.Errorf("failed to write git config: %w", err)
		}
	}

	return &GoGitRepo{
		dir:          dir,
		defaultName:  defaultName,
		defaultEmail: defaultEmail,
		repo:         repo,
	}, nil
}

// FS returns a read-only filesystem view of the repository's working directory.
func (r *GoGitRepo) FS() fs.FS {
	return os.DirFS(r.dir)
}

// FSAtCommit returns a read-only filesystem view at a specific commit.
func (r *GoGitRepo) FSAtCommit(_ context.Context, hash string) fs.FS {
	return &goGitCommitFS{repo: r.repo, hash: hash}
}

// CommitTx executes fn while holding a lock and commits the returned files atomically.
func (r *GoGitRepo) CommitTx(ctx context.Context, author Author, fn func() (msg string, files []string, err error)) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	msg, files, err := fn()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	// Detach from HTTP request context but keep a timeout.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	_ = ctx // go-git operations don't use context directly, but we keep the pattern.

	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Stage specified files.
	for _, f := range files {
		if _, err := w.Add(f); err != nil {
			return fmt.Errorf("failed to stage files: %w", err)
		}
	}

	// Check if there are staged changes.
	status, err := w.Status()
	if err != nil {
		return fmt.Errorf("failed to get worktree status: %w", err)
	}
	if status.IsClean() {
		return nil
	}

	name := author.Name
	email := author.Email
	if name == "" {
		name = r.defaultName
	}
	if email == "" {
		email = r.defaultEmail
	}

	now := time.Now()
	_, err = w.Commit(msg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  name,
			Email: email,
			When:  now,
		},
		Committer: &object.Signature{
			Name:  r.defaultName,
			Email: r.defaultEmail,
			When:  now,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	return nil
}

// CommitCount returns the total number of commits in the repository.
func (r *GoGitRepo) CommitCount(_ context.Context) (int, error) {
	iter, err := r.repo.Log(&gogit.LogOptions{})
	if err != nil {
		return 0, nil // no commits yet is not an error
	}
	defer iter.Close()

	n := 0
	for {
		if _, err := iter.Next(); err != nil {
			break
		}
		n++
	}
	return n, nil
}

// GetHistory returns commit history for a specific path, limited to n commits.
func (r *GoGitRepo) GetHistory(_ context.Context, path string, n int) ([]*Commit, error) {
	if n <= 0 || n > 1000 {
		n = 1000
	}

	opts := &gogit.LogOptions{}
	if path != "" && path != "." {
		opts.FileName = &path
	}

	iter, err := r.repo.Log(opts)
	if err != nil {
		return nil, nil // no commits yet is not an error
	}
	defer iter.Close()

	var commits []*Commit
	for range n {
		c, err := iter.Next()
		if err != nil {
			break
		}
		// Split message into subject and body.
		subject, body, _ := strings.Cut(c.Message, "\n")
		commits = append(commits, &Commit{
			Hash:           c.Hash.String(),
			Message:        subject,
			Body:           strings.TrimSpace(body),
			Author:         c.Author.Name,
			AuthorEmail:    c.Author.Email,
			AuthorDate:     c.Author.When,
			Committer:      c.Committer.Name,
			CommitterEmail: c.Committer.Email,
			CommitDate:     c.Committer.When,
		})
	}
	return commits, nil
}

// GetFileAtCommit retrieves the content of a file at a specific commit.
func (r *GoGitRepo) GetFileAtCommit(_ context.Context, hash, filePath string) ([]byte, error) {
	h := plumbing.NewHash(hash)
	if hash == "HEAD" {
		ref, err := r.repo.Head()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve HEAD: %w", err)
		}
		h = ref.Hash()
	}

	c, err := r.repo.CommitObject(h)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	f, err := c.File(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file at commit: %w", err)
	}

	reader, err := f.Reader()
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = reader.Close() }()

	return io.ReadAll(reader)
}

// SetRemote adds or updates a remote in the repository.
func (r *GoGitRepo) SetRemote(_ context.Context, name, url string) error {
	if url == "" {
		err := r.repo.DeleteRemote(name)
		if err != nil && err.Error() == "remote not found" {
			return nil
		}
		return err
	}

	// Check if remote already exists.
	_, err := r.repo.Remote(name)
	if err == nil {
		// Exists — delete and re-create (go-git has no set-url).
		if err := r.repo.DeleteRemote(name); err != nil {
			return fmt.Errorf("failed to update remote: %w", err)
		}
	}

	_, err = r.repo.CreateRemote(&config.RemoteConfig{
		Name: name,
		URLs: []string{url},
	})
	return err
}

// Push pushes changes to a remote repository.
func (r *GoGitRepo) Push(ctx context.Context, remoteName, branch string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()

	if branch == "" {
		ref, err := r.repo.Head()
		if err == nil {
			branch = ref.Name().Short()
		} else {
			branch = "master"
		}
	}

	remote, err := r.repo.Remote(remoteName)
	if err != nil {
		return fmt.Errorf("failed to get remote: %w", err)
	}

	refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch))
	return remote.PushContext(ctx, &gogit.PushOptions{
		RemoteName: remoteName,
		RefSpecs:   []config.RefSpec{refSpec},
	})
}

// goGitCommitFS implements fs.FS for a specific commit using go-git.
type goGitCommitFS struct {
	repo *gogit.Repository
	hash string
}

func (c *goGitCommitFS) Open(name string) (fs.File, error) {
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		name = "."
	}

	h := plumbing.NewHash(c.hash)
	commit, err := c.repo.CommitObject(h)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	if name == "." {
		return &commitDir{fs: nil, name: "."}, nil
	}

	// Try as a file first.
	f, err := tree.File(name)
	if err == nil {
		reader, err := f.Reader()
		if err != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		data, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		return &memFile{name: filepath.Base(name), data: data}, nil
	}

	// Try as a directory — check if any entry starts with name/.
	found := false
	for _, entry := range tree.Entries {
		if strings.HasPrefix(entry.Name, name+"/") || entry.Name == name {
			found = true
			break
		}
	}

	// Also check subtree.
	if !found {
		if _, err := tree.Tree(name); err == nil {
			found = true
		}
	}

	if found {
		return &commitDir{fs: nil, name: name}, nil
	}

	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}
