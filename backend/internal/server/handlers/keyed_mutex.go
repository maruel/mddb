// Provides short-lived per-resource locks for idempotent creation handlers.

package handlers

import (
	"sync"

	"github.com/maruel/ksid"
)

type keyedMutex struct {
	mu      sync.Mutex
	entries map[ksid.ID]*keyedMutexEntry
}

func (m *keyedMutex) lock(id ksid.ID) func() {
	m.mu.Lock()
	if m.entries == nil {
		m.entries = make(map[ksid.ID]*keyedMutexEntry)
	}
	entry := m.entries[id]
	if entry == nil {
		entry = &keyedMutexEntry{}
		m.entries[id] = entry
	}
	entry.refs++
	m.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		m.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(m.entries, id)
		}
		m.mu.Unlock()
	}
}

type keyedMutexEntry struct {
	mu   sync.Mutex
	refs int
}
