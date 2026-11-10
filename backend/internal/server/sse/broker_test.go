// Tests for the SSE broker.

package sse

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
)

func TestEventInterest(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	ws, target, foreign := ksid.NewID(), ksid.NewID(), ksid.NewID()
	ctx, stop := context.WithCancel(t.Context())
	events, err := b.Subscribe(ctx, ws, 0, &EventInterest{Nodes: []ksid.ID{target}, Catalog: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	for _, kind := range []dto.EventAspect{dto.EventAspectBody, dto.EventAspectRecords} {
		b.Publish(ws, &dto.WorkspaceEvent{NodeID: foreign, Type: dto.EventNodeUpdated, Aspects: []dto.EventAspect{kind}})
		select {
		case event := <-events.Events:
			t.Fatalf("uninterested event=%+v", event)
		default:
		}
	}
	b.Publish(ws, &dto.WorkspaceEvent{NodeID: target, Type: dto.EventNodeUpdated, Aspects: []dto.EventAspect{dto.EventAspectBody}})
	if got := <-events.Events; got.NodeID != target {
		t.Fatalf("target=%+v", got)
	}
	b.Publish(ws, &dto.WorkspaceEvent{NodeID: foreign, Type: dto.EventNodeUpdated, Aspects: []dto.EventAspect{dto.EventAspectCatalog}})
	if got := <-events.Events; got.NodeID != foreign {
		t.Fatalf("catalog=%+v", got)
	}
	// Moving the watched node changes its own ParentID; moving its child
	// only changes a child list absent from this resource.
	b.Publish(ws, &dto.WorkspaceEvent{NodeID: foreign, ParentID: target, Type: dto.EventNodeMoved, Aspects: []dto.EventAspect{dto.EventAspectHierarchy}})
	select {
	case event := <-events.Events:
		t.Fatalf("child hierarchy=%+v", event)
	default:
	}
	b.Publish(ws, &dto.WorkspaceEvent{NodeID: target, ParentID: foreign, Type: dto.EventNodeMoved, Aspects: []dto.EventAspect{dto.EventAspectHierarchy}})
	if event := <-events.Events; event.NodeID != target {
		t.Fatalf("own move=%+v", event)
	}
	// A subtree deletion carries the removed IDs, without probing each reader.
	b.Publish(ws, &dto.WorkspaceEvent{NodeID: foreign, Type: dto.EventNodeDeleted, AffectedIDs: []ksid.ID{target}})
	if got := <-events.Events; got.Type != dto.EventNodeDeleted {
		t.Fatalf("subtree=%+v", got)
	}
}

func TestBroker(t *testing.T) {
	t.Parallel()
	t.Run("Publish/OverflowResync", func(t *testing.T) {
		t.Parallel()
		b := NewBroker()
		ws, user, target := ksid.NewID(), ksid.NewID(), ksid.NewID()
		ctx, stop := context.WithCancel(t.Context())
		events, err := b.Subscribe(ctx, ws, 0, &EventInterest{Nodes: []ksid.ID{target}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(stop)
		screenCtx, stopScreen := context.WithCancel(t.Context())
		screen, err := b.Subscribe(screenCtx, ws, user, &EventInterest{All: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(stopScreen)
		for range 65 {
			b.Publish(ws, &dto.WorkspaceEvent{Type: dto.EventNodeUpdated, NodeID: target, Aspects: []dto.EventAspect{dto.EventAspectBody}, ActorID: user, RequestID: "private-request"})
		}
		got := <-events.Events
		if got.Type != dto.EventResync || got.ChangeID == "" || got.NodeID != 0 || got.ActorID != 0 || got.RequestID != "" {
			t.Fatalf("gap=%+v", got)
		}
		frame, err := (<-screen.Events).SSE()
		if err != nil {
			t.Fatal(err)
		}
		_, data, ok := bytes.Cut(frame, []byte("data: "))
		if !ok {
			t.Fatalf("frame=%s", frame)
		}
		var decoded dto.WorkspaceEvent
		if err := json.Unmarshal(bytes.TrimSpace(data), &decoded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded, got.WorkspaceEvent) {
			t.Fatalf("screen gap=%+v, MCP=%+v", decoded, got)
		}
		// Empty interests still receive scope-control events, so overflow cannot
		// hide membership revocation from a transport with no content subscriptions.
		controlCtx, stopControl := context.WithCancel(t.Context())
		control, err := b.Subscribe(controlCtx, ws, 0, &EventInterest{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(stopControl)
		b.Publish(ws, &dto.WorkspaceEvent{})
		if got := <-control.Events; got.NodeID != 0 {
			t.Fatalf("control=%+v", got)
		}
	})
	t.Run("ContextAdmissionAndScope", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			broker := NewBroker()
			ws, user := ksid.NewID(), ksid.NewID()
			screens := make([]<-chan *Change, 0, maxConnsPerUser)
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			for range maxConnsPerUser {
				events, err := broker.Subscribe(ctx, ws, user, &EventInterest{All: true})
				if err != nil {
					t.Fatal(err)
				}
				screens = append(screens, events.Events)
			}
			if _, err := broker.Subscribe(ctx, ws, user, &EventInterest{All: true}); err == nil {
				t.Fatal("screen admission exceeded")
			}
			// MCP admission is owned by its authenticated transport, not screen slots.
			internal, err := broker.Subscribe(ctx, ws, 0, &EventInterest{All: true})
			if err != nil {
				t.Fatal(err)
			}
			broker.Publish(ws+1, &dto.WorkspaceEvent{Type: dto.EventNodeCreated})
			select {
			case got := <-internal.Events:
				t.Fatalf("foreign workspace: %+v", got)
			default:
			}
			broker.Publish(ws, &dto.WorkspaceEvent{Type: dto.EventNodeCreated})
			change := <-internal.Events
			for _, screen := range screens {
				got := <-screen
				if got != change {
					t.Fatal("readers did not share immutable event")
				}
				frame, err := got.SSE()
				if err != nil {
					t.Fatal(err)
				}
				shared, err := change.SSE()
				if err != nil {
					t.Fatal(err)
				}
				if len(frame) == 0 || &frame[0] != &shared[0] {
					t.Fatal("screen encoding was not shared")
				}
			}
			cancel()
			synctest.Wait()
			for _, screen := range append(screens, internal.Events) {
				if _, ok := <-screen; ok {
					t.Fatal("cancel did not close feed")
				}
			}
			nextCtx, nextCancel := context.WithCancel(t.Context())
			t.Cleanup(nextCancel)
			if _, err := broker.Subscribe(nextCtx, ws, user, &EventInterest{All: true}); err != nil {
				t.Fatalf("cancellation retained admission: %v", err)
			}
			if _, err := broker.Subscribe(ctx, ws, user, &EventInterest{}); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled registration: %v", err)
			}
		})
	})
}

func TestPublishedChangeOwnership(t *testing.T) {
	t.Parallel()
	broker := NewBroker()
	ws := ksid.NewID()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	feed, err := broker.Subscribe(ctx, ws, 0, &EventInterest{All: true})
	if err != nil {
		t.Fatal(err)
	}
	event := &dto.WorkspaceEvent{Type: dto.EventNodeDeleted, NodeID: 11, Aspects: []dto.EventAspect{dto.EventAspectHierarchy}, AffectedIDs: []ksid.ID{12}, SummaryParents: []ksid.ID{13}}
	broker.Publish(ws, event)
	// Producer buffers may be reused once publication returns. Lazy encoding and
	// typed consumers must retain the same committed contents independently.
	event.Aspects[0] = dto.EventAspectBody
	event.AffectedIDs[0] = 99
	event.SummaryParents[0] = 99
	change := <-feed.Events
	if change.Aspects[0] != dto.EventAspectHierarchy || change.AffectedIDs[0] != 12 || change.SummaryParents[0] != 13 {
		t.Fatalf("published change aliases producer: %+v", change)
	}
	frame, err := change.SSE()
	if err != nil || !bytes.Contains(frame, []byte(`"summary_parents":["`+ksid.ID(13).String()+`"]`)) {
		t.Fatalf("lazy frame changed: %s,%v", frame, err)
	}
}
