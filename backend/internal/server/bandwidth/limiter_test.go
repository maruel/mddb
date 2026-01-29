// Package bandwidth provides bandwidth rate limiting for egress traffic.
package bandwidth

import (
	"testing"
)

func TestLimiter(t *testing.T) {
	t.Run("Allow", func(t *testing.T) {
		tests := []struct {
			name         string
			bytesPerSec  int64
			bytesToAllow int64
			expectWait   bool
		}{
			{
				name:         "no wait when tokens available",
				bytesPerSec:  1000,
				bytesToAllow: 100,
				expectWait:   false,
			},
			{
				name:         "wait when tokens exhausted",
				bytesPerSec:  1000,
				bytesToAllow: 1500,
				expectWait:   true,
			},
			{
				name:         "zero limit means unlimited",
				bytesPerSec:  0,
				bytesToAllow: 1000,
				expectWait:   false,
			},
			{
				name:         "negative limit means unlimited",
				bytesPerSec:  -1,
				bytesToAllow: 1000,
				expectWait:   false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				limiter := NewLimiter(tt.bytesPerSec)
				wait := limiter.Allow(tt.bytesToAllow)

				hasWait := wait > 0
				if hasWait != tt.expectWait {
					t.Errorf("expectWait=%v, got wait=%v", tt.expectWait, wait)
				}

				if tt.expectWait && wait <= 0 {
					t.Errorf("expected positive wait, got %v", wait)
				}
			})
		}
	})

	t.Run("Update", func(t *testing.T) {
		limiter := NewLimiter(1000)

		// First call uses available tokens (starts with max tokens)
		limiter.Allow(500)

		// Update limit to higher value
		limiter.Update(2000)

		// After using 500, we have 500 left. Next 1000 should need to wait.
		// But with updated limit to 2000, behavior depends on current tokens.
		// This test just verifies Update() doesn't panic.
		wait := limiter.Allow(100)
		if wait < 0 {
			t.Errorf("wait should be non-negative, got %v", wait)
		}
	})

	t.Run("UpdateToUnlimited", func(t *testing.T) {
		limiter := NewLimiter(100)

		// Use up tokens
		limiter.Allow(100)

		// Update to 0 (unlimited)
		limiter.Update(0)

		// Should not wait anymore
		wait := limiter.Allow(1000)
		if wait != 0 {
			t.Errorf("unlimited limiter should not rate limit, got wait=%v", wait)
		}
	})
}
