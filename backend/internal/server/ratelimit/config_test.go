package ratelimit

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	defer cfg.Close()

	// Verify scopes
	if cfg.Auth.Scope != ScopeIP {
		t.Error("Auth tier should have IP scope")
	}
	if cfg.Write.Scope != ScopeUser {
		t.Error("Write tier should have User scope")
	}
	if cfg.ReadAuth.Scope != ScopeUser {
		t.Error("ReadAuth tier should have User scope")
	}
	if cfg.ReadUnauth.Scope != ScopeIP {
		t.Error("ReadUnauth tier should have IP scope")
	}

	// Verify limiters are initialized
	if cfg.Auth.Limiter == nil {
		t.Error("Auth limiter should not be nil")
	}
	if cfg.Write.Limiter == nil {
		t.Error("Write limiter should not be nil")
	}
	if cfg.ReadAuth.Limiter == nil {
		t.Error("ReadAuth limiter should not be nil")
	}
	if cfg.ReadUnauth.Limiter == nil {
		t.Error("ReadUnauth limiter should not be nil")
	}
}

func TestConfig_MatchUnauth(t *testing.T) {
	cfg := DefaultConfig()
	defer cfg.Close()

	tests := []struct {
		method   string
		path     string
		wantTier string
	}{
		{"GET", "/api/health", ""},                         // No rate limit for health check
		{"POST", "/api/auth/login", "auth"},                // Auth tier
		{"POST", "/api/auth/register", "auth"},             // Auth tier
		{"GET", "/api/auth/oauth/google/callback", "auth"}, // Auth tier (OAuth callback)
		{"GET", "/api/something", "read"},                  // Unauth read tier
		{"GET", "/api/pages/123", "read"},                  // Unauth read tier
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			tier := cfg.MatchUnauth(tt.method, tt.path)
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
}

func TestConfig_MatchAuth(t *testing.T) {
	cfg := DefaultConfig()
	defer cfg.Close()

	tests := []struct {
		method   string
		path     string
		wantTier string
	}{
		{"GET", "/api/health", ""},            // No rate limit for health check
		{"GET", "/api/pages", "read"},         // Read tier
		{"GET", "/api/users", "read"},         // Read tier
		{"POST", "/api/pages", "write"},       // Write tier
		{"POST", "/api/tables", "write"},      // Write tier
		{"DELETE", "/api/pages/123", "write"}, // Write tier (DELETE)
		{"POST", "/api/search", "read"},       // Search is a read operation
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			tier := cfg.MatchAuth(tt.method, tt.path)
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
}

func TestIsAuthEndpoint(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{"POST", "/api/auth/login", true},
		{"POST", "/api/auth/register", true},
		{"GET", "/api/auth/oauth/google/callback", true},
		{"GET", "/api/auth/oauth/microsoft/callback", true},
		{"GET", "/api/auth/oauth/github/callback", true},
		{"GET", "/api/auth/me", false},
		{"POST", "/api/pages", false},
		{"GET", "/api/pages", false},
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
