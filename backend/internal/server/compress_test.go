// Tests that the compression middleware leaves bodyless responses untouched.

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
		h := compressMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusEarlyHints)
			_, _ = w.Write([]byte("hello"))
		}))
		r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		r.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if got := w.Header().Get("Content-Encoding"); got != "gzip" {
			t.Fatalf("Content-Encoding = %q, want gzip", got)
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
