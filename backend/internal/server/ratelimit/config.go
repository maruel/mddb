// Defines rate limit tiers and routing rules.

package ratelimit

import (
	"os"
	"strings"
	"time"
)

// Scope defines how rate limit keys are determined.
type Scope int

const (
	// ScopeIP uses client IP address as the rate limit key.
	ScopeIP Scope = iota
	// ScopeUser uses authenticated user ID as the rate limit key.
	ScopeUser
)

// TierConfig holds settings for a rate limit tier (pure data).
type TierConfig struct {
	Name   string
	Rate   int // requests per window
	Window time.Duration
	Burst  int
	Scope  Scope
}

// Config holds rate limit settings (pure data).
type Config struct {
	Auth       TierConfig
	Write      TierConfig
	ReadAuth   TierConfig // authenticated read
	ReadUnauth TierConfig // unauthenticated read
}

// ConfigFromStorage creates a rate limit Config from storage.RateLimits values.
func ConfigFromStorage(authRate, writeRate, readAuthRate, readUnauthRate int) *Config {
	m := 1 // multiplier
	if os.Getenv("TEST_FAST_RATE_LIMIT") == "1" {
		m = 10000
	}
	return &Config{
		Auth:       TierConfig{Name: "auth", Rate: authRate * m, Window: time.Minute, Burst: max(authRate*m, 1), Scope: ScopeIP},
		Write:      TierConfig{Name: "write", Rate: writeRate * m, Window: time.Minute, Burst: max(writeRate*m/6, 1), Scope: ScopeUser},
		ReadAuth:   TierConfig{Name: "read", Rate: readAuthRate * m, Window: time.Minute, Burst: max(readAuthRate*m/6, 1), Scope: ScopeUser},
		ReadUnauth: TierConfig{Name: "read", Rate: readUnauthRate * m, Window: time.Minute, Burst: max(readUnauthRate*m/6, 1), Scope: ScopeIP},
	}
}

// DefaultConfig returns default rate limit settings per the design doc:
//   - Auth: 5 req/min, IP scope
//   - Write: 60 req/min, User scope
//   - Read (auth): 30,000 req/min, User scope
//   - Read (unauth): 6,000 req/min, IP scope.
//
// Set TEST_FAST_RATE_LIMIT=1 to increase rate limits 10000x (for e2e tests).
func DefaultConfig() *Config {
	m := 1 // multiplier
	if os.Getenv("TEST_FAST_RATE_LIMIT") == "1" {
		m = 10000
	}
	return &Config{
		Auth:       TierConfig{Name: "auth", Rate: 5 * m, Window: time.Minute, Burst: 5 * m, Scope: ScopeIP},
		Write:      TierConfig{Name: "write", Rate: 60 * m, Window: time.Minute, Burst: 10 * m, Scope: ScopeUser},
		ReadAuth:   TierConfig{Name: "read", Rate: 30000 * m, Window: time.Minute, Burst: 5000 * m, Scope: ScopeUser},
		ReadUnauth: TierConfig{Name: "read", Rate: 6000 * m, Window: time.Minute, Burst: 1000 * m, Scope: ScopeIP},
	}
}

// Tier is a live rate limiter with its config.
type Tier struct {
	TierConfig
	Limiter *Limiter
}

// Limiters holds active rate limiters.
type Limiters struct {
	Auth       Tier
	Write      Tier
	ReadAuth   Tier
	ReadUnauth Tier
}

// NewLimiters creates limiters from config.
func NewLimiters(cfg *Config) *Limiters {
	return &Limiters{
		Auth:       Tier{TierConfig: cfg.Auth, Limiter: NewLimiter(cfg.Auth.Rate, cfg.Auth.Window, cfg.Auth.Burst)},
		Write:      Tier{TierConfig: cfg.Write, Limiter: NewLimiter(cfg.Write.Rate, cfg.Write.Window, cfg.Write.Burst)},
		ReadAuth:   Tier{TierConfig: cfg.ReadAuth, Limiter: NewLimiter(cfg.ReadAuth.Rate, cfg.ReadAuth.Window, cfg.ReadAuth.Burst)},
		ReadUnauth: Tier{TierConfig: cfg.ReadUnauth, Limiter: NewLimiter(cfg.ReadUnauth.Rate, cfg.ReadUnauth.Window, cfg.ReadUnauth.Burst)},
	}
}

// Close stops all limiter cleanup goroutines.
func (l *Limiters) Close() {
	l.Auth.Limiter.Close()
	l.Write.Limiter.Close()
	l.ReadAuth.Limiter.Close()
	l.ReadUnauth.Limiter.Close()
}

// Update changes the rate limits at runtime. 0 means unlimited.
func (l *Limiters) Update(authRate, writeRate, readAuthRate, readUnauthRate int) {
	window := time.Minute

	// Update Auth tier
	l.Auth.Rate = authRate
	l.Auth.Burst = max(authRate, 1)
	l.Auth.Limiter.Update(authRate, window, l.Auth.Burst)

	// Update Write tier
	l.Write.Rate = writeRate
	l.Write.Burst = max(writeRate/6, 1) // burst = rate/6 like default
	l.Write.Limiter.Update(writeRate, window, l.Write.Burst)

	// Update ReadAuth tier
	l.ReadAuth.Rate = readAuthRate
	l.ReadAuth.Burst = max(readAuthRate/6, 1)
	l.ReadAuth.Limiter.Update(readAuthRate, window, l.ReadAuth.Burst)

	// Update ReadUnauth tier
	l.ReadUnauth.Rate = readUnauthRate
	l.ReadUnauth.Burst = max(readUnauthRate/6, 1)
	l.ReadUnauth.Limiter.Update(readUnauthRate, window, l.ReadUnauth.Burst)
}

// MatchUnauth returns the tier for unauthenticated requests.
// Returns nil for paths that should not be rate limited.
func (l *Limiters) MatchUnauth(method, path string) *Tier {
	// Skip health check
	if path == "/api/v1/health" {
		return nil
	}

	// Auth endpoints (login, register, OAuth callbacks)
	if isAuthEndpoint(method, path) {
		return &l.Auth
	}

	// All other unauthenticated GETs
	if method == "GET" {
		return &l.ReadUnauth
	}

	return nil
}

// MatchAuth returns the tier for authenticated requests.
// Returns nil for paths that should not be rate limited.
func (l *Limiters) MatchAuth(method, path string) *Tier {
	// Skip health check
	if path == "/api/v1/health" {
		return nil
	}

	// Search is a read operation even though it uses POST
	if method == "POST" && strings.HasSuffix(path, "/search") {
		return &l.ReadAuth
	}

	// Write operations: POST and DELETE
	if method == "POST" {
		return &l.Write
	}

	// DELETE operations
	if method == "DELETE" {
		return &l.Write
	}

	// Read operations
	if method == "GET" {
		return &l.ReadAuth
	}

	return nil
}

// isAuthEndpoint checks if the path is an authentication endpoint.
func isAuthEndpoint(method, path string) bool {
	if method != "POST" && method != "GET" {
		return false
	}

	// POST /api/v1/auth/login or /api/v1/auth/register
	if method == "POST" {
		if path == "/api/v1/auth/login" || path == "/api/v1/auth/register" {
			return true
		}
	}

	// GET /api/v1/auth/oauth/*/callback
	if method == "GET" && strings.HasPrefix(path, "/api/v1/auth/oauth/") && strings.HasSuffix(path, "/callback") {
		return true
	}

	return false
}
