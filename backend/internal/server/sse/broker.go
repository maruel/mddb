// In-process pub/sub broker keyed by workspace ID for SSE event distribution.

package sse

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
)

const (
	// subscriberBufSize is the channel buffer for each subscriber.
	subscriberBufSize = 64
	// maxConnsPerUser is the maximum concurrent SSE connections per user per workspace.
	maxConnsPerUser = 5
)

// Subscriber receives pre-formatted SSE messages on its channel.
type Subscriber struct {
	C      <-chan []byte
	ch     chan []byte
	userID ksid.ID
}

// Broker fans out workspace events to connected SSE subscribers.
type Broker struct {
	mu sync.RWMutex
	// workspaces maps workspace ID -> set of subscribers.
	workspaces map[ksid.ID]map[*Subscriber]struct{}
	// eventID is a monotonically increasing SSE event ID.
	eventID atomic.Int64
}

// NewBroker creates a ready-to-use Broker.
func NewBroker() *Broker {
	return &Broker{
		workspaces: make(map[ksid.ID]map[*Subscriber]struct{}),
	}
}

// Subscribe registers a new subscriber for the given workspace.
// Returns the subscriber and a cleanup function. The caller must call cleanup
// when done (typically deferred). Returns an error if the user exceeds
// maxConnsPerUser for this workspace.
func (b *Broker) Subscribe(wsID, userID ksid.ID) (*Subscriber, func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs, ok := b.workspaces[wsID]
	if !ok {
		subs = make(map[*Subscriber]struct{})
		b.workspaces[wsID] = subs
	}

	// Enforce per-user connection limit.
	var count int
	for s := range subs {
		if s.userID == userID {
			count++
		}
	}
	if count >= maxConnsPerUser {
		return nil, nil, fmt.Errorf("too many SSE connections (max %d per user per workspace)", maxConnsPerUser)
	}

	ch := make(chan []byte, subscriberBufSize)
	sub := &Subscriber{C: ch, ch: ch, userID: userID}
	subs[sub] = struct{}{}

	cleanup := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if ws, ok := b.workspaces[wsID]; ok {
			delete(ws, sub)
			if len(ws) == 0 {
				delete(b.workspaces, wsID)
			}
		}
		close(ch)
	}
	return sub, cleanup, nil
}

// Publish sends an event to all subscribers of the given workspace.
// The event is JSON-serialized once and wrapped as an SSE frame. Slow
// subscribers whose buffers are full will have the event dropped.
func (b *Broker) Publish(wsID ksid.ID, event dto.WorkspaceEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	id := b.eventID.Add(1)
	msg := fmt.Appendf(nil, "id: %d\nevent: workspace\ndata: %s\n\n", id, data)

	b.mu.RLock()
	defer b.mu.RUnlock()

	subs, ok := b.workspaces[wsID]
	if !ok {
		return
	}
	for s := range subs {
		// Non-blocking send; drop if full.
		select {
		case s.ch <- msg:
		default:
		}
	}
}

// SubscriberCount returns the number of active subscribers for a workspace.
// Intended for testing and diagnostics.
func (b *Broker) SubscriberCount(wsID ksid.ID) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.workspaces[wsID])
}
