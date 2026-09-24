// Tests cache headers and SPA fallback for precompressed embedded frontend assets.

package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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
