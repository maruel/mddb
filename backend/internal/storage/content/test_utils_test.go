package content

import (
	"context"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)

// newTestContext returns a context with a test user and organization.
func newTestContext(t testing.TB, orgIDStr string) context.Context {
	t.Helper()
	orgID, err := jsonldb.DecodeID(orgIDStr)
	if err != nil {
		orgID = jsonldb.ID(999)
	}
	ctx := context.WithValue(t.Context(), entity.UserKey, &entity.User{ID: jsonldb.ID(1000)})
	return context.WithValue(ctx, entity.OrgKey, orgID)
}
