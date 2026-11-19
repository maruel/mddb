// Persisted host session rows.

package v1

import (
	"github.com/maruel/ksid"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// Session represents an active user session.
type Session struct {
	ID          ksid.ID        `json:"id" jsonschema:"description=Unique session identifier"`
	UserID      ksid.ID        `json:"user_id" jsonschema:"description=User who owns this session"`
	TokenHash   string         `json:"token_hash" jsonschema:"description=SHA-256 hash of the JWT token"`
	DeviceInfo  string         `json:"device_info" jsonschema:"description=Parsed User-Agent (browser/OS)"`
	IPAddress   string         `json:"ip_address" jsonschema:"description=Client IP address at login"`
	CountryCode string         `json:"country_code,omitempty" jsonschema:"description=ISO 3166-1 alpha-2 country code at login"`
	Created     storagev1.Time `json:"created" jsonschema:"description=Session creation timestamp"`
	LastUsed    storagev1.Time `json:"last_used" jsonschema:"description=Last activity timestamp"`
	ExpiresAt   storagev1.Time `json:"expires_at" jsonschema:"description=Session expiration timestamp"`
	RevokedAt   storagev1.Time `json:"revoked_at,omitempty" jsonschema:"description=Revocation timestamp if revoked"`
}

// Clone returns a deep copy of the session.
func (s *Session) Clone() *Session {
	c := *s
	return &c
}

// GetID returns the session's ID.
func (s *Session) GetID() ksid.ID {
	return s.ID
}

// Validate checks that the session is valid.
func (s *Session) Validate() error {
	if s.ID.IsZero() {
		return ErrSessionIDRequired
	}
	if s.UserID.IsZero() {
		return ErrSessionUserIDRequired
	}
	if s.TokenHash == "" {
		return ErrSessionTokenHashRequired
	}
	return nil
}
