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

// Tier defines a rate limit tier with its limiter and scope.
type Tier struct {
	Name    string
	Limiter *Limiter
	Scope   Scope
}

// Config holds rate limiters for different tiers.
type Config struct {
	Auth       Tier
	Write      Tier
	ReadAuth   Tier // authenticated read
	ReadUnauth Tier // unauthenticated read
}

// DefaultConfig creates a Config with default rate limits per the design doc:
//   - Auth: 5 req/min, IP scope
//   - Write: 60 req/min, User scope
//   - Read (auth): 30,000 req/min, User scope
//   - Read (unauth): 6,000 req/min, IP scope.
//
// In test mode (TEST_OAUTH=1), rate limits are increased 1000x to effectively disable them.
func DefaultConfig() *Config {
	multiplier := 1
	if os.Getenv("TEST_OAUTH") == "1" {
		multiplier = 1000
	}
	return &Config{
		Auth: Tier{
			Name:    "auth",
			Limiter: NewLimiter(5*multiplier, time.Minute, 5*multiplier),
			Scope:   ScopeIP,
		},
		Write: Tier{
			Name:    "write",
			Limiter: NewLimiter(60*multiplier, time.Minute, 10*multiplier),
			Scope:   ScopeUser,
		},
		ReadAuth: Tier{
			Name:    "read",
			Limiter: NewLimiter(30000*multiplier, time.Minute, 5000*multiplier),
			Scope:   ScopeUser,
		},
		ReadUnauth: Tier{
			Name:    "read",
			Limiter: NewLimiter(6000*multiplier, time.Minute, 1000*multiplier),
			Scope:   ScopeIP,
		},
	}
}

// MatchUnauth returns the tier for unauthenticated requests.
// Returns nil for paths that should not be rate limited.
func (c *Config) MatchUnauth(method, path string) *Tier {
	// Skip health check
	if path == "/api/health" {
		return nil
	}

	// Auth endpoints (login, register, OAuth callbacks)
	if isAuthEndpoint(method, path) {
		return &c.Auth
	}

	// All other unauthenticated GETs
	if method == "GET" {
		return &c.ReadUnauth
	}

	return nil
}

// MatchAuth returns the tier for authenticated requests.
// Returns nil for paths that should not be rate limited.
func (c *Config) MatchAuth(method, path string) *Tier {
	// Skip health check
	if path == "/api/health" {
		return nil
	}

	// Search is a read operation even though it uses POST
	if method == "POST" && strings.HasSuffix(path, "/search") {
		return &c.ReadAuth
	}

	// Write operations: POST and DELETE
	if method == "POST" {
		return &c.Write
	}

	// DELETE operations
	if method == "DELETE" {
		return &c.Write
	}

	// Read operations
	if method == "GET" {
		return &c.ReadAuth
	}

	return nil
}

// Close stops all limiter cleanup goroutines.
func (c *Config) Close() {
	c.Auth.Limiter.Close()
	c.Write.Limiter.Close()
	c.ReadAuth.Limiter.Close()
	c.ReadUnauth.Limiter.Close()
}

// isAuthEndpoint checks if the path is an authentication endpoint.
func isAuthEndpoint(method, path string) bool {
	if method != "POST" && method != "GET" {
		return false
	}

	// POST /api/auth/login or /api/auth/register
	if method == "POST" {
		if path == "/api/auth/login" || path == "/api/auth/register" {
			return true
		}
	}

	// GET /api/auth/oauth/*/callback
	if method == "GET" && strings.HasPrefix(path, "/api/auth/oauth/") && strings.HasSuffix(path, "/callback") {
		return true
	}

	return false
}
