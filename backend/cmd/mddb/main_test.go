// Tests production HTTP shutdown under ordinary and stalled-body requests.

package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestShutdownHTTPServer(t *testing.T) {
	t.Run("graceful", func(t *testing.T) {
		idle := make(chan struct{})
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
			if state == http.StateIdle {
				close(idle)
			}
		}
		srv.Start()
		t.Cleanup(srv.Close)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		// Avoid Shutdown's backoff polling when the HTTP connection is still
		// transitioning from the completed response to idle.
		<-idle
		if err := shutdownHTTPServer(t.Context(), srv.Config); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("stalled body", func(t *testing.T) {
		entered := make(chan struct{})
		finished := make(chan error, 1)
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			close(entered)
			_, err := io.Copy(io.Discard, r.Body)
			finished <- err
			// No response remains deliverable after forced connection closure. Abort
			// instead of entering net/http's 500 ms unread-body FIN/RST delay.
			panic(http.ErrAbortHandler)
		}))
		t.Cleanup(srv.Close)
		conn, err := net.Dial("tcp", srv.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := conn.Close(); err != nil {
				t.Error(err)
			}
		})
		if _, err := io.WriteString(conn, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Length: 100\r\n\r\nx"); err != nil {
			t.Fatal(err)
		}
		<-entered
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		t.Cleanup(cancel)
		if err := shutdownHTTPServer(ctx, srv.Config); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("shutdown error = %v", err)
		}
		select {
		case err := <-finished:
			if err == nil {
				t.Fatal("stalled body read succeeded after forced shutdown")
			}
		case <-time.After(time.Second):
			t.Fatal("shutdown left an active request blocked reading its body")
		}
	})
}
