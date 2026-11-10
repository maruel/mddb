// Routes immutable workspace changes through one context-owned feed with shared screen framing and scoped gap recovery.

package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
)

const (
	// subscriberBufSize is the channel buffer for each subscriber.
	subscriberBufSize = 64
	// maxConnsPerUser is the maximum concurrent SSE connections per user per workspace.
	maxConnsPerUser = 5
)

// Change is an immutable workspace invalidation shared by all interested readers.
// WorkspaceEvent and the frame returned by SSE must not be modified. ChangeID
// identifies invalidation delivery within this process, not resource revisions.
type Change struct {
	dto.WorkspaceEvent

	id        int64
	frameOnce sync.Once
	frame     []byte
	frameErr  error
}

// Sequence orders invalidations within this broker lifetime. It is used only
// for a registered read's handoff, never as a resource or cross-process revision.
func (c *Change) Sequence() int64 { return c.id }

// SSE returns the shared screen frame, encoding it only when a screen consumes
// the change. MCP readers use the typed effects without JSON encoding.
func (c *Change) SSE() ([]byte, error) {
	c.frameOnce.Do(func() {
		data, err := json.Marshal(&c.WorkspaceEvent)
		if err != nil {
			c.frameErr = err
			return
		}
		c.frame = fmt.Appendf(nil, "id: %d\nevent: workspace\ndata: %s\n\n", c.id, data)
	})
	return c.frame, c.frameErr
}

// EventInterest routes committed effects to consumers of the affected projections.
// All is reserved for screen consumers that maintain the complete workspace.
type EventInterest struct {
	All      bool
	Catalog  bool
	Summary  bool
	Children []ksid.ID
	Nodes    []ksid.ID
	Records  []ksid.ID
}

type subscriber struct {
	userID   ksid.ID
	ch       chan *Change
	interest EventInterest
	wake     chan struct{}
}

// setInterest freezes caller-owned target buffers at registration/replacement.
func (s *subscriber) setInterest(interest *EventInterest) {
	s.interest = *interest
	s.interest.Nodes = slices.Clone(interest.Nodes)
	s.interest.Records = slices.Clone(interest.Records)
	s.interest.Children = slices.Clone(interest.Children)
}

func (s *subscriber) matches(event *dto.WorkspaceEvent) bool {
	return s.interest.All || event.NodeID.IsZero() || event.Type == dto.EventResync ||
		slices.ContainsFunc(s.interest.Nodes, func(id ksid.ID) bool { return NodeResourceChanged(event, id) }) ||
		slices.ContainsFunc(s.interest.Records, func(id ksid.ID) bool { return RecordResourceChanged(event, id, 0) }) ||
		(s.interest.Catalog && slices.Contains(event.Aspects, dto.EventAspectCatalog)) ||
		(s.interest.Summary && slices.Contains(event.Aspects, dto.EventAspectSummary)) ||
		slices.ContainsFunc(s.interest.Children, func(id ksid.ID) bool { return ChildrenResourceChanged(event, id, nil) })
}

// RecordResourceChanged identifies changes to a table's data/query projection,
// independently of the node's schema-only resource.
func RecordResourceChanged(event *dto.WorkspaceEvent, id, record ksid.ID) bool {
	return event.Type == dto.EventResync ||
		(event.Type == dto.EventNodeDeleted && (event.NodeID == id || slices.Contains(event.AffectedIDs, id))) ||
		(event.NodeID == id &&
			((slices.Contains(event.Aspects, dto.EventAspectRecords) && (record == 0 || event.RecordID == record)) ||
				(record == 0 && slices.Contains(event.Aspects, dto.EventAspectSchema))))
}

// Feed is a context-owned typed broker registration. Events are immutable;
// UpdateInterest atomically installs dependencies before their snapshot read.
type Feed struct {
	Events <-chan *Change
	Wake   <-chan struct{}

	broker *Broker
	sub    *subscriber
	ctx    context.Context
}

// Position synchronizes a synchronous reader with complete publication to all
// subscribers. Wake is a coalesced hint; it never consumes an Events value.
func (f *Feed) Position() (int64, error) {
	f.broker.mu.Lock()
	defer f.broker.mu.Unlock()
	return f.broker.eventID, f.ctx.Err()
}

// UpdateInterest replaces this registration's targets and returns the read-start
// sequence under the publisher lock. Events after it cannot pass registration.
// No consumer or I/O runs under the broker lock.
func (f *Feed) UpdateInterest(interest *EventInterest) (int64, error) {
	f.broker.mu.Lock()
	defer f.broker.mu.Unlock()
	if err := f.ctx.Err(); err != nil {
		return 0, err
	}
	f.sub.setInterest(interest)
	return f.broker.eventID, nil
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

// Broker fans out typed workspace invalidations without blocking publishers.
type Broker struct {
	mu         sync.Mutex
	workspaces map[ksid.ID]map[*subscriber]struct{}
	// epoch prevents watermarks from comparing equal across process lifetimes.
	epoch   string
	eventID int64
}

// NewBroker creates a ready-to-use Broker.
func NewBroker() *Broker {
	return &Broker{workspaces: make(map[ksid.ID]map[*subscriber]struct{}), epoch: ksid.NewID().String()}
}

// Subscribe registers before returning and closes the feed when ctx ends. A
// nonzero screenUser applies the per-user screen connection limit; internal MCP
// streams use zero and retain their transport-owned session admission. Overflow discards queued effects
// and delivers a scope-local resync without actor, request or target identities.
func (b *Broker) Subscribe(ctx context.Context, wsID, screenUser ksid.ID, interest *EventInterest) (*Feed, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	subs := b.workspaces[wsID]
	if screenUser != 0 {
		var count int
		for sub := range subs {
			if sub.userID == screenUser {
				count++
			}
		}
		if count >= maxConnsPerUser {
			return nil, fmt.Errorf("too many SSE connections (max %d per user per workspace)", maxConnsPerUser)
		}
	}
	if subs == nil {
		subs = make(map[*subscriber]struct{})
		b.workspaces[wsID] = subs
	}
	sub := &subscriber{userID: screenUser, ch: make(chan *Change, subscriberBufSize), wake: make(chan struct{}, 1)}
	sub.setInterest(interest)
	subs[sub] = struct{}{}
	context.AfterFunc(ctx, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(subs, sub)
		if len(subs) == 0 {
			delete(b.workspaces, wsID)
		}
		close(sub.ch)
		close(sub.wake)
	})
	return &Feed{Events: sub.ch, Wake: sub.wake, broker: b, sub: sub, ctx: ctx}, nil
}

// Publish shares one immutable typed change and one lazy screen encoding across
// all interested consumers. Slow readers receive a sanitized resync marker.
func (b *Broker) Publish(wsID ksid.ID, event *dto.WorkspaceEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.eventID++
	change := &Change{WorkspaceEvent: *event, id: b.eventID}
	change.Aspects = slices.Clone(event.Aspects)
	change.AffectedIDs = slices.Clone(event.AffectedIDs)
	change.SummaryParents = slices.Clone(event.SummaryParents)
	change.ChangeID = fmt.Sprintf("%s:%d", b.epoch, b.eventID)
	var gap *Change
	for sub := range b.workspaces[wsID] {
		if !sub.matches(event) {
			continue
		}
		select {
		case sub.ch <- change:
		default:
			drain(sub.ch)
			if gap == nil {
				gap = &Change{WorkspaceEvent: dto.WorkspaceEvent{Type: dto.EventResync, ChangeID: change.ChangeID}, id: change.id}
			}
			sub.ch <- gap
		}
		select {
		case sub.wake <- struct{}{}:
		default:
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

// ChildrenResourceChanged routes a bounded listing's membership and represented
// summaries, including HasChildren under its returned members, never page bodies.
func ChildrenResourceChanged(event *dto.WorkspaceEvent, parent ksid.ID, members []ksid.ID) bool {
	if event.Type == dto.EventResync {
		return true
	}
	if event.Type == dto.EventNodeDeleted && parent != 0 && (event.NodeID == parent || slices.Contains(event.AffectedIDs, parent)) {
		return true
	}
	if slices.Contains(event.Aspects, dto.EventAspectHierarchy) {
		if members == nil && slices.Contains(event.SummaryParents, parent) {
			return true
		}
		if event.ParentID == parent || (event.Type == dto.EventNodeMoved && event.OldParentID == parent) {
			return true
		}
		if slices.Contains(members, event.ParentID) || (event.Type == dto.EventNodeMoved && slices.Contains(members, event.OldParentID)) {
			return true
		}
	}
	return event.ParentID == parent && slices.Contains(event.Aspects, dto.EventAspectSummary) && (members == nil || slices.Contains(members, event.NodeID))
}
