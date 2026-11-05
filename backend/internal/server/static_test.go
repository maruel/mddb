// Tests cache headers and SPA fallback for precompressed embedded frontend assets.

package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/andybalholm/brotli"
)

func brotliCompress(t *testing.T, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := brotli.NewWriter(&buf)
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatalf("brotli write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("brotli close: %v", err)
	}
	return buf.Bytes()
}

func TestSetCacheHeaders(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "document", path: "/index.html", want: "no-cache"},
		{name: "hashed asset", path: "/assets/app-abc123.js", want: "public, max-age=31536000, immutable"},
		{name: "service worker", path: "/sw.js", want: "no-cache, no-store, must-revalidate"},
		{name: "web manifest", path: "/manifest.webmanifest", want: "public, max-age=3600"},
		{name: "icon", path: "/icon.svg", want: "public, max-age=3600"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			setCacheHeaders(rec, test.path)
			if got := rec.Header().Get("Cache-Control"); got != test.want {
				t.Fatalf("Cache-Control = %q, want %q", got, test.want)
			}
		})
	}
}

// TestStaticHandlerDocumentRevalidates guards that the document itself, including
// the SPA fallback for unknown routes, is revalidated rather than cached, while
// hashed assets stay immutable.
func TestStaticHandlerDocumentRevalidates(t *testing.T) {
	t.Parallel()

	dist := fstest.MapFS{
		"index.html.br":        &fstest.MapFile{Data: brotliCompress(t, "<html>mddb</html>")},
		"assets/app-abc.js.br": &fstest.MapFile{Data: brotliCompress(t, "console.log(1)")},
	}
	handler := newStaticHandler(dist)

	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "root", path: "/", want: "no-cache"},
		{name: "document", path: "/index.html", want: "no-cache"},
		{name: "spa route", path: "/w/@example/some-page", want: "no-cache"},
		{name: "hashed asset", path: "/assets/app-abc.js", want: "public, max-age=31536000, immutable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, test.path, http.NoBody)
			req.Header.Set("Accept-Encoding", "br")
			rec := httptest.NewRecorder()
			handler(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Header().Get("Cache-Control"); got != test.want {
				t.Fatalf("Cache-Control = %q, want %q", got, test.want)
			}
		})
	}
}
func TestEncodingNegotiation(t *testing.T) {
	t.Parallel()
	dist := fstest.MapFS{"assets/app.js.br": {Data: brotliCompress(t, "test payload")}}
	handlers := []struct {
		name    string
		handler http.Handler
		tie     string
	}{
		{name: "static", handler: newStaticHandler(dist), tie: "br"},
		{name: "dynamic", handler: compressMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("test payload"))
		})), tie: "zstd"},
	}
	for _, handler := range handlers {
		t.Run(handler.name, func(t *testing.T) {
			t.Parallel()
			for _, test := range []struct {
				name   string
				header string
				want   string
			}{
				{name: "excluded brotli", header: "br;q=0, gzip", want: "gzip"},
				{name: "quality preference", header: "br;q=0.1, gzip;q=0.9, zstd;q=0.5", want: "gzip"},
				{name: "tie", header: "gzip, br, zstd", want: handler.tie},
				{name: "wildcard", header: "*;q=0.7, br;q=0, zstd;q=0", want: "gzip"},
				{name: "explicit overrides wildcard", header: "*;q=1, br;q=0.5, zstd;q=0.3", want: "gzip"},
				{name: "identity preference", header: "identity;q=1, gzip;q=0.5", want: ""},
				{name: "identity fallback", header: "deflate", want: ""},
				{name: "empty", header: "", want: ""},
				{name: "all compressed excluded", header: "br;q=0, gzip;q=0, zstd;q=0", want: ""},
				{name: "explicit identity", header: "*;q=0, identity;q=0.5", want: ""},
				{name: "case insensitive", header: "GZip;Q=1, BR;q=0", want: "gzip"},
				{name: "invalid quality", header: "br;q=bogus, gzip;q=0.5", want: "gzip"},
			} {
				t.Run(test.name, func(t *testing.T) {
					t.Parallel()
					req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/assets/app.js", http.NoBody)
					req.Header.Set("Accept-Encoding", test.header)
					rec := httptest.NewRecorder()
					handler.handler.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Fatalf("status = %d, want 200", rec.Code)
					}
					if got := rec.Header().Get("Content-Encoding"); got != test.want {
						t.Errorf("Content-Encoding = %q, want %q", got, test.want)
					}
					if !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
						t.Error("response does not vary by Accept-Encoding")
					}
				})
			}
			t.Run("unacceptable", func(t *testing.T) {
				t.Parallel()
				for _, header := range []string{"*;q=0", "deflate, identity;q=0", "br;q=0, gzip;q=0, zstd;q=0, identity;q=0"} {
					req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/assets/app.js", http.NoBody)
					req.Header.Set("Accept-Encoding", header)
					rec := httptest.NewRecorder()
					handler.handler.ServeHTTP(rec, req)
					if rec.Code != http.StatusNotAcceptable {
						t.Errorf("Accept-Encoding %q: status = %d, want 406", header, rec.Code)
					}
				}
			})
		})
	}
}
