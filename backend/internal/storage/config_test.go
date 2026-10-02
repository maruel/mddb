// Tests for server settings persistence in the config directory.

package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/email"
)

func TestServerConfig(t *testing.T) {
	t.Parallel()

	t.Run("LoadWritesSettingsFile", func(t *testing.T) {
		t.Parallel()
		// The config directory may not exist yet on a fresh instance.
		cfgDir := filepath.Join(t.TempDir(), "mddb")
		dataDir := t.TempDir()

		cfg, err := LoadServerConfig(cfgDir, false)
		if err != nil {
			t.Fatalf("LoadServerConfig() failed: %v", err)
		}
		if len(cfg.JWTSecret) < 32 {
			t.Errorf("JWTSecret has %d bytes, want at least 32", len(cfg.JWTSecret))
		}
		if cfg.VAPID.PublicKey == "" || cfg.VAPID.PrivateKey == "" {
			t.Error("VAPID key pair was not generated")
		}
		if cfg.Quotas != DefaultServerQuotas(false) {
			t.Errorf("got quotas %+v, want the defaults", cfg.Quotas)
		}
		if cfg.RateLimits != DefaultRateLimits() {
			t.Errorf("got rate limits %+v, want the defaults", cfg.RateLimits)
		}

		fi, err := os.Stat(filepath.Join(cfgDir, "settings.json"))
		if err != nil {
			t.Fatalf("settings.json not written: %v", err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("settings.json mode is %o, want 600", perm)
		}

		// Settings no longer live in the data directory.
		entries, err := os.ReadDir(dataDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			t.Errorf("data directory should stay empty, got: %v", names)
		}
	})

	t.Run("SaveAndReload", func(t *testing.T) {
		t.Parallel()
		cfgDir := t.TempDir()

		cfg, err := LoadServerConfig(cfgDir, false)
		if err != nil {
			t.Fatalf("LoadServerConfig() failed: %v", err)
		}
		cfg.SMTP = email.Config{Host: "smtp.example.com", Port: 587, Username: "mddb", Password: "secret", From: "mddb@example.com"}
		cfg.RateLimits.AuthRatePerMin = 11
		if err := cfg.Save(cfgDir); err != nil {
			t.Fatalf("Save() failed: %v", err)
		}

		// A reload must see the saved settings and keep the generated secrets.
		reloaded, err := LoadServerConfig(cfgDir, false)
		if err != nil {
			t.Fatalf("LoadServerConfig() failed: %v", err)
		}
		if reloaded.SMTP.Host != "smtp.example.com" {
			t.Errorf("got SMTP host %q, want smtp.example.com", reloaded.SMTP.Host)
		}
		if reloaded.RateLimits.AuthRatePerMin != 11 {
			t.Errorf("got auth rate limit %d, want 11", reloaded.RateLimits.AuthRatePerMin)
		}
		if !bytes.Equal(reloaded.JWTSecret, cfg.JWTSecret) {
			t.Error("JWT secret changed on reload")
		}
		if reloaded.VAPID != cfg.VAPID {
			t.Error("VAPID key pair changed on reload")
		}

		fi, err := os.Stat(filepath.Join(cfgDir, "settings.json"))
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("settings.json mode is %o, want 600", perm)
		}
	})
}
