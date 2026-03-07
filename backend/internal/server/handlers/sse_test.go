package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/server/sse"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestSSEHandler_NoUser(t *testing.T) {
	h := &SSEHandler{
		Svc: &Services{Broker: sse.NewBroker()},
	}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/abc/events", http.NoBody)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestSSEHandler_InvalidWsID(t *testing.T) {
	h := &SSEHandler{
		Svc: &Services{Broker: sse.NewBroker()},
	}
	user := &identity.User{ID: ksid.NewID(), Name: "test", Email: "t@t.com"}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/!/events", http.NoBody)
	r.SetPathValue("wsID", "!")
	r = r.WithContext(reqctx.WithUser(r.Context(), user))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestSSEHandler_StreamsEvent(t *testing.T) {
	broker := sse.NewBroker()
	h := &SSEHandler{
		Svc: &Services{Broker: broker},
		Cfg: &Config{Revision: "test-revision"},
	}

	wsID := ksid.NewID()
	user := &identity.User{ID: ksid.NewID(), Name: "test", Email: "t@t.com"}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	r := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+wsID.String()+"/events", http.NoBody)
	r.SetPathValue("wsID", wsID.String())
	r = r.WithContext(reqctx.WithUser(ctx, user))

	w := newFlushRecorder()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(w, r)
		close(done)
	}()

	// Wait for subscriber to be registered.
	deadline := time.Now().Add(2 * time.Second)
	for broker.SubscriberCount(wsID) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if broker.SubscriberCount(wsID) == 0 {
		t.Fatal("subscriber not registered")
	}

	// Publish an event.
	broker.Publish(wsID, dto.WorkspaceEvent{
		Type:     dto.EventNodeUpdated,
		NodeID:   ksid.NewID(),
		ActorID:  ksid.NewID(),
		Modified: storage.Now(),
	})

	// Wait for the event to be written.
	deadline = time.Now().Add(2 * time.Second)
	for !strings.Contains(w.Body(), "event: workspace") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	<-done

	body := w.Body()
	if !strings.Contains(body, "event: server") {
		t.Fatalf("expected server revision event in body, got: %s", body)
	}
	if !strings.Contains(body, "test-revision") {
		t.Fatalf("expected revision in body, got: %s", body)
	}
	if !strings.Contains(body, "event: workspace") {
		t.Fatalf("expected SSE event in body, got: %s", body)
	}
	if !strings.Contains(body, `"type":"node_updated"`) {
		t.Fatalf("expected node_updated in body, got: %s", body)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Fatalf("want Content-Type text/event-stream, got %s", ct)
	}
}

// flushRecorder implements http.ResponseWriter and http.Flusher for testing SSE.
type flushRecorder struct {
	*httptest.ResponseRecorder
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (f *flushRecorder) Flush() {
	f.ResponseRecorder.Flush()
}

func (f *flushRecorder) Body() string {
	return f.ResponseRecorder.Body.String()
}
