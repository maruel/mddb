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
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// Wrap wraps a handler function to work as an http.Handler.
// The function must have signature: func(context.Context, In) (*Out, error)
// where In can be unmarshalled from JSON and Out is a struct.
// Path parameters can be extracted by tagging struct fields with `path:"name"`.
//
// Example:
//
//	type GetPageRequest struct {
//	    ID string `path:"id"`
//	}
//
//	func (h *Handler) GetPage(ctx context.Context, req GetPageRequest) (*Response, error)
func Wrap[In any, Out any](fn func(context.Context, In) (*Out, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
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
		var input In
		if len(body) > 0 {
			d := json.NewDecoder(bytes.NewReader(body))
			d.DisallowUnknownFields()
			if err := d.Decode(&input); err != nil {
				slog.ErrorContext(ctx, "Failed to decode request body", "err", err)
				writeBadRequestError(w, "Invalid request body")
				return
			}
		}

		// Extract path parameters and populate request struct
		populatePathParams(r, &input)
		// Extract query parameters and populate request struct
		populateQueryParams(r, &input)

		output, err := fn(ctx, input)
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
// The function must have signature: func(context.Context, jsonldb.ID, *identity.User, In) (*Out, error)
// where orgID is the organization ID from the path (zero if not present),
// user is the authenticated user, In can be unmarshalled from JSON, and Out is a struct.
func WrapAuth[In any, Out any](
	userService *identity.UserService,
	memService *identity.MembershipService,
	jwtSecret []byte,
	requiredRole identity.UserRole,
	fn func(context.Context, jsonldb.ID, *identity.User, In) (*Out, error),
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Validate JWT
		user, err := validateJWT(r, userService, jwtSecret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
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

			membership, err := memService.Get(user.ID, orgID)
			if err != nil {
				http.Error(w, "Forbidden: not a member of this organization", http.StatusForbidden)
				return
			}

			if !hasPermission(membership.Role, requiredRole) {
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
		var input In
		if len(body) > 0 {
			d := json.NewDecoder(bytes.NewReader(body))
			d.DisallowUnknownFields()
			if err := d.Decode(&input); err != nil {
				slog.ErrorContext(ctx, "Failed to decode request body", "err", err)
				writeBadRequestError(w, "Invalid request body")
				return
			}
		}

		populatePathParams(r, &input)
		populateQueryParams(r, &input)

		output, err := fn(ctx, orgID, user, input)
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
// extract orgID from the path via r.PathValue("orgID") if needed.
func WrapAuthRaw(
	userService *identity.UserService,
	memService *identity.MembershipService,
	jwtSecret []byte,
	requiredRole identity.UserRole,
	fn http.HandlerFunc,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate JWT
		_, err := validateJWT(r, userService, jwtSecret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		// Check organization membership if orgID is in path
		orgIDStr := r.PathValue("orgID")
		if orgIDStr != "" {
			orgID, err := jsonldb.DecodeID(orgIDStr)
			if err != nil {
				http.Error(w, "Invalid organization ID format", http.StatusBadRequest)
				return
			}

			// Re-validate JWT to get user (we already validated above, so this should not fail)
			user, err := validateJWT(r, userService, jwtSecret)
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
			membership, err := memService.Get(user.ID, orgID)
			if err != nil {
				http.Error(w, "Forbidden: not a member of this organization", http.StatusForbidden)
				return
			}

			if !hasPermission(membership.Role, requiredRole) {
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
func WrapGlobalAdmin[In any, Out any](
	userService *identity.UserService,
	jwtSecret []byte,
	fn func(context.Context, *identity.User, In) (*Out, error),
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		user, err := validateJWT(r, userService, jwtSecret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		if !user.IsGlobalAdmin {
			http.Error(w, "Forbidden: global admin required", http.StatusForbidden)
			return
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
		var input In
		if len(body) > 0 {
			d := json.NewDecoder(bytes.NewReader(body))
			d.DisallowUnknownFields()
			if err := d.Decode(&input); err != nil {
				slog.ErrorContext(ctx, "Failed to decode request body", "err", err)
				writeBadRequestError(w, "Invalid request body")
				return
			}
		}

		populatePathParams(r, &input)
		populateQueryParams(r, &input)

		output, err := fn(ctx, user, input)
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
)

// validateJWT extracts and validates the JWT token from the request.
func validateJWT(r *http.Request, userService *identity.UserService, jwtSecret []byte) (*identity.User, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, errUnauthorized
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, errInvalidAuthHdr
	}

	tokenString := parts[1]
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		return nil, errInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errInvalidClaims
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok {
		return nil, errInvalidUserIDToken
	}

	userID, err := jsonldb.DecodeID(userIDStr)
	if err != nil {
		return nil, errInvalidUserIDFmt
	}

	user, err := userService.Get(userID)
	if err != nil {
		return nil, errUserNotFound
	}

	return user, nil
}

// populatePathParams extracts path parameters from the request and populates
// struct fields tagged with `path:"paramName"`.
func populatePathParams(r *http.Request, input any) {
	val := reflect.ValueOf(input)
	if val.Kind() != reflect.Ptr {
		return // Skip if not a pointer
	}

	elem := val.Elem()
	if elem.Kind() != reflect.Struct {
		return // Skip if not a struct
	}

	typ := elem.Type()
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

		// Set the field value if it's a string field
		if field.Type.Kind() == reflect.String {
			elem.Field(i).SetString(paramValue)
		}
	}
}

// populateQueryParams extracts query parameters from the request and populates
// struct fields tagged with `query:"paramName"`.
func populateQueryParams(r *http.Request, input any) {
	val := reflect.ValueOf(input)
	if val.Kind() != reflect.Ptr {
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
