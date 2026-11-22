// Publishes immutable server settings after validated, serialized persistence.

package storage

import (
	"sync"
	"sync/atomic"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

// ServerSettings owns the current server configuration and serializes updates.
type ServerSettings struct {
	mu      sync.Mutex
	current atomic.Pointer[storagev1.ServerConfig]
}

// NewServerSettings takes ownership of cfg. Callers must not modify it afterward.
func NewServerSettings(cfg *storagev1.ServerConfig) *ServerSettings {
	s := &ServerSettings{}
	s.current.Store(cfg)
	return s
}

// Snapshot returns the complete published settings. Immutable.
// An operation may finish using a snapshot admitted before an update.
func (s *ServerSettings) Snapshot() *storagev1.ServerConfig { return s.current.Load() }

// Update validates and saves one edited snapshot before publishing it.
// publish applies dependent runtime settings while updates are serialized.
// It must not fail, call Update, or retain the mutable candidate.
// Errors leave both the active configuration and settings.json unchanged.
func (s *ServerSettings) Update(dir string, edit func(*storagev1.ServerConfig) error, publish func(*storagev1.ServerConfig)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := *s.current.Load()
	if err := edit(&next); err != nil {
		return err
	}
	if err := SaveServerConfig(&next, dir); err != nil {
		return err
	}
	if publish != nil {
		publish(&next)
	}
	s.current.Store(&next)
	return nil
}
