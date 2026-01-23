package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWriteHeaders(t *testing.T) {
	w := httptest.NewRecorder()

	result := Result{
		Allowed:    true,
		Limit:      60,
		Remaining:  45,
		ResetAt:    time.Unix(1706012345, 0),
		RetryAfter: 0,
	}

	WriteHeaders(w, result)

	if got := w.Header().Get("X-RateLimit-Limit"); got != "60" {
		t.Errorf("X-RateLimit-Limit = %s, want 60", got)
	}
	if got := w.Header().Get("X-RateLimit-Remaining"); got != "45" {
		t.Errorf("X-RateLimit-Remaining = %s, want 45", got)
	}
	if got := w.Header().Get("X-RateLimit-Reset"); got != "1706012345" {
		t.Errorf("X-RateLimit-Reset = %s, want 1706012345", got)
	}
	if got := w.Header().Get("Retry-After"); got != "" {
		t.Errorf("Retry-After should not be set for allowed requests, got %s", got)
	}
}

func TestWriteHeaders_RateLimited(t *testing.T) {
	w := httptest.NewRecorder()

	result := Result{
		Allowed:    false,
		Limit:      60,
		Remaining:  0,
		ResetAt:    time.Unix(1706012345, 0),
		RetryAfter: 30 * time.Second,
	}

	WriteHeaders(w, result)

	if got := w.Header().Get("X-RateLimit-Limit"); got != "60" {
		t.Errorf("X-RateLimit-Limit = %s, want 60", got)
	}
	if got := w.Header().Get("X-RateLimit-Remaining"); got != "0" {
		t.Errorf("X-RateLimit-Remaining = %s, want 0", got)
	}
	if got := w.Header().Get("Retry-After"); got != "30" {
		t.Errorf("Retry-After = %s, want 30", got)
	}
}

func TestBuildKey(t *testing.T) {
	tests := []struct {
		scope      Scope
		identifier string
		tierName   string
		want       string
	}{
		{ScopeIP, "192.168.1.1", "auth", "ip:192.168.1.1:auth"},
		{ScopeUser, "user-123", "write", "user:user-123:write"},
		{ScopeUser, "abc", "read", "user:abc:read"},
		{ScopeIP, "10.0.0.1", "read", "ip:10.0.0.1:read"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := BuildKey(tt.scope, tt.identifier, tt.tierName)
			if got != tt.want {
				t.Errorf("BuildKey() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRateLimitResponseWriter(t *testing.T) {
	underlying := httptest.NewRecorder()

	result := Result{
		Allowed:   true,
		Limit:     100,
		Remaining: 99,
		ResetAt:   time.Unix(1706012345, 0),
	}

	rw := NewResponseWriter(underlying, result)

	// Write should inject headers
	if _, err := rw.Write([]byte("test")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if got := underlying.Header().Get("X-RateLimit-Limit"); got != "100" {
		t.Errorf("X-RateLimit-Limit not set after Write, got %s", got)
	}
	if underlying.Body.String() != "test" {
		t.Errorf("Body = %s, want test", underlying.Body.String())
	}
}

func TestRateLimitResponseWriter_WriteHeader(t *testing.T) {
	underlying := httptest.NewRecorder()

	result := Result{
		Allowed:   true,
		Limit:     100,
		Remaining: 99,
		ResetAt:   time.Unix(1706012345, 0),
	}

	rw := NewResponseWriter(underlying, result)

	// WriteHeader should inject rate limit headers first
	rw.WriteHeader(http.StatusOK)

	if got := underlying.Header().Get("X-RateLimit-Limit"); got != "100" {
		t.Errorf("X-RateLimit-Limit not set after WriteHeader, got %s", got)
	}
	if underlying.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", underlying.Code, http.StatusOK)
	}
}

func TestRateLimitResponseWriter_HeadersOnlyOnce(t *testing.T) {
	underlying := httptest.NewRecorder()

	result := Result{
		Allowed:   true,
		Limit:     100,
		Remaining: 99,
		ResetAt:   time.Unix(1706012345, 0),
	}

	rw := NewResponseWriter(underlying, result)

	// Multiple writes should only set headers once
	rw.WriteHeader(http.StatusOK)
	if _, err := rw.Write([]byte("first")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if _, err := rw.Write([]byte("second")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Headers should still be set correctly
	if got := underlying.Header().Get("X-RateLimit-Limit"); got != "100" {
		t.Errorf("X-RateLimit-Limit = %s, want 100", got)
	}
}
