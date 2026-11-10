// Streams workspace events and observable heartbeat frames to connected clients.

package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/maruel/gomode/sse"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	workspacefeed "github.com/maruel/mddb/backend/internal/server/sse"
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

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	events, err := h.Svc.Broker.Subscribe(ctx, wsID, user.ID, &workspacefeed.EventInterest{All: true})
	if err != nil {
		slog.WarnContext(r.Context(), "SSE subscribe failed", "ws", wsID, "user", user.ID, "err", err)
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}

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

	for {
		select {
		case <-ctx.Done():
			return
		case change, ok := <-events.Events:
			if !ok {
				return
			}
			msg, err := change.SSE()
			if err != nil {
				slog.ErrorContext(ctx, "encode workspace event", "err", err)
				return
			}
			if err := stream.Writef("%s", msg); err != nil {
				return
			}
			if err := stream.Flush(); err != nil {
				return
			}
		case <-ticker.C:
			if err := stream.Writef("event: heartbeat\ndata: {}\n\n"); err != nil {
				return
			}
			if err := stream.Flush(); err != nil {
				return
			}
		}
	}
}
