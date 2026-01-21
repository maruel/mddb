package identity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	mu    sync.RWMutex
	byID  map[string]*entity.Membership // key: userID_orgID (as strings)
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

	s := &MembershipService{
		table: table,
		byID:  make(map[string]*entity.Membership),
	}

	for m := range table.Iter(0) {
		s.byID[membershipKey(m.UserID, m.OrganizationID)] = m
	}

	return s, nil
}

// CreateMembership adds a user to an organization.
func (s *MembershipService) CreateMembership(userID, orgID jsonldb.ID, role entity.UserRole) (*entity.Membership, error) {
	if userID.IsZero() {
		return nil, errUserIDEmpty
	}
	if orgID.IsZero() {
		return nil, errOrgIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := membershipKey(userID, orgID)
	if _, ok := s.byID[key]; ok {
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

	s.byID[key] = membership

	return membership, nil
}

// GetMembership retrieves a specific user-org relationship.
func (s *MembershipService) GetMembership(userID, orgID jsonldb.ID) (*entity.Membership, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.byID[membershipKey(userID, orgID)]
	if !ok {
		return nil, errMembershipNotFound
	}

	return m, nil
}

// ListByUser returns all organizations a user belongs to.
func (s *MembershipService) ListByUser(userID jsonldb.ID) ([]entity.Membership, error) {
	if userID.IsZero() {
		return nil, errUserIDEmpty
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var memberships []entity.Membership
	for _, m := range s.byID {
		if m.UserID == userID {
			memberships = append(memberships, *m)
		}
	}
	return memberships, nil
}

// UpdateRole updates a user's role in an organization.
func (s *MembershipService) UpdateRole(userID, orgID jsonldb.ID, role entity.UserRole) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.byID[membershipKey(userID, orgID)]
	if !ok {
		return errMembershipNotFound
	}

	m.Role = role
	if _, err := s.table.Update(m); err != nil {
		return err
	}
	return nil
}

// UpdateSettings updates user preferences within a specific organization.
func (s *MembershipService) UpdateSettings(userID, orgID jsonldb.ID, settings entity.MembershipSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.byID[membershipKey(userID, orgID)]
	if !ok {
		return errMembershipNotFound
	}

	m.Settings = settings
	if _, err := s.table.Update(m); err != nil {
		return err
	}
	return nil
}
