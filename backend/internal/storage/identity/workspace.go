// Manages workspace entities, quotas, and settings.

package identity

import (
	"context"
	"errors"
	"iter"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// DefaultWorkspaceQuotas returns the default quotas for a new workspace.
// All fields default to -1 (inherit from server/org layer).
func DefaultWorkspaceQuotas() storagev1.ResourceQuotas {
	return storagev1.ResourceQuotas{
		MaxPages:              -1,
		MaxStorageBytes:       -1,
		MaxRecordsPerTable:    -1,
		MaxAssetSizeBytes:     -1,
		MaxTablesPerWorkspace: -1,
		MaxColumnsPerTable:    -1,
	}
}

// WorkspaceService handles workspace management.
type WorkspaceService struct {
	table *jsonldb.Table[*identityv1.Workspace]
	byOrg *jsonldb.Index[ksid.ID, *identityv1.Workspace]
}

// NewWorkspaceService creates a new workspace service.
func NewWorkspaceService(tablePath string) (*WorkspaceService, error) {
	table, err := jsonldb.NewTable[*identityv1.Workspace](tablePath)
	if err != nil {
		return nil, err
	}
	byOrg := jsonldb.NewIndex(table, func(w *identityv1.Workspace) ksid.ID { return w.OrganizationID })
	return &WorkspaceService{
		table: table,
		byOrg: byOrg,
	}, nil
}

// Create creates a new workspace in an organization.
func (s *WorkspaceService) Create(_ context.Context, orgID ksid.ID, name string) (*identityv1.Workspace, error) {
	if orgID.IsZero() {
		return nil, identityv1.ErrOrgIDEmpty
	}
	if name == "" {
		return nil, errWorkspaceNameRequired
	}

	ws := &identityv1.Workspace{
		ID:             ksid.NewID(),
		OrganizationID: orgID,
		Name:           name,
		Quotas:         DefaultWorkspaceQuotas(),
		Created:        storagev1.Now(),
	}
	if err := s.table.Append(ws, nil); err != nil {
		return nil, err
	}
	return ws, nil
}

// Get retrieves a workspace by ID.
func (s *WorkspaceService) Get(id ksid.ID) (*identityv1.Workspace, error) {
	ws := s.table.Get(id)
	if ws == nil {
		return nil, errWorkspaceNotFound
	}
	return ws, nil
}

// Modify atomically modifies a workspace.
func (s *WorkspaceService) Modify(id ksid.ID, fn func(ws *identityv1.Workspace) error) (*identityv1.Workspace, error) {
	if id.IsZero() {
		return nil, errWorkspaceNotFound
	}
	return s.table.Modify(id, fn)
}

// IterByOrg iterates over workspaces in an organization. O(1) via index.
func (s *WorkspaceService) IterByOrg(orgID ksid.ID) iter.Seq[*identityv1.Workspace] {
	return s.byOrg.Iter(orgID)
}

// Iter iterates over all workspaces with ID greater than startID.
func (s *WorkspaceService) Iter(startID ksid.ID) iter.Seq[*identityv1.Workspace] {
	return s.table.Iter(startID)
}

// CountByOrg returns the number of workspaces in an organization.
func (s *WorkspaceService) CountByOrg(orgID ksid.ID) int {
	count := 0
	for range s.byOrg.Iter(orgID) {
		count++
	}
	return count
}

// Count returns the total number of workspaces.
func (s *WorkspaceService) Count() int {
	return s.table.Len()
}

// Delete deletes a workspace.
func (s *WorkspaceService) Delete(id ksid.ID) error {
	if id.IsZero() {
		return errWorkspaceNotFound
	}
	if s.table.Get(id) == nil {
		return errWorkspaceNotFound
	}
	if _, err := s.table.Delete(id); err != nil {
		return err
	}
	return nil
}

//

var (
	errWorkspaceNameRequired = errors.New("workspace name is required")
	errWorkspaceNotFound     = errors.New("workspace not found")
)
