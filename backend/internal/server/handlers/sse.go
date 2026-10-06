// SSE handler for streaming workspace events to connected clients.

package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/maruel/gomode/sse"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
)

const sseKeepAliveInterval = 30 * time.Second

// InjectTokenFromQuery copies a "token" query parameter into the Authorization
// header when no Authorization header is present. This is needed because the
// browser EventSource API cannot set custom headers.
func InjectTokenFromQuery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if t := r.URL.Query().Get("token"); t != "" && r.Header.Get("Authorization") == "" {
			r.Header.Set("Authorization", "Bearer "+t)
		}
		next.ServeHTTP(w, r)
	})
}

// SSEHandler serves the Server-Sent Events endpoint for workspace events.
type SSEHandler struct {
	Svc *Services
	Cfg *Config
}

// ServeHTTP streams SSE events for a workspace to an authenticated client.
// EventSource can't set custom headers, so the token can be passed as a query
// parameter. InjectTokenFromQuery must wrap this handler before auth middleware.
func (h *SSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user := reqctx.User(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	wsIDStr := r.PathValue("wsID")
	wsID, err := ksid.Parse(wsIDStr)
	if err != nil {
		http.Error(w, "invalid workspace ID", http.StatusBadRequest)
		return
	}

	sub, cleanup, err := h.Svc.Broker.Subscribe(wsID, user.ID)
	if err != nil {
		slog.WarnContext(r.Context(), "SSE subscribe failed", "ws", wsID, "user", user.ID, "err", err)
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	defer cleanup()

	// Every write is deadline-bounded so a client that stops reading cannot pin
	// this handler and its subscription.
	stream := sse.New(w)

	// Send server revision on connect so clients can detect binary upgrades.
	if err := stream.Writef("event: server\ndata: {\"revision\":%q}\n\n", h.Cfg.Revision); err != nil {
		return
	}
	if err := stream.Flush(); err != nil {
		return
	}

	ticker := time.NewTicker(sseKeepAliveInterval)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-sub.C:
			if !ok {
				return
			}
			if err := stream.Writef("%s", msg); err != nil {
				return
			}
			if err := stream.Flush(); err != nil {
				return
			}
		case <-ticker.C:
			if err := stream.KeepAlive(); err != nil {
				return
			}
		}
	}
}
