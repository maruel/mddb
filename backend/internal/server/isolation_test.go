package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/models"
	"github.com/maruel/mddb/backend/internal/storage"
)

func testID(n uint64) string {
	return jsonldb.ID(n).Encode()
}

func TestOrgIsolationMiddleware(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "mddb-isolation-test-*")
	defer func() { _ = os.RemoveAll(tempDir) }()
	memService, _ := storage.NewMembershipService(tempDir)

	tests := []struct {
		name           string
		membershipOrg  string
		membershipRole models.UserRole
		requestOrgID   string
		expectedStatus int
	}{
		{
			name:           "Access own organization",
			membershipOrg:  testID(1),
			membershipRole: models.UserRoleViewer,
			requestOrgID:   testID(1),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Access different organization",
			membershipOrg:  testID(1),
			membershipRole: models.UserRoleViewer,
			requestOrgID:   testID(2),
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Access with no org context in request",
			membershipOrg:  testID(1),
			membershipRole: models.UserRoleViewer,
			requestOrgID:   "",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := "user1"
			// Clear and setup membership for each test case
			_ = memService.DeleteMembership(userID, testID(1))
			_ = memService.DeleteMembership(userID, testID(2))

			if tt.membershipOrg != "" {
				_, _ = memService.CreateMembership(userID, tt.membershipOrg, tt.membershipRole)
			}

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			middleware := RequireRole(memService, models.UserRoleViewer)(next)

			req := httptest.NewRequest("GET", "/api/"+tt.requestOrgID+"/nodes", http.NoBody)
			if tt.requestOrgID != "" {
				req.SetPathValue("orgID", tt.requestOrgID)
			}

			user := &models.User{ID: userID}
			ctx := context.WithValue(req.Context(), models.UserKey, user)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			middleware.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestRolePermissions(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "mddb-role-test-*")
	defer func() { _ = os.RemoveAll(tempDir) }()
	memService, _ := storage.NewMembershipService(tempDir)

	tests := []struct {
		name           string
		userRole       models.UserRole
		requiredRole   models.UserRole
		expectedStatus int
	}{
		{
			name:           "Viewer accessing Viewer endpoint",
			userRole:       models.UserRoleViewer,
			requiredRole:   models.UserRoleViewer,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Viewer accessing Editor endpoint",
			userRole:       models.UserRoleViewer,
			requiredRole:   models.UserRoleEditor,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Editor accessing Viewer endpoint",
			userRole:       models.UserRoleEditor,
			requiredRole:   models.UserRoleViewer,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Editor accessing Admin endpoint",
			userRole:       models.UserRoleEditor,
			requiredRole:   models.UserRoleAdmin,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Admin accessing Editor endpoint",
			userRole:       models.UserRoleAdmin,
			requiredRole:   models.UserRoleEditor,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := "user-role-test"
			orgID := testID(1)
			// Clear and setup membership for each test case
			_ = memService.DeleteMembership(userID, orgID)
			_, _ = memService.CreateMembership(userID, orgID, tt.userRole)

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			middleware := RequireRole(memService, tt.requiredRole)(next)

			req := httptest.NewRequest("GET", "/api/"+orgID+"/nodes", http.NoBody)
			req.SetPathValue("orgID", orgID)

			user := &models.User{ID: userID}
			ctx := context.WithValue(req.Context(), models.UserKey, user)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			middleware.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}
