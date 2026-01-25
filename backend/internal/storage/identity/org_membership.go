package identity

import (
	"errors"
	"iter"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
)

// OrganizationRole defines the role of a user within an organization.
type OrganizationRole string

const (
	// OrgRoleOwner has full control including billing.
	OrgRoleOwner OrganizationRole = "owner"
	// OrgRoleAdmin can manage workspaces and members.
	OrgRoleAdmin OrganizationRole = "admin"
	// OrgRoleMember can only access granted workspaces.
	OrgRoleMember OrganizationRole = "member"
)

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

// OrganizationMembership represents a user's relationship with an organization.
type OrganizationMembership struct {
	ID             jsonldb.ID       `json:"id" jsonschema:"description=Unique membership identifier"`
	UserID         jsonldb.ID       `json:"user_id" jsonschema:"description=User ID this membership belongs to"`
	OrganizationID jsonldb.ID       `json:"organization_id" jsonschema:"description=Organization ID the user is a member of"`
	Role           OrganizationRole `json:"role" jsonschema:"description=User role within the organization (owner/admin/member)"`
	Created        storage.Time     `json:"created" jsonschema:"description=Membership creation timestamp"`
}

// Clone returns a copy of the OrganizationMembership.
func (m *OrganizationMembership) Clone() *OrganizationMembership {
	c := *m
	return &c
}

// GetID returns the OrganizationMembership's ID.
func (m *OrganizationMembership) GetID() jsonldb.ID {
	return m.ID
}

// Validate checks that the OrganizationMembership is valid.
func (m *OrganizationMembership) Validate() error {
	if m.ID.IsZero() {
		return errIDRequired
	}
	if m.UserID.IsZero() {
		return errUserIDEmpty
	}
	if m.OrganizationID.IsZero() {
		return errOrgIDEmpty
	}
	if !m.Role.IsValid() {
		return errInvalidOrgRole
	}
	return nil
}

// OrganizationMembershipService handles user-organization relationships.
type OrganizationMembershipService struct {
	table       *jsonldb.Table[*OrganizationMembership]
	byUserID    *jsonldb.Index[jsonldb.ID, *OrganizationMembership]
	byOrgID     *jsonldb.Index[jsonldb.ID, *OrganizationMembership]
	byUserOrg   *jsonldb.UniqueIndex[userOrgKey, *OrganizationMembership]
	userService *UserService
	orgService  *OrganizationService
}

// NewOrganizationMembershipService creates a new organization membership service.
func NewOrganizationMembershipService(tablePath string, userService *UserService, orgService *OrganizationService) (*OrganizationMembershipService, error) {
	table, err := jsonldb.NewTable[*OrganizationMembership](tablePath)
	if err != nil {
		return nil, err
	}
	byUserID := jsonldb.NewIndex(table, func(m *OrganizationMembership) jsonldb.ID { return m.UserID })
	byOrgID := jsonldb.NewIndex(table, func(m *OrganizationMembership) jsonldb.ID { return m.OrganizationID })
	byUserOrg := jsonldb.NewUniqueIndex(table, func(m *OrganizationMembership) userOrgKey {
		return userOrgKey{UserID: m.UserID, OrgID: m.OrganizationID}
	})
	return &OrganizationMembershipService{
		table:       table,
		byUserID:    byUserID,
		byOrgID:     byOrgID,
		byUserOrg:   byUserOrg,
		userService: userService,
		orgService:  orgService,
	}, nil
}

// findByUserAndOrg finds a membership by user and organization IDs. O(1) via index.
func (s *OrganizationMembershipService) findByUserAndOrg(userID, orgID jsonldb.ID) *OrganizationMembership {
	return s.byUserOrg.Get(userOrgKey{UserID: userID, OrgID: orgID})
}

// Create adds a user to an organization.
func (s *OrganizationMembershipService) Create(userID, orgID jsonldb.ID, role OrganizationRole) (*OrganizationMembership, error) {
	if userID.IsZero() {
		return nil, errUserIDEmpty
	}
	if orgID.IsZero() {
		return nil, errOrgIDEmpty
	}
	if !role.IsValid() {
		return nil, errInvalidOrgRole
	}
	if s.findByUserAndOrg(userID, orgID) != nil {
		return nil, errOrgMembershipExists
	}

	// Check user org quota
	user, err := s.userService.Get(userID)
	if err != nil {
		return nil, err
	}
	if s.CountUserMemberships(userID) >= user.Quotas.MaxOrganizations {
		return nil, errQuotaExceeded
	}

	// Check org member quota
	org, err := s.orgService.Get(orgID)
	if err != nil {
		return nil, err
	}
	if s.CountOrgMemberships(orgID) >= org.Quotas.MaxMembersPerOrg {
		return nil, errQuotaExceeded
	}

	membership := &OrganizationMembership{
		ID:             jsonldb.NewID(),
		UserID:         userID,
		OrganizationID: orgID,
		Role:           role,
		Created:        storage.Now(),
	}
	if err := s.table.Append(membership); err != nil {
		return nil, err
	}
	return membership, nil
}

// Get retrieves a specific user-org relationship.
func (s *OrganizationMembershipService) Get(userID, orgID jsonldb.ID) (*OrganizationMembership, error) {
	m := s.findByUserAndOrg(userID, orgID)
	if m == nil {
		return nil, errOrgMembershipNotFound
	}
	return m, nil
}

// GetByID retrieves a membership by its ID.
func (s *OrganizationMembershipService) GetByID(id jsonldb.ID) (*OrganizationMembership, error) {
	m := s.table.Get(id)
	if m == nil {
		return nil, errOrgMembershipNotFound
	}
	return m, nil
}

// IterByUser iterates over all org memberships for a user. O(1) via index.
func (s *OrganizationMembershipService) IterByUser(userID jsonldb.ID) iter.Seq[*OrganizationMembership] {
	return s.byUserID.Iter(userID)
}

// IterByOrg iterates over all memberships in an organization. O(1) via index.
func (s *OrganizationMembershipService) IterByOrg(orgID jsonldb.ID) iter.Seq[*OrganizationMembership] {
	return s.byOrgID.Iter(orgID)
}

// CountUserMemberships returns the number of organizations a user belongs to.
func (s *OrganizationMembershipService) CountUserMemberships(userID jsonldb.ID) int {
	count := 0
	for range s.byUserID.Iter(userID) {
		count++
	}
	return count
}

// CountOrgMemberships returns the number of members in an organization.
func (s *OrganizationMembershipService) CountOrgMemberships(orgID jsonldb.ID) int {
	count := 0
	for range s.byOrgID.Iter(orgID) {
		count++
	}
	return count
}

// Modify atomically modifies a membership.
func (s *OrganizationMembershipService) Modify(id jsonldb.ID, fn func(m *OrganizationMembership) error) (*OrganizationMembership, error) {
	if id.IsZero() {
		return nil, errOrgMembershipNotFound
	}
	return s.table.Modify(id, fn)
}

// Delete removes a membership.
func (s *OrganizationMembershipService) Delete(id jsonldb.ID) error {
	if id.IsZero() {
		return errOrgMembershipNotFound
	}
	if s.table.Get(id) == nil {
		return errOrgMembershipNotFound
	}
	if _, err := s.table.Delete(id); err != nil {
		return err
	}
	return nil
}

// HasOwner checks if an organization has at least one owner.
func (s *OrganizationMembershipService) HasOwner(orgID jsonldb.ID) bool {
	for m := range s.byOrgID.Iter(orgID) {
		if m.Role == OrgRoleOwner {
			return true
		}
	}
	return false
}

//

var (
	errOrgMembershipExists   = errors.New("organization membership already exists")
	errOrgMembershipNotFound = errors.New("organization membership not found")
	errInvalidOrgRole        = errors.New("invalid organization role")
)
