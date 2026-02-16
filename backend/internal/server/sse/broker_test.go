package sse

import (
	"testing"
	"time"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
)

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
	b.Publish(wsID, evt)

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
	b.Publish(wsID, dto.WorkspaceEvent{
		Type:     dto.EventNodeCreated,
		NodeID:   ksid.NewID(),
		ActorID:  ksid.NewID(),
		Modified: storage.Now(),
	})
}

func TestPublishDropsWhenFull(t *testing.T) {
	b := NewBroker()
	wsID := ksid.NewID()
	userID := ksid.NewID()

	sub, cleanup, err := b.Subscribe(wsID, userID)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	evt := dto.WorkspaceEvent{
		Type:     dto.EventNodeUpdated,
		NodeID:   ksid.NewID(),
		ActorID:  userID,
		Modified: storage.Now(),
	}

	// Fill the buffer.
	for range subscriberBufSize {
		b.Publish(wsID, evt)
	}
	// This should be dropped, not block.
	b.Publish(wsID, evt)

	// Drain and count.
	var count int
	for range subscriberBufSize {
		select {
		case <-sub.C:
			count++
		default:
		}
	}
	if count != subscriberBufSize {
		t.Fatalf("want %d messages, got %d", subscriberBufSize, count)
	}
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
	b.Publish(ws1, dto.WorkspaceEvent{
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
