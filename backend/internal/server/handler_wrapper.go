// Provides middleware for standardizing HTTP handlers.

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/ratelimit"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// addRequestMetadataToContext adds client IP and User-Agent to the context.
func addRequestMetadataToContext(ctx context.Context, r *http.Request) context.Context {
	ctx = reqctx.WithClientIP(ctx, reqctx.GetClientIP(r))
	ctx = reqctx.WithUserAgent(ctx, r.Header.Get("User-Agent"))
	return ctx
}

// Wrap wraps a handler function to work as an http.Handler.
// The function must have signature: func(context.Context, *In) (*Out, error)
// where In can be unmarshalled from JSON and Out is a struct.
// Path parameters can be extracted by tagging struct fields with `path:"name"`.
// *In must implement dto.Validatable.
//
// Example:
//
//	type GetPageRequest struct {
//	    ID string `path:"id"`
//	}
//
//	func (h *Handler) GetPage(ctx context.Context, req *GetPageRequest) (*Response, error)
func Wrap[In any, PtrIn interface {
	*In
	dto.Validatable
}, Out any](fn func(context.Context, PtrIn) (*Out, error), rlConfig *ratelimit.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := addRequestMetadataToContext(r.Context(), r)

		// Rate limit check for unauthenticated endpoints
		if tier := rlConfig.MatchUnauth(r.Method, r.URL.Path); tier != nil {
			clientIP := reqctx.GetClientIP(r)
			key := ratelimit.BuildKey(tier.Scope, clientIP, tier.Name)
			result := tier.Limiter.Allow(key)

			// Wrap response writer to inject headers
			w = ratelimit.NewResponseWriter(w, result)

			if !result.Allowed {
				writeRateLimitError(w, result)
				return
			}
		}

		// Read request body
		body, err := io.ReadAll(r.Body)
		if err2 := r.Body.Close(); err == nil {
			err = err2
		}
		if err != nil {
			slog.ErrorContext(ctx, "Failed to read request body", "err", err)
			writeBadRequestError(w, "Failed to read request body")
			return
		}
		input := new(In)
		if len(body) > 0 {
			d := json.NewDecoder(bytes.NewReader(body))
			d.DisallowUnknownFields()
			if err := d.Decode(input); err != nil {
				slog.ErrorContext(ctx, "Failed to decode request body", "err", err)
				writeBadRequestError(w, "Invalid request body")
				return
			}
		}

		// Extract path parameters and populate request struct
		populatePathParams(r, input)
		// Extract query parameters and populate request struct
		populateQueryParams(r, input)

		// Validate the request
		if err := PtrIn(input).Validate(); err != nil {
			handleValidationError(ctx, w, err)
			return
		}

		output, err := fn(ctx, PtrIn(input))
		if err != nil {
			statusCode := http.StatusInternalServerError
			errorCode := dto.ErrorCodeInternal
			details := make(map[string]any)

			var ewsErr dto.ErrorWithStatus
			if errors.As(err, &ewsErr) {
				statusCode = ewsErr.StatusCode()
				errorCode = ewsErr.Code()
				if d := ewsErr.Details(); d != nil {
					details = d
				}
			}

			slog.ErrorContext(ctx, "Handler error", "err", err, "statusCode", statusCode, "code", errorCode)
			writeErrorResponseWithCode(w, statusCode, errorCode, err.Error(), details)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(output); err != nil {
			slog.ErrorContext(ctx, "Failed to encode response", "err", err)
		}
	})
}

// WrapAuth wraps an authenticated handler function to work as an http.Handler.
// It combines JWT validation, organization membership checking, and request parsing.
// The function must have signature: func(context.Context, jsonldb.ID, *identity.User, *In) (*Out, error)
// where orgID is the organization ID from the path (zero if not present),
// user is the authenticated user, In can be unmarshalled from JSON, and Out is a struct.
// *In must implement dto.Validatable.
func WrapAuth[In any, PtrIn interface {
	*In
	dto.Validatable
}, Out any](
	userService *identity.UserService,
	orgMemService *identity.OrganizationMembershipService,
	sessionService *identity.SessionService,
	jwtSecret []byte,
	requiredRole identity.OrganizationRole,
	fn func(context.Context, jsonldb.ID, *identity.User, PtrIn) (*Out, error),
	rlConfig *ratelimit.Config,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := addRequestMetadataToContext(r.Context(), r)

		// Validate JWT and session
		user, sessionID, tokenString, err := validateJWTAndSession(r, userService, sessionService, jwtSecret)
		if !sessionID.IsZero() {
			ctx = reqctx.WithSessionID(ctx, sessionID)
		}
		if tokenString != "" {
			ctx = reqctx.WithTokenString(ctx, tokenString)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		// Rate limit check for authenticated endpoints (after auth validation)
		if tier := rlConfig.MatchAuth(r.Method, r.URL.Path); tier != nil {
			// Use user ID for user-scoped limits, IP for IP-scoped
			var identifier string
			if tier.Scope == ratelimit.ScopeUser {
				identifier = user.ID.String()
			} else {
				identifier = reqctx.GetClientIP(r)
			}
			key := ratelimit.BuildKey(tier.Scope, identifier, tier.Name)
			result := tier.Limiter.Allow(key)

			// Wrap response writer to inject headers
			w = ratelimit.NewResponseWriter(w, result)

			if !result.Allowed {
				writeRateLimitError(w, result)
				return
			}
		}

		// Check organization membership if orgID is in path
		var orgID jsonldb.ID
		orgIDStr := r.PathValue("orgID")
		if orgIDStr != "" {
			orgID, err = jsonldb.DecodeID(orgIDStr)
			if err != nil {
				http.Error(w, "Invalid organization ID format", http.StatusBadRequest)
				return
			}

			membership, err := orgMemService.Get(user.ID, orgID)
			if err != nil {
				http.Error(w, "Forbidden: not a member of this organization", http.StatusForbidden)
				return
			}

			if !hasOrgPermission(membership.Role, requiredRole) {
				http.Error(w, "Forbidden: insufficient permissions", http.StatusForbidden)
				return
			}
		}

		// Parse request body
		body, err := io.ReadAll(r.Body)
		if err2 := r.Body.Close(); err == nil {
			err = err2
		}
		if err != nil {
			slog.ErrorContext(ctx, "Failed to read request body", "err", err)
			writeBadRequestError(w, "Failed to read request body")
			return
		}
		input := new(In)
		if len(body) > 0 {
			d := json.NewDecoder(bytes.NewReader(body))
			d.DisallowUnknownFields()
			if err := d.Decode(input); err != nil {
				slog.ErrorContext(ctx, "Failed to decode request body", "err", err)
				writeBadRequestError(w, "Invalid request body")
				return
			}
		}

		populatePathParams(r, input)
		populateQueryParams(r, input)

		// Validate the request
		if err := PtrIn(input).Validate(); err != nil {
			handleValidationError(ctx, w, err)
			return
		}

		output, err := fn(ctx, orgID, user, PtrIn(input))
		if err != nil {
			statusCode := http.StatusInternalServerError
			errorCode := dto.ErrorCodeInternal
			details := make(map[string]any)

			var ewsErr dto.ErrorWithStatus
			if errors.As(err, &ewsErr) {
				statusCode = ewsErr.StatusCode()
				errorCode = ewsErr.Code()
				if d := ewsErr.Details(); d != nil {
					details = d
				}
			}

			slog.ErrorContext(ctx, "Handler error", "err", err, "statusCode", statusCode, "code", errorCode)
			writeErrorResponseWithCode(w, statusCode, errorCode, err.Error(), details)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(output); err != nil {
			slog.ErrorContext(ctx, "Failed to encode response", "err", err)
		}
	})
}

// WrapWSAuth wraps an authenticated handler function for workspace-scoped routes.
// It validates JWT, checks workspace membership (or org admin access), and parses the request.
// The function must have signature: func(context.Context, jsonldb.ID, *identity.User, *In) (*Out, error)
// where wsID is the workspace ID from the path, user is the authenticated user.
// Org admins/owners automatically have admin access to workspaces within their org.
// *In must implement dto.Validatable.
func WrapWSAuth[In any, PtrIn interface {
	*In
	dto.Validatable
}, Out any](
	userService *identity.UserService,
	orgMemService *identity.OrganizationMembershipService,
	wsMemService *identity.WorkspaceMembershipService,
	wsService *identity.WorkspaceService,
	sessionService *identity.SessionService,
	jwtSecret []byte,
	requiredRole identity.WorkspaceRole,
	fn func(context.Context, jsonldb.ID, *identity.User, PtrIn) (*Out, error),
	rlConfig *ratelimit.Config,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := addRequestMetadataToContext(r.Context(), r)

		// Validate JWT and session
		user, sessionID, tokenString, err := validateJWTAndSession(r, userService, sessionService, jwtSecret)
		if sessionID != 0 {
			ctx = reqctx.WithSessionID(ctx, sessionID)
		}
		if tokenString != "" {
			ctx = reqctx.WithTokenString(ctx, tokenString)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		// Rate limit check
		if tier := rlConfig.MatchAuth(r.Method, r.URL.Path); tier != nil {
			var identifier string
			if tier.Scope == ratelimit.ScopeUser {
				identifier = user.ID.String()
			} else {
				identifier = reqctx.GetClientIP(r)
			}
			key := ratelimit.BuildKey(tier.Scope, identifier, tier.Name)
			result := tier.Limiter.Allow(key)
			w = ratelimit.NewResponseWriter(w, result)
			if !result.Allowed {
				writeRateLimitError(w, result)
				return
			}
		}

		// Check workspace membership
		var wsID jsonldb.ID
		wsIDStr := r.PathValue("wsID")
		if wsIDStr != "" {
			wsID, err = jsonldb.DecodeID(wsIDStr)
			if err != nil {
				http.Error(w, "Invalid workspace ID format", http.StatusBadRequest)
				return
			}

			// Get workspace to find org
			ws, err := wsService.Get(wsID)
			if err != nil {
				http.Error(w, "Workspace not found", http.StatusNotFound)
				return
			}

			// Check org membership first
			orgMem, err := orgMemService.Get(user.ID, ws.OrganizationID)
			if err != nil {
				http.Error(w, "Forbidden: not a member of this organization", http.StatusForbidden)
				return
			}

			// Org owners and admins have implicit admin access to all workspaces
			var effectiveRole identity.WorkspaceRole
			if orgMem.Role == identity.OrgRoleOwner || orgMem.Role == identity.OrgRoleAdmin {
				effectiveRole = identity.WSRoleAdmin
			} else {
				// Check explicit workspace membership
				wsMem, err := wsMemService.Get(user.ID, wsID)
				if err != nil {
					http.Error(w, "Forbidden: not a member of this workspace", http.StatusForbidden)
					return
				}
				effectiveRole = wsMem.Role
			}

			if !hasWSPermission(effectiveRole, requiredRole) {
				http.Error(w, "Forbidden: insufficient permissions", http.StatusForbidden)
				return
			}
		}

		// Parse request body
		body, err := io.ReadAll(r.Body)
		if err2 := r.Body.Close(); err == nil {
			err = err2
		}
		if err != nil {
			slog.ErrorContext(ctx, "Failed to read request body", "err", err)
			writeBadRequestError(w, "Failed to read request body")
			return
		}
		input := new(In)
		if len(body) > 0 {
			d := json.NewDecoder(bytes.NewReader(body))
			d.DisallowUnknownFields()
			if err := d.Decode(input); err != nil {
				slog.ErrorContext(ctx, "Failed to decode request body", "err", err)
				writeBadRequestError(w, "Invalid request body")
				return
			}
		}

		populatePathParams(r, input)
		populateQueryParams(r, input)

		if err := PtrIn(input).Validate(); err != nil {
			handleValidationError(ctx, w, err)
			return
		}

		output, err := fn(ctx, wsID, user, PtrIn(input))
		if err != nil {
			statusCode := http.StatusInternalServerError
			errorCode := dto.ErrorCodeInternal
			details := make(map[string]any)

			var ewsErr dto.ErrorWithStatus
			if errors.As(err, &ewsErr) {
				statusCode = ewsErr.StatusCode()
				errorCode = ewsErr.Code()
				if d := ewsErr.Details(); d != nil {
					details = d
				}
			}

			slog.ErrorContext(ctx, "Handler error", "err", err, "statusCode", statusCode, "code", errorCode)
			writeErrorResponseWithCode(w, statusCode, errorCode, err.Error(), details)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(output); err != nil {
			slog.ErrorContext(ctx, "Failed to encode response", "err", err)
		}
	})
}

// WrapAuthRaw wraps a raw http.HandlerFunc with authentication and role checking.
// Use this for handlers that need to handle requests directly (e.g., multipart forms).
// The wrapped handler receives the request with validated auth - the handler should
// extract wsID from the path via r.PathValue("wsID") if needed.
func WrapAuthRaw(
	userService *identity.UserService,
	orgMemService *identity.OrganizationMembershipService,
	wsMemService *identity.WorkspaceMembershipService,
	wsService *identity.WorkspaceService,
	sessionService *identity.SessionService,
	jwtSecret []byte,
	requiredRole identity.WorkspaceRole,
	fn http.HandlerFunc,
	rlConfig *ratelimit.Config,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate JWT and session
		user, _, _, err := validateJWTAndSession(r, userService, sessionService, jwtSecret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		// Rate limit check for authenticated endpoints (after auth validation)
		if tier := rlConfig.MatchAuth(r.Method, r.URL.Path); tier != nil {
			var identifier string
			if tier.Scope == ratelimit.ScopeUser {
				identifier = user.ID.String()
			} else {
				identifier = reqctx.GetClientIP(r)
			}
			key := ratelimit.BuildKey(tier.Scope, identifier, tier.Name)
			result := tier.Limiter.Allow(key)

			// Wrap response writer to inject headers
			w = ratelimit.NewResponseWriter(w, result)

			if !result.Allowed {
				writeRateLimitError(w, result)
				return
			}
		}

		// Check workspace membership if wsID is in path
		wsIDStr := r.PathValue("wsID")
		if wsIDStr != "" {
			wsID, err := jsonldb.DecodeID(wsIDStr)
			if err != nil {
				http.Error(w, "Invalid workspace ID format", http.StatusBadRequest)
				return
			}

			ws, err := wsService.Get(wsID)
			if err != nil {
				http.Error(w, "Workspace not found", http.StatusNotFound)
				return
			}

			orgMem, err := orgMemService.Get(user.ID, ws.OrganizationID)
			if err != nil {
				http.Error(w, "Forbidden: not a member of this organization", http.StatusForbidden)
				return
			}

			var effectiveRole identity.WorkspaceRole
			if orgMem.Role == identity.OrgRoleOwner || orgMem.Role == identity.OrgRoleAdmin {
				effectiveRole = identity.WSRoleAdmin
			} else {
				wsMem, err := wsMemService.Get(user.ID, wsID)
				if err != nil {
					http.Error(w, "Forbidden: not a member of this workspace", http.StatusForbidden)
					return
				}
				effectiveRole = wsMem.Role
			}

			if !hasWSPermission(effectiveRole, requiredRole) {
				http.Error(w, "Forbidden: insufficient permissions", http.StatusForbidden)
				return
			}
		}

		// Call the raw handler
		fn(w, r)
	})
}

// WrapGlobalAdmin wraps a handler that requires global admin privileges.
// These endpoints are for server-wide administration (stats, all users, all orgs).
// No organization context is required - just valid JWT and IsGlobalAdmin flag.
// *In must implement dto.Validatable.
func WrapGlobalAdmin[In any, PtrIn interface {
	*In
	dto.Validatable
}, Out any](
	userService *identity.UserService,
	sessionService *identity.SessionService,
	jwtSecret []byte,
	fn func(context.Context, *identity.User, PtrIn) (*Out, error),
	rlConfig *ratelimit.Config,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := addRequestMetadataToContext(r.Context(), r)

		user, _, _, err := validateJWTAndSession(r, userService, sessionService, jwtSecret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		if !user.IsGlobalAdmin {
			http.Error(w, "Forbidden: global admin required", http.StatusForbidden)
			return
		}

		// Rate limit check for admin endpoints (after auth validation)
		if tier := rlConfig.MatchAuth(r.Method, r.URL.Path); tier != nil {
			var identifier string
			if tier.Scope == ratelimit.ScopeUser {
				identifier = user.ID.String()
			} else {
				identifier = reqctx.GetClientIP(r)
			}
			key := ratelimit.BuildKey(tier.Scope, identifier, tier.Name)
			result := tier.Limiter.Allow(key)

			// Wrap response writer to inject headers
			w = ratelimit.NewResponseWriter(w, result)

			if !result.Allowed {
				writeRateLimitError(w, result)
				return
			}
		}

		body, err := io.ReadAll(r.Body)
		if err2 := r.Body.Close(); err == nil {
			err = err2
		}
		if err != nil {
			slog.ErrorContext(ctx, "Failed to read request body", "err", err)
			writeBadRequestError(w, "Failed to read request body")
			return
		}
		input := new(In)
		if len(body) > 0 {
			d := json.NewDecoder(bytes.NewReader(body))
			d.DisallowUnknownFields()
			if err := d.Decode(input); err != nil {
				slog.ErrorContext(ctx, "Failed to decode request body", "err", err)
				writeBadRequestError(w, "Invalid request body")
				return
			}
		}

		populatePathParams(r, input)
		populateQueryParams(r, input)

		// Validate the request
		if err := PtrIn(input).Validate(); err != nil {
			handleValidationError(ctx, w, err)
			return
		}

		output, err := fn(ctx, user, PtrIn(input))
		if err != nil {
			statusCode := http.StatusInternalServerError
			errorCode := dto.ErrorCodeInternal
			details := make(map[string]any)

			var ewsErr dto.ErrorWithStatus
			if errors.As(err, &ewsErr) {
				statusCode = ewsErr.StatusCode()
				errorCode = ewsErr.Code()
				if d := ewsErr.Details(); d != nil {
					details = d
				}
			}

			slog.ErrorContext(ctx, "Handler error", "err", err, "statusCode", statusCode, "code", errorCode)
			writeErrorResponseWithCode(w, statusCode, errorCode, err.Error(), details)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(output); err != nil {
			slog.ErrorContext(ctx, "Failed to encode response", "err", err)
		}
	})
}

var (
	errUnauthorized       = errors.New("unauthorized")
	errInvalidAuthHdr     = errors.New("invalid authorization header")
	errInvalidToken       = errors.New("invalid token")
	errInvalidClaims      = errors.New("invalid claims")
	errInvalidUserIDToken = errors.New("invalid user ID in token")
	errInvalidUserIDFmt   = errors.New("invalid user ID format")
	errUserNotFound       = errors.New("user not found")
	errSessionRevoked     = errors.New("session revoked")
)

// validateJWTAndSession extracts and validates the JWT token and session from the request.
// Returns the user, session ID, token string, and any error.
// If sessionService is nil, session validation is skipped (backwards compatible).
func validateJWTAndSession(r *http.Request, userService *identity.UserService, sessionService *identity.SessionService, jwtSecret []byte) (*identity.User, jsonldb.ID, string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, 0, "", errUnauthorized
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, 0, "", errInvalidAuthHdr
	}

	tokenString := parts[1]
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		return nil, 0, "", errInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, 0, "", errInvalidClaims
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok {
		return nil, 0, "", errInvalidUserIDToken
	}

	userID, err := jsonldb.DecodeID(userIDStr)
	if err != nil {
		return nil, 0, "", errInvalidUserIDFmt
	}

	user, err := userService.Get(userID)
	if err != nil {
		return nil, 0, "", errUserNotFound
	}

	// Validate session if sessionService is provided and token has session ID
	var sessionID jsonldb.ID
	if sessionService != nil {
		if sidStr, ok := claims["sid"].(string); ok && sidStr != "" {
			sessionID, err = jsonldb.DecodeID(sidStr)
			if err != nil {
				return nil, 0, "", errInvalidToken
			}

			valid, err := sessionService.IsValid(sessionID)
			if err != nil {
				return nil, 0, "", errInvalidToken
			}
			if !valid {
				return nil, 0, "", errSessionRevoked
			}
		}
	}

	return user, sessionID, tokenString, nil
}

// hasOrgPermission checks if the user's org role meets the required level.
// Role hierarchy: owner > admin > member.
func hasOrgPermission(userRole, requiredRole identity.OrganizationRole) bool {
	roleLevel := map[identity.OrganizationRole]int{
		identity.OrgRoleMember: 0,
		identity.OrgRoleAdmin:  1,
		identity.OrgRoleOwner:  2,
	}
	return roleLevel[userRole] >= roleLevel[requiredRole]
}

// hasWSPermission checks if the user's workspace role meets the required level.
// Role hierarchy: admin > editor > viewer.
func hasWSPermission(userRole, requiredRole identity.WorkspaceRole) bool {
	roleLevel := map[identity.WorkspaceRole]int{
		identity.WSRoleViewer: 0,
		identity.WSRoleEditor: 1,
		identity.WSRoleAdmin:  2,
	}
	return roleLevel[userRole] >= roleLevel[requiredRole]
}

// populatePathParams extracts path parameters from the request and populates
// struct fields tagged with `path:"paramName"`.
func populatePathParams(r *http.Request, input any) {
	val := reflect.ValueOf(input)
	if val.Kind() != reflect.Pointer {
		return // Skip if not a pointer
	}

	elem := val.Elem()
	if elem.Kind() != reflect.Struct {
		return // Skip if not a struct
	}

	typ := elem.Type()
	jsonldbIDType := reflect.TypeFor[jsonldb.ID]()
	for i := range typ.NumField() {
		field := typ.Field(i)
		tag := field.Tag.Get("path")
		if tag == "" {
			continue
		}

		paramValue := r.PathValue(tag)
		if paramValue == "" {
			continue
		}

		// Set the field value based on type
		switch {
		case field.Type.Kind() == reflect.String:
			elem.Field(i).SetString(paramValue)
		case field.Type == jsonldbIDType:
			if id, err := jsonldb.DecodeID(paramValue); err == nil {
				elem.Field(i).Set(reflect.ValueOf(id))
			}
		}
	}
}

// populateQueryParams extracts query parameters from the request and populates
// struct fields tagged with `query:"paramName"`.
func populateQueryParams(r *http.Request, input any) {
	val := reflect.ValueOf(input)
	if val.Kind() != reflect.Pointer {
		return // Skip if not a pointer
	}

	elem := val.Elem()
	if elem.Kind() != reflect.Struct {
		return // Skip if not a struct
	}

	query := r.URL.Query()
	typ := elem.Type()
	for i := range typ.NumField() {
		field := typ.Field(i)
		tag := field.Tag.Get("query")
		if tag == "" {
			continue
		}

		paramValue := query.Get(tag)
		if paramValue == "" {
			continue
		}

		// Set the field value based on its type
		switch field.Type.Kind() {
		case reflect.String:
			elem.Field(i).SetString(paramValue)
		case reflect.Int:
			if intVal, err := strconv.Atoi(paramValue); err == nil {
				elem.Field(i).SetInt(int64(intVal))
			}
		default:
			// Other types are not supported for query params yet
		}
	}
}

// handleValidationError handles a validation error from a request's Validate method.
func handleValidationError(ctx context.Context, w http.ResponseWriter, err error) {
	statusCode := http.StatusBadRequest
	errorCode := dto.ErrorCodeValidationFailed
	details := make(map[string]any)

	var ewsErr dto.ErrorWithStatus
	if errors.As(err, &ewsErr) {
		statusCode = ewsErr.StatusCode()
		errorCode = ewsErr.Code()
		if d := ewsErr.Details(); d != nil {
			details = d
		}
	}

	slog.ErrorContext(ctx, "Validation error", "err", err, "statusCode", statusCode, "code", errorCode)
	writeErrorResponseWithCode(w, statusCode, errorCode, err.Error(), details)
}

// writeBadRequestError writes a 400 Bad Request error response as JSON (internal use).
func writeBadRequestError(w http.ResponseWriter, message string) {
	writeErrorResponseWithCode(w, http.StatusBadRequest, dto.ErrorCodeInternal, message, nil)
}

// writeErrorResponseWithCode writes a detailed error response as JSON with code and details.
func writeErrorResponseWithCode(w http.ResponseWriter, statusCode int, code dto.ErrorCode, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := dto.ErrorResponse{
		Error: dto.ErrorDetails{
			Code:    code,
			Message: message,
		},
		Details: details,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("Failed to encode error response", "error", err)
	}
}

// writeRateLimitError writes a 429 rate limit error response.
func writeRateLimitError(w http.ResponseWriter, result ratelimit.Result) {
	retryAfter := int(result.RetryAfter.Seconds())
	apiErr := dto.RateLimitExceeded(retryAfter)
	writeErrorResponseWithCode(w, apiErr.StatusCode(), apiErr.Code(), apiErr.Error(), apiErr.Details())
}
