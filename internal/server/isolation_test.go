package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maruel/mddb/internal/models"
)

func TestOrgIsolationMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		userOrgID      string
		requestOrgID   string
		expectedStatus int
	}{
		{
			name:           "Access own organization",
			userOrgID:      "org1",
			requestOrgID:   "org1",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Access different organization",
			userOrgID:      "org1",
			requestOrgID:   "org2",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Access with no org context in request (e.g. global endpoint)",
			userOrgID:      "org1",
			requestOrgID:   "",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock handler that returns 200 OK
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// Create middleware
			middleware := RequireRole(models.RoleViewer)(next)

			// Create request
			req := httptest.NewRequest("GET", "/api/"+tt.requestOrgID+"/nodes", http.NoBody)
			if tt.requestOrgID != "" {
				req.SetPathValue("orgID", tt.requestOrgID)
			}

			// Add user to context
			user := &models.User{
				ID:             "user1",
				OrganizationID: tt.userOrgID,
				Role:           models.RoleViewer,
			}
			ctx := context.WithValue(req.Context(), models.UserKey, user)
			req = req.WithContext(ctx)

			// Record response
			rr := httptest.NewRecorder()
			middleware.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestRolePermissions(t *testing.T) {
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
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			middleware := RequireRole(tt.requiredRole)(next)

			req := httptest.NewRequest("GET", "/api/org1/nodes", http.NoBody)
			req.SetPathValue("orgID", "org1")

			user := &models.User{
				ID:             "user1",
				OrganizationID: "org1",
				Role:           tt.userRole,
			}
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
