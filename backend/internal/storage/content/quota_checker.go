package content

import (
	"context"
	"fmt"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)

// QuotaGetter retrieves quota limits for an organization.
type QuotaGetter interface {
	GetQuota(ctx context.Context, orgID jsonldb.ID) (entity.Quota, error)
}

// UsageGetter retrieves current usage for an organization.
type UsageGetter interface {
	GetOrganizationUsage(orgID jsonldb.ID) (pageCount int, storageBytes int64, err error)
}

// OrgQuotaChecker implements QuotaChecker using organization quotas.
type OrgQuotaChecker struct {
	quotaGetter QuotaGetter
	usageGetter UsageGetter
}

// NewOrgQuotaChecker creates a quota checker with the given quota and usage providers.
func NewOrgQuotaChecker(quotaGetter QuotaGetter, usageGetter UsageGetter) *OrgQuotaChecker {
	return &OrgQuotaChecker{
		quotaGetter: quotaGetter,
		usageGetter: usageGetter,
	}
}

// CheckPageQuota returns an error if creating a new page would exceed quota.
func (c *OrgQuotaChecker) CheckPageQuota(ctx context.Context, orgID jsonldb.ID) error {
	quota, err := c.quotaGetter.GetQuota(ctx, orgID)
	if err != nil {
		return err
	}
	if quota.MaxPages <= 0 {
		return nil // No limit
	}

	count, _, err := c.usageGetter.GetOrganizationUsage(orgID)
	if err != nil {
		return err
	}
	if count >= quota.MaxPages {
		return fmt.Errorf("page quota exceeded (%d/%d)", count, quota.MaxPages)
	}
	return nil
}

// CheckStorageQuota returns an error if adding the given bytes would exceed storage quota.
func (c *OrgQuotaChecker) CheckStorageQuota(ctx context.Context, orgID jsonldb.ID, additionalBytes int64) error {
	quota, err := c.quotaGetter.GetQuota(ctx, orgID)
	if err != nil {
		return err
	}
	if quota.MaxStorage <= 0 {
		return nil // No limit
	}

	_, usage, err := c.usageGetter.GetOrganizationUsage(orgID)
	if err != nil {
		return err
	}
	if usage+additionalBytes > quota.MaxStorage {
		return fmt.Errorf("storage quota exceeded (%d/%d bytes)", usage+additionalBytes, quota.MaxStorage)
	}
	return nil
}
