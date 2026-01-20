package server

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/storage/entity"
)

func TestHasPermission(t *testing.T) {
	tests := []struct {
		name         string
		userRole     entity.UserRole
		requiredRole entity.UserRole
		expected     bool
	}{
		{
			name:         "Viewer accessing Viewer endpoint",
			userRole:     entity.UserRoleViewer,
			requiredRole: entity.UserRoleViewer,
			expected:     true,
		},
		{
			name:         "Viewer accessing Editor endpoint",
			userRole:     entity.UserRoleViewer,
			requiredRole: entity.UserRoleEditor,
			expected:     false,
		},
		{
			name:         "Viewer accessing Admin endpoint",
			userRole:     entity.UserRoleViewer,
			requiredRole: entity.UserRoleAdmin,
			expected:     false,
		},
		{
			name:         "Editor accessing Viewer endpoint",
			userRole:     entity.UserRoleEditor,
			requiredRole: entity.UserRoleViewer,
			expected:     true,
		},
		{
			name:         "Editor accessing Editor endpoint",
			userRole:     entity.UserRoleEditor,
			requiredRole: entity.UserRoleEditor,
			expected:     true,
		},
		{
			name:         "Editor accessing Admin endpoint",
			userRole:     entity.UserRoleEditor,
			requiredRole: entity.UserRoleAdmin,
			expected:     false,
		},
		{
			name:         "Admin accessing Viewer endpoint",
			userRole:     entity.UserRoleAdmin,
			requiredRole: entity.UserRoleViewer,
			expected:     true,
		},
		{
			name:         "Admin accessing Editor endpoint",
			userRole:     entity.UserRoleAdmin,
			requiredRole: entity.UserRoleEditor,
			expected:     true,
		},
		{
			name:         "Admin accessing Admin endpoint",
			userRole:     entity.UserRoleAdmin,
			requiredRole: entity.UserRoleAdmin,
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
