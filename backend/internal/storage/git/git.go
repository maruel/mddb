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

// Manager creates and caches git repositories.
type Manager struct {
	rootDir      string
	defaultName  string
	defaultEmail string
	repos        sync.Map // path -> *Repo
}

// NewManager creates a new git repository manager.
func NewManager(rootDir, defaultName, defaultEmail string) *Manager {
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
	}
}

// Repo returns or creates a repository for the given subdirectory.
// The subdir is relative to the manager's root directory.
func (m *Manager) Repo(ctx context.Context, subdir string) (*Repo, error) {
	dir := filepath.Join(m.rootDir, subdir)
	if r, ok := m.repos.Load(dir); ok {
		return r.(*Repo), nil
	}

	r, err := newRepo(ctx, dir, m.defaultName, m.defaultEmail)
	if err != nil {
		return nil, err
	}

	actual, _ := m.repos.LoadOrStore(dir, r)
	return actual.(*Repo), nil
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

// Repo represents a single git repository.
type Repo struct {
	dir          string
	defaultName  string
	defaultEmail string
	mu           sync.Mutex
}

func newRepo(ctx context.Context, dir, defaultName, defaultEmail string) (*Repo, error) {
	r := &Repo{
		dir:          dir,
		defaultName:  defaultName,
		defaultEmail: defaultEmail,
	}
	if err := r.init(ctx); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Repo) init(ctx context.Context) error {
	gitDir := filepath.Join(r.dir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := os.MkdirAll(r.dir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
			return fmt.Errorf("failed to create repo directory: %w", err)
		}
		if err := r.git(ctx, "init").Run(); err != nil {
			return fmt.Errorf("failed to initialize git repo: %w", err)
		}
		if err := r.git(ctx, "config", "user.email", r.defaultEmail).Run(); err != nil {
			return fmt.Errorf("failed to configure git user.email: %w", err)
		}
		if err := r.git(ctx, "config", "user.name", r.defaultName).Run(); err != nil {
			return fmt.Errorf("failed to configure git user.name: %w", err)
		}
	}
	return nil
}

// FS returns a read-only filesystem view of the repository's working directory.
func (r *Repo) FS() fs.FS {
	return os.DirFS(r.dir)
}

// FSAtCommit returns a read-only filesystem view at a specific commit.
func (r *Repo) FSAtCommit(ctx context.Context, hash string) fs.FS {
	return &commitFS{repo: r, ctx: ctx, hash: hash}
}

// CommitTx executes fn while holding a lock and commits the returned files atomically.
// If fn returns an error or no files, no commit is made.
func (r *Repo) CommitTx(ctx context.Context, author Author, fn func() (msg string, files []string, err error)) error {
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

func (r *Repo) commit(ctx context.Context, author Author, message string, files []string) error {
	// Stage specified files
	args := append([]string{"add", "--"}, files...)
	if out, err := r.git(ctx, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stage files: %w\nOutput: %s", err, string(out))
	}

	// Check if there are staged changes
	out, err := r.git(ctx, "status", "--porcelain").CombinedOutput()
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
	if err := r.git(ctx, "commit", "-m", message, "--author", authorStr).Run(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// GetHistory returns commit history for a specific path, limited to n commits.
// n is capped at 1000. If n <= 0, defaults to 1000.
func (r *Repo) GetHistory(ctx context.Context, path string, n int) ([]*Commit, error) {
	if n <= 0 || n > 1000 {
		n = 1000
	}

	// Use record separator (%x1e) between commits since body can contain newlines
	format := "%H%x00%an%x00%ae%x00%ai%x00%cn%x00%ce%x00%ci%x00%s%x00%b%x1e"
	args := []string{"log", "--pretty=format:" + format, fmt.Sprintf("-n%d", n), "--", path}
	out, err := r.git(ctx, args...).CombinedOutput()
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
func (r *Repo) GetFileAtCommit(ctx context.Context, hash, filePath string) ([]byte, error) {
	fullPath := fmt.Sprintf("%s:%s", hash, filePath)
	out, err := r.git(ctx, "show", fullPath).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get file at commit: %w", err)
	}
	return out, nil
}

// SetRemote adds or updates a remote in the repository.
// If url is empty, the remote is removed.
func (r *Repo) SetRemote(ctx context.Context, name, url string) error {
	// Check if remote already exists
	out, err := r.git(ctx, "remote").CombinedOutput()
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
			return r.git(ctx, "remote", "remove", name).Run()
		}
		return nil
	}

	if exists {
		return r.git(ctx, "remote", "set-url", name, url).Run()
	}
	return r.git(ctx, "remote", "add", name, url).Run()
}

// Push pushes changes to a remote repository.
func (r *Repo) Push(ctx context.Context, remoteName, branch string) error {
	if branch == "" {
		branch = "master"
		// Check if current branch is different
		out, err := r.git(ctx, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
		if err == nil {
			branch = strings.TrimSpace(string(out))
		}
	}

	return r.git(ctx, "push", remoteName, branch).Run()
}

func (r *Repo) git(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	return cmd
}

// commitFS implements fs.FS for a specific commit.
type commitFS struct {
	repo *Repo
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
	out, err := c.repo.git(c.ctx, "ls-tree", c.hash, name).CombinedOutput()
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	line := strings.TrimSpace(string(out))
	if line == "" {
		// Could be a directory - check with trailing slash
		out, err = c.repo.git(c.ctx, "ls-tree", c.hash, name+"/").CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) == "" {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		// It's a directory
		return &commitDir{fs: c, name: name}, nil
	}

	// Parse ls-tree output: "mode type hash\tname"
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	if parts[1] == "tree" {
		return &commitDir{fs: c, name: name}, nil
	}

	// It's a file - get the content
	data, err := c.repo.GetFileAtCommit(c.ctx, c.hash, name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	return &memFile{name: filepath.Base(name), data: data}, nil
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
