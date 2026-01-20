package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/entity"
	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// MembershipService handles user-organization relationships.
type MembershipService struct {
	rootDir string
	table   *jsonldb.Table[*entity.Membership]
	mu      sync.RWMutex
	byID    map[string]*entity.Membership // key: userID_orgID (as strings)
}

// NewMembershipService creates a new membership service.
func NewMembershipService(rootDir string) (*MembershipService, error) {
	dbDir := filepath.Join(rootDir, "db")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	tablePath := filepath.Join(dbDir, "memberships.jsonl")
	table, err := jsonldb.NewTable[*entity.Membership](tablePath)
	if err != nil {
		return nil, err
	}

	s := &MembershipService{
		rootDir: rootDir,
		table:   table,
		byID:    make(map[string]*entity.Membership),
	}

	for m := range table.Iter(0) {
		s.byID[m.UserID.String()+"_"+m.OrganizationID.String()] = m
	}

	return s, nil
}

// CreateMembership adds a user to an organization.
func (s *MembershipService) CreateMembership(userIDStr, orgIDStr string, role entity.UserRole) (*entity.Membership, error) {
	userID, err := jsonldb.DecodeID(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}
	orgID, err := jsonldb.DecodeID(orgIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid organization id: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := userIDStr + "_" + orgIDStr
	if _, ok := s.byID[key]; ok {
		return nil, fmt.Errorf("membership already exists")
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

	// Update local cache
	newM := s.table.Last()
	s.byID[key] = newM

	return membership, nil
}

// GetMembership retrieves a specific user-org relationship.
func (s *MembershipService) GetMembership(userIDStr, orgIDStr string) (*entity.Membership, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := userIDStr + "_" + orgIDStr
	m, ok := s.byID[key]
	if !ok {
		return nil, fmt.Errorf("membership not found")
	}

	return m, nil
}

// ListByUser returns all organizations a user belongs to.
func (s *MembershipService) ListByUser(userIDStr string) ([]entity.Membership, error) {
	userID, err := jsonldb.DecodeID(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
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

// ListByOrganization returns all users in an organization.
func (s *MembershipService) ListByOrganization(orgIDStr string) ([]entity.Membership, error) {
	orgID, err := jsonldb.DecodeID(orgIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid organization id: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var memberships []entity.Membership
	for _, m := range s.byID {
		if m.OrganizationID == orgID {
			memberships = append(memberships, *m)
		}
	}
	return memberships, nil
}

// UpdateRole updates a user's role in an organization.
func (s *MembershipService) UpdateRole(userIDStr, orgIDStr string, role entity.UserRole) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := userIDStr + "_" + orgIDStr
	m, ok := s.byID[key]
	if !ok {
		return fmt.Errorf("membership not found")
	}

	m.Role = role
	return s.table.Replace(s.getAllFromCache())
}

// UpdateSettings updates user preferences within a specific organization.
func (s *MembershipService) UpdateSettings(userIDStr, orgIDStr string, settings entity.MembershipSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := userIDStr + "_" + orgIDStr
	m, ok := s.byID[key]
	if !ok {
		return fmt.Errorf("membership not found")
	}

	m.Settings = settings
	return s.table.Replace(s.getAllFromCache())
}

// DeleteMembership removes a user from an organization.
func (s *MembershipService) DeleteMembership(userIDStr, orgIDStr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := userIDStr + "_" + orgIDStr
	if _, ok := s.byID[key]; !ok {
		return fmt.Errorf("membership not found")
	}

	delete(s.byID, key)
	return s.table.Replace(s.getAllFromCache())
}

func (s *MembershipService) getAllFromCache() []*entity.Membership {
	rows := make([]*entity.Membership, 0, len(s.byID))
	for _, v := range s.byID {
		rows = append(rows, v)
	}
	return rows
}
