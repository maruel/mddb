package storage

import (
	"context"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)

// testID returns a LUCI-style ID for the given number.
// Use for deterministic test IDs.
func testID(n uint64) jsonldb.ID {
	return jsonldb.ID(n)
}

// newTestContext returns a context with a test user and organization.
func newTestContext(t testing.TB, orgIDStr string) context.Context {
	t.Helper()
	orgID, err := jsonldb.DecodeID(orgIDStr)
	if err != nil {
		orgID = testID(999)
	}
	ctx := context.WithValue(t.Context(), entity.UserKey, &entity.User{ID: testID(1000)})
	return context.WithValue(ctx, entity.OrgKey, orgID)
}
