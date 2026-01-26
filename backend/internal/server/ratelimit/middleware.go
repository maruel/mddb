// Provides HTTP middleware and response writers for rate limiting.

package ratelimit

import (
	"net/http"
	"strconv"
)

// WriteHeaders writes rate limit headers to the response.
// Headers are written on all responses (both success and 429).
func WriteHeaders(w http.ResponseWriter, result Result) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))

	// Retry-After only on 429 responses
	if !result.Allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(result.RetryAfter.Seconds())))
	}
}

// rateLimitResponseWriter wraps http.ResponseWriter to inject rate limit headers
// before any response is written.
type rateLimitResponseWriter struct {
	http.ResponseWriter
	result      Result
	wroteHeader bool
}

// NewResponseWriter creates a response writer that injects rate limit headers.
func NewResponseWriter(w http.ResponseWriter, result Result) *rateLimitResponseWriter {
	return &rateLimitResponseWriter{
		ResponseWriter: w,
		result:         result,
	}
}

// WriteHeader injects rate limit headers before writing the status code.
func (rw *rateLimitResponseWriter) WriteHeader(statusCode int) {
	if !rw.wroteHeader {
		WriteHeaders(rw.ResponseWriter, rw.result)
		rw.wroteHeader = true
	}
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write ensures headers are written before any body content.
func (rw *rateLimitResponseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		WriteHeaders(rw.ResponseWriter, rw.result)
		rw.wroteHeader = true
	}
	return rw.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter for middleware that needs it.
func (rw *rateLimitResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// BuildKey creates a rate limit bucket key from scope, identifier, and tier name.
func BuildKey(scope Scope, identifier, tierName string) string {
	var prefix string
	switch scope {
	case ScopeIP:
		prefix = "ip"
	case ScopeUser:
		prefix = "user"
	default:
		prefix = "unknown"
	}
	return prefix + ":" + identifier + ":" + tierName
}
