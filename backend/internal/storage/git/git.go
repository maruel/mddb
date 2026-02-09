// Defines the Repository interface, Manager, and shared types for git operations.

package git

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// InjectTokenInURL injects an authentication token into a git remote URL.
// Supports GitHub (x-access-token) and GitLab (oauth2) URL patterns.
func InjectTokenInURL(remoteURL, token, remoteType string) string {
	if token == "" {
		return remoteURL
	}
	switch {
	case strings.Contains(remoteURL, "github.com") || remoteType == "github":
		return strings.Replace(remoteURL, "https://github.com", fmt.Sprintf("https://x-access-token:%s@github.com", token), 1)
	case strings.Contains(remoteURL, "gitlab.com") || remoteType == "gitlab":
		return strings.Replace(remoteURL, "https://gitlab.com", fmt.Sprintf("https://oauth2:%s@gitlab.com", token), 1)
	default:
		return remoteURL
	}
}

// Repository is the interface for git operations on a single repository.
type Repository interface {
	// FS returns a read-only filesystem view of the repository's working directory.
	FS() fs.FS
	// FSAtCommit returns a read-only filesystem view at a specific commit.
	FSAtCommit(ctx context.Context, hash string) fs.FS
	// CommitTx executes fn while holding a lock and commits the returned files atomically.
	// If fn returns an error or no files, no commit is made.
	CommitTx(ctx context.Context, author Author, fn func() (msg string, files []string, err error)) error
	// CommitCount returns the total number of commits in the repository.
	CommitCount(ctx context.Context) (int, error)
	// GetHistory returns commit history for a specific path, limited to n commits.
	// n is capped at 1000. If n <= 0, defaults to 1000.
	GetHistory(ctx context.Context, path string, n int) ([]*Commit, error)
	// GetFileAtCommit retrieves the content of a file at a specific commit.
	GetFileAtCommit(ctx context.Context, hash, filePath string) ([]byte, error)
	// SetRemote adds or updates a remote in the repository.
	// If url is empty, the remote is removed.
	SetRemote(ctx context.Context, name, url string) error
	// Push pushes changes to a remote repository.
	Push(ctx context.Context, remoteName, branch string) error
	// Fetch fetches from a remote repository.
	Fetch(ctx context.Context, remoteName, branch string) error
	// Pull fetches and merges from a remote. Returns true if files changed.
	// Returns an error if there are merge conflicts.
	Pull(ctx context.Context, remoteName, branch string) (changed bool, err error)
	// HasUnmergedFiles returns true if there are unmerged files (merge conflicts).
	HasUnmergedFiles(ctx context.Context) (bool, error)
	// AbortMerge aborts an in-progress merge.
	AbortMerge(ctx context.Context) error
}

// Backend selects which git implementation to use.
type Backend int

const (
	// BackendExec uses the git CLI via os/exec (default).
	BackendExec Backend = iota
	// BackendGoGit uses go-git (pure Go, no git binary needed).
	BackendGoGit
)

// Manager creates and caches git repositories.
type Manager struct {
	rootDir      string
	defaultName  string
	defaultEmail string
	backend      Backend
	repos        sync.Map // path -> Repository
}

// NewManager creates a new git repository manager using the exec backend.
func NewManager(rootDir, defaultName, defaultEmail string) *Manager {
	return NewManagerWithBackend(rootDir, defaultName, defaultEmail, BackendExec)
}

// NewManagerWithBackend creates a new git repository manager with the given backend.
func NewManagerWithBackend(rootDir, defaultName, defaultEmail string, backend Backend) *Manager {
	if defaultName == "" {
		defaultName = "mddb"
	}
	if defaultEmail == "" {
		defaultEmail = "mddb@localhost"
	}
	return &Manager{
		rootDir:      rootDir,
		defaultName:  defaultName,
		defaultEmail: defaultEmail,
		backend:      backend,
	}
}

// Repo returns or creates a repository for the given subdirectory.
// The subdir is relative to the manager's root directory.
func (m *Manager) Repo(ctx context.Context, subdir string) (Repository, error) {
	dir := filepath.Join(m.rootDir, subdir)
	if r, ok := m.repos.Load(dir); ok {
		return r.(Repository), nil
	}

	var r Repository
	var err error
	switch m.backend {
	case BackendGoGit:
		r, err = newGoGitRepo(ctx, dir, m.defaultName, m.defaultEmail)
	default:
		r, err = newExecRepo(ctx, dir, m.defaultName, m.defaultEmail)
	}
	if err != nil {
		return nil, err
	}

	actual, _ := m.repos.LoadOrStore(dir, r)
	return actual.(Repository), nil
}

// Author identifies who made a change for git commits.
type Author struct {
	Name  string
	Email string
}

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

// commitDir implements fs.File for a directory in a commit.
type commitDir struct {
	fs   *commitFS
	name string
}

func (d *commitDir) Stat() (fs.FileInfo, error) {
	return &dirInfo{name: filepath.Base(d.name)}, nil
}

func (d *commitDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: fs.ErrInvalid}
}

func (d *commitDir) Close() error {
	return nil
}

// dirInfo implements fs.FileInfo for a directory.
type dirInfo struct {
	name string
}

func (d *dirInfo) Name() string       { return d.name }
func (d *dirInfo) Size() int64        { return 0 }
func (d *dirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o755 }
func (d *dirInfo) ModTime() time.Time { return time.Time{} }
func (d *dirInfo) IsDir() bool        { return true }
func (d *dirInfo) Sys() any           { return nil }

// memFile implements fs.File for an in-memory file.
type memFile struct {
	name   string
	data   []byte
	offset int
}

func (f *memFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{name: f.name, size: int64(len(f.data))}, nil
}

func (f *memFile) Read(b []byte) (int, error) {
	if f.offset >= len(f.data) {
		return 0, &fs.PathError{Op: "read", Path: f.name, Err: fs.ErrClosed}
	}
	n := copy(b, f.data[f.offset:])
	f.offset += n
	return n, nil
}

func (f *memFile) Close() error {
	return nil
}

// fileInfo implements fs.FileInfo for a file.
type fileInfo struct {
	name string
	size int64
}

func (f *fileInfo) Name() string       { return f.name }
func (f *fileInfo) Size() int64        { return f.size }
func (f *fileInfo) Mode() fs.FileMode  { return 0o644 }
func (f *fileInfo) ModTime() time.Time { return time.Time{} }
func (f *fileInfo) IsDir() bool        { return false }
func (f *fileInfo) Sys() any           { return nil }
