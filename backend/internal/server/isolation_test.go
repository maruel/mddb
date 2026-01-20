package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)

func testID(n uint64) jsonldb.ID {
	return jsonldb.ID(n)
}

func TestOrgIsolationMiddleware(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "mddb-isolation-test-*")
	defer func() { _ = os.RemoveAll(tempDir) }()
	memService, _ := storage.NewMembershipService(tempDir)

	tests := []struct {
		name           string
		membershipOrg  jsonldb.ID
		membershipRole entity.UserRole
		requestOrgID   string
		expectedStatus int
	}{
		{
			name:           "Access own organization",
			membershipOrg:  testID(1),
			membershipRole: entity.UserRoleViewer,
			requestOrgID:   testID(1).String(),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Access different organization",
			membershipOrg:  testID(1),
			membershipRole: entity.UserRoleViewer,
			requestOrgID:   testID(2).String(),
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Access with no org context in request",
			membershipOrg:  testID(1),
			membershipRole: entity.UserRoleViewer,
			requestOrgID:   "",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := testID(100)
			// Clear and setup membership for each test case
			_ = memService.DeleteMembership(userID, testID(1))
			_ = memService.DeleteMembership(userID, testID(2))

			if !tt.membershipOrg.IsZero() {
				_, _ = memService.CreateMembership(userID, tt.membershipOrg, tt.membershipRole)
			}

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			middleware := RequireRole(memService, entity.UserRoleViewer)(next)

			req := httptest.NewRequest("GET", "/api/"+tt.requestOrgID+"/nodes", http.NoBody)
			if tt.requestOrgID != "" {
				req.SetPathValue("orgID", tt.requestOrgID)
			}

			user := &entity.User{ID: userID}
			ctx := context.WithValue(req.Context(), entity.UserKey, user)
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
		userRole       entity.UserRole
		requiredRole   entity.UserRole
		expectedStatus int
	}{
		{
			name:           "Viewer accessing Viewer endpoint",
			userRole:       entity.UserRoleViewer,
			requiredRole:   entity.UserRoleViewer,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Viewer accessing Editor endpoint",
			userRole:       entity.UserRoleViewer,
			requiredRole:   entity.UserRoleEditor,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Editor accessing Viewer endpoint",
			userRole:       entity.UserRoleEditor,
			requiredRole:   entity.UserRoleViewer,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Editor accessing Admin endpoint",
			userRole:       entity.UserRoleEditor,
			requiredRole:   entity.UserRoleAdmin,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Admin accessing Editor endpoint",
			userRole:       entity.UserRoleAdmin,
			requiredRole:   entity.UserRoleEditor,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := testID(200)
			orgID := testID(1)
			// Clear and setup membership for each test case
			_ = memService.DeleteMembership(userID, orgID)
			_, _ = memService.CreateMembership(userID, orgID, tt.userRole)

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			middleware := RequireRole(memService, tt.requiredRole)(next)

			req := httptest.NewRequest("GET", "/api/"+orgID.String()+"/nodes", http.NoBody)
			req.SetPathValue("orgID", orgID.String())

			user := &entity.User{ID: userID}
			ctx := context.WithValue(req.Context(), entity.UserKey, user)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			middleware.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}
