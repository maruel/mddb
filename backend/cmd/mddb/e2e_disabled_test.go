// Verifies a production build rejects the flag, fills no fakes, and keeps quota.

//go:build !e2e

package main

import (
	"flag"
	"io"
	"testing"

	"github.com/maruel/mddb/backend/internal/storage"
)

func TestRegisterFastRateLimitFlag(t *testing.T) {
	fs := flag.NewFlagSet("mddb", flag.ContinueOnError)
	registerFastRateLimitFlag(fs)
	if fastRateLimit {
		t.Error("a production build must not run with fast rate limits")
	}
	// A production binary must reject -fast-rate-limit instead of ignoring it.
	fs.SetOutput(io.Discard)
	if err := fs.Parse([]string{"-fast-rate-limit"}); err == nil {
		t.Error("a production build must reject -fast-rate-limit")
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
