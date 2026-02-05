// Provides a fake OAuth login handler for TEST_OAUTH=1 mode.
// Bypasses real OAuth providers and logs in with deterministic fake accounts.

package handlers

import (
	"log/slog"
	"net/http"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// TestOAuthHandler bypasses real OAuth providers and logs in with fake accounts.
type TestOAuthHandler struct {
	svc       *Services
	cfg       *Config
	providers map[identity.OAuthProvider]bool
}

// NewTestOAuthHandler creates a handler that fakes OAuth login for testing.
func NewTestOAuthHandler(svc *Services, cfg *Config, providers []identity.OAuthProvider) *TestOAuthHandler {
	m := make(map[identity.OAuthProvider]bool, len(providers))
	for _, p := range providers {
		m[p] = true
	}
	return &TestOAuthHandler{svc: svc, cfg: cfg, providers: m}
}

// LoginRedirect bypasses real OAuth and immediately logs in with a fake account.
func (h *TestOAuthHandler) LoginRedirect(w http.ResponseWriter, r *http.Request) {
	provider := identity.OAuthProvider(r.PathValue("provider"))
	if !h.providers[provider] {
		writeErrorResponse(w, dto.InvalidProvider())
		return
	}
	slog.InfoContext(r.Context(), "TEST_OAUTH: bypassing real OAuth, using fake account", "provider", provider)
	finishOAuthLogin(h.svc, h.cfg, w, r, provider, testUserInfo(provider))
}

// testUserInfo returns deterministic fake user info for test mode.
func testUserInfo(provider identity.OAuthProvider) oauthUserInfo {
	switch provider {
	case identity.OAuthProviderGoogle:
		return oauthUserInfo{ID: "test-google-id", Email: "test-google@example.com", Name: "Google Test User"}
	case identity.OAuthProviderMicrosoft:
		return oauthUserInfo{ID: "test-ms-id", Email: "test-microsoft@example.com", Name: "Microsoft Test User"}
	case identity.OAuthProviderGitHub:
		return oauthUserInfo{ID: "test-github-id", Email: "test-github@example.com", Name: "GitHub Test User"}
	default:
		return oauthUserInfo{}
	}
}
