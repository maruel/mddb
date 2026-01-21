package server

import "github.com/maruel/mddb/backend/internal/storage/identity"

func hasPermission(userRole, requiredRole identity.UserRole) bool {
	weights := map[identity.UserRole]int{
		identity.UserRoleViewer: 1,
		identity.UserRoleEditor: 2,
		identity.UserRoleAdmin:  3,
	}

	return weights[userRole] >= weights[requiredRole]
}
