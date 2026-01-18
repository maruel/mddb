package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/storage"
)

// AuthMiddleware validates JWT tokens and adds user info to the context.
func AuthMiddleware(userService *storage.UserService, jwtSecret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for login and register
			if r.URL.Path == "/api/auth/login" || r.URL.Path == "/api/auth/register" || !strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return jwtSecret, nil
			})

			if err != nil || !token.Valid {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, "Invalid claims", http.StatusUnauthorized)
				return
			}

			userID, ok := claims["sub"].(string)
			if !ok {
				http.Error(w, "Invalid user ID in token", http.StatusUnauthorized)
				return
			}

			user, err := userService.GetUser(userID)
			if err != nil {
				http.Error(w, "User not found", http.StatusUnauthorized)
				return
			}

			// Add user to context
			ctx := context.WithValue(r.Context(), models.UserKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole ensures the authenticated user has at least the required role in the target organization.
func RequireRole(memService *storage.MembershipService, requiredRole models.UserRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := r.Context().Value(models.UserKey).(*models.User)
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Get organization from path
			orgID := r.PathValue("orgID")
			if orgID == "" {
				// Some global endpoints might not have orgID
				next.ServeHTTP(w, r)
				return
			}

			// Verify membership and get role
			membership, err := memService.GetMembership(user.ID, orgID)
			if err != nil {
				http.Error(w, "Forbidden: not a member of this organization", http.StatusForbidden)
				return
			}

			if !hasPermission(membership.Role, requiredRole) {
				http.Error(w, "Forbidden: insufficient permissions", http.StatusForbidden)
				return
			}

			// Add org to context for handlers
			ctx := context.WithValue(r.Context(), models.OrgKey, orgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func hasPermission(userRole, requiredRole models.UserRole) bool {
	weights := map[models.UserRole]int{
		models.UserRoleViewer: 1,
		models.UserRoleEditor: 2,
		models.UserRoleAdmin:  3,
	}

	return weights[userRole] >= weights[requiredRole]
}
