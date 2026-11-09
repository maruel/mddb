// Carries validated identity, user-agent and parent-session metadata through request contexts.

// Package reqctx provides request context utilities for passing request metadata.
package reqctx

import (
	"context"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// Context keys for request metadata.
type contextKey string

const (
	keyUserAgent   contextKey = "userAgent"
	keySessionID   contextKey = "sessionID"
	keySession     contextKey = "session"
	keyTokenString contextKey = "tokenString"
	keyUser        contextKey = "user"
	keyRequestID   contextKey = "requestID"
)

// WithRequestID adds an opaque write echo-correlation ID to the context.
// Oversized hints are ignored; they never affect authentication or authorization.
func WithRequestID(ctx context.Context, id string) context.Context {
	if len(id) > 64 {
		id = ""
	}
	return context.WithValue(ctx, keyRequestID, id)
}

// RequestID returns the originating HTTP request's echo-correlation ID, if supplied.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(keyRequestID).(string)
	return id
}

// WithUserAgent adds the User-Agent to the context.
func WithUserAgent(ctx context.Context, ua string) context.Context {
	return context.WithValue(ctx, keyUserAgent, ua)
}

// WithSessionID adds the session ID to the context.
func WithSessionID(ctx context.Context, id ksid.ID) context.Context {
	return context.WithValue(ctx, keySessionID, id)
}

// WithTokenString adds the JWT token string to the context.
func WithTokenString(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, keyTokenString, token)
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

// WithSession binds the validated host session to downstream authorization and asset links.
func WithSession(ctx context.Context, session *identity.Session) context.Context {
	return context.WithValue(ctx, keySession, session)
}

// Session returns the host session validated for this request, if any.
func Session(ctx context.Context) *identity.Session {
	session, _ := ctx.Value(keySession).(*identity.Session)
	return session
}
