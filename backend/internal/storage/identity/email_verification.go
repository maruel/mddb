// Manages email verification tokens for magic link authentication.

package identity

import (
	"errors"
	"iter"
	"time"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/utils"
)

// EmailVerificationService handles email verification token management.
type EmailVerificationService struct {
	table    *jsonldb.Table[*identityv1.EmailVerification]
	byToken  *jsonldb.UniqueIndex[string, *identityv1.EmailVerification]
	byUserID *jsonldb.Index[ksid.ID, *identityv1.EmailVerification]
}

// NewEmailVerificationService creates a new email verification service.
func NewEmailVerificationService(tablePath string) (*EmailVerificationService, error) {
	table, err := jsonldb.NewTable[*identityv1.EmailVerification](tablePath)
	if err != nil {
		return nil, err
	}
	byToken := jsonldb.NewUniqueIndex(table, func(e *identityv1.EmailVerification) string { return e.Token })
	byUserID := jsonldb.NewIndex(table, func(e *identityv1.EmailVerification) ksid.ID { return e.UserID })
	return &EmailVerificationService{table: table, byToken: byToken, byUserID: byUserID}, nil
}

// Create creates a new email verification token with 24-hour expiry.
// It first deletes any existing verification tokens for the user.
func (s *EmailVerificationService) Create(userID ksid.ID, email string) (*identityv1.EmailVerification, error) {
	if userID.IsZero() {
		return nil, identityv1.ErrVerificationUserIDRequired
	}
	if email == "" {
		return nil, identityv1.ErrVerificationEmailRequired
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

	now := storagev1.Now()
	verification := &identityv1.EmailVerification{
		ID:        ksid.NewID(),
		UserID:    userID,
		Email:     email,
		Token:     token,
		ExpiresAt: storagev1.ToTime(time.Now().Add(24 * time.Hour)),
		Created:   now,
	}

	if err := s.table.Append(verification); err != nil {
		return nil, err
	}
	return verification.Clone(), nil
}

// GetByToken retrieves a verification by token. O(1) via unique index.
func (s *EmailVerificationService) GetByToken(token string) (*identityv1.EmailVerification, error) {
	if token == "" {
		return nil, identityv1.ErrVerificationTokenRequired
	}
	verification := s.byToken.Get(token)
	if verification == nil {
		return nil, errVerificationNotFound
	}
	return verification.Clone(), nil
}

// Get retrieves a verification by ID.
func (s *EmailVerificationService) Get(id ksid.ID) (*identityv1.EmailVerification, error) {
	if id.IsZero() {
		return nil, identityv1.ErrVerificationIDRequired
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
		return identityv1.ErrVerificationIDRequired
	}
	_, err := s.table.Delete(id)
	return err
}

// DeleteByUserID removes all verifications for a user.
func (s *EmailVerificationService) DeleteByUserID(userID ksid.ID) error {
	if userID.IsZero() {
		return identityv1.ErrVerificationUserIDRequired
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
func (s *EmailVerificationService) IsExpired(verification *identityv1.EmailVerification) bool {
	return verification.ExpiresAt.Before(storagev1.Now())
}

// Iter iterates over all verifications.
func (s *EmailVerificationService) Iter(startID ksid.ID) iter.Seq[*identityv1.EmailVerification] {
	return func(yield func(*identityv1.EmailVerification) bool) {
		for v := range s.table.Iter(startID) {
			if !yield(v.Clone()) {
				return
			}
		}
	}
}

var (
	errVerificationNotFound = errors.New("verification not found")
)
