// Manages user memberships within organizations.

package identity

import (
	"errors"
	"iter"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// OrganizationMembershipService handles user-organization relationships.
type OrganizationMembershipService struct {
	table       *jsonldb.Table[*identityv1.OrganizationMembership]
	byUserID    *jsonldb.Index[ksid.ID, *identityv1.OrganizationMembership]
	byOrgID     *jsonldb.Index[ksid.ID, *identityv1.OrganizationMembership]
	byUserOrg   *jsonldb.UniqueIndex[userOrgKey, *identityv1.OrganizationMembership]
	userService *UserService
	orgService  *OrganizationService
}

// NewOrganizationMembershipService creates a new organization membership service.
func NewOrganizationMembershipService(tablePath string, userService *UserService, orgService *OrganizationService) (*OrganizationMembershipService, error) {
	table, err := jsonldb.NewTable[*identityv1.OrganizationMembership](tablePath)
	if err != nil {
		return nil, err
	}
	byUserID := jsonldb.NewIndex(table, func(m *identityv1.OrganizationMembership) ksid.ID { return m.UserID })
	byOrgID := jsonldb.NewIndex(table, func(m *identityv1.OrganizationMembership) ksid.ID { return m.OrganizationID })
	byUserOrg := jsonldb.NewUniqueIndex(table, func(m *identityv1.OrganizationMembership) userOrgKey {
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
func (s *OrganizationMembershipService) findByUserAndOrg(userID, orgID ksid.ID) *identityv1.OrganizationMembership {
	return s.byUserOrg.Get(userOrgKey{UserID: userID, OrgID: orgID})
}

// Create adds a user to an organization.
func (s *OrganizationMembershipService) Create(userID, orgID ksid.ID, role identityv1.OrganizationRole) (*identityv1.OrganizationMembership, error) {
	if userID.IsZero() {
		return nil, identityv1.ErrUserIDEmpty
	}
	if orgID.IsZero() {
		return nil, identityv1.ErrOrgIDEmpty
	}
	if !role.IsValid() {
		return nil, identityv1.ErrInvalidOrgRole
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

	membership := &identityv1.OrganizationMembership{
		ID:             ksid.NewID(),
		UserID:         userID,
		OrganizationID: orgID,
		Role:           role,
		Created:        storagev1.Now(),
	}
	if err := s.table.Append(membership, nil); err != nil {
		return nil, err
	}
	return membership, nil
}

// Get retrieves a specific user-org relationship.
func (s *OrganizationMembershipService) Get(userID, orgID ksid.ID) (*identityv1.OrganizationMembership, error) {
	m := s.findByUserAndOrg(userID, orgID)
	if m == nil {
		return nil, errOrgMembershipNotFound
	}
	return m, nil
}

// GetByID retrieves a membership by its ID.
func (s *OrganizationMembershipService) GetByID(id ksid.ID) (*identityv1.OrganizationMembership, error) {
	m := s.table.Get(id)
	if m == nil {
		return nil, errOrgMembershipNotFound
	}
	return m, nil
}

// IterByUser iterates over all org memberships for a user. O(1) via index.
func (s *OrganizationMembershipService) IterByUser(userID ksid.ID) iter.Seq[*identityv1.OrganizationMembership] {
	return s.byUserID.Iter(userID)
}

// IterByOrg iterates over all memberships in an organization. O(1) via index.
func (s *OrganizationMembershipService) IterByOrg(orgID ksid.ID) iter.Seq[*identityv1.OrganizationMembership] {
	return s.byOrgID.Iter(orgID)
}

// CountUserMemberships returns the number of organizations a user belongs to.
func (s *OrganizationMembershipService) CountUserMemberships(userID ksid.ID) int {
	count := 0
	for range s.byUserID.Iter(userID) {
		count++
	}
	return count
}

// CountOrgMemberships returns the number of members in an organization.
func (s *OrganizationMembershipService) CountOrgMemberships(orgID ksid.ID) int {
	count := 0
	for range s.byOrgID.Iter(orgID) {
		count++
	}
	return count
}

// Modify atomically modifies a membership.
func (s *OrganizationMembershipService) Modify(id ksid.ID, fn func(m *identityv1.OrganizationMembership) error) (*identityv1.OrganizationMembership, error) {
	if id.IsZero() {
		return nil, errOrgMembershipNotFound
	}
	return s.table.Modify(id, fn)
}

// Delete removes a membership.
func (s *OrganizationMembershipService) Delete(id ksid.ID) error {
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
func (s *OrganizationMembershipService) HasOwner(orgID ksid.ID) bool {
	for m := range s.byOrgID.Iter(orgID) {
		if m.Role == identityv1.OrgRoleOwner {
			return true
		}
	}
	return false
}

//

var (
	errOrgMembershipExists   = errors.New("organization membership already exists")
	errOrgMembershipNotFound = errors.New("organization membership not found")
)
