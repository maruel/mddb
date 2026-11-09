// Tests for the SSE broker.

package sse

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
)

func TestEventInterest(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	ws, target, foreign := ksid.NewID(), ksid.NewID(), ksid.NewID()
	events, stop := b.SubscribeEvents(ws, EventInterest{Nodes: []ksid.ID{target}, Catalog: true})
	t.Cleanup(stop)
	for _, kind := range []dto.EventAspect{dto.EventAspectBody, dto.EventAspectRecords} {
		b.Publish(ws, &dto.WorkspaceEvent{NodeID: foreign, Type: dto.EventNodeUpdated, Aspects: []dto.EventAspect{kind}})
		select {
		case event := <-events:
			t.Fatalf("uninterested event=%+v", event)
		default:
		}
	}
	b.Publish(ws, &dto.WorkspaceEvent{NodeID: target, Type: dto.EventNodeUpdated, Aspects: []dto.EventAspect{dto.EventAspectBody}})
	if got := <-events; got.NodeID != target {
		t.Fatalf("target=%+v", got)
	}
	b.Publish(ws, &dto.WorkspaceEvent{NodeID: foreign, Type: dto.EventNodeUpdated, Aspects: []dto.EventAspect{dto.EventAspectCatalog}})
	if got := <-events; got.NodeID != foreign {
		t.Fatalf("catalog=%+v", got)
	}
	// Moving the watched node changes its own ParentID; moving its child
	// only changes a child list absent from this resource.
	b.Publish(ws, &dto.WorkspaceEvent{NodeID: foreign, ParentID: target, Type: dto.EventNodeMoved, Aspects: []dto.EventAspect{dto.EventAspectHierarchy}})
	select {
	case event := <-events:
		t.Fatalf("child hierarchy=%+v", event)
	default:
	}
	b.Publish(ws, &dto.WorkspaceEvent{NodeID: target, ParentID: foreign, Type: dto.EventNodeMoved, Aspects: []dto.EventAspect{dto.EventAspectHierarchy}})
	if event := <-events; event.NodeID != target {
		t.Fatalf("own move=%+v", event)
	}
	// A subtree deletion carries the removed IDs, without probing each reader.
	b.Publish(ws, &dto.WorkspaceEvent{NodeID: foreign, Type: dto.EventNodeDeleted, AffectedIDs: []ksid.ID{target}})
	if got := <-events; got.Type != dto.EventNodeDeleted {
		t.Fatalf("subtree=%+v", got)
	}
}

func TestBrokerOverflowResync(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	ws, user, target := ksid.NewID(), ksid.NewID(), ksid.NewID()
	events, stop := b.SubscribeEvents(ws, EventInterest{Nodes: []ksid.ID{target}})
	t.Cleanup(stop)
	screen, stopScreen, err := b.Subscribe(ws, user)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stopScreen)
	for range 65 {
		b.Publish(ws, &dto.WorkspaceEvent{Type: dto.EventNodeUpdated, NodeID: target, Aspects: []dto.EventAspect{dto.EventAspectBody}, ActorID: user, RequestID: "private-request"})
	}
	got := <-events
	if got.Type != dto.EventResync || got.ChangeID == "" || got.NodeID != 0 || got.ActorID != 0 || got.RequestID != "" {
		t.Fatalf("gap=%+v", got)
	}
	frame := <-screen.C
	_, data, ok := bytes.Cut(frame, []byte("data: "))
	if !ok {
		t.Fatalf("frame=%s", frame)
	}
	var decoded dto.WorkspaceEvent
	if err := json.Unmarshal(bytes.TrimSpace(data), &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, got) {
		t.Fatalf("screen gap=%+v, MCP=%+v", decoded, got)
	}
	// Empty interests still receive scope-control events, so overflow cannot
	// hide membership revocation from a transport with no content subscriptions.
	control, stopControl := b.SubscribeEvents(ws, EventInterest{})
	t.Cleanup(stopControl)
	b.Publish(ws, &dto.WorkspaceEvent{})
	if got := <-control; got.NodeID != 0 {
		t.Fatalf("control=%+v", got)
	}
}

func TestSubscribeAndPublish(t *testing.T) {
	b := NewBroker()
	wsID := ksid.NewID()
	userID := ksid.NewID()

	sub, cleanup, err := b.Subscribe(wsID, userID)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if b.SubscriberCount(wsID) != 1 {
		t.Fatalf("want 1 subscriber, got %d", b.SubscriberCount(wsID))
	}

	evt := dto.WorkspaceEvent{
		Type:     dto.EventNodeUpdated,
		NodeID:   ksid.NewID(),
		ActorID:  userID,
		Modified: storage.Now(),
	}
	b.Publish(wsID, &evt)

	select {
	case msg := <-sub.C:
		if len(msg) == 0 {
			t.Fatal("received empty message")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestCleanupRemovesSubscriber(t *testing.T) {
	b := NewBroker()
	wsID := ksid.NewID()
	userID := ksid.NewID()

	_, cleanup, err := b.Subscribe(wsID, userID)
	if err != nil {
		t.Fatal(err)
	}
	cleanup()

	if b.SubscriberCount(wsID) != 0 {
		t.Fatalf("want 0 subscribers after cleanup, got %d", b.SubscriberCount(wsID))
	}
}

func TestMaxConnectionsPerUser(t *testing.T) {
	b := NewBroker()
	wsID := ksid.NewID()
	userID := ksid.NewID()

	cleanups := make([]func(), 0, maxConnsPerUser)
	for range maxConnsPerUser {
		_, cleanup, err := b.Subscribe(wsID, userID)
		if err != nil {
			t.Fatal(err)
		}
		cleanups = append(cleanups, cleanup)
	}

	// The next subscribe should fail.
	_, _, err := b.Subscribe(wsID, userID)
	if err == nil {
		t.Fatal("expected error for exceeding max connections")
	}

	// Different user should still be able to subscribe.
	otherUser := ksid.NewID()
	_, cleanup, err := b.Subscribe(wsID, otherUser)
	if err != nil {
		t.Fatal(err)
	}
	cleanup()

	for _, c := range cleanups {
		c()
	}
}

func TestPublishToEmptyWorkspace(t *testing.T) {
	b := NewBroker()
	wsID := ksid.NewID()
	// Should not panic.
	b.Publish(wsID, &dto.WorkspaceEvent{
		Type:     dto.EventNodeCreated,
		NodeID:   ksid.NewID(),
		ActorID:  ksid.NewID(),
		Modified: storage.Now(),
	})
}

func TestMultipleWorkspaces(t *testing.T) {
	b := NewBroker()
	ws1 := ksid.NewID()
	ws2 := ksid.NewID()
	user1 := ksid.NewID()
	user2 := ksid.NewID()

	sub1, cleanup1, err := b.Subscribe(ws1, user1)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup1()

	sub2, cleanup2, err := b.Subscribe(ws2, user2)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup2()

	// Publish to ws1 only.
	b.Publish(ws1, &dto.WorkspaceEvent{
		Type:     dto.EventNodeCreated,
		NodeID:   ksid.NewID(),
		ActorID:  user1,
		Modified: storage.Now(),
	})

	select {
	case <-sub1.C:
		// ok
	case <-time.After(time.Second):
		t.Fatal("sub1 should have received message")
	}

	select {
	case <-sub2.C:
		t.Fatal("sub2 should not have received message for ws1")
	default:
		// ok
	}
}

func TestSubscribeEvents(t *testing.T) {
	b := NewBroker()
	wsID := ksid.NewID()
	events, cleanup := b.SubscribeEvents(wsID, EventInterest{All: true})

	evt := dto.WorkspaceEvent{
		Type:     dto.EventNodeUpdated,
		NodeID:   ksid.NewID(),
		ActorID:  ksid.NewID(),
		Modified: storage.Now(),
	}
	b.Publish(wsID, &evt)

	select {
	case got := <-events:
		if got.NodeID != evt.NodeID || got.Type != evt.Type || got.Modified != evt.Modified {
			t.Fatalf("event = %#v, want %#v", got, evt)
		}
	case <-time.After(time.Second):
		t.Fatal("no structured event received")
	}

	// Another workspace's events are not delivered.
	b.Publish(ksid.NewID(), &evt)
	select {
	case got := <-events:
		t.Fatalf("received event for another workspace: %#v", got)
	case <-time.After(10 * time.Millisecond):
	}

	cleanup()
	if _, ok := <-events; ok {
		t.Fatal("cleanup did not close the event channel")
	}
}
