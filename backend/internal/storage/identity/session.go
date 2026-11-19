// Persists host sessions and cancels bound request lifetimes on expiry or revocation.

package identity

import (
	"cmp"
	"context"
	"errors"
	"iter"
	"slices"
	"sync"
	"time"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// SessionService handles session management.
type SessionService struct {
	// creationMu serializes quota checking and insertion for concurrent logins.
	creationMu sync.Mutex

	table     *jsonldb.Table[*identityv1.Session]
	byUserID  *jsonldb.Index[ksid.ID, *identityv1.Session]
	lifetimes *sessionLifetimes

	// usesMu guards uses: the latest use of each session that FlushUses has
	// not stored yet.
	usesMu sync.Mutex
	uses   map[ksid.ID]storagev1.Time
}

// NewSessionService creates a new session service.
func NewSessionService(tablePath string) (*SessionService, error) {
	table, err := jsonldb.NewTable[*identityv1.Session](tablePath)
	if err != nil {
		return nil, err
	}
	byUserID := jsonldb.NewIndex(table, func(s *identityv1.Session) ksid.ID { return s.UserID })
	lifetimes := &sessionLifetimes{bindings: make(map[ksid.ID]map[*sessionBinding]struct{})}
	table.AddObserver(lifetimes)
	return &SessionService{table: table, byUserID: byUserID, lifetimes: lifetimes, uses: make(map[ksid.ID]storagev1.Time)}, nil
}

// Context binds a long-lived authenticated request to its validated session.
// The returned cancel function releases the binding. Expiry, revocation, row
// removal, or credential replacement cancels every bound request, including
// when persistence reports failure after installing the changed row.
func (s *SessionService) Context(ctx context.Context, session *identityv1.Session) (context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithDeadline(ctx, session.ExpiresAt.AsTime())
	b := &sessionBinding{cancel: cancel}
	s.lifetimes.mu.Lock()
	bindings := s.lifetimes.bindings[session.ID]
	if bindings == nil {
		bindings = make(map[*sessionBinding]struct{})
		s.lifetimes.bindings[session.ID] = bindings
	}
	bindings[b] = struct{}{}
	s.lifetimes.mu.Unlock()
	release := func() {
		s.lifetimes.mu.Lock()
		defer s.lifetimes.mu.Unlock()
		delete(s.lifetimes.bindings[session.ID], b)
		if len(s.lifetimes.bindings[session.ID]) == 0 {
			delete(s.lifetimes.bindings, session.ID)
		}
	}
	stop := context.AfterFunc(ctx, release)
	cleanup := func() { stop(); cancel(); release() }
	// Register before reading: a mutation either cancels the registered request
	// or is visible in this read. Never acquire a table lock while holding mu.
	current, err := s.Get(session.ID)
	if err != nil || !current.RevokedAt.IsZero() || current.UserID != session.UserID ||
		current.TokenHash != session.TokenHash || current.ExpiresAt != session.ExpiresAt || ctx.Err() != nil {
		cleanup()
		return nil, nil, errSessionInactive
	}
	return ctx, cleanup, nil
}

// Create creates a new session with an auto-generated ID.
// maxSessions limits the number of active sessions per user. Use 0 to disable the limit.
func (s *SessionService) Create(userID ksid.ID, tokenHash, deviceInfo, ipAddress, countryCode string, expiresAt storagev1.Time, maxSessions int) (*identityv1.Session, error) {
	return s.CreateWithID(ksid.NewID(), userID, tokenHash, deviceInfo, ipAddress, countryCode, expiresAt, maxSessions)
}

// CreateWithID creates a new session with a pre-specified ID.
// This is useful when the session ID needs to be included in the JWT before creating the session.
//
// maxSessions limits the number of active sessions per user; 0 disables the
// limit. At the limit, CreateWithID revokes the user's least recently used
// active sessions, including uses not yet flushed, so a new sign-in succeeds and the
// user keeps at most maxSessions. A revocation failure returns before creation
// and preserves earlier revocations.
// A row installed before a persistence error is returned alongside that error for cleanup.
func (s *SessionService) CreateWithID(id, userID ksid.ID, tokenHash, deviceInfo, ipAddress, countryCode string, expiresAt storagev1.Time, maxSessions int) (*identityv1.Session, error) {
	s.creationMu.Lock()
	defer s.creationMu.Unlock()
	if id.IsZero() {
		return nil, identityv1.ErrSessionIDRequired
	}
	if userID.IsZero() {
		return nil, identityv1.ErrSessionUserIDRequired
	}
	if tokenHash == "" {
		return nil, identityv1.ErrSessionTokenHashRequired
	}

	if maxSessions > 0 {
		s.usesMu.Lock()
		lastUsed := func(session *identityv1.Session) storagev1.Time { return max(session.LastUsed, s.uses[session.ID]) }
		active := slices.SortedFunc(s.GetActiveByUserID(userID), func(a, b *identityv1.Session) int {
			return cmp.Or(cmp.Compare(lastUsed(a), lastUsed(b)), cmp.Compare(a.Created, b.Created), cmp.Compare(a.ID, b.ID))
		})
		s.usesMu.Unlock()
		for _, old := range active[:max(0, len(active)-maxSessions+1)] {
			if err := s.Revoke(old.ID); err != nil {
				return nil, err
			}
		}
	}

	now := storagev1.Now()
	session := &identityv1.Session{
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

	if s.table.Get(id) != nil {
		return nil, errors.New("session already exists")
	}
	if err := s.table.Append(session); err != nil {
		// Append can install the row before a deferred file close reports failure.
		// The caller needs that handle to audit and revoke the partial creation.
		return s.table.Get(id), err
	}
	return session.Clone(), nil
}

// Get retrieves a session by ID.
func (s *SessionService) Get(id ksid.ID) (*identityv1.Session, error) {
	session := s.table.Get(id)
	if session == nil {
		return nil, errSessionNotFound
	}
	return session.Clone(), nil
}

// GetByUserID returns an iterator over all sessions for a user.
func (s *SessionService) GetByUserID(userID ksid.ID) iter.Seq[*identityv1.Session] {
	return func(yield func(*identityv1.Session) bool) {
		for session := range s.byUserID.Iter(userID) {
			if !yield(session.Clone()) {
				return
			}
		}
	}
}

// GetActiveByUserID returns an iterator over active (non-revoked, non-expired) sessions for a user.
func (s *SessionService) GetActiveByUserID(userID ksid.ID) iter.Seq[*identityv1.Session] {
	now := storagev1.Now()
	return func(yield func(*identityv1.Session) bool) {
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
	now := storagev1.Now()
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
	_, err := s.table.Modify(id, func(session *identityv1.Session) error {
		if !session.RevokedAt.IsZero() {
			return nil // Already revoked
		}
		session.RevokedAt = storagev1.Now()
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

// RecordUse notes that the session was used now. It performs no I/O:
// FlushUses stores pending uses, and eviction reads them before they are stored.
func (s *SessionService) RecordUse(id ksid.ID) {
	now := storagev1.Now()
	s.usesMu.Lock()
	defer s.usesMu.Unlock()
	s.uses[id] = now
}

// FlushUses stores pending uses in Session.LastUsed with one table rewrite and
// drops uses of deleted sessions. A failure keeps the uses pending.
func (s *SessionService) FlushUses() error {
	s.usesMu.Lock()
	pending := s.uses
	s.uses = make(map[ksid.ID]storagev1.Time)
	s.usesMu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	_, err := s.table.UpdateFunc(func(session *identityv1.Session) bool {
		used, ok := pending[session.ID]
		if !ok || !used.After(session.LastUsed) {
			return false
		}
		session.LastUsed = used
		return true
	})
	if err != nil {
		s.usesMu.Lock()
		defer s.usesMu.Unlock()
		for id, used := range pending {
			s.uses[id] = max(s.uses[id], used)
		}
	}
	return err
}

// CleanupExpired removes sessions that have been expired for more than the given duration.
func (s *SessionService) CleanupExpired(olderThan time.Duration) (int, error) {
	cutoff := storagev1.ToTime(time.Now().Add(-olderThan))
	deleted, err := s.table.DeleteFunc(func(session *identityv1.Session) bool { return session.ExpiresAt.Before(cutoff) })
	return len(deleted), err
}

type sessionBinding struct {
	cancel context.CancelFunc
}

type sessionLifetimes struct {
	mu       sync.Mutex
	bindings map[ksid.ID]map[*sessionBinding]struct{}
}

func (s *sessionLifetimes) OnAppend(*identityv1.Session) {}

func (s *sessionLifetimes) OnUpdate(prev, next *identityv1.Session) {
	if !next.RevokedAt.IsZero() || prev.UserID != next.UserID || prev.TokenHash != next.TokenHash || prev.ExpiresAt != next.ExpiresAt {
		s.cancel(next.ID)
	}
}

func (s *sessionLifetimes) OnDelete(session *identityv1.Session) {
	s.cancel(session.ID)
}

func (s *sessionLifetimes) cancel(id ksid.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for b := range s.bindings[id] {
		b.cancel()
	}
}

var (
	errSessionNotFound = errors.New("session not found")
	errSessionInactive = errors.New("session is no longer active")
)
