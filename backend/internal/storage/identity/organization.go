// Manages organization entities and their settings.

package identity

import (
	"context"
	"errors"
	"iter"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// DefaultOrganizationQuotas returns the default quotas for a new organization.
// ResourceQuotas fields default to -1 (inherit from server).
func DefaultOrganizationQuotas() identityv1.OrganizationQuotas {
	return identityv1.OrganizationQuotas{
		ResourceQuotas: storagev1.ResourceQuotas{
			MaxPages:              -1,
			MaxStorageBytes:       -1,
			MaxRecordsPerTable:    -1,
			MaxAssetSizeBytes:     -1,
			MaxTablesPerWorkspace: -1,
			MaxColumnsPerTable:    -1,
		},
		MaxWorkspacesPerOrg:    3,
		MaxMembersPerOrg:       10,
		MaxMembersPerWorkspace: 10,
		MaxTotalStorageBytes:   5 * 1024 * 1024 * 1024, // 5 GiB
	}
}

// OrganizationService handles organization management.
type OrganizationService struct {
	table *jsonldb.Table[*identityv1.Organization]
}

// NewOrganizationService creates a new organization service.
func NewOrganizationService(tablePath string) (*OrganizationService, error) {
	table, err := jsonldb.NewTable[*identityv1.Organization](tablePath)
	if err != nil {
		return nil, err
	}
	return &OrganizationService{
		table: table,
	}, nil
}

// Create creates a new organization.
func (s *OrganizationService) Create(_ context.Context, name, billingEmail string) (*identityv1.Organization, error) {
	if name == "" {
		return nil, errOrgNameRequired
	}
	org := &identityv1.Organization{
		ID:           ksid.NewID(),
		Name:         name,
		BillingEmail: billingEmail,
		Quotas:       DefaultOrganizationQuotas(),
		Settings: identityv1.OrganizationSettings{
			DefaultWorkspaceQuotas: DefaultWorkspaceQuotas(),
		},
		Created: storagev1.Now(),
	}
	if err := s.table.Append(org, nil); err != nil {
		return nil, err
	}
	return org, nil
}

// Get retrieves an organization by ID.
func (s *OrganizationService) Get(id ksid.ID) (*identityv1.Organization, error) {
	org := s.table.Get(id)
	if org == nil {
		return nil, errOrgNotFound
	}
	return org, nil
}

// Modify atomically modifies an organization.
func (s *OrganizationService) Modify(id ksid.ID, fn func(org *identityv1.Organization) error) (*identityv1.Organization, error) {
	if id.IsZero() {
		return nil, errOrgNotFound
	}
	return s.table.Modify(id, fn)
}

// Iter iterates over organizations with ID greater than startID.
func (s *OrganizationService) Iter(startID ksid.ID) iter.Seq[*identityv1.Organization] {
	return s.table.Iter(startID)
}

// Delete deletes an organization.
func (s *OrganizationService) Delete(id ksid.ID) error {
	if id.IsZero() {
		return errOrgNotFound
	}
	if s.table.Get(id) == nil {
		return errOrgNotFound
	}
	if _, err := s.table.Delete(id); err != nil {
		return err
	}
	return nil
}

// Count returns the total number of organizations.
func (s *OrganizationService) Count() int {
	return s.table.Len()
}

//

var (
	errOrgNameRequired = errors.New("organization name is required")
	errOrgNotFound     = errors.New("organization not found")
)
