// Manages server configuration stored in server_config.json.

package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maruel/mddb/backend/internal/email"
	"github.com/maruel/mddb/backend/internal/utils"
)

// ServerConfig stores all server-wide configuration.
// Loaded from server_config.json, created with defaults if missing.
type ServerConfig struct {
	// JWTSecret is the secret used to sign JWT tokens.
	// Auto-generated if empty on first load.
	JWTSecret string `json:"jwt_secret"`

	// SMTP holds email configuration. Empty host disables email features.
	SMTP email.Config `json:"smtp"`

	// Quotas defines server-wide resource limits.
	Quotas ServerQuotas `json:"quotas"`
}

// ServerQuotas defines server-wide resource limits.
type ServerQuotas struct {
	// MaxRequestBodyBytes limits the size of any single HTTP request body.
	MaxRequestBodyBytes int64 `json:"max_request_body_bytes"`

	// MaxSessionsPerUser limits active sessions per user.
	MaxSessionsPerUser int `json:"max_sessions_per_user"`

	// MaxTablesPerWorkspace limits tables within a single workspace.
	MaxTablesPerWorkspace int `json:"max_tables_per_workspace"`

	// MaxColumnsPerTable limits properties/columns per table.
	MaxColumnsPerTable int `json:"max_columns_per_table"`

	// MaxRowsPerTable limits records/rows per table.
	MaxRowsPerTable int `json:"max_rows_per_table"`

	// MaxOrganizations limits total organizations on the server.
	MaxOrganizations int `json:"max_organizations"`

	// MaxWorkspaces limits total workspaces on the server.
	MaxWorkspaces int `json:"max_workspaces"`

	// MaxUsers limits total users on the server.
	MaxUsers int `json:"max_users"`

	// MaxTotalStorageBytes limits total storage across all workspaces.
	MaxTotalStorageBytes int64 `json:"max_total_storage_bytes"`
}

// Validate checks that all quota values are non-negative.
func (q *ServerQuotas) Validate() error {
	if q.MaxRequestBodyBytes < 0 {
		return errors.New("max_request_body_bytes must be non-negative")
	}
	if q.MaxSessionsPerUser < 0 {
		return errors.New("max_sessions_per_user must be non-negative")
	}
	if q.MaxTablesPerWorkspace < 0 {
		return errors.New("max_tables_per_workspace must be non-negative")
	}
	if q.MaxColumnsPerTable < 0 {
		return errors.New("max_columns_per_table must be non-negative")
	}
	if q.MaxRowsPerTable < 0 {
		return errors.New("max_rows_per_table must be non-negative")
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
		MaxRequestBodyBytes:   10 * 1024 * 1024, // 10 MiB
		MaxSessionsPerUser:    10,               // 10 sessions
		MaxTablesPerWorkspace: 100,              // 100 tables
		MaxColumnsPerTable:    50,               // 50 columns
		MaxRowsPerTable:       1000,             // 1000 rows
		MaxOrganizations:      1000,             // 1000 organizations
		MaxWorkspaces:         10000,            // 10000 workspaces
		MaxUsers:              maxUsers,
		MaxTotalStorageBytes:  100 * 1024 * 1024 * 1024, // 100 GiB
	}
}

// Validate checks that the configuration is valid.
func (c *ServerConfig) Validate() error {
	if c.JWTSecret == "" {
		return errors.New("jwt_secret is required")
	}
	if len(c.JWTSecret) < 32 {
		return errors.New("jwt_secret must be at least 32 characters")
	}
	if err := c.SMTP.Validate(); err != nil {
		return fmt.Errorf("smtp: %w", err)
	}
	if err := c.Quotas.Validate(); err != nil {
		return fmt.Errorf("quotas: %w", err)
	}
	return nil
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
// JWTSecret is left empty and must be set before use.
// SMTP is left empty (disabled) by default.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Quotas: DefaultServerQuotas(),
	}
}

// LoadServerConfig loads configuration from dataDir/server_config.json.
// Creates the file with defaults if it doesn't exist.
// Auto-generates JWTSecret if empty.
func LoadServerConfig(dataDir string) (*ServerConfig, error) {
	path := filepath.Join(dataDir, "server_config.json")

	cfg := DefaultServerConfig()

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
	if cfg.JWTSecret == "" {
		secret, err := utils.GenerateToken(32)
		if err != nil {
			return nil, fmt.Errorf("failed to generate JWT secret: %w", err)
		}
		cfg.JWTSecret = secret
		modified = true
	}

	// Save if we created defaults or generated a secret
	if modified || errors.Is(err, os.ErrNotExist) {
		if err := SaveServerConfig(dataDir, &cfg); err != nil {
			return nil, err
		}
	}

	// Validate the loaded configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid server_config.json: %w", err)
	}

	return &cfg, nil
}

// SaveServerConfig saves configuration to dataDir/server_config.json.
func SaveServerConfig(dataDir string, cfg *ServerConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dataDir, "server_config.json"), data, 0o600); err != nil {
		return fmt.Errorf("failed to write config.json: %w", err)
	}
	return nil
}
