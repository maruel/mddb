// Persisted email verification rows.

package v1

import (
	"github.com/maruel/ksid"
	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// EmailVerification represents a pending email verification.
type EmailVerification struct {
	ID        ksid.ID        `json:"id" jsonschema:"description=Unique verification identifier"`
	UserID    ksid.ID        `json:"user_id" jsonschema:"description=User who requested verification"`
	Email     string         `json:"email" jsonschema:"description=Email address being verified"`
	Token     string         `json:"token" jsonschema:"description=64-char hex verification token"`
	ExpiresAt storagev1.Time `json:"expires_at" jsonschema:"description=Token expiration timestamp (24h from creation)"`
	Created   storagev1.Time `json:"created" jsonschema:"description=Verification creation timestamp"`
}

// Clone returns a deep copy of the email verification.
func (e *EmailVerification) Clone() *EmailVerification {
	c := *e
	return &c
}

// GetID returns the verification's ID.
func (e *EmailVerification) GetID() ksid.ID {
	return e.ID
}

// Validate checks that the email verification is valid.
func (e *EmailVerification) Validate() error {
	if e.ID.IsZero() {
		return ErrVerificationIDRequired
	}
	if e.UserID.IsZero() {
		return ErrVerificationUserIDRequired
	}
	if e.Email == "" {
		return ErrVerificationEmailRequired
	}
	if e.Token == "" {
		return ErrVerificationTokenRequired
	}
	return nil
}
