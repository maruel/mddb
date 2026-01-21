package content

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// QuotaGetter retrieves quota limits for an organization.
type QuotaGetter interface {
	GetQuota(ctx context.Context, orgID jsonldb.ID) (identity.OrganizationQuota, error)
}
