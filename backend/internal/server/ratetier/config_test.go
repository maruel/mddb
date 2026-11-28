// Tests rate limit tier configuration, request routing, and bucket keys.

package ratetier

import (
	"testing"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

func TestNewLimiters(t *testing.T) {
	limits := storagev1.RateLimits{AuthRatePerMin: 1, WriteRatePerMin: 2, ReadAuthRatePerMin: 3, ReadUnauthRatePerMin: 4}
	rates := func(l *Limiters) [4]int {
		return [4]int{l.Auth.Limiter.Allow("k").Limit, l.Write.Limiter.Allow("k").Limit, l.ReadAuth.Limiter.Allow("k").Limit, l.ReadUnauth.Limiter.Allow("k").Limit}
	}
	t.Run("production limits keep the stored rates and scope each tier", func(t *testing.T) {
		l := NewLimiters(limits, false)
		if got := rates(l); got != [4]int{1, 2, 3, 4} {
			t.Errorf("got rates %v", got)
		}
		if l.Auth.Scope != ScopeIP || l.Write.Scope != ScopeUser || l.ReadAuth.Scope != ScopeUser || l.ReadUnauth.Scope != ScopeIP {
			t.Errorf("got scopes %v, %v, %v, %v", l.Auth.Scope, l.Write.Scope, l.ReadAuth.Scope, l.ReadUnauth.Scope)
		}
	})
	t.Run("an update keeps the e2e multiplier", func(t *testing.T) {
		l := NewLimiters(limits, true)
		if got := rates(l); got != [4]int{10000, 20000, 30000, 40000} {
			t.Errorf("got rates %v", got)
		}
		l.Update(storagev1.RateLimits{AuthRatePerMin: 5, WriteRatePerMin: 6, ReadAuthRatePerMin: 7, ReadUnauthRatePerMin: 8})
		if got := rates(l); got != [4]int{50000, 60000, 70000, 80000} {
			t.Errorf("after Update got rates %v", got)
		}
	})
}

func TestLimiters(t *testing.T) {
	limits := storagev1.RateLimits{AuthRatePerMin: 5, WriteRatePerMin: 60, ReadAuthRatePerMin: 30000, ReadUnauthRatePerMin: 6000}
	t.Run("MatchUnauth", func(t *testing.T) {
		limiters := NewLimiters(limits, false)

		tests := []struct {
			method   string
			path     string
			wantTier string
		}{
			{"GET", "/api/v1/health", ""},             // No rate limit for health check
			{"POST", "/api/v1/auth/login", "auth"},    // Auth tier
			{"POST", "/api/v1/auth/register", "auth"}, // Auth tier
			{"POST", "/api/v1/auth/browser/login", "auth"},
			{"POST", "/api/v1/auth/browser/register", "auth"},
			{"GET", "/api/v1/auth/oauth/google/callback", "auth"}, // Auth tier (OAuth callback)
			{"GET", "/api/v1/something", "read"},                  // Unauth read tier
			{"GET", "/api/v1/pages/123", "read"},                  // Unauth read tier
		}

		for _, tt := range tests {
			t.Run(tt.method+" "+tt.path, func(t *testing.T) {
				tier := limiters.MatchUnauth(tt.method, tt.path)
				if tt.wantTier == "" {
					if tier != nil {
						t.Errorf("expected nil tier, got %s", tier.Name)
					}
				} else {
					if tier == nil {
						t.Errorf("expected tier %s, got nil", tt.wantTier)
					} else if tier.Name != tt.wantTier {
						t.Errorf("expected tier %s, got %s", tt.wantTier, tier.Name)
					}
				}
			})
		}
	})

	t.Run("MatchAuth", func(t *testing.T) {
		limiters := NewLimiters(limits, false)

		tests := []struct {
			method   string
			path     string
			wantTier string
		}{
			{"GET", "/api/v1/health", ""},                            // No rate limit for health check
			{"GET", "/api/v1/pages", "read"},                         // Read tier
			{"GET", "/api/v1/users", "read"},                         // Read tier
			{"POST", "/api/v1/pages", "write"},                       // Write tier
			{"POST", "/api/v1/tables", "write"},                      // Write tier
			{"DELETE", "/api/v1/pages/123", "write"},                 // Write tier (DELETE)
			{"POST", "/api/v1/search", "read"},                       // Search is a read operation
			{"POST", "/api/v1/gomode/mcp", "read"},                   // MCP read transport
			{"POST", "/api/v1/workspaces/123/gomode/focus", "read"},  // Ephemeral conversation selection
			{"POST", "/api/voicegateway/v1/voice/rtc/offer", "read"}, // Voice offer has a dedicated limiter
			{"POST", "/api/voicegateway/v2/voice/rtc/offer", "read"},
		}

		for _, tt := range tests {
			t.Run(tt.method+" "+tt.path, func(t *testing.T) {
				tier := limiters.MatchAuth(tt.method, tt.path)
				if tt.wantTier == "" {
					if tier != nil {
						t.Errorf("expected nil tier, got %s", tier.Name)
					}
				} else {
					if tier == nil {
						t.Errorf("expected tier %s, got nil", tt.wantTier)
					} else if tier.Name != tt.wantTier {
						t.Errorf("expected tier %s, got %s", tt.wantTier, tier.Name)
					}
				}
			})
		}
	})
}

func TestIsAuthEndpoint(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{"POST", "/api/v1/auth/login", true},
		{"POST", "/api/v1/auth/register", true},
		{"GET", "/api/v1/auth/oauth/google/callback", true},
		{"GET", "/api/v1/auth/oauth/microsoft/callback", true},
		{"GET", "/api/v1/auth/oauth/github/callback", true},
		{"GET", "/api/v1/auth/me", false},
		{"POST", "/api/v1/pages", false},
		{"GET", "/api/v1/pages", false},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			got := isAuthEndpoint(tt.method, tt.path)
			if got != tt.want {
				t.Errorf("isAuthEndpoint(%s, %s) = %v, want %v", tt.method, tt.path, got, tt.want)
			}
		})
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
