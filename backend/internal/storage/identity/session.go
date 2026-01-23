package identity

import (
	"errors"
	"iter"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// Session represents an active user session.
type Session struct {
	ID         jsonldb.ID `json:"id" jsonschema:"description=Unique session identifier"`
	UserID     jsonldb.ID `json:"user_id" jsonschema:"description=User who owns this session"`
	TokenHash  string     `json:"token_hash" jsonschema:"description=SHA-256 hash of the JWT token"`
	DeviceInfo string     `json:"device_info" jsonschema:"description=Parsed User-Agent (browser/OS)"`
	IPAddress  string     `json:"ip_address" jsonschema:"description=Client IP address at login"`
	Created    time.Time  `json:"created" jsonschema:"description=Session creation timestamp"`
	LastUsed   time.Time  `json:"last_used" jsonschema:"description=Last activity timestamp"`
	ExpiresAt  time.Time  `json:"expires_at" jsonschema:"description=Session expiration timestamp"`
	Revoked    bool       `json:"revoked" jsonschema:"description=Whether session has been revoked"`
	RevokedAt  time.Time  `json:"revoked_at,omitempty" jsonschema:"description=Revocation timestamp if revoked"`
}

// Clone returns a deep copy of the session.
func (s *Session) Clone() *Session {
	c := *s
	return &c
}

// GetID returns the session's ID.
func (s *Session) GetID() jsonldb.ID {
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
	byUserID *jsonldb.Index[jsonldb.ID, *Session]
}

// NewSessionService creates a new session service.
func NewSessionService(tablePath string) (*SessionService, error) {
	table, err := jsonldb.NewTable[*Session](tablePath)
	if err != nil {
		return nil, err
	}
	byUserID := jsonldb.NewIndex(table, func(s *Session) jsonldb.ID { return s.UserID })
	return &SessionService{table: table, byUserID: byUserID}, nil
}

// Create creates a new session with an auto-generated ID.
func (s *SessionService) Create(userID jsonldb.ID, tokenHash, deviceInfo, ipAddress string, expiresAt time.Time) (*Session, error) {
	return s.CreateWithID(jsonldb.NewID(), userID, tokenHash, deviceInfo, ipAddress, expiresAt)
}

// CreateWithID creates a new session with a pre-specified ID.
// This is useful when the session ID needs to be included in the JWT before creating the session.
func (s *SessionService) CreateWithID(id, userID jsonldb.ID, tokenHash, deviceInfo, ipAddress string, expiresAt time.Time) (*Session, error) {
	if id.IsZero() {
		return nil, errSessionIDRequired
	}
	if userID.IsZero() {
		return nil, errSessionUserIDRequired
	}
	if tokenHash == "" {
		return nil, errSessionTokenHashRequired
	}

	now := time.Now()
	session := &Session{
		ID:         id,
		UserID:     userID,
		TokenHash:  tokenHash,
		DeviceInfo: deviceInfo,
		IPAddress:  ipAddress,
		Created:    now,
		LastUsed:   now,
		ExpiresAt:  expiresAt,
		Revoked:    false,
	}

	if err := s.table.Append(session); err != nil {
		return nil, err
	}
	return session.Clone(), nil
}

// Get retrieves a session by ID.
func (s *SessionService) Get(id jsonldb.ID) (*Session, error) {
	session := s.table.Get(id)
	if session == nil {
		return nil, errSessionNotFound
	}
	return session.Clone(), nil
}

// GetByUserID returns an iterator over all sessions for a user.
func (s *SessionService) GetByUserID(userID jsonldb.ID) iter.Seq[*Session] {
	return func(yield func(*Session) bool) {
		for session := range s.byUserID.Iter(userID) {
			if !yield(session.Clone()) {
				return
			}
		}
	}
}

// GetActiveByUserID returns an iterator over active (non-revoked, non-expired) sessions for a user.
func (s *SessionService) GetActiveByUserID(userID jsonldb.ID) iter.Seq[*Session] {
	now := time.Now()
	return func(yield func(*Session) bool) {
		for session := range s.byUserID.Iter(userID) {
			if !session.Revoked && session.ExpiresAt.After(now) {
				if !yield(session.Clone()) {
					return
				}
			}
		}
	}
}

// Revoke marks a session as revoked.
func (s *SessionService) Revoke(id jsonldb.ID) error {
	_, err := s.table.Modify(id, func(session *Session) error {
		if session.Revoked {
			return nil // Already revoked
		}
		session.Revoked = true
		session.RevokedAt = time.Now()
		return nil
	})
	return err
}

// RevokeAllForUser revokes all sessions for a user. Returns the count of revoked sessions.
func (s *SessionService) RevokeAllForUser(userID jsonldb.ID) (int, error) {
	var ids []jsonldb.ID
	for session := range s.byUserID.Iter(userID) {
		if !session.Revoked {
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
func (s *SessionService) UpdateLastUsed(id jsonldb.ID) error {
	_, err := s.table.Modify(id, func(session *Session) error {
		session.LastUsed = time.Now()
		return nil
	})
	return err
}

// IsValid checks if a session is valid (not revoked and not expired).
func (s *SessionService) IsValid(id jsonldb.ID) (bool, error) {
	session := s.table.Get(id)
	if session == nil {
		return false, errSessionNotFound
	}
	if session.Revoked {
		return false, nil
	}
	if session.ExpiresAt.Before(time.Now()) {
		return false, nil
	}
	return true, nil
}

// CleanupExpired removes sessions that have been expired for more than the given duration.
func (s *SessionService) CleanupExpired(olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	var toDelete []jsonldb.ID

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
)
