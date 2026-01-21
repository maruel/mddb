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

var (
	errMembershipExists   = errors.New("membership already exists")
	errMembershipNotFound = errors.New("membership not found")
)

// MembershipService handles user-organization relationships.
type MembershipService struct {
	table *jsonldb.Table[*Membership]
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

	return &MembershipService{table: table}, nil
}

// findByUserAndOrg finds a membership by user and organization IDs.
func (s *MembershipService) findByUserAndOrg(userID, orgID jsonldb.ID) *Membership {
	for m := range s.table.Iter(0) {
		if m.UserID == userID && m.OrganizationID == orgID {
			return m
		}
	}
	return nil
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

// Iter iterates over all memberships for a user.
func (s *MembershipService) Iter(userID jsonldb.ID) (iter.Seq[*Membership], error) {
	if userID.IsZero() {
		return nil, errUserIDEmpty
	}
	return func(yield func(*Membership) bool) {
		for m := range s.table.Iter(0) {
			if m.UserID == userID && !yield(m) {
				return
			}
		}
	}, nil
}

// Modify atomically modifies a membership.
func (s *MembershipService) Modify(id jsonldb.ID, fn func(m *Membership) error) (*Membership, error) {
	if id.IsZero() {
		return nil, errMembershipNotFound
	}
	return s.table.Modify(id, fn)
}
