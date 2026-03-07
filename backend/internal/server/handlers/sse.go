// SSE handler for streaming workspace events to connected clients.

package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

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

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	sub, cleanup, err := h.Svc.Broker.Subscribe(wsID, user.ID)
	if err != nil {
		slog.Warn("SSE subscribe failed", "ws", wsID, "user", user.ID, "error", err)
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	defer cleanup()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	// Send server revision on connect so clients can detect binary upgrades.
	revMsg := fmt.Appendf(nil, "event: server\ndata: {\"revision\":%q}\n\n", h.Cfg.Revision)
	if _, err := w.Write(revMsg); err != nil {
		return
	}
	flusher.Flush()

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
			if _, err := w.Write(msg); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := w.Write([]byte(": keepalive\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
