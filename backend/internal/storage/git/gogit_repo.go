// Implements Repository using go-git (pure Go, no git binary dependency).

package git

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

var errUnbornHEAD = errors.New("current Git branch has no commits")

// GoGitRepo implements Repository using go-git (pure Go).
type GoGitRepo struct {
	dir          string
	defaultName  string
	defaultEmail string
	repo         *gogit.Repository
	mu           sync.Mutex
}

func newGoGitRepo(ctx context.Context, dir, defaultName, defaultEmail string) (*GoGitRepo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return nil, fmt.Errorf("failed to create repo directory: %w", err)
	}

	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		if !errors.Is(err, gogit.ErrRepositoryNotExists) {
			return nil, fmt.Errorf("open git repository: %w", err)
		}
		if _, statErr := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("open existing git repository: %w", errors.Join(err, statErr))
		}
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
func (r *GoGitRepo) FSAtCommit(ctx context.Context, hash string) fs.FS {
	return &goGitCommitFS{repo: r, ctx: ctx, hash: hash}
}

// ReadTx reads saved files while excluding repository-owned mutations, independently of Git metadata.
func (r *GoGitRepo) ReadTx(ctx context.Context, fn func() error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}

// CommitTx executes a serialized mutation and records its changed files.
func (r *GoGitRepo) CommitTx(ctx context.Context, author Author, fn func() (msg string, files []string, err error)) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	// Validate current metadata before invoking go-git's internal HEAD reads or
	// allowing the callback to save a new mutation over a corrupt reference.
	if _, err := r.head(ctx); err != nil {
		return fmt.Errorf("resolve mutation HEAD: %w", err)
	}

	msg, files, err := fn()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	// Complete saved request changes after disconnection. Recovery uses commit
	// directly with its owner context instead.
	return r.commit(context.WithoutCancel(ctx), author, msg, files)
}

func (r *GoGitRepo) commit(ctx context.Context, author Author, msg string, files []string) error {
	// Go-git filesystem calls cannot be interrupted. Check the owner context
	// between operations without promising a hard timeout for those calls.

	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Stage specified files.
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := w.Add(f); err != nil {
			return fmt.Errorf("failed to stage files: %w", err)
		}
	}

	// Check if there are staged changes.
	status, err := w.Status()
	if err != nil {
		return fmt.Errorf("failed to get worktree status: %w", err)
	}
	staged := false
	for _, st := range status {
		if st.Staging != gogit.Unmodified && st.Staging != gogit.Untracked {
			staged = true
			break
		}
	}
	if !staged {
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

	if err := ctx.Err(); err != nil {
		return err
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
func (r *GoGitRepo) CommitCount(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	head, err := r.head(ctx)
	if err != nil {
		return 0, fmt.Errorf("resolve history HEAD: %w", err)
	}
	if head == plumbing.ZeroHash {
		return 0, nil
	}
	it, err := r.repo.Log(&gogit.LogOptions{From: head})
	if err != nil {
		return 0, fmt.Errorf("count commits: %w", err)
	}
	defer it.Close()
	n := 0
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		_, err := it.Next()
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if errors.Is(err, io.EOF) {
			return n, nil
		}
		if err != nil {
			return 0, fmt.Errorf("iterate commits: %w", err)
		}
		n++
	}
}

// head resolves HEAD without hiding unreadable or malformed loose references.
// A zero hash is returned only for a missing branch of a valid symbolic HEAD.
func (r *GoGitRepo) head(ctx context.Context) (plumbing.Hash, error) {
	ref, err := r.resolveReference(ctx, plumbing.HEAD)
	if errors.Is(err, errUnbornHEAD) {
		return plumbing.ZeroHash, ctx.Err()
	}
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if err := ctx.Err(); err != nil {
		return plumbing.ZeroHash, err
	}
	return ref.Hash(), nil
}

// resolveReference retains the resolved branch name for remote branch selection.
// A genuinely unborn HEAD is identified separately from corrupt references.
func (r *GoGitRepo) resolveReference(ctx context.Context, name plumbing.ReferenceName) (*plumbing.Reference, error) {
	start := name
	for range storer.MaxResolveRecursion + 1 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ref, err := r.reference(ctx, name)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, plumbing.ErrReferenceNotFound) && start == plumbing.HEAD && name.IsBranch() {
			return nil, errUnbornHEAD
		}
		if err != nil {
			return nil, err
		}
		if ref.Type() == plumbing.HashReference {
			return ref, nil
		}
		name = ref.Target()
	}
	return nil, storer.ErrMaxResolveRecursion
}

func (r *GoGitRepo) reference(ctx context.Context, name plumbing.ReferenceName) (ref *plumbing.Reference, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	defer func() {
		if ctx.Err() != nil {
			ref = nil
			if !errors.Is(err, ctx.Err()) {
				err = errors.Join(err, ctx.Err())
			}
		}
	}()
	if err := name.Validate(); err != nil {
		return nil, err
	}
	// PlainOpen constructs filesystem storage, including absorbed .git files.
	// DotGit.Ref retries packed refs after every loose-read error; inspect the
	// loose file ourselves and retry only genuine absence, observing the owner
	// between filesystem steps without promising to interrupt a filesystem call.
	disk := r.repo.Storer.(interface{ Filesystem() billy.Filesystem }).Filesystem()
	f, openErr := disk.Open(name.String())
	if openErr != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !os.IsNotExist(openErr) {
			return nil, openErr
		}
		return r.packedReference(ctx, name)
	}
	if ctx.Err() != nil {
		return nil, errors.Join(ctx.Err(), f.Close())
	}
	// Ref files contain a hash or symbolic target, not an arbitrarily sized blob.
	const maxRefBytes = 64 * 1024
	data, readErr := io.ReadAll(io.LimitReader(f, maxRefBytes+1))
	if err := errors.Join(readErr, f.Close(), ctx.Err()); err != nil {
		return nil, err
	}
	if len(data) > maxRefBytes {
		return nil, fmt.Errorf("reference %s is too large", name)
	}
	value := strings.TrimSpace(string(data))
	if target, ok := strings.CutPrefix(value, "ref: "); ok {
		dest := plumbing.ReferenceName(target)
		if err := dest.Validate(); err != nil {
			return nil, fmt.Errorf("invalid symbolic reference %s: %w", name, err)
		}
		return plumbing.NewSymbolicReference(name, dest), nil
	}
	if !plumbing.IsHash(value) || plumbing.NewHash(value) == plumbing.ZeroHash {
		return nil, fmt.Errorf("invalid reference %s", name)
	}
	return plumbing.NewHashReference(name, plumbing.NewHash(value)), nil
}

// packedReference streams packed refs without go-git's permissive hash decoder.
func (r *GoGitRepo) packedReference(ctx context.Context, name plumbing.ReferenceName) (ref *plumbing.Reference, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	disk := r.repo.Storer.(interface{ Filesystem() billy.Filesystem }).Filesystem()
	f, openErr := disk.Open("packed-refs")
	if openErr != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if os.IsNotExist(openErr) {
			return nil, plumbing.ErrReferenceNotFound
		}
		return nil, openErr
	}
	defer func() {
		err = errors.Join(err, f.Close(), ctx.Err())
		if err != nil {
			ref = nil
		}
	}()
	scanner := bufio.NewScanner(f)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ok := scanner.Scan()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !ok {
			break
		}
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "^") {
			if !plumbing.IsHash(strings.TrimPrefix(line, "^")) {
				return nil, errors.New("invalid peeled packed reference")
			}
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 || !plumbing.IsHash(parts[0]) || plumbing.NewHash(parts[0]) == plumbing.ZeroHash {
			return nil, errors.New("invalid packed reference")
		}
		refName := plumbing.ReferenceName(parts[1])
		if err := refName.Validate(); err != nil {
			return nil, fmt.Errorf("invalid packed reference name: %w", err)
		}
		if refName == name {
			return plumbing.NewHashReference(refName, plumbing.NewHash(parts[0])), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, plumbing.ErrReferenceNotFound
}

// commitHash resolves supported commit identifiers through the same validated
// owner-aware HEAD boundary used by history, including file and filesystem reads.
func (r *GoGitRepo) commitHash(ctx context.Context, hash string) (plumbing.Hash, error) {
	if err := ctx.Err(); err != nil {
		return plumbing.ZeroHash, err
	}
	if hash == "HEAD" {
		h, err := r.head(ctx)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		if h == plumbing.ZeroHash {
			return h, plumbing.ErrReferenceNotFound
		}
		return h, nil
	}
	if !plumbing.IsHash(hash) {
		return plumbing.ZeroHash, errors.New("invalid commit hash")
	}
	return plumbing.NewHash(hash), nil
}

// GetHistory returns commit history for a specific path, limited to n commits.
func (r *GoGitRepo) GetHistory(ctx context.Context, path string, n int) ([]*Commit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if n <= 0 || n > 1000 {
		n = 1000
	}

	head, err := r.head(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve history HEAD: %w", err)
	}
	if head == plumbing.ZeroHash {
		return nil, nil
	}
	raw, err := r.repo.Log(&gogit.LogOptions{From: head})
	if err != nil {
		return nil, fmt.Errorf("read commit history: %w", err)
	}
	var iter object.CommitIter = &contextCommitIter{CommitIter: raw, ctx: ctx}
	if path != "" && path != "." {
		// Preserve go-git's path history algorithm while checking cancellation at
		// every source step, including the commits skipped by its filtered Next.
		iter = object.NewCommitPathIterFromIter(func(name string) bool { return name == path }, iter, false)
	}

	defer iter.Close()

	var commits []*Commit
	for range n {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c, err := iter.Next()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("iterate commit history: %w", err)
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
func (r *GoGitRepo) GetFileAtCommit(ctx context.Context, hash, filePath string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h, err := r.commitHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("resolve file version: %w", err)
	}

	c, err := r.repo.CommitObject(h)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	tree, err := c.Tree()
	if err = errors.Join(err, ctx.Err()); err != nil {
		return nil, fmt.Errorf("read commit tree: %w", err)
	}
	entry, err := findTreeEntry(ctx, r.repo.Storer, tree, filePath)
	if err != nil {
		return nil, fmt.Errorf("find commit file: %w", err)
	}
	if !entry.Mode.IsFile() {
		return nil, fmt.Errorf("commit path %s is not a file", filePath)
	}
	blob, err := object.GetBlob(r.repo.Storer, entry.Hash)
	if err = errors.Join(err, ctx.Err()); err != nil {
		return nil, fmt.Errorf("read commit blob: %w", err)
	}
	f := object.NewFile(filePath, entry.Mode, blob)

	reader, err := f.Reader()
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, reader.Close())
	}
	data, readErr := io.ReadAll(reader)
	if err := errors.Join(readErr, reader.Close(), ctx.Err()); err != nil {
		return nil, err
	}
	return data, nil
}

// SetRemote adds or updates a remote in the repository.
func (r *GoGitRepo) SetRemote(ctx context.Context, name, url string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if url == "" {
		err := r.repo.DeleteRemote(name)
		if errors.Is(err, gogit.ErrRemoteNotFound) {
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

	if err != nil && !errors.Is(err, gogit.ErrRemoteNotFound) {
		return fmt.Errorf("read remote: %w", err)
	}
	_, err = r.repo.CreateRemote(&config.RemoteConfig{
		Name: name,
		URLs: []string{url},
	})
	return err
}

// Push pushes changes to a remote repository.
func (r *GoGitRepo) Push(ctx context.Context, remoteName, branch string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	if branch == "" {
		ref, err := r.resolveReference(ctx, plumbing.HEAD)
		if err != nil {
			return fmt.Errorf("resolve current branch: %w", err)
		}
		branch = ref.Name().Short()
	} else if _, err := r.resolveReference(ctx, plumbing.NewBranchReferenceName(branch)); err != nil {
		return fmt.Errorf("resolve push branch: %w", err)
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

// Fetch fetches from a remote repository.
func (r *GoGitRepo) Fetch(ctx context.Context, remoteName, branch string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	remote, err := r.repo.Remote(remoteName)
	if err != nil {
		return fmt.Errorf("failed to get remote: %w", err)
	}

	opts := &gogit.FetchOptions{RemoteName: remoteName}
	if branch != "" {
		refSpec := config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", branch, remoteName, branch))
		opts.RefSpecs = []config.RefSpec{refSpec}
	}
	_ = remote
	err = r.repo.FetchContext(ctx, opts)
	if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return nil
	}
	return err
}

// Pull fetches and merges from a remote. go-git doesn't support merge natively,
// so this is a limited implementation that only works for fast-forward merges.
func (r *GoGitRepo) Pull(ctx context.Context, remoteName, branch string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	w, err := r.repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("failed to get worktree: %w", err)
	}

	head, err := r.resolveReference(ctx, plumbing.HEAD)
	if err != nil && (!errors.Is(err, errUnbornHEAD) || branch == "") {
		return false, fmt.Errorf("resolve current branch: %w", err)
	}
	if branch == "" {
		branch = head.Name().Short()
	}

	refName := plumbing.NewRemoteReferenceName(remoteName, branch)
	err = w.PullContext(ctx, &gogit.PullOptions{
		RemoteName:    remoteName,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
	})
	_ = refName
	if err != nil {
		if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
			return false, nil
		}
		return false, fmt.Errorf("pull failed: %w", err)
	}
	return true, nil
}

// HasUnmergedFiles returns true if there are unmerged files. go-git has limited
// merge support, so this always returns false.
func (r *GoGitRepo) HasUnmergedFiles(ctx context.Context) (bool, error) {
	return false, ctx.Err()
}

// AbortMerge aborts an in-progress merge. go-git has limited merge support.
func (r *GoGitRepo) AbortMerge(ctx context.Context) error {
	return ctx.Err()
}

// contextCommitIter makes cancellation visible inside a path-filtered scan.
type contextCommitIter struct {
	object.CommitIter
	ctx context.Context
}

func (i *contextCommitIter) Next() (*object.Commit, error) {
	if err := i.ctx.Err(); err != nil {
		return nil, err
	}
	c, err := i.CommitIter.Next()
	if i.ctx.Err() != nil {
		return nil, i.ctx.Err()
	}
	return c, err
}

func (i *contextCommitIter) ForEach(fn func(*object.Commit) error) error {
	for {
		c, err := i.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fn(c); err != nil {
			if errors.Is(err, storer.ErrStop) {
				return nil
			}
			return err
		}
	}
}

// goGitCommitFS implements fs.FS for a specific commit using go-git.
type goGitCommitFS struct {
	repo *GoGitRepo
	ctx  context.Context
	hash string
}

func (c *goGitCommitFS) Open(name string) (fs.File, error) {
	if err := c.ctx.Err(); err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		name = "."
	}

	h, err := c.repo.commitHash(c.ctx, c.hash)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	commit, err := c.repo.repo.CommitObject(h)
	if err = errors.Join(err, c.ctx.Err()); err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	tree, err := commit.Tree()
	if err = errors.Join(err, c.ctx.Err()); err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	if name == "." {
		return &commitDir{fs: nil, name: "."}, nil
	}

	entry, err := findTreeEntry(c.ctx, c.repo.repo.Storer, tree, name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	if entry.Mode == filemode.Dir {
		// A directory result still requires a readable directory object.
		_, err := object.GetTree(c.repo.repo.Storer, entry.Hash)
		if err = errors.Join(err, c.ctx.Err()); err != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: err}
		}
		return &commitDir{fs: nil, name: name}, nil
	}
	if !entry.Mode.IsFile() {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	blob, err := object.GetBlob(c.repo.repo.Storer, entry.Hash)
	if err = errors.Join(err, c.ctx.Err()); err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	reader, err := blob.Reader()
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	if err := c.ctx.Err(); err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: errors.Join(err, reader.Close())}
	}
	data, readErr := io.ReadAll(reader)
	if err := errors.Join(readErr, reader.Close(), c.ctx.Err()); err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return &memFile{name: filepath.Base(name), data: data}, nil
}

// findTreeEntry distinguishes an absent path from unreadable tree objects.
func findTreeEntry(ctx context.Context, s storer.EncodedObjectStorer, tree *object.Tree, name string) (*object.TreeEntry, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	parts := strings.Split(name, "/")
	for i, part := range parts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// A single-component lookup cannot hide a failing subtree traversal.
		entry, err := tree.FindEntry(part)
		if errors.Is(err, object.ErrEntryNotFound) {
			return nil, fs.ErrNotExist
		}
		if err != nil {
			return nil, err
		}
		if i == len(parts)-1 {
			return entry, nil
		}
		if entry.Mode != filemode.Dir {
			return nil, fs.ErrNotExist
		}
		tree, err = object.GetTree(s, entry.Hash)
		if err := errors.Join(err, ctx.Err()); err != nil {
			return nil, err
		}
	}
	return nil, fs.ErrNotExist
}
