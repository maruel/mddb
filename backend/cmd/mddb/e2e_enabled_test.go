// Verifies the e2e-only flag, the fake OAuth credentials, and the raised quota.

//go:build e2e

package main

import (
	"flag"
	"testing"

	"github.com/maruel/mddb/backend/internal/storage"
)

func TestRegisterFastRateLimitFlag(t *testing.T) {
	fs := flag.NewFlagSet("mddb", flag.ContinueOnError)
	registerFastRateLimitFlag(fs)
	t.Cleanup(func() { fastRateLimit = false })
	if fastRateLimit {
		t.Error("fast rate limits must be off until -fast-rate-limit is given")
	}
	if err := fs.Parse([]string{"-fast-rate-limit"}); err != nil {
		t.Fatal(err)
	}
	if !fastRateLimit {
		t.Error("-fast-rate-limit must enable the fast rate limits")
	}
}

func TestResolveConfigFakeOAuth(t *testing.T) {
	t.Run("empty credentials become fakes", func(t *testing.T) {
		tc := defaultConfig()
		got, err := resolveConfig(&tc, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if !got.TestOAuth {
			t.Error("an e2e build must select the fake OAuth providers")
		}
		want := oauthCredentialsSet{
			Google:    oauthCredentials{ClientID: "test-google-client-id", ClientSecret: "test-google-client-secret"},
			Microsoft: oauthCredentials{ClientID: "test-ms-client-id", ClientSecret: "test-ms-client-secret"},
			GitHub:    oauthCredentials{ClientID: "test-github-client-id", ClientSecret: "test-github-client-secret"},
		}
		if got.OAuth != want {
			t.Errorf("got %+v, want %+v", got.OAuth, want)
		}
	})

	t.Run("configured credentials win", func(t *testing.T) {
		tc := defaultConfig()
		tc.OAuth.Google = tomlOAuthProvider{ClientID: "google-id", ClientSecret: "google-secret"}
		got, err := resolveConfig(&tc, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if got.OAuth.Google != (oauthCredentials{ClientID: "google-id", ClientSecret: "google-secret"}) {
			t.Errorf("configured credentials must win: %+v", got.OAuth.Google)
		}
		if got.OAuth.Microsoft.ClientID != "test-ms-client-id" || got.OAuth.GitHub.ClientID != "test-github-client-id" {
			t.Errorf("got fake credentials %+v", got.OAuth)
		}
	})
}

func TestDefaultServerQuotas(t *testing.T) {
	if got := storage.DefaultServerQuotas(e2eBuild).MaxUsers; got != 200 {
		t.Errorf("got max users %d, want 200", got)
	}
}
