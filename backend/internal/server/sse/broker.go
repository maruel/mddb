// Routes workspace projection invalidations to bounded SSE and MCP subscribers with gap recovery.

package sse

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
)

const (
	// subscriberBufSize is the channel buffer for each subscriber.
	subscriberBufSize = 64
	// eventBufSize is the channel buffer for each structured event subscriber.
	eventBufSize = 64
	// maxConnsPerUser is the maximum concurrent SSE connections per user per workspace.
	maxConnsPerUser = 5
)

// Subscriber receives pre-formatted SSE messages on its channel.
type Subscriber struct {
	C      <-chan []byte
	ch     chan []byte
	userID ksid.ID
}

// EventInterest routes committed effects to consumers of the affected projections.
// All is reserved for screen consumers that maintain the complete workspace.
type EventInterest struct {
	All     bool
	Catalog bool
	Summary bool
	Nodes   []ksid.ID
}

type eventSubscriber struct {
	ch       chan dto.WorkspaceEvent
	interest EventInterest
}

func (s *eventSubscriber) matches(event *dto.WorkspaceEvent) bool {
	return s.interest.All || event.NodeID.IsZero() || event.Type == dto.EventResync ||
		slices.ContainsFunc(s.interest.Nodes, func(id ksid.ID) bool { return NodeResourceChanged(event, id) }) ||
		(s.interest.Catalog && slices.Contains(event.Aspects, dto.EventAspectCatalog)) ||
		(s.interest.Summary && slices.Contains(event.Aspects, dto.EventAspectSummary))
}

// NodeResourceChanged projects committed effects onto a node resource: its own
// page/schema/metadata and ParentID, without records, assets or child listings.
func NodeResourceChanged(event *dto.WorkspaceEvent, id ksid.ID) bool {
	if event.Type == dto.EventResync {
		return true
	}
	if event.Type == dto.EventNodeDeleted {
		return id == event.NodeID || slices.Contains(event.AffectedIDs, id)
	}
	if id != event.NodeID {
		return false
	}
	return event.Type == dto.EventNodeMoved || slices.ContainsFunc(event.Aspects, func(aspect dto.EventAspect) bool {
		return aspect == dto.EventAspectBody || aspect == dto.EventAspectMetadata || aspect == dto.EventAspectSchema || aspect == dto.EventAspectSummary
	})
}

// Broker fans out workspace events to connected SSE subscribers.
type Broker struct {
	mu sync.RWMutex
	// workspaces maps workspace ID -> set of subscribers.
	workspaces map[ksid.ID]map[*Subscriber]struct{}
	// events maps workspace ID -> set of structured in-process subscribers.
	events map[ksid.ID]map[*eventSubscriber]struct{}
	// epoch prevents watermarks from comparing equal across process lifetimes.
	epoch string
	// eventID is a monotonically increasing SSE event ID.
	eventID atomic.Int64
}

// NewBroker creates a ready-to-use Broker.
func NewBroker() *Broker {
	return &Broker{
		workspaces: make(map[ksid.ID]map[*Subscriber]struct{}),
		events:     make(map[ksid.ID]map[*eventSubscriber]struct{}),
		epoch:      ksid.NewID().String(),
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

// SubscribeEvents registers a structured in-process subscriber for the given
// workspace. It returns the event channel and a cleanup function the caller must
// call when done; the channel is closed by cleanup. Publishers never block, so a
// slow consumer receives a resync invalidation instead of losing freshness.
func (b *Broker) SubscribeEvents(wsID ksid.ID, interest EventInterest) (events <-chan dto.WorkspaceEvent, unsubscribe func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs, ok := b.events[wsID]
	if !ok {
		subs = make(map[*eventSubscriber]struct{})
		b.events[wsID] = subs
	}
	ch := make(chan dto.WorkspaceEvent, eventBufSize)
	sub := &eventSubscriber{ch: ch, interest: interest}
	subs[sub] = struct{}{}

	unsubscribe = func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		ws, ok := b.events[wsID]
		if !ok {
			return
		}
		if _, ok := ws[sub]; !ok {
			return
		}
		delete(ws, sub)
		close(ch)
		if len(ws) == 0 {
			delete(b.events, wsID)
		}
	}
	return ch, unsubscribe
}

// Publish sends an event to all subscribers of the given workspace.
// The event is JSON-serialized once and wrapped as an SSE frame. Slow
// subscribers receive a resync marker when their bounded queue overflows.
func (b *Broker) Publish(wsID ksid.ID, event *dto.WorkspaceEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.eventID.Add(1)
	published := *event
	published.ChangeID = fmt.Sprintf("%s:%d", b.epoch, id)
	event = &published
	var msg []byte
	if len(b.workspaces[wsID]) > 0 {
		data, err := json.Marshal(event)
		if err != nil {
			return
		}
		msg = fmt.Appendf(nil, "id: %d\nevent: workspace\ndata: %s\n\n", id, data)
	}
	gap := dto.WorkspaceEvent{Type: dto.EventResync, ChangeID: event.ChangeID}
	var gapMsg []byte

	for s := range b.workspaces[wsID] {
		select {
		case s.ch <- msg:
		default:
			drain(s.ch)
			if gapMsg == nil {
				gapData, _ := json.Marshal(gap)
				gapMsg = fmt.Appendf(nil, "id: %d\nevent: workspace\ndata: %s\n\n", id, gapData)
			}
			s.ch <- gapMsg
		}
	}
	for sub := range b.events[wsID] {
		if !sub.matches(event) {
			continue
		}
		select {
		case sub.ch <- *event:
		default:
			drain(sub.ch)
			sub.ch <- gap
		}
	}
}

func drain[T any](ch <-chan T) {
	for {
		select {
		case <-ch:
		default:
			return
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
