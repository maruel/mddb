// Manages email verification tokens for magic link authentication.

package identity

import (
	"errors"
	"iter"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/ksid"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/utils"
)

// EmailVerification represents a pending email verification.
type EmailVerification struct {
	ID        ksid.ID      `json:"id" jsonschema:"description=Unique verification identifier"`
	UserID    ksid.ID      `json:"user_id" jsonschema:"description=User who requested verification"`
	Email     string       `json:"email" jsonschema:"description=Email address being verified"`
	Token     string       `json:"token" jsonschema:"description=64-char hex verification token"`
	ExpiresAt storage.Time `json:"expires_at" jsonschema:"description=Token expiration timestamp (24h from creation)"`
	Created   storage.Time `json:"created" jsonschema:"description=Verification creation timestamp"`
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
		return errVerificationIDRequired
	}
	if e.UserID.IsZero() {
		return errVerificationUserIDRequired
	}
	if e.Email == "" {
		return errVerificationEmailRequired
	}
	if e.Token == "" {
		return errVerificationTokenRequired
	}
	return nil
}

// EmailVerificationService handles email verification token management.
type EmailVerificationService struct {
	table    *jsonldb.Table[*EmailVerification]
	byToken  *jsonldb.UniqueIndex[string, *EmailVerification]
	byUserID *jsonldb.Index[ksid.ID, *EmailVerification]
}

// NewEmailVerificationService creates a new email verification service.
func NewEmailVerificationService(tablePath string) (*EmailVerificationService, error) {
	table, err := jsonldb.NewTable[*EmailVerification](tablePath)
	if err != nil {
		return nil, err
	}
	byToken := jsonldb.NewUniqueIndex(table, func(e *EmailVerification) string { return e.Token })
	byUserID := jsonldb.NewIndex(table, func(e *EmailVerification) ksid.ID { return e.UserID })
	return &EmailVerificationService{table: table, byToken: byToken, byUserID: byUserID}, nil
}

// Create creates a new email verification token with 24-hour expiry.
// It first deletes any existing verification tokens for the user.
func (s *EmailVerificationService) Create(userID ksid.ID, email string) (*EmailVerification, error) {
	if userID.IsZero() {
		return nil, errVerificationUserIDRequired
	}
	if email == "" {
		return nil, errVerificationEmailRequired
	}

	// Delete existing tokens for this user
	if err := s.DeleteByUserID(userID); err != nil {
		return nil, err
	}

	// Generate a secure 64-character hex token (32 bytes = 64 hex chars)
	token, err := utils.GenerateToken(32)
	if err != nil {
		return nil, err
	}

	now := storage.Now()
	verification := &EmailVerification{
		ID:        ksid.NewID(),
		UserID:    userID,
		Email:     email,
		Token:     token,
		ExpiresAt: storage.ToTime(time.Now().Add(24 * time.Hour)),
		Created:   now,
	}

	if err := s.table.Append(verification); err != nil {
		return nil, err
	}
	return verification.Clone(), nil
}

// GetByToken retrieves a verification by token. O(1) via unique index.
func (s *EmailVerificationService) GetByToken(token string) (*EmailVerification, error) {
	if token == "" {
		return nil, errVerificationTokenRequired
	}
	verification := s.byToken.Get(token)
	if verification == nil {
		return nil, errVerificationNotFound
	}
	return verification.Clone(), nil
}

// Get retrieves a verification by ID.
func (s *EmailVerificationService) Get(id ksid.ID) (*EmailVerification, error) {
	if id.IsZero() {
		return nil, errVerificationIDRequired
	}
	verification := s.table.Get(id)
	if verification == nil {
		return nil, errVerificationNotFound
	}
	return verification.Clone(), nil
}

// Delete removes a verification by ID.
func (s *EmailVerificationService) Delete(id ksid.ID) error {
	if id.IsZero() {
		return errVerificationIDRequired
	}
	_, err := s.table.Delete(id)
	return err
}

// DeleteByUserID removes all verifications for a user.
func (s *EmailVerificationService) DeleteByUserID(userID ksid.ID) error {
	if userID.IsZero() {
		return errVerificationUserIDRequired
	}

	var toDelete []ksid.ID
	for verification := range s.byUserID.Iter(userID) {
		toDelete = append(toDelete, verification.ID)
	}

	for _, id := range toDelete {
		if _, err := s.table.Delete(id); err != nil {
			return err
		}
	}
	return nil
}

// IsExpired checks if a verification token has expired.
func (s *EmailVerificationService) IsExpired(verification *EmailVerification) bool {
	return verification.ExpiresAt.Before(storage.Now())
}

// Iter iterates over all verifications.
func (s *EmailVerificationService) Iter(startID ksid.ID) iter.Seq[*EmailVerification] {
	return func(yield func(*EmailVerification) bool) {
		for v := range s.table.Iter(startID) {
			if !yield(v.Clone()) {
				return
			}
		}
	}
}

var (
	errVerificationIDRequired     = errors.New("verification id is required")
	errVerificationUserIDRequired = errors.New("verification user_id is required")
	errVerificationEmailRequired  = errors.New("verification email is required")
	errVerificationTokenRequired  = errors.New("verification token is required")
	errVerificationNotFound       = errors.New("verification not found")
)
