// Handles active user sessions and token management.

package identity

import (
	"errors"
	"iter"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/ksid"
	"github.com/maruel/mddb/backend/internal/storage"
)

// Session represents an active user session.
type Session struct {
	ID          ksid.ID      `json:"id" jsonschema:"description=Unique session identifier"`
	UserID      ksid.ID      `json:"user_id" jsonschema:"description=User who owns this session"`
	TokenHash   string       `json:"token_hash" jsonschema:"description=SHA-256 hash of the JWT token"`
	DeviceInfo  string       `json:"device_info" jsonschema:"description=Parsed User-Agent (browser/OS)"`
	IPAddress   string       `json:"ip_address" jsonschema:"description=Client IP address at login"`
	CountryCode string       `json:"country_code,omitempty" jsonschema:"description=ISO 3166-1 alpha-2 country code at login"`
	Created     storage.Time `json:"created" jsonschema:"description=Session creation timestamp"`
	LastUsed    storage.Time `json:"last_used" jsonschema:"description=Last activity timestamp"`
	ExpiresAt   storage.Time `json:"expires_at" jsonschema:"description=Session expiration timestamp"`
	RevokedAt   storage.Time `json:"revoked_at,omitempty" jsonschema:"description=Revocation timestamp if revoked"`
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
		return errSessionIDRequired
	}
	if s.UserID.IsZero() {
		return errSessionUserIDRequired
	}
	if s.TokenHash == "" {
		return errSessionTokenHashRequired
	}
	return nil
}

// SessionService handles session management.
type SessionService struct {
	table    *jsonldb.Table[*Session]
	byUserID *jsonldb.Index[ksid.ID, *Session]
}

// NewSessionService creates a new session service.
func NewSessionService(tablePath string) (*SessionService, error) {
	table, err := jsonldb.NewTable[*Session](tablePath)
	if err != nil {
		return nil, err
	}
	byUserID := jsonldb.NewIndex(table, func(s *Session) ksid.ID { return s.UserID })
	return &SessionService{table: table, byUserID: byUserID}, nil
}

// Create creates a new session with an auto-generated ID.
// maxSessions limits the number of active sessions per user. Use 0 to disable the limit.
func (s *SessionService) Create(userID ksid.ID, tokenHash, deviceInfo, ipAddress, countryCode string, expiresAt storage.Time, maxSessions int) (*Session, error) {
	return s.CreateWithID(ksid.NewID(), userID, tokenHash, deviceInfo, ipAddress, countryCode, expiresAt, maxSessions)
}

// CreateWithID creates a new session with a pre-specified ID.
// This is useful when the session ID needs to be included in the JWT before creating the session.
// maxSessions limits the number of active sessions per user. Use 0 to disable the limit.
func (s *SessionService) CreateWithID(id, userID ksid.ID, tokenHash, deviceInfo, ipAddress, countryCode string, expiresAt storage.Time, maxSessions int) (*Session, error) {
	if id.IsZero() {
		return nil, errSessionIDRequired
	}
	if userID.IsZero() {
		return nil, errSessionUserIDRequired
	}
	if tokenHash == "" {
		return nil, errSessionTokenHashRequired
	}

	// Check session quota if enabled
	if maxSessions > 0 {
		activeCount := 0
		for range s.GetActiveByUserID(userID) {
			activeCount++
		}
		if activeCount >= maxSessions {
			return nil, ErrSessionQuotaExceeded
		}
	}

	now := storage.Now()
	session := &Session{
		ID:          id,
		UserID:      userID,
		TokenHash:   tokenHash,
		DeviceInfo:  deviceInfo,
		IPAddress:   ipAddress,
		CountryCode: countryCode,
		Created:     now,
		LastUsed:    now,
		ExpiresAt:   expiresAt,
	}

	if err := s.table.Append(session); err != nil {
		return nil, err
	}
	return session.Clone(), nil
}

// Get retrieves a session by ID.
func (s *SessionService) Get(id ksid.ID) (*Session, error) {
	session := s.table.Get(id)
	if session == nil {
		return nil, errSessionNotFound
	}
	return session.Clone(), nil
}

// GetByUserID returns an iterator over all sessions for a user.
func (s *SessionService) GetByUserID(userID ksid.ID) iter.Seq[*Session] {
	return func(yield func(*Session) bool) {
		for session := range s.byUserID.Iter(userID) {
			if !yield(session.Clone()) {
				return
			}
		}
	}
}

// GetActiveByUserID returns an iterator over active (non-revoked, non-expired) sessions for a user.
func (s *SessionService) GetActiveByUserID(userID ksid.ID) iter.Seq[*Session] {
	now := storage.Now()
	return func(yield func(*Session) bool) {
		for session := range s.byUserID.Iter(userID) {
			if session.RevokedAt.IsZero() && session.ExpiresAt.After(now) {
				if !yield(session.Clone()) {
					return
				}
			}
		}
	}
}

// CountActive returns the number of active (non-revoked, non-expired) sessions.
func (s *SessionService) CountActive() int {
	now := storage.Now()
	count := 0
	for session := range s.table.Iter(0) {
		if session.RevokedAt.IsZero() && session.ExpiresAt.After(now) {
			count++
		}
	}
	return count
}

// Revoke marks a session as revoked.
func (s *SessionService) Revoke(id ksid.ID) error {
	_, err := s.table.Modify(id, func(session *Session) error {
		if !session.RevokedAt.IsZero() {
			return nil // Already revoked
		}
		session.RevokedAt = storage.Now()
		return nil
	})
	return err
}

// RevokeAllForUser revokes all sessions for a user. Returns the count of revoked sessions.
func (s *SessionService) RevokeAllForUser(userID ksid.ID) (int, error) {
	var ids []ksid.ID
	for session := range s.byUserID.Iter(userID) {
		if session.RevokedAt.IsZero() {
			ids = append(ids, session.ID)
		}
	}

	count := 0
	for _, id := range ids {
		if err := s.Revoke(id); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// UpdateLastUsed updates the LastUsed timestamp for a session.
func (s *SessionService) UpdateLastUsed(id ksid.ID) error {
	_, err := s.table.Modify(id, func(session *Session) error {
		session.LastUsed = storage.Now()
		return nil
	})
	return err
}

// IsValid checks if a session is valid (not revoked and not expired).
func (s *SessionService) IsValid(id ksid.ID) (bool, error) {
	session := s.table.Get(id)
	if session == nil {
		return false, errSessionNotFound
	}
	if !session.RevokedAt.IsZero() {
		return false, nil
	}
	if session.ExpiresAt.Before(storage.Now()) {
		return false, nil
	}
	return true, nil
}

// CleanupExpired removes sessions that have been expired for more than the given duration.
func (s *SessionService) CleanupExpired(olderThan time.Duration) (int, error) {
	cutoff := storage.ToTime(time.Now().Add(-olderThan))
	var toDelete []ksid.ID

	for session := range s.table.Iter(0) {
		if session.ExpiresAt.Before(cutoff) {
			toDelete = append(toDelete, session.ID)
		}
	}

	count := 0
	for _, id := range toDelete {
		if _, err := s.table.Delete(id); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

var (
	errSessionIDRequired        = errors.New("session id is required")
	errSessionUserIDRequired    = errors.New("session user_id is required")
	errSessionTokenHashRequired = errors.New("session token_hash is required")
	errSessionNotFound          = errors.New("session not found")
	// ErrSessionQuotaExceeded is returned when a user has too many active sessions.
	ErrSessionQuotaExceeded = errors.New("maximum number of active sessions exceeded")
)
