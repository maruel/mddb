// Manages server configuration stored in server_config.json.

package storage

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maruel/mddb/backend/internal/email"
)

// ServerConfig stores all server-wide configuration.
// Loaded from server_config.json, created with defaults if missing.
type ServerConfig struct {
	// JWTSecret is the secret used to sign JWT tokens.
	// Auto-generated if empty on first load.
	JWTSecret []byte `json:"jwt_secret"`

	// SMTP holds email configuration. Empty host disables email features.
	SMTP email.Config `json:"smtp"`

	// Quotas defines server-wide resource limits.
	Quotas ServerQuotas `json:"quotas"`

	// RateLimits defines rate limiting configuration.
	RateLimits RateLimits `json:"rate_limits"`
}

// RateLimits defines rate limiting configuration (requests per minute).
type RateLimits struct {
	// AuthRatePerMin limits authentication attempts (login, register, OAuth).
	// 0 means unlimited.
	AuthRatePerMin int `json:"auth_rate_per_min"`

	// WriteRatePerMin limits write operations (POST/DELETE).
	// 0 means unlimited.
	WriteRatePerMin int `json:"write_rate_per_min"`

	// ReadAuthRatePerMin limits authenticated read operations.
	// 0 means unlimited.
	ReadAuthRatePerMin int `json:"read_auth_rate_per_min"`

	// ReadUnauthRatePerMin limits unauthenticated read operations.
	// 0 means unlimited.
	ReadUnauthRatePerMin int `json:"read_unauth_rate_per_min"`
}

// Validate checks that rate limit values are non-negative.
func (r *RateLimits) Validate() error {
	if r.AuthRatePerMin < 0 {
		return errors.New("auth_rate_per_min must be non-negative")
	}
	if r.WriteRatePerMin < 0 {
		return errors.New("write_rate_per_min must be non-negative")
	}
	if r.ReadAuthRatePerMin < 0 {
		return errors.New("read_auth_rate_per_min must be non-negative")
	}
	if r.ReadUnauthRatePerMin < 0 {
		return errors.New("read_unauth_rate_per_min must be non-negative")
	}
	return nil
}

// DefaultRateLimits returns the default rate limits.
func DefaultRateLimits() RateLimits {
	return RateLimits{
		AuthRatePerMin:       5,     // 5 req/min for auth
		WriteRatePerMin:      60,    // 60 req/min for writes
		ReadAuthRatePerMin:   30000, // 30k req/min for authenticated reads
		ReadUnauthRatePerMin: 6000,  // 6k req/min for unauthenticated reads
	}
}

// ServerQuotas defines server-wide resource limits.
// ResourceQuotas fields are shared with org and workspace layers;
// the effective quota is min(server, org, workspace) per field.
type ServerQuotas struct {
	ResourceQuotas

	// MaxRequestBodyBytes limits the size of any single HTTP request body.
	MaxRequestBodyBytes int64 `json:"max_request_body_bytes"`

	// MaxSessionsPerUser limits active sessions per user.
	MaxSessionsPerUser int `json:"max_sessions_per_user"`

	// MaxOrganizations limits total organizations on the server.
	MaxOrganizations int `json:"max_organizations"`

	// MaxWorkspaces limits total workspaces on the server.
	MaxWorkspaces int `json:"max_workspaces"`

	// MaxUsers limits total users on the server.
	MaxUsers int `json:"max_users"`

	// MaxTotalStorageBytes limits total storage across all workspaces.
	MaxTotalStorageBytes int64 `json:"max_total_storage_bytes"`

	// MaxEgressBandwidthBps limits total egress bandwidth in bytes per second.
	// 0 means unlimited.
	MaxEgressBandwidthBps int64 `json:"max_egress_bandwidth_bps"`
}

// Validate checks that all quota values are non-negative.
// MaxAssetSizeBytes must be positive (it's the ultimate fallback).
func (q *ServerQuotas) Validate() error {
	if err := q.ResourceQuotas.Validate(); err != nil {
		return err
	}
	if q.MaxAssetSizeBytes <= 0 {
		return errors.New("max_asset_size_bytes must be positive")
	}
	if q.MaxRequestBodyBytes < 0 {
		return errors.New("max_request_body_bytes must be non-negative")
	}
	if q.MaxSessionsPerUser < 0 {
		return errors.New("max_sessions_per_user must be non-negative")
	}
	if q.MaxOrganizations < 0 {
		return errors.New("max_organizations must be non-negative")
	}
	if q.MaxWorkspaces < 0 {
		return errors.New("max_workspaces must be non-negative")
	}
	if q.MaxUsers < 0 {
		return errors.New("max_users must be non-negative")
	}
	if q.MaxTotalStorageBytes < 0 {
		return errors.New("max_total_storage_bytes must be non-negative")
	}
	if q.MaxEgressBandwidthBps < 0 {
		return errors.New("max_egress_bandwidth_bps must be non-negative")
	}
	return nil
}

// DefaultServerQuotas returns the default server-wide quotas.
func DefaultServerQuotas() ServerQuotas {
	maxUsers := 50 // 50 users
	// Increase quota for e2e tests (TEST_OAUTH=1 indicates test mode)
	if os.Getenv("TEST_OAUTH") == "1" {
		maxUsers = 200
	}
	return ServerQuotas{
		ResourceQuotas:        DefaultResourceQuotas(),
		MaxRequestBodyBytes:   10 * 1024 * 1024, // 10 MiB
		MaxSessionsPerUser:    10,               // 10 sessions
		MaxOrganizations:      1000,             // 1000 organizations
		MaxWorkspaces:         10000,            // 10000 workspaces
		MaxUsers:              maxUsers,
		MaxTotalStorageBytes:  100 * 1024 * 1024 * 1024, // 100 GiB
		MaxEgressBandwidthBps: 0,                        // unlimited
	}
}

// Validate checks that the configuration is valid.
func (c *ServerConfig) Validate() error {
	if len(c.JWTSecret) == 0 {
		return errors.New("jwt_secret is required")
	}
	if len(c.JWTSecret) < 32 {
		return errors.New("jwt_secret must be at least 32 bytes")
	}
	if err := c.SMTP.Validate(); err != nil {
		return fmt.Errorf("smtp: %w", err)
	}
	if err := c.Quotas.Validate(); err != nil {
		return fmt.Errorf("quotas: %w", err)
	}
	if err := c.RateLimits.Validate(); err != nil {
		return fmt.Errorf("rate_limits: %w", err)
	}
	return nil
}

// LoadServerConfig loads configuration from dataDir/server_config.json.
// Creates the file with defaults if it doesn't exist.
// Auto-generates JWTSecret if empty.
func LoadServerConfig(dataDir string) (*ServerConfig, error) {
	path := filepath.Join(dataDir, "server_config.json")

	cfg := ServerConfig{Quotas: DefaultServerQuotas(), RateLimits: DefaultRateLimits()}

	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed from dataDir, not user input
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read config.json: %w", err)
		}
		// File doesn't exist, will create with defaults
	} else {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config.json: %w", err)
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

	// Save if we created defaults or generated a secret
	if modified || errors.Is(err, os.ErrNotExist) {
		if err := cfg.Save(dataDir); err != nil {
			return nil, err
		}
	}

	// Validate the loaded configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid server_config.json: %w", err)
	}

	return &cfg, nil
}

// Save saves configuration to dataDir/server_config.json.
func (c *ServerConfig) Save(dataDir string) error {
	if err := c.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dataDir, "server_config.json"), data, 0o600); err != nil {
		return fmt.Errorf("failed to write config.json: %w", err)
	}
	return nil
}
