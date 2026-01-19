package storage

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/models"
)

// testID returns an encoded LUCI-style ID for the given number.
// Use for deterministic test IDs.
func testID(n uint64) string {
	return jsonldb.ID(n).String()
}

// newTestContext returns a context with a test user and organization.
//
//nolint:unparam // keep for future use even if currently always "org1"
func newTestContext(orgID string) context.Context {
	if orgID == "" {
		orgID = testID(999)
	}
	user := &models.User{
		ID: "test-user",
	}
	ctx := context.WithValue(context.Background(), models.UserKey, user)
	return context.WithValue(ctx, models.OrgKey, orgID)
}
