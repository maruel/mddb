package ratelimit

import (
	"testing"
	"time"
)

func TestNewLimiter(t *testing.T) {
	l := NewLimiter(60, time.Minute, 10)
	defer l.Close()

	if l == nil {
		t.Fatal("NewLimiter returned nil")
	}

	if l.burst != 10 {
		t.Errorf("expected burst=10, got %d", l.burst)
	}
}

func TestLimiter_Allow(t *testing.T) {
	// 5 requests per minute, burst of 5
	l := NewLimiter(5, time.Minute, 5)
	defer l.Close()

	key := "test:key"

	// First 5 requests should be allowed (within burst)
	for i := range 5 {
		result := l.Allow(key)
		if !result.Allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
		if result.Limit != 5 {
			t.Errorf("expected Limit=5, got %d", result.Limit)
		}
	}

	// 6th request should be rate limited
	result := l.Allow(key)
	if result.Allowed {
		t.Error("6th request should be rate limited")
	}
	if result.RetryAfter < time.Second {
		t.Errorf("expected RetryAfter >= 1s, got %v", result.RetryAfter)
	}
}

func TestLimiter_DifferentKeys(t *testing.T) {
	l := NewLimiter(5, time.Minute, 5)
	defer l.Close()

	// Exhaust limit for key1
	for range 5 {
		l.Allow("key1")
	}
	result := l.Allow("key1")
	if result.Allowed {
		t.Error("key1 should be rate limited")
	}

	// key2 should still have full quota
	for range 5 {
		result := l.Allow("key2")
		if !result.Allowed {
			t.Error("key2 should not be rate limited")
		}
	}
}

func TestLimiter_Result(t *testing.T) {
	l := NewLimiter(10, time.Minute, 10)
	defer l.Close()

	key := "test:result"
	result := l.Allow(key)

	if !result.Allowed {
		t.Error("first request should be allowed")
	}
	if result.Limit != 10 {
		t.Errorf("expected Limit=10, got %d", result.Limit)
	}
	if result.Remaining < 0 || result.Remaining > 10 {
		t.Errorf("Remaining out of range: %d", result.Remaining)
	}
	if result.ResetAt.IsZero() {
		t.Error("ResetAt should not be zero")
	}
	if result.RetryAfter != 0 {
		t.Errorf("RetryAfter should be 0 for allowed requests, got %v", result.RetryAfter)
	}
}

func TestResult_RateLimited(t *testing.T) {
	l := NewLimiter(1, time.Minute, 1)
	defer l.Close()

	key := "test:limited"
	l.Allow(key) // Exhaust the single token

	result := l.Allow(key)
	if result.Allowed {
		t.Error("should be rate limited")
	}
	if result.Remaining != 0 {
		t.Errorf("expected Remaining=0, got %d", result.Remaining)
	}
	if result.RetryAfter == 0 {
		t.Error("RetryAfter should be set for rate limited requests")
	}
}
