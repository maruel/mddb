package storage

import (
	"context"

	"github.com/maruel/mddb/backend/internal/models"
)

// newTestContext returns a context with a test user and organization.
//
//nolint:unparam // keep for future use even if currently always "org1"
func newTestContext(orgID string) context.Context {
	if orgID == "" {
		orgID = EncodeID(999) // "6Oc" for 999
	}
	user := &models.User{
		ID: "test-user",
	}
	ctx := context.WithValue(context.Background(), models.UserKey, user)
	return context.WithValue(ctx, models.OrgKey, orgID)
}
