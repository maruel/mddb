// Benchmarks the body-limit overhead for read-only multipart transports.

package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
