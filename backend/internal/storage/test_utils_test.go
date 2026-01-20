package storage

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)

// testID returns a LUCI-style ID for the given number.
// Use for deterministic test IDs.
func testID(n uint64) jsonldb.ID {
	return jsonldb.ID(n)
}

// newTestContext returns a context with a test user and organization.
func newTestContext(orgIDStr string) context.Context {
	orgID, err := jsonldb.DecodeID(orgIDStr)
	if err != nil {
		// For backward compat, if it's not a valid ID, use a deterministic one
		orgID = testID(999)
	}
	user := &entity.User{
		ID: testID(1000),
	}
	ctx := context.WithValue(context.Background(), entity.UserKey, user)
	return context.WithValue(ctx, entity.OrgKey, orgID)
}
