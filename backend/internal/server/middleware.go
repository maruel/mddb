package server

import "github.com/maruel/mddb/backend/internal/storage/entity"

func hasPermission(userRole, requiredRole entity.UserRole) bool {
	weights := map[entity.UserRole]int{
		entity.UserRoleViewer: 1,
		entity.UserRoleEditor: 2,
		entity.UserRoleAdmin:  3,
	}

	return weights[userRole] >= weights[requiredRole]
}
