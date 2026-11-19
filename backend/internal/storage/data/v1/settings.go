// Defines settings.json: server secrets, SMTP, Web Push keys, quotas, and rate limits.

package v1

import (
	"errors"
	"fmt"
	"strings"
)

// VAPIDConfig holds the ECDSA P-256 key pair for Web Push (RFC 8030).
// Keys are base64url-encoded. Generated once on first boot if missing.
type VAPIDConfig struct {
	PublicKey  string `json:"vapid_public_key"`
	PrivateKey string `json:"vapid_private_key"`
}

// ServerConfig stores all server-wide settings.
// Loaded from settings.json in the config directory, created with defaults if
// missing.
type ServerConfig struct {
	// JWTSecret is the secret used to sign JWT tokens.
	// Auto-generated if empty on first load.
	JWTSecret []byte `json:"jwt_secret"`

	// SMTP holds email configuration. Empty host disables email features.
	SMTP SMTPConfig `json:"smtp"`

	// VAPID holds the Web Push VAPID key pair. Auto-generated if empty on first load.
	VAPID VAPIDConfig `json:"vapid"`

	// Quotas defines server-wide resource limits.
	Quotas ServerQuotas `json:"quotas"`

	// RateLimits defines rate limiting configuration.
	RateLimits RateLimits `json:"rate_limits"`
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

// SMTPConfig holds the SMTP settings in settings.json.
type SMTPConfig struct {
	Host     string `json:"host"`
	Port     int32  `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
}

// IsZero returns true if the config is completely empty (SMTP disabled).
func (c *SMTPConfig) IsZero() bool {
	return c.Host == "" && c.Port == 0 && c.Username == "" && c.Password == "" && c.From == ""
}

// Validate checks that required fields are set and have valid values.
// An empty config (IsZero) is valid and means SMTP is disabled.
func (c *SMTPConfig) Validate() error {
	if c.IsZero() {
		return nil
	}
	if c.Host == "" {
		return errors.New("host is required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port %d is invalid (must be 1-65535)", c.Port)
	}
	if c.Username == "" {
		return errors.New("username is required")
	}
	if c.Password == "" {
		return errors.New("password is required")
	}
	if c.From == "" {
		return errors.New("from is required")
	}
	if !strings.Contains(c.From, "@") {
		return fmt.Errorf("from %q is not a valid email address", c.From)
	}
	return nil
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

// ServerQuotas defines server-wide resource limits.
// ResourceQuotas fields are shared with org and workspace layers;
// the effective quota is min(server, org, workspace) per field.
type ServerQuotas struct {
	ResourceQuotas

	// MaxRequestBodyBytes limits the size of any single HTTP request body.
	MaxRequestBodyBytes int64 `json:"max_request_body_bytes"`

	// MaxSessionsPerUser limits active sessions per user. A sign-in at the
	// limit revokes the least recently used session.
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

	// NotificationRetentionDays is how many days to keep notifications before GC.
	// 0 means no age-based deletion.
	NotificationRetentionDays int `json:"notification_retention_days"`

	// MaxNotificationsPerUser caps notifications per user. Oldest are deleted when exceeded.
	// 0 means unlimited.
	MaxNotificationsPerUser int `json:"max_notifications_per_user"`
}

// Validate checks that all quota values are valid.
// All ResourceQuotas fields must be positive (server is the ultimate fallback; zero is not allowed).
func (q *ServerQuotas) Validate() error {
	if err := q.ValidatePositive(); err != nil {
		return err
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
	if q.NotificationRetentionDays < 0 {
		return errors.New("notification_retention_days must be non-negative")
	}
	if q.MaxNotificationsPerUser < 0 {
		return errors.New("max_notifications_per_user must be non-negative")
	}
	return nil
}
