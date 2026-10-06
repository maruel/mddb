// Defines request context keys and helper functions for metadata access.

// Package reqctx provides request context utilities for passing request metadata.
package reqctx

import (
	"context"
	"net/http"
	"net/netip"
	"slices"
	"strings"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// ResolveClientIP returns the client address for r. It accepts forwarding
// headers only when r's direct peer is in trustedProxies. For an
// X-Forwarded-For chain, it returns the rightmost address outside
// trustedProxies. A malformed header or an all-trusted chain yields the direct
// peer.
func ResolveClientIP(r *http.Request, trustedProxies []netip.Prefix) string {
	direct := directClientIP(r.RemoteAddr)
	peer, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil || !trusts(peer.Addr(), trustedProxies) {
		return direct
	}
	if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
		return clientIPFromXForwardedFor(xff, trustedProxies, direct)
	}
	if xri := r.Header.Values("X-Real-IP"); len(xri) > 0 {
		return clientIPFromRealIP(xri, direct)
	}
	return direct
}

func clientIPFromXForwardedFor(values []string, trustedProxies []netip.Prefix, direct string) string {
	var chain []netip.Addr
	for _, value := range values {
		for part := range strings.SplitSeq(value, ",") {
			addr, err := netip.ParseAddr(strings.TrimSpace(part))
			if err != nil {
				return direct
			}
			chain = append(chain, addr)
		}
	}
	for _, addr := range slices.Backward(chain) {
		if !trusts(addr, trustedProxies) {
			return addr.String()
		}
	}
	return direct
}

func clientIPFromRealIP(values []string, direct string) string {
	if len(values) != 1 {
		return direct
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(values[0]))
	if err != nil {
		return direct
	}
	return addr.String()
}

func directClientIP(remoteAddr string) string {
	if addr, err := netip.ParseAddrPort(remoteAddr); err == nil {
		return addr.Addr().String()
	}
	if addr, err := netip.ParseAddr(remoteAddr); err == nil {
		return addr.String()
	}
	return ""
}

// trusts reports whether addr belongs to one of trustedProxies.
func trusts(addr netip.Addr, trustedProxies []netip.Prefix) bool {
	addr = addr.Unmap()
	return slices.ContainsFunc(trustedProxies, func(p netip.Prefix) bool { return p.Contains(addr) })
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
func WithSessionID(ctx context.Context, id ksid.ID) context.Context {
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
func SessionID(ctx context.Context) ksid.ID {
	if v, ok := ctx.Value(keySessionID).(ksid.ID); ok {
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
