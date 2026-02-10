// Manages user memberships within workspaces.

package identity

import (
	"errors"
	"iter"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/rid"
	"github.com/maruel/mddb/backend/internal/storage"
)

// WorkspaceRole defines the permissions for a user within a workspace.
type WorkspaceRole string

const (
	// WSRoleAdmin has full workspace control.
	WSRoleAdmin WorkspaceRole = "ws:admin"
	// WSRoleEditor can create and modify content.
	WSRoleEditor WorkspaceRole = "ws:editor"
	// WSRoleViewer can only read content.
	WSRoleViewer WorkspaceRole = "ws:viewer"
)

// IsValid returns true if the role is a valid workspace role.
func (r WorkspaceRole) IsValid() bool {
	switch r {
	case WSRoleAdmin, WSRoleEditor, WSRoleViewer:
		return true
	}
	return false
}

// CanEdit returns true if the role can create/modify content.
func (r WorkspaceRole) CanEdit() bool {
	return r == WSRoleAdmin || r == WSRoleEditor
}

// CanManageMembers returns true if the role can invite/remove workspace members.
func (r WorkspaceRole) CanManageMembers() bool {
	return r == WSRoleAdmin
}

// CanManageSettings returns true if the role can configure workspace settings.
func (r WorkspaceRole) CanManageSettings() bool {
	return r == WSRoleAdmin
}

// WorkspaceMembership represents a user's relationship with a workspace.
type WorkspaceMembership struct {
	ID          rid.ID                      `json:"id" jsonschema:"description=Unique membership identifier"`
	UserID      rid.ID                      `json:"user_id" jsonschema:"description=User ID this membership belongs to"`
	WorkspaceID rid.ID                      `json:"workspace_id" jsonschema:"description=Workspace ID the user is a member of"`
	Role        WorkspaceRole               `json:"role" jsonschema:"description=User role within the workspace (admin/editor/viewer)"`
	Settings    WorkspaceMembershipSettings `json:"settings" jsonschema:"description=User preferences within this workspace"`
	Created     storage.Time                `json:"created" jsonschema:"description=Membership creation timestamp"`
}

// Clone returns a copy of the WorkspaceMembership.
func (m *WorkspaceMembership) Clone() *WorkspaceMembership {
	c := *m
	return &c
}

// GetID returns the WorkspaceMembership's ID.
func (m *WorkspaceMembership) GetID() rid.ID {
	return m.ID
}

// Validate checks that the WorkspaceMembership is valid.
func (m *WorkspaceMembership) Validate() error {
	if m.ID.IsZero() {
		return errIDRequired
	}
	if m.UserID.IsZero() {
		return errUserIDEmpty
	}
	if m.WorkspaceID.IsZero() {
		return errWSIDEmpty
	}
	if !m.Role.IsValid() {
		return errInvalidWSRole
	}
	return nil
}

// WorkspaceMembershipSettings represents user preferences within a specific workspace.
type WorkspaceMembershipSettings struct {
	Notifications bool `json:"notifications" jsonschema:"description=Whether notifications are enabled"`
}

// WorkspaceMembershipService handles user-workspace relationships.
type WorkspaceMembershipService struct {
	table      *jsonldb.Table[*WorkspaceMembership]
	byUserID   *jsonldb.Index[rid.ID, *WorkspaceMembership]
	byWSID     *jsonldb.Index[rid.ID, *WorkspaceMembership]
	byUserWS   *jsonldb.UniqueIndex[userWSKey, *WorkspaceMembership]
	wsService  *WorkspaceService
	orgService *OrganizationService
}

// NewWorkspaceMembershipService creates a new workspace membership service.
func NewWorkspaceMembershipService(tablePath string, wsService *WorkspaceService, orgService *OrganizationService) (*WorkspaceMembershipService, error) {
	table, err := jsonldb.NewTable[*WorkspaceMembership](tablePath)
	if err != nil {
		return nil, err
	}
	byUserID := jsonldb.NewIndex(table, func(m *WorkspaceMembership) rid.ID { return m.UserID })
	byWSID := jsonldb.NewIndex(table, func(m *WorkspaceMembership) rid.ID { return m.WorkspaceID })
	byUserWS := jsonldb.NewUniqueIndex(table, func(m *WorkspaceMembership) userWSKey {
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
func (s *WorkspaceMembershipService) findByUserAndWorkspace(userID, wsID rid.ID) *WorkspaceMembership {
	return s.byUserWS.Get(userWSKey{UserID: userID, WSID: wsID})
}

// Create adds a user to a workspace.
func (s *WorkspaceMembershipService) Create(userID, wsID rid.ID, role WorkspaceRole) (*WorkspaceMembership, error) {
	if userID.IsZero() {
		return nil, errUserIDEmpty
	}
	if wsID.IsZero() {
		return nil, errWSIDEmpty
	}
	if !role.IsValid() {
		return nil, errInvalidWSRole
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

	membership := &WorkspaceMembership{
		ID:          rid.NewID(),
		UserID:      userID,
		WorkspaceID: wsID,
		Role:        role,
		Created:     storage.Now(),
	}
	if err := s.table.Append(membership); err != nil {
		return nil, err
	}
	return membership, nil
}

// Get retrieves a specific user-workspace relationship.
func (s *WorkspaceMembershipService) Get(userID, wsID rid.ID) (*WorkspaceMembership, error) {
	m := s.findByUserAndWorkspace(userID, wsID)
	if m == nil {
		return nil, errWSMembershipNotFound
	}
	return m, nil
}

// GetByID retrieves a membership by its ID.
func (s *WorkspaceMembershipService) GetByID(id rid.ID) (*WorkspaceMembership, error) {
	m := s.table.Get(id)
	if m == nil {
		return nil, errWSMembershipNotFound
	}
	return m, nil
}

// IterByUser iterates over all workspace memberships for a user. O(1) via index.
func (s *WorkspaceMembershipService) IterByUser(userID rid.ID) iter.Seq[*WorkspaceMembership] {
	return s.byUserID.Iter(userID)
}

// IterByWorkspace iterates over all memberships in a workspace. O(1) via index.
func (s *WorkspaceMembershipService) IterByWorkspace(wsID rid.ID) iter.Seq[*WorkspaceMembership] {
	return s.byWSID.Iter(wsID)
}

// CountWSMemberships returns the number of members in a workspace.
func (s *WorkspaceMembershipService) CountWSMemberships(wsID rid.ID) int {
	count := 0
	for range s.byWSID.Iter(wsID) {
		count++
	}
	return count
}

// Modify atomically modifies a membership.
func (s *WorkspaceMembershipService) Modify(id rid.ID, fn func(m *WorkspaceMembership) error) (*WorkspaceMembership, error) {
	if id.IsZero() {
		return nil, errWSMembershipNotFound
	}
	return s.table.Modify(id, fn)
}

// Delete removes a membership.
func (s *WorkspaceMembershipService) Delete(id rid.ID) error {
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
func (s *WorkspaceMembershipService) DeleteAllByWorkspace(wsID rid.ID) error {
	var toDelete []rid.ID //nolint:prealloc // Iterator length unknown
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
func (s *WorkspaceMembershipService) DeleteByUserInOrg(userID, orgID rid.ID) error {
	var toDelete []rid.ID
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
	errInvalidWSRole        = errors.New("invalid workspace role")
	errWSIDEmpty            = errors.New("workspace id cannot be empty")
)

// userWSKey is a composite key for user+workspace lookups.
type userWSKey struct {
	UserID rid.ID
	WSID   rid.ID
}
