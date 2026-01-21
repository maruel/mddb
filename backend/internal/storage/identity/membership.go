package identity

import (
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// UserRole defines the permissions for a user.
type UserRole string

const (
	// UserRoleAdmin has full access to all resources and settings.
	UserRoleAdmin UserRole = "admin"
	// UserRoleEditor can create and modify content but cannot manage users.
	UserRoleEditor UserRole = "editor"
	// UserRoleViewer can only read content.
	UserRoleViewer UserRole = "viewer"
)

// Membership represents a user's relationship with an organization.
type Membership struct {
	ID             jsonldb.ID         `json:"id" jsonschema:"description=Unique membership identifier"`
	UserID         jsonldb.ID         `json:"user_id" jsonschema:"description=User ID this membership belongs to"`
	OrganizationID jsonldb.ID         `json:"organization_id" jsonschema:"description=Organization ID the user is a member of"`
	Role           UserRole           `json:"role" jsonschema:"description=User role within the organization (admin/editor/viewer)"`
	Settings       MembershipSettings `json:"settings" jsonschema:"description=User preferences within this organization"`
	Created        time.Time          `json:"created" jsonschema:"description=Membership creation timestamp"`
}

// Clone returns a copy of the Membership.
func (m *Membership) Clone() *Membership {
	c := *m
	return &c
}

// GetID returns the Membership's ID.
func (m *Membership) GetID() jsonldb.ID {
	return m.ID
}

// Validate checks that the Membership is valid.
func (m *Membership) Validate() error {
	if m.ID.IsZero() {
		return errIDRequired
	}
	if m.UserID.IsZero() {
		return errUserIDEmpty
	}
	if m.OrganizationID.IsZero() {
		return errOrgIDEmpty
	}
	if m.Role == "" {
		return errRoleRequired
	}
	return nil
}

// MembershipSettings represents user preferences within a specific organization.
type MembershipSettings struct {
	Notifications bool `json:"notifications" jsonschema:"description=Whether email notifications are enabled"`
}

// userOrgKey is a composite key for user+organization lookups.
type userOrgKey struct {
	UserID jsonldb.ID
	OrgID  jsonldb.ID
}

// MembershipService handles user-organization relationships.
type MembershipService struct {
	table     *jsonldb.Table[*Membership]
	byUserID  *jsonldb.Index[jsonldb.ID, *Membership]
	byUserOrg *jsonldb.UniqueIndex[userOrgKey, *Membership]
}

// NewMembershipService creates a new membership service.
func NewMembershipService(rootDir string) (*MembershipService, error) {
	dbDir := filepath.Join(rootDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}
	tablePath := filepath.Join(dbDir, "memberships.jsonl")
	table, err := jsonldb.NewTable[*Membership](tablePath)
	if err != nil {
		return nil, err
	}
	byUserID := jsonldb.NewIndex(table, func(m *Membership) jsonldb.ID { return m.UserID })
	byUserOrg := jsonldb.NewUniqueIndex(table, func(m *Membership) userOrgKey {
		return userOrgKey{UserID: m.UserID, OrgID: m.OrganizationID}
	})
	return &MembershipService{table: table, byUserID: byUserID, byUserOrg: byUserOrg}, nil
}

// findByUserAndOrg finds a membership by user and organization IDs. O(1) via index.
func (s *MembershipService) findByUserAndOrg(userID, orgID jsonldb.ID) *Membership {
	return s.byUserOrg.Get(userOrgKey{UserID: userID, OrgID: orgID})
}

// Create adds a user to an organization.
func (s *MembershipService) Create(userID, orgID jsonldb.ID, role UserRole) (*Membership, error) {
	if userID.IsZero() {
		return nil, errUserIDEmpty
	}
	if orgID.IsZero() {
		return nil, errOrgIDEmpty
	}
	if s.findByUserAndOrg(userID, orgID) != nil {
		return nil, errMembershipExists
	}
	membership := &Membership{
		ID:             jsonldb.NewID(),
		UserID:         userID,
		OrganizationID: orgID,
		Role:           role,
		Created:        time.Now(),
	}
	if err := s.table.Append(membership); err != nil {
		return nil, err
	}
	return membership, nil
}

// Get retrieves a specific user-org relationship.
func (s *MembershipService) Get(userID, orgID jsonldb.ID) (*Membership, error) {
	m := s.findByUserAndOrg(userID, orgID)
	if m == nil {
		return nil, errMembershipNotFound
	}
	return m, nil
}

// Iter iterates over all memberships for a user. O(1) via index.
func (s *MembershipService) Iter(userID jsonldb.ID) (iter.Seq[*Membership], error) {
	if userID.IsZero() {
		return nil, errUserIDEmpty
	}
	return s.byUserID.Iter(userID), nil
}

// Modify atomically modifies a membership.
func (s *MembershipService) Modify(id jsonldb.ID, fn func(m *Membership) error) (*Membership, error) {
	if id.IsZero() {
		return nil, errMembershipNotFound
	}
	return s.table.Modify(id, fn)
}

//

var (
	errMembershipExists   = errors.New("membership already exists")
	errMembershipNotFound = errors.New("membership not found")
)
