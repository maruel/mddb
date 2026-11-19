// Persisted organization membership rows and organization roles.

package v1

import (
	"github.com/maruel/ksid"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// OrganizationRole defines the role of a user within an organization.
type OrganizationRole string

// IsValid returns true if the role is a valid organization role.
func (r OrganizationRole) IsValid() bool {
	switch r {
	case OrgRoleOwner, OrgRoleAdmin, OrgRoleMember:
		return true
	}
	return false
}

// CanManageMembers returns true if the role can invite/remove members.
func (r OrganizationRole) CanManageMembers() bool {
	return r == OrgRoleOwner || r == OrgRoleAdmin
}

// CanManageWorkspaces returns true if the role can create/delete workspaces.
func (r OrganizationRole) CanManageWorkspaces() bool {
	return r == OrgRoleOwner || r == OrgRoleAdmin
}

// CanManageBilling returns true if the role can manage billing.
func (r OrganizationRole) CanManageBilling() bool {
	return r == OrgRoleOwner
}

// CanDeleteOrg returns true if the role can delete the organization.
func (r OrganizationRole) CanDeleteOrg() bool {
	return r == OrgRoleOwner
}

const (
	// OrgRoleOwner has full control including billing.
	OrgRoleOwner OrganizationRole = "org:owner"
	// OrgRoleAdmin can manage workspaces and members.
	OrgRoleAdmin OrganizationRole = "org:admin"
	// OrgRoleMember can only access granted workspaces.
	OrgRoleMember OrganizationRole = "org:member"
)

// OrganizationMembership represents a user's relationship with an organization.
type OrganizationMembership struct {
	ID             ksid.ID          `json:"id" jsonschema:"description=Unique membership identifier"`
	UserID         ksid.ID          `json:"user_id" jsonschema:"description=User ID this membership belongs to"`
	OrganizationID ksid.ID          `json:"organization_id" jsonschema:"description=Organization ID the user is a member of"`
	Role           OrganizationRole `json:"role" jsonschema:"description=User role within the organization (owner/admin/member)"`
	Created        storagev1.Time   `json:"created" jsonschema:"description=Membership creation timestamp"`
}

// Clone returns a copy of the OrganizationMembership.
func (m *OrganizationMembership) Clone() *OrganizationMembership {
	c := *m
	return &c
}

// GetID returns the OrganizationMembership's ID.
func (m *OrganizationMembership) GetID() ksid.ID {
	return m.ID
}

// Validate checks that the OrganizationMembership is valid.
func (m *OrganizationMembership) Validate() error {
	if m.ID.IsZero() {
		return ErrIDRequired
	}
	if m.UserID.IsZero() {
		return ErrUserIDEmpty
	}
	if m.OrganizationID.IsZero() {
		return ErrOrgIDEmpty
	}
	if !m.Role.IsValid() {
		return ErrInvalidOrgRole
	}
	return nil
}
