// Defines request context keys and helper functions for metadata access.

// Package reqctx provides request context utilities for passing request metadata.
package reqctx

import (
	"context"
	"net/http"
	"strings"

	"github.com/maruel/mddb/backend/internal/rid"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// GetClientIP extracts the client IP from an HTTP request,
// checking X-Forwarded-For and X-Real-IP headers for proxied requests.
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	// X-Forwarded-For can contain multiple IPs: "client, proxy1, proxy2"
	// The leftmost IP is the original client.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, found := strings.Cut(xff, ","); found {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr, stripping port if present
	addr := r.RemoteAddr
	// Handle IPv6 addresses like [::1]:8080
	if strings.HasPrefix(addr, "[") {
		if host, _, found := strings.Cut(addr, "]:"); found {
			return host[1:] // Strip leading "["
		}
		// Malformed but try to be lenient
		return strings.Trim(addr, "[]")
	}
	// IPv4 or hostname with port
	if host, _, found := strings.Cut(addr, ":"); found {
		return host
	}
	return addr
}

// Context keys for request metadata.
type contextKey string

const (
	keyClientIP    contextKey = "clientIP"
	keyUserAgent   contextKey = "userAgent"
	keyCountryCode contextKey = "countryCode"
	keySessionID   contextKey = "sessionID"
	keyTokenString contextKey = "tokenString"
	keyUser        contextKey = "user"
)

// WithClientIP adds the client IP to the context.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, keyClientIP, ip)
}

// WithUserAgent adds the User-Agent to the context.
func WithUserAgent(ctx context.Context, ua string) context.Context {
	return context.WithValue(ctx, keyUserAgent, ua)
}

// WithCountryCode adds the country code to the context.
func WithCountryCode(ctx context.Context, cc string) context.Context {
	return context.WithValue(ctx, keyCountryCode, cc)
}

// CountryCode extracts the country code from the context.
func CountryCode(ctx context.Context) string {
	if v, ok := ctx.Value(keyCountryCode).(string); ok {
		return v
	}
	return ""
}

// WithSessionID adds the session ID to the context.
func WithSessionID(ctx context.Context, id rid.ID) context.Context {
	return context.WithValue(ctx, keySessionID, id)
}

// WithTokenString adds the JWT token string to the context.
func WithTokenString(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, keyTokenString, token)
}

// ClientIP extracts the client IP from the context.
func ClientIP(ctx context.Context) string {
	if v, ok := ctx.Value(keyClientIP).(string); ok {
		return v
	}
	return ""
}

// UserAgent extracts the User-Agent from the context.
func UserAgent(ctx context.Context) string {
	if v, ok := ctx.Value(keyUserAgent).(string); ok {
		return v
	}
	return ""
}

// SessionID extracts the session ID from the context.
func SessionID(ctx context.Context) rid.ID {
	if v, ok := ctx.Value(keySessionID).(rid.ID); ok {
		return v
	}
	return 0
}

// TokenString extracts the JWT token string from the context.
func TokenString(ctx context.Context) string {
	if v, ok := ctx.Value(keyTokenString).(string); ok {
		return v
	}
	return ""
}

// WithUser adds the authenticated user to the context.
func WithUser(ctx context.Context, user *identity.User) context.Context {
	return context.WithValue(ctx, keyUser, user)
}

// User extracts the authenticated user from the context.
func User(ctx context.Context) *identity.User {
	if v, ok := ctx.Value(keyUser).(*identity.User); ok {
		return v
	}
	return nil
}
