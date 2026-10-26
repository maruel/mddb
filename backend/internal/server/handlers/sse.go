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

// SSEHandler serves the Server-Sent Events endpoint for workspace events.
type SSEHandler struct {
	Svc *Services
	Cfg *Config
}

// ServeHTTP streams SSE events for a workspace to an authenticated client.
// Browser EventSource authenticates with the host session cookie; API clients
// may supply an Authorization header. Credentials never travel in query strings.
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
