// Provides middleware for standardizing HTTP handlers.

package server

import (
	"bytes"
	"context"
	"encoding"
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
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/ratelimit"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// addRequestMetadataToContext adds client IP and User-Agent to the context.
func addRequestMetadataToContext(ctx context.Context, r *http.Request) context.Context {
	ctx = reqctx.WithClientIP(ctx, reqctx.GetClientIP(r))
	ctx = reqctx.WithUserAgent(ctx, r.Header.Get("User-Agent"))
	return ctx
}

// isMutating returns true for HTTP methods that modify state.
func isMutating(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

// commitDBIfMutating commits DB changes after a mutating request.
//
// It always attempts the commit regardless of handler outcome: if the handler
// wrote data before returning an error, the change is already on disk and must
// be tracked. When no files changed, CommitDBChanges is a no-op.
func commitDBIfMutating(ctx context.Context, r *http.Request, rootRepo *git.RootRepo, author git.Author) {
	if !isMutating(r.Method) {
		return
	}
	msg := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
	if err := rootRepo.CommitDBChanges(ctx, author, msg); err != nil {
		slog.ErrorContext(ctx, "Failed to commit DB changes", "err", err)
	}
}

// authResult holds the result of JWT/session validation.
type authResult struct {
	user        *identity.User
	sessionID   ksid.ID
	tokenString string
}

// checkRateLimit checks rate limit and wraps the response writer if needed.
// Returns the (possibly wrapped) writer and whether the request should proceed.
func checkRateLimit(w http.ResponseWriter, tier *ratelimit.Tier, identifier string) (http.ResponseWriter, bool) {
	if tier == nil {
		return w, true
	}
	key := ratelimit.BuildKey(tier.Scope, identifier, tier.Name)
	result := tier.Limiter.Allow(key)
	w = ratelimit.NewResponseWriter(w, result)
	if !result.Allowed {
		writeRateLimitError(w, result)
		return w, false
	}
	return w, true
}

// readAndDecodeBody reads the request body with size limit and decodes JSON into input.
// Returns false if an error occurred and was written to the response.
func readAndDecodeBody[In any](ctx context.Context, w http.ResponseWriter, r *http.Request, input *In, cfg *handlers.Config) bool {
	// Limit request body size
	if cfg != nil && cfg.Quotas.MaxRequestBodyBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, cfg.Quotas.MaxRequestBodyBytes)
	}

	body, err := io.ReadAll(r.Body)
	if err2 := r.Body.Close(); err == nil {
		err = err2
	}
	if err != nil {
		if maxBytesErr := checkMaxBytesError(err); maxBytesErr != nil {
			apiErr := dto.PayloadTooLarge(maxBytesErr.Limit)
			writeErrorResponseWithCode(w, apiErr.StatusCode(), apiErr.Code(), apiErr.Error(), apiErr.Details())
			return false
		}
		slog.ErrorContext(ctx, "Failed to read request body", "err", err)
		writeBadRequestError(w, "Failed to read request body")
		return false
	}

	if len(body) > 0 {
		d := json.NewDecoder(bytes.NewReader(body))
		d.DisallowUnknownFields()
		if err := d.Decode(input); err != nil {
			slog.ErrorContext(ctx, "Failed to decode request body", "err", err)
			writeBadRequestError(w, "Invalid request body")
			return false
		}
	}
	return true
}

// writeJSONResponse writes a JSON response or error response.
func writeJSONResponse[Out any](ctx context.Context, w http.ResponseWriter, output *Out, err error) {
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
}

// getRateLimitIdentifier returns the appropriate identifier for rate limiting based on scope.
func getRateLimitIdentifier(tier *ratelimit.Tier, user *identity.User, r *http.Request) string {
	if tier.Scope == ratelimit.ScopeUser && user != nil {
		return user.ID.String()
	}
	return reqctx.GetClientIP(r)
}

// validateAuthWithContext validates JWT and session, updating context with session info.
func validateAuthWithContext(ctx context.Context, r *http.Request, svc *handlers.Services, cfg *handlers.Config) (*authResult, context.Context, error) {
	user, sessionID, tokenString, err := validateJWTAndSession(r, svc.User, svc.Session, cfg.JWTSecret)
	if err != nil {
		return nil, ctx, err
	}
	if !sessionID.IsZero() {
		ctx = reqctx.WithSessionID(ctx, sessionID)
	}
	if tokenString != "" {
		ctx = reqctx.WithTokenString(ctx, tokenString)
	}
	return &authResult{user: user, sessionID: sessionID, tokenString: tokenString}, ctx, nil
}

// checkWSMembership validates workspace membership and returns the effective role.
// Returns an error string and HTTP status code if access is denied.
func checkWSMembership(
	user *identity.User,
	wsID ksid.ID,
	svc *handlers.Services,
	requiredRole identity.WorkspaceRole,
) (errMsg string, statusCode int) {
	ws, err := svc.Workspace.Get(wsID)
	if err != nil {
		return "Workspace not found", http.StatusNotFound
	}

	orgMem, err := svc.OrgMembership.Get(user.ID, ws.OrganizationID)
	if err != nil {
		return "Forbidden: not a member of this organization", http.StatusForbidden
	}

	var effectiveRole identity.WorkspaceRole
	if orgMem.Role == identity.OrgRoleOwner || orgMem.Role == identity.OrgRoleAdmin {
		effectiveRole = identity.WSRoleAdmin
	} else {
		wsMem, err := svc.WSMembership.Get(user.ID, wsID)
		if err != nil {
			return "Forbidden: not a member of this workspace", http.StatusForbidden
		}
		effectiveRole = wsMem.Role
	}

	if !hasWSPermission(effectiveRole, requiredRole) {
		return "Forbidden: insufficient permissions", http.StatusForbidden
	}
	return "", 0
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
}, Out any](fn func(context.Context, PtrIn) (*Out, error), cfg *handlers.Config, limiters *ratelimit.Limiters) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := addRequestMetadataToContext(r.Context(), r)

		// Rate limit check for unauthenticated endpoints
		var ok bool
		if tier := limiters.MatchUnauth(r.Method, r.URL.Path); tier != nil {
			w, ok = checkRateLimit(w, tier, reqctx.GetClientIP(r))
			if !ok {
				return
			}
		}

		input := new(In)
		if !readAndDecodeBody(ctx, w, r, input, cfg) {
			return
		}

		populatePathParams(r, input)
		populateQueryParams(r, input)

		if err := PtrIn(input).Validate(); err != nil {
			handleValidationError(ctx, w, err)
			return
		}

		output, err := fn(ctx, PtrIn(input))
		writeJSONResponse(ctx, w, output, err)
	})
}

// WrapWithSvc wraps an unauthenticated handler with access to services (for DB commit hook).
func WrapWithSvc[In any, PtrIn interface {
	*In
	dto.Validatable
}, Out any](fn func(context.Context, PtrIn) (*Out, error), svc *handlers.Services, cfg *handlers.Config, limiters *ratelimit.Limiters) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := addRequestMetadataToContext(r.Context(), r)

		// Rate limit check for unauthenticated endpoints
		var ok bool
		if tier := limiters.MatchUnauth(r.Method, r.URL.Path); tier != nil {
			w, ok = checkRateLimit(w, tier, reqctx.GetClientIP(r))
			if !ok {
				return
			}
		}

		input := new(In)
		if !readAndDecodeBody(ctx, w, r, input, cfg) {
			return
		}

		populatePathParams(r, input)
		populateQueryParams(r, input)

		if err := PtrIn(input).Validate(); err != nil {
			handleValidationError(ctx, w, err)
			return
		}

		output, err := fn(ctx, PtrIn(input))
		commitDBIfMutating(ctx, r, svc.RootRepo, git.Author{})
		writeJSONResponse(ctx, w, output, err)
	})
}

// checkMaxBytesError checks if an error is a MaxBytesError and returns it, or nil.
func checkMaxBytesError(err error) *http.MaxBytesError {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return maxBytesErr
	}
	return nil
}

// WrapAuth wraps an authenticated handler function to work as an http.Handler.
// Use this for endpoints that require authentication but no organization context.
// The function must have signature: func(context.Context, *identity.User, *In) (*Out, error)
// *In must implement dto.Validatable.
func WrapAuth[In any, PtrIn interface {
	*In
	dto.Validatable
}, Out any](
	fn func(context.Context, *identity.User, PtrIn) (*Out, error),
	svc *handlers.Services,
	cfg *handlers.Config,
	limiters *ratelimit.Limiters,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := addRequestMetadataToContext(r.Context(), r)

		// Validate JWT and session
		auth, ctx, err := validateAuthWithContext(ctx, r, svc, cfg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		// Rate limit check for authenticated endpoints
		if tier := limiters.MatchAuth(r.Method, r.URL.Path); tier != nil {
			var ok bool
			w, ok = checkRateLimit(w, tier, getRateLimitIdentifier(tier, auth.user, r))
			if !ok {
				return
			}
		}

		input := new(In)
		if !readAndDecodeBody(ctx, w, r, input, cfg) {
			return
		}

		populatePathParams(r, input)
		populateQueryParams(r, input)

		if err := PtrIn(input).Validate(); err != nil {
			handleValidationError(ctx, w, err)
			return
		}

		output, err := fn(ctx, auth.user, PtrIn(input))
		commitDBIfMutating(ctx, r, svc.RootRepo, handlers.GitAuthor(auth.user))
		writeJSONResponse(ctx, w, output, err)
	})
}

// WrapOrgAuth wraps an authenticated handler function for organization-scoped routes.
// It validates JWT, checks organization membership with required role, and parses the request.
// The function must have signature: func(context.Context, ksid.ID, *identity.User, *In) (*Out, error)
// where orgID is the organization ID from the path.
// *In must implement dto.Validatable.
func WrapOrgAuth[In any, PtrIn interface {
	*In
	dto.Validatable
}, Out any](
	fn func(context.Context, ksid.ID, *identity.User, PtrIn) (*Out, error),
	svc *handlers.Services,
	cfg *handlers.Config,
	requiredRole identity.OrganizationRole,
	limiters *ratelimit.Limiters,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := addRequestMetadataToContext(r.Context(), r)

		// Validate JWT and session
		auth, ctx, err := validateAuthWithContext(ctx, r, svc, cfg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		// Rate limit check for authenticated endpoints
		if tier := limiters.MatchAuth(r.Method, r.URL.Path); tier != nil {
			var ok bool
			w, ok = checkRateLimit(w, tier, getRateLimitIdentifier(tier, auth.user, r))
			if !ok {
				return
			}
		}

		// Extract and validate organization ID from path
		orgIDStr := r.PathValue("orgID")
		if orgIDStr == "" {
			http.Error(w, "Organization ID required", http.StatusBadRequest)
			return
		}
		orgID, err := ksid.Parse(orgIDStr)
		if err != nil {
			http.Error(w, "Invalid organization ID format", http.StatusBadRequest)
			return
		}

		// Check organization membership and role
		membership, err := svc.OrgMembership.Get(auth.user.ID, orgID)
		if err != nil {
			http.Error(w, "Forbidden: not a member of this organization", http.StatusForbidden)
			return
		}
		if !hasOrgPermission(membership.Role, requiredRole) {
			http.Error(w, "Forbidden: insufficient permissions", http.StatusForbidden)
			return
		}

		input := new(In)
		if !readAndDecodeBody(ctx, w, r, input, cfg) {
			return
		}

		populatePathParams(r, input)
		populateQueryParams(r, input)

		if err := PtrIn(input).Validate(); err != nil {
			handleValidationError(ctx, w, err)
			return
		}

		output, err := fn(ctx, orgID, auth.user, PtrIn(input))
		commitDBIfMutating(ctx, r, svc.RootRepo, handlers.GitAuthor(auth.user))
		writeJSONResponse(ctx, w, output, err)
	})
}

// WrapWSAuth wraps an authenticated handler function for workspace-scoped routes.
// It validates JWT, checks workspace membership (or org admin access), and parses the request.
// The function must have signature: func(context.Context, ksid.ID, *identity.User, *In) (*Out, error)
// where wsID is the workspace ID from the path, user is the authenticated user.
// Org admins/owners automatically have admin access to workspaces within their org.
// *In must implement dto.Validatable.
func WrapWSAuth[In any, PtrIn interface {
	*In
	dto.Validatable
}, Out any](
	fn func(context.Context, ksid.ID, *identity.User, PtrIn) (*Out, error),
	svc *handlers.Services,
	cfg *handlers.Config,
	requiredRole identity.WorkspaceRole,
	limiters *ratelimit.Limiters,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := addRequestMetadataToContext(r.Context(), r)

		// Validate JWT and session
		auth, ctx, err := validateAuthWithContext(ctx, r, svc, cfg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		// Rate limit check
		if tier := limiters.MatchAuth(r.Method, r.URL.Path); tier != nil {
			var ok bool
			w, ok = checkRateLimit(w, tier, getRateLimitIdentifier(tier, auth.user, r))
			if !ok {
				return
			}
		}

		// Check workspace membership
		var wsID ksid.ID
		wsIDStr := r.PathValue("wsID")
		if wsIDStr != "" {
			wsID, err = ksid.Parse(wsIDStr)
			if err != nil {
				http.Error(w, "Invalid workspace ID format", http.StatusBadRequest)
				return
			}

			if errMsg, status := checkWSMembership(auth.user, wsID, svc, requiredRole); errMsg != "" {
				http.Error(w, errMsg, status)
				return
			}
		}

		input := new(In)
		if !readAndDecodeBody(ctx, w, r, input, cfg) {
			return
		}

		populatePathParams(r, input)
		populateQueryParams(r, input)

		if err := PtrIn(input).Validate(); err != nil {
			handleValidationError(ctx, w, err)
			return
		}

		output, err := fn(ctx, wsID, auth.user, PtrIn(input))
		commitDBIfMutating(ctx, r, svc.RootRepo, handlers.GitAuthor(auth.user))
		triggerAutoPush(svc, r.Method, wsID, err)
		writeJSONResponse(ctx, w, output, err)
	})
}

// triggerAutoPush fires an async push if the request was mutating, successful,
// and the workspace has auto-push enabled.
func triggerAutoPush(svc *handlers.Services, method string, wsID ksid.ID, handlerErr error) {
	if handlerErr != nil || !isMutating(method) || wsID.IsZero() || svc.SyncService == nil {
		return
	}
	svc.SyncService.TriggerPush(wsID)
}

// WrapAuthRaw wraps a raw http.HandlerFunc with authentication and role checking.
// Use this for handlers that need to handle requests directly (e.g., multipart forms).
// The wrapped handler receives the request with validated auth - the handler should
// extract wsID from the path via r.PathValue("wsID") if needed.
func WrapAuthRaw(
	fn http.HandlerFunc,
	svc *handlers.Services,
	cfg *handlers.Config,
	requiredRole identity.WorkspaceRole,
	limiters *ratelimit.Limiters,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate JWT and session (don't need context for raw handlers)
		user, _, _, err := validateJWTAndSession(r, svc.User, svc.Session, cfg.JWTSecret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		// Rate limit check for authenticated endpoints
		if tier := limiters.MatchAuth(r.Method, r.URL.Path); tier != nil {
			var ok bool
			w, ok = checkRateLimit(w, tier, getRateLimitIdentifier(tier, user, r))
			if !ok {
				return
			}
		}

		// Check workspace membership if wsID is in path
		var wsID ksid.ID
		wsIDStr := r.PathValue("wsID")
		if wsIDStr != "" {
			var err error
			wsID, err = ksid.Parse(wsIDStr)
			if err != nil {
				http.Error(w, "Invalid workspace ID format", http.StatusBadRequest)
				return
			}

			if errMsg, status := checkWSMembership(user, wsID, svc, requiredRole); errMsg != "" {
				http.Error(w, errMsg, status)
				return
			}
		}

		// Limit request body size for raw handlers
		if cfg != nil && cfg.Quotas.MaxRequestBodyBytes > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, cfg.Quotas.MaxRequestBodyBytes)
		}

		// Store user in context for raw handlers
		ctx := reqctx.WithUser(r.Context(), user)
		fn(w, r.WithContext(ctx))
		// Commit DB changes for mutating raw handlers (e.g., asset upload)
		commitDBIfMutating(ctx, r, svc.RootRepo, handlers.GitAuthor(user))
		triggerAutoPush(svc, r.Method, wsID, nil)
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
	fn func(context.Context, *identity.User, PtrIn) (*Out, error),
	svc *handlers.Services,
	cfg *handlers.Config,
	limiters *ratelimit.Limiters,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := addRequestMetadataToContext(r.Context(), r)

		user, _, _, err := validateJWTAndSession(r, svc.User, svc.Session, cfg.JWTSecret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		if !user.IsGlobalAdmin {
			http.Error(w, "Forbidden: global admin required", http.StatusForbidden)
			return
		}

		// Rate limit check for admin endpoints
		if tier := limiters.MatchAuth(r.Method, r.URL.Path); tier != nil {
			var ok bool
			w, ok = checkRateLimit(w, tier, getRateLimitIdentifier(tier, user, r))
			if !ok {
				return
			}
		}

		input := new(In)
		if !readAndDecodeBody(ctx, w, r, input, cfg) {
			return
		}

		populatePathParams(r, input)
		populateQueryParams(r, input)

		if err := PtrIn(input).Validate(); err != nil {
			handleValidationError(ctx, w, err)
			return
		}

		output, err := fn(ctx, user, PtrIn(input))
		commitDBIfMutating(ctx, r, svc.RootRepo, handlers.GitAuthor(user))
		writeJSONResponse(ctx, w, output, err)
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
func validateJWTAndSession(r *http.Request, userService *identity.UserService, sessionService *identity.SessionService, jwtSecret []byte) (*identity.User, ksid.ID, string, error) {
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

	userID, err := ksid.Parse(userIDStr)
	if err != nil {
		return nil, 0, "", errInvalidUserIDFmt
	}

	user, err := userService.Get(userID)
	if err != nil {
		return nil, 0, "", errUserNotFound
	}

	// Validate session if sessionService is provided and token has session ID
	var sessionID ksid.ID
	if sessionService != nil {
		if sidStr, ok := claims["sid"].(string); ok && sidStr != "" {
			sessionID, err = ksid.Parse(sidStr)
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
	jsonldbIDType := reflect.TypeFor[ksid.ID]()
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
			if id, err := ksid.Parse(paramValue); err == nil {
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
		fieldVal := elem.Field(i)
		switch field.Type.Kind() {
		case reflect.String:
			fieldVal.SetString(paramValue)
		case reflect.Int:
			if intVal, err := strconv.Atoi(paramValue); err == nil {
				fieldVal.SetInt(int64(intVal))
			}
		default:
			// Try to use encoding.TextUnmarshaler interface for custom types
			if fieldVal.CanAddr() {
				if unmarshaler, ok := fieldVal.Addr().Interface().(encoding.TextUnmarshaler); ok {
					_ = unmarshaler.UnmarshalText([]byte(paramValue))
				}
			}
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
