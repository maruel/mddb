// Package content provides versioned file storage for the mddb system.
package content

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// FileStoreService manages versioned file storage for all workspaces.
// Each workspace has its own WorkspaceFileStore instance.
// Global operations (org quotas, workspace initialization) are handled here.
type FileStoreService struct {
	rootDir string
	git     *git.Manager
	wsSvc   *identity.WorkspaceService
	orgSvc  *identity.OrganizationService
	mu      sync.RWMutex
	stores  map[jsonldb.ID]*WorkspaceFileStore // wsID -> WorkspaceFileStore
}

// page is an internal type for reading/writing page markdown files.
type page struct {
	id       jsonldb.ID
	title    string
	content  string
	created  storage.Time
	modified storage.Time
	tags     []string
}

// NewFileStoreService creates a versioned file store service.
// gitMgr is required - all operations are versioned.
// wsSvc provides quota limits for workspaces.
// orgSvc provides quota limits for organizations.
func NewFileStoreService(rootDir string, gitMgr *git.Manager, wsSvc *identity.WorkspaceService, orgSvc *identity.OrganizationService) (*FileStoreService, error) {
	if gitMgr == nil {
		return nil, errors.New("git manager is required")
	}
	if wsSvc == nil {
		return nil, errors.New("workspace service is required")
	}
	if orgSvc == nil {
		return nil, errors.New("organization service is required")
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for user data directories
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}

	return &FileStoreService{
		rootDir: rootDir,
		git:     gitMgr,
		wsSvc:   wsSvc,
		orgSvc:  orgSvc,
		stores:  make(map[jsonldb.ID]*WorkspaceFileStore),
	}, nil
}

// GetWorkspaceStore returns a WorkspaceFileStore for the given workspace.
// Creates and caches the store on first access.
func (svc *FileStoreService) GetWorkspaceStore(ctx context.Context, wsID jsonldb.ID) (*WorkspaceFileStore, error) {
	if wsID.IsZero() {
		return nil, errWSIDRequired
	}

	// Fast path: check cache without lock
	svc.mu.RLock()
	if store, ok := svc.stores[wsID]; ok {
		svc.mu.RUnlock()
		return store, nil
	}
	svc.mu.RUnlock()

	// Slow path: load and cache
	svc.mu.Lock()
	defer svc.mu.Unlock()

	// Double-check after acquiring lock
	if store, ok := svc.stores[wsID]; ok {
		return store, nil
	}

	// Fetch workspace config and git repo
	ws, err := svc.wsSvc.Get(wsID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	repo, err := svc.git.Repo(ctx, wsID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get git repo: %w", err)
	}

	wsDir := filepath.Join(svc.rootDir, wsID.String())
	store := newWorkspaceFileStore(wsDir, repo, &ws.Quotas)
	svc.stores[wsID] = store
	return store, nil
}

// InitWorkspace initializes storage for a new workspace.
// Creates the workspace directory structure and initializes git.
func (svc *FileStoreService) InitWorkspace(ctx context.Context, wsID jsonldb.ID) error {
	if wsID.IsZero() {
		return errWSIDRequired
	}
	wsDir := filepath.Join(svc.rootDir, wsID.String())
	pagesDir := filepath.Join(wsDir, "pages")
	if err := os.MkdirAll(pagesDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Getting the repo initializes git if needed
	repo, err := svc.git.Repo(ctx, wsID.String())
	if err != nil {
		return fmt.Errorf("failed to initialize git repo for workspace %s: %w", wsID, err)
	}

	// Write AGENTS.md in the root of the workspace.
	agentsPath := filepath.Join(wsDir, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte(storage.AgentsMD), 0o644); err != nil { //nolint:gosec // G306: 0o644 is intentional for documentation files
		return fmt.Errorf("failed to write AGENTS.md: %w", err)
	}

	// Commit AGENTS.md using default author.
	if err := repo.CommitTx(ctx, git.Author{}, func() (string, []string, error) {
		return "initial: add AGENTS.md", []string{"AGENTS.md"}, nil
	}); err != nil {
		return fmt.Errorf("failed to commit AGENTS.md: %w", err)
	}

	return nil
}

// CheckOrgStorageQuota returns an error if adding the given bytes would exceed the organization's total storage quota.
// This checks the sum of storage usage across all workspaces in the organization.
func (svc *FileStoreService) CheckOrgStorageQuota(wsID jsonldb.ID, additionalBytes int64) error {
	ws, err := svc.wsSvc.Get(wsID)
	if err != nil {
		return err
	}
	org, err := svc.orgSvc.Get(ws.OrganizationID)
	if err != nil {
		return err
	}

	orgUsage, err := svc.GetOrganizationUsage(ws.OrganizationID)
	if err != nil {
		return err
	}

	maxOrgStorageBytes := int64(org.Quotas.MaxTotalStorageGB) * 1024 * 1024 * 1024
	if orgUsage+additionalBytes > maxOrgStorageBytes {
		return errQuotaExceeded
	}
	return nil
}

// GetOrganizationUsage returns the total storage usage across all workspaces in the organization.
func (svc *FileStoreService) GetOrganizationUsage(orgID jsonldb.ID) (int64, error) {
	if orgID.IsZero() {
		return 0, errOrgIDRequired
	}

	var totalUsage int64

	for ws := range svc.wsSvc.IterByOrg(orgID) {
		wsDir := filepath.Join(svc.rootDir, ws.ID.String())
		err := filepath.Walk(wsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil //nolint:nilerr // Intentionally continue walking on error
			}
			if info != nil && !info.IsDir() {
				totalUsage += info.Size()
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			slog.Error("failed to calculate workspace usage", "wsID", ws.ID, "error", err)
		}
	}

	return totalUsage, nil
}
