// Implements a thread-safe token bucket rate limiter.

// Package ratelimit implements token bucket rate limiting for HTTP handlers.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Result contains the outcome of a rate limit check.
type Result struct {
	Allowed    bool
	Limit      int           // requests per window
	Remaining  int           // requests left in current window
	ResetAt    time.Time     // when the bucket will be full again
	RetryAfter time.Duration // how long to wait before retrying (0 if allowed)
}

// Limiter manages rate limit buckets per key using the token bucket algorithm.
type Limiter struct {
	mu      sync.RWMutex
	buckets map[string]*bucket
	rate    rate.Limit
	burst   int
	window  time.Duration
	stop    chan struct{}
}

type bucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewLimiter creates a rate limiter allowing requests tokens per window with burst capacity.
func NewLimiter(requests int, window time.Duration, burst int) *Limiter {
	// Convert requests/window to tokens/second
	tokensPerSecond := float64(requests) / window.Seconds()

	l := &Limiter{
		buckets: make(map[string]*bucket),
		rate:    rate.Limit(tokensPerSecond),
		burst:   burst,
		window:  window,
		stop:    make(chan struct{}),
	}

	// Start cleanup goroutine
	go l.cleanupLoop()

	return l
}

// Allow checks if a request with the given key is allowed.
func (l *Limiter) Allow(key string) Result {
	l.mu.Lock()
	b, exists := l.buckets[key]
	if !exists {
		b = &bucket{
			limiter:  rate.NewLimiter(l.rate, l.burst),
			lastSeen: time.Now(),
		}
		l.buckets[key] = b
	}
	b.lastSeen = time.Now()
	l.mu.Unlock()

	// Check reservation to get timing info
	now := time.Now()
	reservation := b.limiter.ReserveN(now, 1)

	// Calculate result
	allowed := reservation.OK() && reservation.Delay() == 0
	if !allowed && reservation.OK() {
		reservation.Cancel()
	}

	// Calculate remaining tokens (approximate)
	tokens := b.limiter.Tokens()
	remaining := max(int(tokens), 0)

	// Calculate reset time: when bucket will be full
	// Time to refill = (burst - current) / rate
	tokensNeeded := float64(l.burst) - tokens
	refillTime := time.Duration(tokensNeeded/float64(l.rate)) * time.Second
	resetAt := now.Add(refillTime)

	// Calculate retry after for rate limited requests
	var retryAfter time.Duration
	if !allowed {
		// Wait until at least one token is available
		retryAfter = max(time.Duration(1/float64(l.rate))*time.Second, time.Second)
	}

	// Convert rate to requests per window for the Limit field
	limit := int(float64(l.rate) * l.window.Seconds())

	return Result{
		Allowed:    allowed,
		Limit:      limit,
		Remaining:  remaining,
		ResetAt:    resetAt,
		RetryAfter: retryAfter,
	}
}

// cleanupLoop removes stale buckets every 10 minutes.
func (l *Limiter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.cleanup()
		case <-l.stop:
			return
		}
	}
}

// cleanup removes buckets that haven't been used recently and are full.
func (l *Limiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	staleThreshold := time.Now().Add(-10 * time.Minute)
	for key, b := range l.buckets {
		// Remove if not seen recently and bucket is full (inactive)
		if b.lastSeen.Before(staleThreshold) && b.limiter.Tokens() >= float64(l.burst) {
			delete(l.buckets, key)
		}
	}
}

// Close stops the cleanup goroutine.
func (l *Limiter) Close() {
	close(l.stop)
}
