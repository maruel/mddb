// Tests the compression middleware: bodyless responses, flush and finalization errors.

package server

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// failingWriter is a ResponseWriter whose connection fails every write and
// flush after the headers, and records the write deadline it is given.
type failingWriter struct {
	*httptest.ResponseRecorder

	err      error
	deadline time.Time
}

func (f *failingWriter) Write([]byte) (int, error) { return 0, f.err }
func (f *failingWriter) FlushError() error         { return f.err }
func (f *failingWriter) SetWriteDeadline(t time.Time) error {
	f.deadline = t
	return nil
}

func TestCompressMiddleware(t *testing.T) {
	t.Run("compresses", func(t *testing.T) {
		h := compressMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("hello"))
		}))
		r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		r.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if got := w.Header().Get("Content-Encoding"); got != "gzip" {
			t.Fatalf("Content-Encoding = %q, want gzip", got)
		}
		if w.Body.String() == "hello" {
			t.Fatal("body was not compressed")
		}
	})

	t.Run("compresses after early hints", func(t *testing.T) {
		srv := httptest.NewServer(compressMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Add("Link", "</app.js>; rel=preload")
			w.WriteHeader(http.StatusEarlyHints)
			_, _ = w.Write([]byte("hello"))
		})))
		t.Cleanup(srv.Close)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Accept-Encoding", "gzip")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
			t.Errorf("Content-Encoding = %q, want gzip", got)
		}
	})

	t.Run("bodyless", func(t *testing.T) {
		tests := []struct {
			name   string
			method string
			status int
		}{
			{"HEAD", http.MethodHead, http.StatusOK},
			{"continue", http.MethodGet, http.StatusContinue},
			{"switching protocols", http.MethodGet, http.StatusSwitchingProtocols},
			{"no content", http.MethodGet, http.StatusNoContent},
			{"not modified", http.MethodGet, http.StatusNotModified},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				h := compressMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tc.status)
				}))
				r := httptest.NewRequest(tc.method, "/", http.NoBody)
				r.Header.Set("Accept-Encoding", "gzip")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if got := w.Header().Get("Content-Encoding"); got != "" {
					t.Errorf("Content-Encoding = %q, want empty", got)
				}
				if w.Body.Len() != 0 {
					t.Errorf("body = %d bytes, want 0", w.Body.Len())
				}
			})
		}
	})
}

func TestCompressWriter(t *testing.T) {
	t.Run("FlushError", func(t *testing.T) {
		t.Run("reports a connection flush error", func(t *testing.T) {
			errConn := errors.New("connection reset")
			cw := &compressWriter{ResponseWriter: &failingWriter{ResponseRecorder: httptest.NewRecorder(), err: errConn}, encoding: "gzip"}
			if err := cw.FlushError(); !errors.Is(err, errConn) {
				t.Errorf("FlushError() = %v, want %v", err, errConn)
			}
		})
		t.Run("reaches the connection through wrappers", func(t *testing.T) {
			errConn := errors.New("connection reset")
			inner := &failingWriter{ResponseRecorder: httptest.NewRecorder(), err: errConn}
			cw := &compressWriter{ResponseWriter: &responseWriter{ResponseWriter: inner}, encoding: "gzip"}
			rc := http.NewResponseController(cw)
			deadline := time.Now().Add(time.Minute)
			if err := rc.SetWriteDeadline(deadline); err != nil {
				t.Fatalf("SetWriteDeadline() = %v", err)
			}
			if !inner.deadline.Equal(deadline) {
				t.Errorf("deadline = %v, want %v", inner.deadline, deadline)
			}
			if err := rc.Flush(); !errors.Is(err, errConn) {
				t.Errorf("Flush() = %v, want %v", err, errConn)
			}
		})
	})

	t.Run("finish", func(t *testing.T) {
		t.Run("logs a failure closing the compressor", func(t *testing.T) {
			var logs bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(prev) })

			errConn := errors.New("connection reset")
			h := compressMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("hello"))
			}))
			r := httptest.NewRequest(http.MethodGet, "/secret?token=abc", http.NoBody)
			r.Header.Set("Accept-Encoding", "gzip")
			h.ServeHTTP(&failingWriter{ResponseRecorder: httptest.NewRecorder(), err: errConn}, r)

			got := logs.String()
			if !strings.Contains(got, "response compression finalization failed") || !strings.Contains(got, "connection reset") {
				t.Errorf("log %q does not report the finalization failure", got)
			}
			if strings.Contains(got, "token=abc") {
				t.Errorf("log %q contains the query string", got)
			}
		})
	})
}
