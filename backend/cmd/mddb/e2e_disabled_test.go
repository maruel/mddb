// Verifies a production build keeps production limits, no fakes, and the quota.

//go:build !e2e

package main

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/server/ratelimit"
	"github.com/maruel/mddb/backend/internal/storage"
)

func TestProductionRateLimits(t *testing.T) {
	if e2eBuild {
		t.Error("a production build must not select the e2e behavior")
	}
	if got := ratelimit.DefaultConfig(e2eBuild).Auth.Rate; got != 5 {
		t.Errorf("got auth rate %d, want the production 5", got)
	}
}

func TestResolveConfigFakeOAuth(t *testing.T) {
	tc := defaultConfig()
	got, err := resolveConfig(&tc, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got.TestOAuth {
		t.Error("a production build must not select the fake OAuth providers")
	}
	if got.OAuth != (oauthCredentialsSet{}) {
		t.Errorf("a production build must not fill fake credentials: %+v", got.OAuth)
	}
}

func TestDefaultServerQuotas(t *testing.T) {
	if got := storage.DefaultServerQuotas(e2eBuild).MaxUsers; got != 50 {
		t.Errorf("got max users %d, want 50", got)
	}
}
