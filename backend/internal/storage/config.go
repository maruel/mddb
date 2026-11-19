// Manages server settings stored in settings.json in the config directory.

package storage

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// LoadServerConfig loads configuration from cfgDir/settings.json.
// Creates the file with defaults if it doesn't exist.
// Auto-generates JWTSecret if empty.
//
// e2e selects the raised user quota that DefaultServerQuotas returns.
func LoadServerConfig(cfgDir string, e2e bool) (*storagev1.ServerConfig, error) {
	path := filepath.Join(cfgDir, "settings.json")

	cfg := storagev1.ServerConfig{Quotas: DefaultServerQuotas(e2e), RateLimits: DefaultRateLimits()}

	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed from cfgDir, not user input
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read settings.json: %w", err)
		}
		// File doesn't exist, will create with defaults
	} else {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse settings.json: %w", err)
		}
	}

	// Auto-generate JWT secret if missing
	modified := false
	if len(cfg.JWTSecret) == 0 {
		cfg.JWTSecret = make([]byte, 32)
		if _, err := rand.Read(cfg.JWTSecret); err != nil {
			return nil, fmt.Errorf("failed to generate JWT secret: %w", err)
		}
		modified = true
	}

	// Auto-generate VAPID key pair if missing
	if cfg.VAPID.PublicKey == "" || cfg.VAPID.PrivateKey == "" {
		key, err := ecdh.P256().GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate VAPID key pair: %w", err)
		}
		cfg.VAPID.PublicKey = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
		cfg.VAPID.PrivateKey = base64.RawURLEncoding.EncodeToString(key.Bytes())
		modified = true
	}

	// Save if we created defaults or generated a secret
	if modified || errors.Is(err, os.ErrNotExist) {
		if err := SaveServerConfig(&cfg, cfgDir); err != nil {
			return nil, err
		}
	}

	// Validate the loaded configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid settings.json: %w", err)
	}

	return &cfg, nil
}

// SaveServerConfig saves c to cfgDir/settings.json.
//
// The file holds the JWT secret and the VAPID key pair, so it is written mode
// 0600, like the onboarding wizard does for config.toml.
func SaveServerConfig(c *storagev1.ServerConfig, cfgDir string) error {
	if err := c.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	data = append(data, '\n')
	// The config directory holds secrets and may not exist on a fresh instance.
	// Match the mode the onboarding wizard creates it with.
	if err := os.MkdirAll(cfgDir, 0o750); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "settings.json"), data, 0o600); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}
	return nil
}

// DefaultRateLimits returns the default rate limits.
func DefaultRateLimits() storagev1.RateLimits {
	return storagev1.RateLimits{
		AuthRatePerMin:       5,     // 5 req/min for auth
		WriteRatePerMin:      60,    // 60 req/min for writes
		ReadAuthRatePerMin:   30000, // 30k req/min for authenticated reads
		ReadUnauthRatePerMin: 6000,  // 6k req/min for unauthenticated reads
	}
}

// DefaultServerQuotas returns the default server-wide quotas.
//
// e2e raises the user quota so isolated browser tests can register a fresh
// account for each journey.
func DefaultServerQuotas(e2e bool) storagev1.ServerQuotas {
	maxUsers := 50 // 50 users
	if e2e {
		maxUsers = 1000
	}
	return storagev1.ServerQuotas{
		ResourceQuotas:            DefaultResourceQuotas(),
		MaxRequestBodyBytes:       10 * 1024 * 1024, // 10 MiB
		MaxSessionsPerUser:        10,               // 10 sessions
		MaxOrganizations:          1000,             // 1000 organizations
		MaxWorkspaces:             10000,            // 10000 workspaces
		MaxUsers:                  maxUsers,
		MaxTotalStorageBytes:      100 * 1024 * 1024 * 1024, // 100 GiB
		MaxEgressBandwidthBps:     0,                        // unlimited
		NotificationRetentionDays: 90,                       // 90 days
		MaxNotificationsPerUser:   500,                      // 500 per user
	}
}
