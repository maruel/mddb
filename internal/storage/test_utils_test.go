package storage

import (
	"context"

	"github.com/maruel/mddb/internal/models"
)

// newTestContext returns a context with a test user and organization.
func newTestContext(orgID string) context.Context {
	if orgID == "" {
		orgID = "test-org"
	}
	user := &models.User{
		ID:             "test-user",
		OrganizationID: orgID,
		Role:           models.RoleAdmin,
	}
	return context.WithValue(context.Background(), models.UserKey, user)
}