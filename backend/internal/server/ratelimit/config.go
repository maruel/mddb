// Defines rate limit tiers and routing rules.

// Package ratelimit routes mddb requests to configurable rate limit tiers.
package ratelimit

import (
	"strings"
	"time"

	gmratelimit "github.com/gomodedev/gomode/httpserver/ratelimit"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// fastRateMultiplier multiplies every rate limit for an e2e server, whose
// tests would otherwise trip the production limits.
const fastRateMultiplier = 10000

// Scope defines how rate limit keys are determined.
type Scope int

const (
	// ScopeIP uses client IP address as the rate limit key.
	ScopeIP Scope = iota
	// ScopeUser uses authenticated user ID as the rate limit key.
	ScopeUser
)

// tierConfig is one tier's limiter settings.
type tierConfig struct {
	rate  int // requests per minute
	burst int
}

// tierConfigs derives every tier's settings from the stored rates. fastLimits multiplies every
// rate by fastRateMultiplier; it is set only for an e2e server that runs without the production
// limits the suite would trip.
func tierConfigs(limits storagev1.RateLimits, fastLimits bool) (auth, write, readAuth, readUnauth tierConfig) {
	m := 1
	if fastLimits {
		m = fastRateMultiplier
	}
	// Auth allows its whole minute at once; the other tiers allow a sixth of it.
	return tierConfig{limits.AuthRatePerMin * m, max(limits.AuthRatePerMin*m, 1)},
		tierConfig{limits.WriteRatePerMin * m, max(limits.WriteRatePerMin*m/6, 1)},
		tierConfig{limits.ReadAuthRatePerMin * m, max(limits.ReadAuthRatePerMin*m/6, 1)},
		tierConfig{limits.ReadUnauthRatePerMin * m, max(limits.ReadUnauthRatePerMin*m/6, 1)}
}

// Tier names a rate limit tier, how it keys requests, and the limiter that owns its rate.
type Tier struct {
	Name    string
	Scope   Scope
	Limiter *gmratelimit.Limiter
}

// Limiters holds active rate limiters.
type Limiters struct {
	Auth       Tier
	Write      Tier
	ReadAuth   Tier
	ReadUnauth Tier
	fastLimits bool
	startTime  time.Time
}

// NewLimiters creates the tiers from the stored rates; see tierConfigs for fastLimits.
func NewLimiters(limits storagev1.RateLimits, fastLimits bool) *Limiters {
	auth, write, readAuth, readUnauth := tierConfigs(limits, fastLimits)
	return &Limiters{
		Auth:       Tier{Name: "auth", Scope: ScopeIP, Limiter: gmratelimit.New(auth.rate, time.Minute, auth.burst)},
		Write:      Tier{Name: "write", Scope: ScopeUser, Limiter: gmratelimit.New(write.rate, time.Minute, write.burst)},
		ReadAuth:   Tier{Name: "read", Scope: ScopeUser, Limiter: gmratelimit.New(readAuth.rate, time.Minute, readAuth.burst)},
		ReadUnauth: Tier{Name: "read", Scope: ScopeIP, Limiter: gmratelimit.New(readUnauth.rate, time.Minute, readUnauth.burst)},
		fastLimits: fastLimits,
		startTime:  time.Now(),
	}
}

// Counts returns the total request counts per tier.
func (l *Limiters) Counts() (auth, write, readAuth, readUnauth int64) {
	return l.Auth.Limiter.Count(), l.Write.Limiter.Count(), l.ReadAuth.Limiter.Count(), l.ReadUnauth.Limiter.Count()
}

// StartTime returns when the limiters were created (server start time).
func (l *Limiters) StartTime() time.Time {
	return l.startTime
}

// Update applies newly stored rates at runtime, with the multiplier the limiters were created with.
// 0 means unlimited.
func (l *Limiters) Update(limits storagev1.RateLimits) {
	auth, write, readAuth, readUnauth := tierConfigs(limits, l.fastLimits)
	l.Auth.Limiter.Update(auth.rate, time.Minute, auth.burst)
	l.Write.Limiter.Update(write.rate, time.Minute, write.burst)
	l.ReadAuth.Limiter.Update(readAuth.rate, time.Minute, readAuth.burst)
	l.ReadUnauth.Limiter.Update(readUnauth.rate, time.Minute, readUnauth.burst)
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

	// Search, MCP, ephemeral selection, and voice signaling have their own access controls. Voice
	// offers also pass voiceGateway.admit, a per-user and global offer and session limit.
	if method == "POST" && (path == "/api/v1/gomode/mcp" || (strings.HasPrefix(path, "/api/v1/workspaces/") && strings.HasSuffix(path, "/gomode/focus")) || strings.HasSuffix(path, "/search") || strings.HasPrefix(path, "/api/voicegateway/v1/voice/rtc/") || strings.HasPrefix(path, "/api/voicegateway/v2/voice/rtc/")) {
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

	// Browser and bearer login share one per-IP authentication budget.
	if method == "POST" {
		if path == "/api/v1/auth/login" || path == "/api/v1/auth/register" || path == "/api/v1/auth/browser/login" || path == "/api/v1/auth/browser/register" {
			return true
		}
	}

	// GET /api/v1/auth/oauth/*/callback
	if method == "GET" && strings.HasPrefix(path, "/api/v1/auth/oauth/") && strings.HasSuffix(path, "/callback") {
		return true
	}

	return false
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
