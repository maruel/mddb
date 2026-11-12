// Tests request body decoding errors and benchmarks the body limit for read-only multipart transports.

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maruel/mddb/backend/internal/server/dto"
)

func TestReadAndDecodeBody(t *testing.T) {
	e := setupTestEnv(t)
	for name, body := range map[string]string{"malformed JSON": "{", "unknown field": `{"email":"a@example.com","password":"x","extra":1}`} {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, e.server.URL+"/api/v1/auth/login", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := e.server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(resp.Body)
			if err := errors.Join(err, resp.Body.Close()); err != nil {
				t.Fatal(err)
			}
			wantErrorResponse(t, resp.StatusCode, data, http.StatusBadRequest, dto.ErrorCodeInvalidFormat)
		})
	}
}

// wantErrorResponse checks a response's status and decoded error code.
func wantErrorResponse(t *testing.T, gotStatus int, body []byte, status int, code dto.ErrorCode) {
	var got dto.ErrorResponse
	if err := json.Unmarshal(body, &got); err != nil || gotStatus != status || got.Error.Code != code {
		t.Fatalf("got %d %s (%v), want %d %q", gotStatus, body, err, status, code)
	}
}

func BenchmarkReadOnlyMultipartBodyLimit(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 1024)
	for _, tc := range []struct {
		name    string
		bounded bool
	}{
		{name: "unbounded"},
		{name: "bounded", bounded: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			w := httptest.NewRecorder()
			for b.Loop() {
				r := io.NopCloser(bytes.NewReader(body))
				if tc.bounded {
					r = http.MaxBytesReader(w, r, 10*1024*1024)
				}
				if _, err := io.Copy(io.Discard, r); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
