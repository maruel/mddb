// Manages user memberships within workspaces.

package identity

import (
	"errors"
	"iter"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// WorkspaceMembershipService handles user-workspace relationships.
type WorkspaceMembershipService struct {
	table      *jsonldb.Table[*identityv1.WorkspaceMembership]
	byUserID   *jsonldb.Index[ksid.ID, *identityv1.WorkspaceMembership]
	byWSID     *jsonldb.Index[ksid.ID, *identityv1.WorkspaceMembership]
	byUserWS   *jsonldb.UniqueIndex[userWSKey, *identityv1.WorkspaceMembership]
	wsService  *WorkspaceService
	orgService *OrganizationService
}

// NewWorkspaceMembershipService creates a new workspace membership service.
func NewWorkspaceMembershipService(tablePath string, wsService *WorkspaceService, orgService *OrganizationService) (*WorkspaceMembershipService, error) {
	table, err := jsonldb.NewTable[*identityv1.WorkspaceMembership](tablePath)
	if err != nil {
		return nil, err
	}
	byUserID := jsonldb.NewIndex(table, func(m *identityv1.WorkspaceMembership) ksid.ID { return m.UserID })
	byWSID := jsonldb.NewIndex(table, func(m *identityv1.WorkspaceMembership) ksid.ID { return m.WorkspaceID })
	byUserWS := jsonldb.NewUniqueIndex(table, func(m *identityv1.WorkspaceMembership) userWSKey {
		return userWSKey{UserID: m.UserID, WSID: m.WorkspaceID}
	})
	return &WorkspaceMembershipService{
		table:      table,
		byUserID:   byUserID,
		byWSID:     byWSID,
		byUserWS:   byUserWS,
		wsService:  wsService,
		orgService: orgService,
	}, nil
}

// findByUserAndWorkspace finds a membership by user and workspace IDs. O(1) via index.
func (s *WorkspaceMembershipService) findByUserAndWorkspace(userID, wsID ksid.ID) *identityv1.WorkspaceMembership {
	return s.byUserWS.Get(userWSKey{UserID: userID, WSID: wsID})
}

// Create adds a user to a workspace.
func (s *WorkspaceMembershipService) Create(userID, wsID ksid.ID, role identityv1.WorkspaceRole) (*identityv1.WorkspaceMembership, error) {
	if userID.IsZero() {
		return nil, identityv1.ErrUserIDEmpty
	}
	if wsID.IsZero() {
		return nil, identityv1.ErrWSIDEmpty
	}
	if !role.IsValid() {
		return nil, identityv1.ErrInvalidWSRole
	}
	if s.findByUserAndWorkspace(userID, wsID) != nil {
		return nil, errWSMembershipExists
	}

	// Check org workspace member quota
	ws, err := s.wsService.Get(wsID)
	if err != nil {
		return nil, err
	}
	org, err := s.orgService.Get(ws.OrganizationID)
	if err != nil {
		return nil, err
	}
	if s.CountWSMemberships(wsID) >= org.Quotas.MaxMembersPerWorkspace {
		return nil, errQuotaExceeded
	}

	membership := &identityv1.WorkspaceMembership{
		ID:          ksid.NewID(),
		UserID:      userID,
		WorkspaceID: wsID,
		Role:        role,
		Created:     storagev1.Now(),
	}
	if err := s.table.Append(membership, nil); err != nil {
		return nil, err
	}
	return membership, nil
}

// Get retrieves a specific user-workspace relationship.
func (s *WorkspaceMembershipService) Get(userID, wsID ksid.ID) (*identityv1.WorkspaceMembership, error) {
	m := s.findByUserAndWorkspace(userID, wsID)
	if m == nil {
		return nil, errWSMembershipNotFound
	}
	return m, nil
}

// GetByID retrieves a membership by its ID.
func (s *WorkspaceMembershipService) GetByID(id ksid.ID) (*identityv1.WorkspaceMembership, error) {
	m := s.table.Get(id)
	if m == nil {
		return nil, errWSMembershipNotFound
	}
	return m, nil
}

// IterByUser iterates over all workspace memberships for a user. O(1) via index.
func (s *WorkspaceMembershipService) IterByUser(userID ksid.ID) iter.Seq[*identityv1.WorkspaceMembership] {
	return s.byUserID.Iter(userID)
}

// IterByWorkspace iterates over all memberships in a workspace. O(1) via index.
func (s *WorkspaceMembershipService) IterByWorkspace(wsID ksid.ID) iter.Seq[*identityv1.WorkspaceMembership] {
	return s.byWSID.Iter(wsID)
}

// CountWSMemberships returns the number of members in a workspace.
func (s *WorkspaceMembershipService) CountWSMemberships(wsID ksid.ID) int {
	count := 0
	for range s.byWSID.Iter(wsID) {
		count++
	}
	return count
}

// Modify atomically modifies a membership.
func (s *WorkspaceMembershipService) Modify(id ksid.ID, fn func(m *identityv1.WorkspaceMembership) error) (*identityv1.WorkspaceMembership, error) {
	if id.IsZero() {
		return nil, errWSMembershipNotFound
	}
	return s.table.Modify(id, fn)
}

// Delete removes a membership.
func (s *WorkspaceMembershipService) Delete(id ksid.ID) error {
	if id.IsZero() {
		return errWSMembershipNotFound
	}
	if s.table.Get(id) == nil {
		return errWSMembershipNotFound
	}
	if _, err := s.table.Delete(id); err != nil {
		return err
	}
	return nil
}

// DeleteAllByWorkspace removes all memberships for a workspace.
func (s *WorkspaceMembershipService) DeleteAllByWorkspace(wsID ksid.ID) error {
	var toDelete []ksid.ID
	for m := range s.byWSID.Iter(wsID) {
		toDelete = append(toDelete, m.ID)
	}
	for _, id := range toDelete {
		if _, err := s.table.Delete(id); err != nil {
			return err
		}
	}
	return nil
}

// DeleteByUserInOrg removes all workspace memberships for a user
// in workspaces belonging to a specific organization.
// This should be called when removing a user from an organization
// to prevent orphaned workspace memberships.
func (s *WorkspaceMembershipService) DeleteByUserInOrg(userID, orgID ksid.ID) error {
	var toDelete []ksid.ID
	for m := range s.byUserID.Iter(userID) {
		ws, err := s.wsService.Get(m.WorkspaceID)
		if err != nil {
			continue // Skip if workspace not found
		}
		if ws.OrganizationID == orgID {
			toDelete = append(toDelete, m.ID)
		}
	}
	for _, id := range toDelete {
		if _, err := s.table.Delete(id); err != nil {
			return err
		}
	}
	return nil
}

//

var (
	errWSMembershipExists   = errors.New("workspace membership already exists")
	errWSMembershipNotFound = errors.New("workspace membership not found")
)

// userWSKey is a composite key for user+workspace lookups.
type userWSKey struct {
	UserID ksid.ID
	WSID   ksid.ID
}
