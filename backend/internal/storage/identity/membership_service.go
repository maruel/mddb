package identity

import (
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)

var (
	errMembershipExists   = errors.New("membership already exists")
	errMembershipNotFound = errors.New("membership not found")
)

// MembershipService handles user-organization relationships.
type MembershipService struct {
	table *jsonldb.Table[*entity.Membership]
}

// NewMembershipService creates a new membership service.
func NewMembershipService(rootDir string) (*MembershipService, error) {
	dbDir := filepath.Join(rootDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional for data directories
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	tablePath := filepath.Join(dbDir, "memberships.jsonl")
	table, err := jsonldb.NewTable[*entity.Membership](tablePath)
	if err != nil {
		return nil, err
	}

	return &MembershipService{table: table}, nil
}

// findByUserAndOrg finds a membership by user and organization IDs.
func (s *MembershipService) findByUserAndOrg(userID, orgID jsonldb.ID) *entity.Membership {
	for m := range s.table.Iter(0) {
		if m.UserID == userID && m.OrganizationID == orgID {
			return m
		}
	}
	return nil
}

// Create adds a user to an organization.
func (s *MembershipService) Create(userID, orgID jsonldb.ID, role entity.UserRole) (*entity.Membership, error) {
	if userID.IsZero() {
		return nil, errUserIDEmpty
	}
	if orgID.IsZero() {
		return nil, errOrgIDEmpty
	}

	if s.findByUserAndOrg(userID, orgID) != nil {
		return nil, errMembershipExists
	}

	membership := &entity.Membership{
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
func (s *MembershipService) Get(userID, orgID jsonldb.ID) (*entity.Membership, error) {
	m := s.findByUserAndOrg(userID, orgID)
	if m == nil {
		return nil, errMembershipNotFound
	}
	return m, nil
}

// Iter iterates over all memberships for a user.
func (s *MembershipService) Iter(userID jsonldb.ID) (iter.Seq[*entity.Membership], error) {
	if userID.IsZero() {
		return nil, errUserIDEmpty
	}
	return func(yield func(*entity.Membership) bool) {
		for m := range s.table.Iter(0) {
			if m.UserID == userID && !yield(m) {
				return
			}
		}
	}, nil
}

// Update persists changes to a membership.
func (s *MembershipService) Update(m *entity.Membership) error {
	if m == nil || m.ID.IsZero() {
		return errMembershipNotFound
	}
	_, err := s.table.Update(m)
	return err
}
