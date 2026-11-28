// Tests rate limit tier configuration, request routing, and bucket keys.

package ratelimit

import (
	"testing"
)

func TestConfigFromStorage(t *testing.T) {
	t.Run("production limits keep the stored rates and scope each tier", func(t *testing.T) {
		cfg := ConfigFromStorage(5, 60, 30000, 6000, false)
		if cfg.Auth.Rate != 5 || cfg.Write.Rate != 60 || cfg.ReadAuth.Rate != 30000 || cfg.ReadUnauth.Rate != 6000 {
			t.Errorf("got rates %d, %d, %d, %d", cfg.Auth.Rate, cfg.Write.Rate, cfg.ReadAuth.Rate, cfg.ReadUnauth.Rate)
		}
		if cfg.Auth.Scope != ScopeIP || cfg.Write.Scope != ScopeUser || cfg.ReadAuth.Scope != ScopeUser || cfg.ReadUnauth.Scope != ScopeIP {
			t.Errorf("got scopes %v, %v, %v, %v", cfg.Auth.Scope, cfg.Write.Scope, cfg.ReadAuth.Scope, cfg.ReadUnauth.Scope)
		}
	})

	t.Run("fast limits scale the stored rates 10000x", func(t *testing.T) {
		cfg := ConfigFromStorage(1, 2, 3, 4, true)
		if cfg.Auth.Rate != 10000 || cfg.Write.Rate != 20000 || cfg.ReadAuth.Rate != 30000 || cfg.ReadUnauth.Rate != 40000 {
			t.Errorf("got rates %d, %d, %d, %d", cfg.Auth.Rate, cfg.Write.Rate, cfg.ReadAuth.Rate, cfg.ReadUnauth.Rate)
		}
	})
}

func TestLimiters(t *testing.T) {
	t.Run("NewLimiters", func(t *testing.T) {
		cfg := ConfigFromStorage(5, 60, 30000, 6000, false)
		limiters := NewLimiters(cfg)

		// Verify limiters are initialized
		if limiters.Auth.Limiter == nil {
			t.Error("Auth limiter should not be nil")
		}
		if limiters.Write.Limiter == nil {
			t.Error("Write limiter should not be nil")
		}
		if limiters.ReadAuth.Limiter == nil {
			t.Error("ReadAuth limiter should not be nil")
		}
		if limiters.ReadUnauth.Limiter == nil {
			t.Error("ReadUnauth limiter should not be nil")
		}

		// Verify tier config is preserved
		if limiters.Auth.Scope != ScopeIP {
			t.Error("Auth tier should have IP scope")
		}
		if limiters.Write.Scope != ScopeUser {
			t.Error("Write tier should have User scope")
		}
	})

	t.Run("MatchUnauth", func(t *testing.T) {
		limiters := NewLimiters(ConfigFromStorage(5, 60, 30000, 6000, false))

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
		limiters := NewLimiters(ConfigFromStorage(5, 60, 30000, 6000, false))

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
