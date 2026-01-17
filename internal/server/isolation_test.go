package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/storage"
)

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
			membershipOrg:  "org1",
			membershipRole: models.RoleViewer,
			requestOrgID:   "org1",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Access different organization",
			membershipOrg:  "org1",
			membershipRole: models.RoleViewer,
			requestOrgID:   "org2",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Access with no org context in request",
			membershipOrg:  "org1",
			membershipRole: models.RoleViewer,
			requestOrgID:   "",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := "user1"
			// Clear and setup membership for each test case
			_ = memService.DeleteMembership(userID, "org1")
			_ = memService.DeleteMembership(userID, "org2")

			if tt.membershipOrg != "" {
				_, _ = memService.CreateMembership(userID, tt.membershipOrg, tt.membershipRole)
			}

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			middleware := RequireRole(memService, models.RoleViewer)(next)

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
			userRole:       models.RoleViewer,
			requiredRole:   models.RoleViewer,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Viewer accessing Editor endpoint",
			userRole:       models.RoleViewer,
			requiredRole:   models.RoleEditor,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Editor accessing Viewer endpoint",
			userRole:       models.RoleEditor,
			requiredRole:   models.RoleViewer,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Editor accessing Admin endpoint",
			userRole:       models.RoleEditor,
			requiredRole:   models.RoleAdmin,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Admin accessing Editor endpoint",
			userRole:       models.RoleAdmin,
			requiredRole:   models.RoleEditor,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := "user-role-test"
			orgID := "org1"
			// Clear and setup membership for each test case
			_ = memService.DeleteMembership(userID, orgID)
			_, _ = memService.CreateMembership(userID, orgID, tt.userRole)

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			middleware := RequireRole(memService, tt.requiredRole)(next)

			req := httptest.NewRequest("GET", "/api/org1/nodes", http.NoBody)
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
