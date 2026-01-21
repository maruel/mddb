package server

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestHasPermission(t *testing.T) {
	tests := []struct {
		name         string
		userRole     identity.UserRole
		requiredRole identity.UserRole
		expected     bool
	}{
		{
			name:         "Viewer accessing Viewer endpoint",
			userRole:     identity.UserRoleViewer,
			requiredRole: identity.UserRoleViewer,
			expected:     true,
		},
		{
			name:         "Viewer accessing Editor endpoint",
			userRole:     identity.UserRoleViewer,
			requiredRole: identity.UserRoleEditor,
			expected:     false,
		},
		{
			name:         "Viewer accessing Admin endpoint",
			userRole:     identity.UserRoleViewer,
			requiredRole: identity.UserRoleAdmin,
			expected:     false,
		},
		{
			name:         "Editor accessing Viewer endpoint",
			userRole:     identity.UserRoleEditor,
			requiredRole: identity.UserRoleViewer,
			expected:     true,
		},
		{
			name:         "Editor accessing Editor endpoint",
			userRole:     identity.UserRoleEditor,
			requiredRole: identity.UserRoleEditor,
			expected:     true,
		},
		{
			name:         "Editor accessing Admin endpoint",
			userRole:     identity.UserRoleEditor,
			requiredRole: identity.UserRoleAdmin,
			expected:     false,
		},
		{
			name:         "Admin accessing Viewer endpoint",
			userRole:     identity.UserRoleAdmin,
			requiredRole: identity.UserRoleViewer,
			expected:     true,
		},
		{
			name:         "Admin accessing Editor endpoint",
			userRole:     identity.UserRoleAdmin,
			requiredRole: identity.UserRoleEditor,
			expected:     true,
		},
		{
			name:         "Admin accessing Admin endpoint",
			userRole:     identity.UserRoleAdmin,
			requiredRole: identity.UserRoleAdmin,
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasPermission(tt.userRole, tt.requiredRole)
			if got != tt.expected {
				t.Errorf("hasPermission(%v, %v) = %v, want %v", tt.userRole, tt.requiredRole, got, tt.expected)
			}
		})
	}
}
