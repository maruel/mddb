// Tests for SSE event handlers.

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/server/sse"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestSSEHandler(t *testing.T) {
	t.Run("ServeHTTP", func(t *testing.T) {
		t.Run("no user", func(t *testing.T) {
			h := &SSEHandler{
				Svc: &Services{Broker: sse.NewBroker()},
			}
			r := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/abc/events", http.NoBody)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("want 401, got %d", w.Code)
			}
		})

		t.Run("invalid workspace ID", func(t *testing.T) {
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
		})

		t.Run("streams event", func(t *testing.T) {
			broker := sse.NewBroker()
			h := &SSEHandler{
				Svc: &Services{Broker: broker},
				Cfg: &Config{Revision: "test-revision"},
			}

			wsID := ksid.NewID()
			user := &identity.User{ID: ksid.NewID(), Name: "test", Email: "t@t.com"}

			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)

			r := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+wsID.String()+"/events", http.NoBody)
			r.SetPathValue("wsID", wsID.String())
			r = r.WithContext(reqctx.WithUser(ctx, user))

			w := newFlushRecorder()

			done := make(chan struct{})
			var wg sync.WaitGroup
			t.Cleanup(func() { cancel(); wg.Wait() })
			wg.Go(func() {
				h.ServeHTTP(w, r)
				close(done)
			})

			select {
			case <-w.flushed:
			case <-time.After(2 * time.Second):
				t.Fatal("initial server frame missing")
			}

			// Publish an event.
			broker.Publish(wsID, &dto.WorkspaceEvent{
				Type:     dto.EventNodeUpdated,
				NodeID:   ksid.NewID(),
				ActorID:  ksid.NewID(),
				Modified: storage.Now(),
			})

			select {
			case <-w.flushed:
			case <-time.After(2 * time.Second):
				t.Fatal("workspace frame missing")
			}

			cancel()
			select {
			case <-done:
				wg.Wait()
			case <-time.After(2 * time.Second):
				t.Fatal("SSE handler did not stop")
			}

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
		})

		t.Run("observable heartbeat", func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				broker := sse.NewBroker()
				h := &SSEHandler{Svc: &Services{Broker: broker}, Cfg: &Config{Revision: "heartbeat-test"}}
				wsID := ksid.NewID()
				user := &identity.User{ID: ksid.NewID()}
				ctx, cancel := context.WithCancel(t.Context())
				r := httptest.NewRequest(http.MethodGet, "/events", http.NoBody)
				r.SetPathValue("wsID", wsID.String())
				r = r.WithContext(reqctx.WithUser(ctx, user))
				w := newFlushRecorder()
				var wg sync.WaitGroup
				wg.Go(func() { h.ServeHTTP(w, r) })
				synctest.Wait()
				time.Sleep(30 * time.Second)
				synctest.Wait()
				cancel()
				wg.Wait()
				synctest.Wait()
				want := "event: server\ndata: {\"revision\":\"heartbeat-test\"}\n\nevent: heartbeat\ndata: {}\n\n"
				if got := w.Body(); got != want {
					t.Fatalf("frames = %q, want %q", got, want)
				}
				// Reclaim every screen admission slot before cleanup cancels the
				// replacement feeds. A retained heartbeat subscription consumes one.
				admission, stop := context.WithCancel(t.Context())
				t.Cleanup(stop)
				for range 5 {
					if _, err := broker.Subscribe(admission, wsID, user.ID, &sse.EventInterest{All: true}); err != nil {
						t.Fatalf("heartbeat retained screen admission: %v", err)
					}
				}
			})
		})

		t.Run("bounds writes", func(t *testing.T) {
			broker := sse.NewBroker()
			h := &SSEHandler{
				Svc: &Services{Broker: broker},
				Cfg: &Config{Revision: "test-revision"},
			}
			wsID := ksid.NewID()
			user := &identity.User{ID: ksid.NewID(), Name: "test", Email: "t@t.com"}

			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			r := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+wsID.String()+"/events", http.NoBody)
			r.SetPathValue("wsID", wsID.String())
			r = r.WithContext(reqctx.WithUser(ctx, user))
			w := &deadlineRecorder{flushRecorder: newFlushRecorder()}

			done := make(chan struct{})
			var wg sync.WaitGroup
			t.Cleanup(func() { cancel(); wg.Wait() })
			wg.Go(func() {
				h.ServeHTTP(w, r)
				close(done)
			})
			select {
			case <-w.flushed:
			case <-time.After(2 * time.Second):
				t.Fatal("initial server frame missing")
			}

			cancel()
			select {
			case <-done:
				wg.Wait()
			case <-time.After(2 * time.Second):
				t.Fatal("SSE handler did not stop")
			}

			got := w.Deadlines()
			if len(got) < 2 {
				t.Fatalf("got %d write deadlines, want a bounded write followed by a clear", len(got))
			}
			if got[0].IsZero() {
				t.Error("the revision write has no deadline")
			}
			if !got[len(got)-1].IsZero() {
				t.Error("the write deadline is not cleared after the flush")
			}
		})
	})
}

// deadlineRecorder records the write deadlines the handler sets.
type deadlineRecorder struct {
	*flushRecorder

	deadlines []time.Time
}

func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deadlines = append(d.deadlines, t)
	return nil
}

func (d *deadlineRecorder) Deadlines() []time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]time.Time(nil), d.deadlines...)
}

// flushRecorder implements http.ResponseWriter and http.Flusher for testing SSE.
type flushRecorder struct {
	*httptest.ResponseRecorder

	mu      sync.RWMutex
	flushed chan struct{}
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{}, 2)}
}

func (f *flushRecorder) Flush() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ResponseRecorder.Flush()
	select {
	case f.flushed <- struct{}{}:
	default:
	}
}

func (f *flushRecorder) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ResponseRecorder.Write(p)
}

func (f *flushRecorder) Body() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.ResponseRecorder.Body.String()
}
