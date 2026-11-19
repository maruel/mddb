// Persisted workspace membership rows and workspace roles.

package v1

import (
	"github.com/maruel/ksid"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// WorkspaceRole defines the permissions for a user within a workspace.
type WorkspaceRole string

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

const (
	// WSRoleAdmin has full workspace control.
	WSRoleAdmin WorkspaceRole = "ws:admin"
	// WSRoleEditor can create and modify content.
	WSRoleEditor WorkspaceRole = "ws:editor"
	// WSRoleViewer can only read content.
	WSRoleViewer WorkspaceRole = "ws:viewer"
)

// WorkspaceMembership represents a user's relationship with a workspace.
type WorkspaceMembership struct {
	ID          ksid.ID                     `json:"id" jsonschema:"description=Unique membership identifier"`
	UserID      ksid.ID                     `json:"user_id" jsonschema:"description=User ID this membership belongs to"`
	WorkspaceID ksid.ID                     `json:"workspace_id" jsonschema:"description=Workspace ID the user is a member of"`
	Role        WorkspaceRole               `json:"role" jsonschema:"description=User role within the workspace (admin/editor/viewer)"`
	Settings    WorkspaceMembershipSettings `json:"settings" jsonschema:"description=User preferences within this workspace"`
	Created     storagev1.Time              `json:"created" jsonschema:"description=Membership creation timestamp"`
}

// Clone returns a copy of the WorkspaceMembership.
func (m *WorkspaceMembership) Clone() *WorkspaceMembership {
	c := *m
	return &c
}

// GetID returns the WorkspaceMembership's ID.
func (m *WorkspaceMembership) GetID() ksid.ID {
	return m.ID
}

// Validate checks that the WorkspaceMembership is valid.
func (m *WorkspaceMembership) Validate() error {
	if m.ID.IsZero() {
		return ErrIDRequired
	}
	if m.UserID.IsZero() {
		return ErrUserIDEmpty
	}
	if m.WorkspaceID.IsZero() {
		return ErrWSIDEmpty
	}
	if !m.Role.IsValid() {
		return ErrInvalidWSRole
	}
	return nil
}

// WorkspaceMembershipSettings represents user preferences within a specific workspace.
type WorkspaceMembershipSettings struct {
	Notifications bool `json:"notifications" jsonschema:"description=Whether notifications are enabled"`
}
