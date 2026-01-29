// Package bandwidth provides bandwidth rate limiting for egress traffic.
package bandwidth

import (
	"sync"
	"time"
)

// Limiter implements token bucket rate limiting for bandwidth.
// A limit of 0 means unlimited.
type Limiter struct {
	mu                sync.Mutex
	maxBytesPerSecond int64
	tokens            float64
	lastRefillTime    time.Time
}

// NewLimiter creates a new bandwidth limiter with the given bytes-per-second limit.
func NewLimiter(maxBytesPerSecond int64) *Limiter {
	return &Limiter{
		maxBytesPerSecond: maxBytesPerSecond,
		tokens:            float64(maxBytesPerSecond),
		lastRefillTime:    time.Now(),
	}
}

// Allow blocks until n bytes can be consumed from the rate limit.
// Returns the actual time waited.
func (l *Limiter) Allow(n int64) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.maxBytesPerSecond <= 0 {
		return 0
	}

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(l.lastRefillTime).Seconds()
	l.tokens += elapsed * float64(l.maxBytesPerSecond)
	if l.tokens > float64(l.maxBytesPerSecond) {
		l.tokens = float64(l.maxBytesPerSecond)
	}
	l.lastRefillTime = now

	// If we have enough tokens, consume them immediately
	if l.tokens >= float64(n) {
		l.tokens -= float64(n)
		return 0
	}

	// Not enough tokens; calculate wait time
	deficit := float64(n) - l.tokens
	waitSeconds := deficit / float64(l.maxBytesPerSecond)
	waitDuration := time.Duration(waitSeconds * float64(time.Second))

	// Reset tokens and refill time as if we waited
	l.tokens = 0
	l.lastRefillTime = now.Add(waitDuration)

	return waitDuration
}

// Update changes the bandwidth limit. 0 means unlimited.
func (l *Limiter) Update(maxBytesPerSecond int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maxBytesPerSecond = maxBytesPerSecond
	if maxBytesPerSecond > 0 && l.tokens > float64(maxBytesPerSecond) {
		l.tokens = float64(maxBytesPerSecond)
	}
}
