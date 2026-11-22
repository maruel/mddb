// Tests server settings persistence and quota/rate value contracts.

package storage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

func TestServerConfig(t *testing.T) {
	t.Parallel()
	t.Run("LoadExistingSettings", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cfg, err := LoadServerConfig(dir, false)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]json.RawMessage
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatal(err)
		}
		// Existing settings can contain retired optional fields. Loading them
		// must retain the supported settings and must not overwrite live files.
		document["retired_option"] = json.RawMessage(`{"enabled":true}`)
		data, err = json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "settings.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadServerConfig(dir, false)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(loaded, cfg) {
			t.Fatal("existing settings lost supported values")
		}
		unchanged, err := os.ReadFile(path) //nolint:gosec // G304: isolated test settings file.
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(unchanged, data) {
			t.Fatal("loading existing settings rewrote the file")
		}
	})

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
		cfg.SMTP = storagev1.SMTPConfig{Host: "smtp.example.com", Port: 587, Username: "mddb", Password: "secret", From: "mddb@example.com"}
		cfg.RateLimits.AuthRatePerMin = 11
		if err := SaveServerConfig(cfg, cfgDir); err != nil {
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

func TestServerQuotas(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			q := storagev1.ServerQuotas{ResourceQuotas: DefaultResourceQuotas()}
			if err := q.Validate(); err != nil {
				t.Fatalf("zero non-resource capacities should be unlimited: %v", err)
			}
			q = DefaultServerQuotas(false)
			if err := q.Validate(); err != nil {
				t.Fatal(err)
			}
		})
		t.Run("error", func(t *testing.T) {
			for _, field := range []string{"MaxRequestBodyBytes", "MaxSessionsPerUser", "MaxOrganizations", "MaxWorkspaces", "MaxUsers", "MaxTotalStorageBytes", "MaxEgressBandwidthBps", "NotificationRetentionDays", "MaxNotificationsPerUser"} {
				t.Run(field, func(t *testing.T) {
					q := DefaultServerQuotas(false)
					reflect.ValueOf(&q).Elem().FieldByName(field).SetInt(-1)
					if err := q.Validate(); err == nil {
						t.Fatal("non-resource quota inherited -1")
					}
				})
			}
			q := DefaultServerQuotas(false)
			q.MaxPages = 0
			if err := q.Validate(); err == nil {
				t.Fatal("server resource floor accepted zero")
			}
		})
	})
}

func TestRateLimits(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			for _, r := range []storagev1.RateLimits{{}, DefaultRateLimits()} {
				if err := r.Validate(); err != nil {
					t.Fatal(err)
				}
			}
		})
		t.Run("error", func(t *testing.T) {
			for _, field := range []string{"AuthRatePerMin", "WriteRatePerMin", "ReadAuthRatePerMin", "ReadUnauthRatePerMin"} {
				t.Run(field, func(t *testing.T) {
					r := DefaultRateLimits()
					reflect.ValueOf(&r).Elem().FieldByName(field).SetInt(-1)
					if err := r.Validate(); err == nil {
						t.Fatal("rate inherited -1")
					}
				})
			}
		})
	})
}
