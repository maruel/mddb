// Tests request body decompression and its errors for undecodable or unsupported encodings.

package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maruel/mddb/backend/internal/server/dto"
)

func TestDecompressMiddleware(t *testing.T) {
	echo := decompressMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(w, r.Body); err != nil {
			t.Error(err)
		}
	}))
	serve := func(encoding string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(t.Context(), "POST", "/", bytes.NewReader(body))
		r.Header.Set("Content-Encoding", encoding)
		w := httptest.NewRecorder()
		echo.ServeHTTP(w, r)
		return w
	}
	t.Run("valid", func(t *testing.T) {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write([]byte("payload")); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if w := serve("gzip", buf.Bytes()); w.Code != http.StatusOK || w.Body.String() != "payload" {
			t.Fatalf("got %d %q", w.Code, w.Body)
		}
	})
	t.Run("error", func(t *testing.T) {
		t.Run("invalid gzip", func(t *testing.T) {
			w := serve("gzip", []byte("not gzip"))
			wantErrorResponse(t, w.Code, w.Body.Bytes(), http.StatusBadRequest, dto.ErrorCodeInvalidFormat)
		})
		t.Run("unsupported encoding", func(t *testing.T) {
			w := serve("deflate", []byte("x"))
			wantErrorResponse(t, w.Code, w.Body.Bytes(), http.StatusUnsupportedMediaType, dto.ErrorCodeInvalidFormat)
		})
	})
}
