package content

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)

// QuotaGetter defines the interface for retrieving organization quotas.
type QuotaGetter interface {
	GetQuota(ctx context.Context, orgID jsonldb.ID) (entity.Quota, error)
}
