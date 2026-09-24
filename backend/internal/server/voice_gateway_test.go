// Tests voice gateway admission limits and reservation cleanup.

package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maruel/ksid"
)

type blockingVoiceBridge struct {
	mu      sync.Mutex
	next    int
	started chan struct{}
	proceed chan struct{}
}

func (b *blockingVoiceBridge) HandleOffer(ctx context.Context, _ string) (answer, sessionID string, err error) {
	b.mu.Lock()
	b.next++
	next := b.next
	b.mu.Unlock()
	if next == 2 {
		close(b.started)
		select {
		case <-b.proceed:
		case <-ctx.Done():
			return "", "", ctx.Err()
		}
	}
	return "answer", fmt.Sprintf("session-%d", next), nil
}

func (b *blockingVoiceBridge) Close(string) {}

type lifecycleVoiceBridge struct {
	mu                sync.Mutex
	next              int
	sessions          map[string]struct{}
	onClosed          func(string)
	closeBeforeReturn bool
}

func (b *lifecycleVoiceBridge) SetOnSessionClosed(fn func(string)) {
	b.mu.Lock()
	b.onClosed = fn
	b.mu.Unlock()
}

func (b *lifecycleVoiceBridge) HasSession(id string) bool {
	b.mu.Lock()
	_, ok := b.sessions[id]
	b.mu.Unlock()
	return ok
}

func (b *lifecycleVoiceBridge) HandleOffer(_ context.Context, _ string) (answer, sessionID string, err error) {
	b.mu.Lock()
	b.next++
	id := fmt.Sprintf("session-%d", b.next)
	b.sessions[id] = struct{}{}
	closeBeforeReturn := b.closeBeforeReturn
	b.mu.Unlock()
	if closeBeforeReturn {
		b.Close(id)
	}
	return "answer", id, nil
}

func (b *lifecycleVoiceBridge) Close(id string) {
	b.mu.Lock()
	_, ok := b.sessions[id]
	if ok {
		delete(b.sessions, id)
	}
	callback := b.onClosed
	b.mu.Unlock()
	if ok && callback != nil {
		callback(id)
	}
}

func admitVoiceAt(g *voiceGateway, id ksid.ID, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.admitLocked(id, now)
}

func TestVoiceGatewayAdmission(t *testing.T) {
	t.Parallel()
	g := newVoiceGateway(&testVoiceBridge{})
	now := time.Unix(1000, 0)
	for i := 1; i <= voiceMaxGlobal; i++ {
		if !admitVoiceAt(g, ksid.ID(i), now) {
			t.Fatalf("user %d denied before global cap", i)
		}
	}
	if admitVoiceAt(g, ksid.ID(voiceMaxGlobal+1), now) {
		t.Fatal("global cap admitted another session")
	}
	if admitVoiceAt(g, 1, now) {
		t.Fatal("per-user cap admitted another session")
	}
	g.release(1)
	if !admitVoiceAt(g, ksid.ID(voiceMaxGlobal+1), now) {
		t.Fatal("global slot was not released")
	}
	for i := 2; i <= voiceMaxGlobal+1; i++ {
		g.release(ksid.ID(i))
	}
	if g.active != 0 || g.inFlight != 0 {
		t.Fatalf("active/in-flight sessions = %d/%d, want zero", g.active, g.inFlight)
	}
}

func TestVoiceGatewayConcurrentOffers(t *testing.T) {
	t.Parallel()
	g := newVoiceGateway(&testVoiceBridge{})
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if g.admit(1) {
				accepted.Add(1)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 1 || g.inFlight != 1 {
		t.Fatalf("accepted = %d, in-flight = %d; want one reservation", accepted.Load(), g.inFlight)
	}
	g.release(1)
}

func TestVoiceGatewayCloseDuringReplacement(t *testing.T) {
	t.Parallel()
	bridge := &blockingVoiceBridge{started: make(chan struct{}), proceed: make(chan struct{})}
	g := newVoiceGateway(bridge)
	if !g.admit(1) {
		t.Fatal("initial offer denied")
	}
	first := &voiceReservation{gateway: g, owner: 1}
	ctx := context.WithValue(t.Context(), voiceReservationKey{}, first)
	_, oldID, err := g.HandleOffer(ctx, "m=audio 9")
	if err != nil {
		t.Fatalf("initial offer: %v", err)
	}
	if !g.admit(1) {
		t.Fatal("replacement offer denied")
	}
	second := &voiceReservation{gateway: g, owner: 1}
	ctx = context.WithValue(t.Context(), voiceReservationKey{}, second)
	type offerResult struct {
		id  string
		err error
	}
	result := make(chan offerResult, 1)
	go func() {
		_, id, err := g.HandleOffer(ctx, "m=audio 9")
		result <- offerResult{id: id, err: err}
	}()
	select {
	case <-bridge.started:
	case <-time.After(3 * time.Second):
		t.Fatal("replacement did not reach bridge")
	}
	if g.admit(1) {
		t.Fatal("concurrent offer admitted during replacement")
	}
	g.Close(oldID)
	close(bridge.proceed)
	select {
	case got := <-result:
		if got.err != nil || got.id != "session-2" {
			t.Fatalf("replacement result = %q, %v", got.id, got.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("replacement did not finish")
	}
	g.mu.Lock()
	active, inFlight, ownerCount := g.active, g.inFlight, g.activeByUser[1]
	g.mu.Unlock()
	if active != 1 || inFlight != 0 || ownerCount != 1 {
		t.Fatalf("active/in-flight/owner = %d/%d/%d, want 1/0/1", active, inFlight, ownerCount)
	}
	g.Close("session-2")
}

func TestVoiceGatewayRollingMinute(t *testing.T) {
	t.Parallel()
	now := time.Unix(1000, 0)
	t.Run("per user", func(t *testing.T) {
		g := newVoiceGateway(&testVoiceBridge{})
		for range voiceOffersUser {
			if !admitVoiceAt(g, 1, now) {
				t.Fatal("offer denied before per-user minute cap")
			}
			g.release(1)
		}
		if admitVoiceAt(g, 1, now.Add(time.Minute-time.Nanosecond)) {
			t.Fatal("offer admitted just before per-user window expired")
		}
		if !admitVoiceAt(g, 1, now.Add(time.Minute)) {
			t.Fatal("offer denied when per-user window expired")
		}
		g.release(1)
	})
	t.Run("global", func(t *testing.T) {
		g := newVoiceGateway(&testVoiceBridge{})
		for i := 1; i <= voiceOffersAll; i++ {
			id := ksid.ID(i)
			if !admitVoiceAt(g, id, now) {
				t.Fatalf("offer %d denied before global minute cap", i)
			}
			g.release(id)
		}
		nextID := ksid.ID(voiceOffersAll + 1)
		if admitVoiceAt(g, nextID, now.Add(time.Minute-time.Nanosecond)) {
			t.Fatal("offer admitted just before global window expired")
		}
		if !admitVoiceAt(g, nextID, now.Add(time.Minute)) {
			t.Fatal("offer denied when global window expired")
		}
		g.release(nextID)
	})
}

func TestVoiceGatewayMediaHangup(t *testing.T) {
	t.Parallel()
	bridge := &lifecycleVoiceBridge{sessions: make(map[string]struct{})}
	g := newVoiceGateway(bridge)
	now := time.Unix(1000, 0)
	for i := range 10 {
		if !admitVoiceAt(g, 1, now.Add(time.Duration(i)*time.Minute)) {
			t.Fatalf("offer %d denied after previous media hangup", i)
		}
		r := &voiceReservation{gateway: g, owner: 1}
		ctx := context.WithValue(t.Context(), voiceReservationKey{}, r)
		_, id, err := g.HandleOffer(ctx, "m=audio 9")
		if err != nil {
			t.Fatalf("offer %d: %v", i, err)
		}
		bridge.Close(id)
		g.mu.Lock()
		active, inFlight, ownerCount := g.active, g.inFlight, g.activeByUser[1]
		g.mu.Unlock()
		if active != 0 || inFlight != 0 || ownerCount != 0 {
			t.Fatalf("after hangup %d: active/in-flight/owner = %d/%d/%d", i, active, inFlight, ownerCount)
		}
	}
}

func TestVoiceGatewayEarlyMediaClose(t *testing.T) {
	t.Parallel()
	bridge := &lifecycleVoiceBridge{sessions: make(map[string]struct{})}
	g := newVoiceGateway(bridge)
	if !g.admit(1) {
		t.Fatal("initial offer denied")
	}
	first := &voiceReservation{gateway: g, owner: 1}
	ctx := context.WithValue(t.Context(), voiceReservationKey{}, first)
	_, oldID, err := g.HandleOffer(ctx, "m=audio 9")
	if err != nil {
		t.Fatalf("initial offer: %v", err)
	}
	bridge.mu.Lock()
	bridge.closeBeforeReturn = true
	bridge.mu.Unlock()
	if !g.admit(1) {
		t.Fatal("replacement offer denied")
	}
	second := &voiceReservation{gateway: g, owner: 1}
	ctx = context.WithValue(t.Context(), voiceReservationKey{}, second)
	if _, _, err := g.HandleOffer(ctx, "m=audio 9"); err == nil {
		t.Fatal("early closed replacement returned success")
	}
	second.release()
	if !bridge.HasSession(oldID) {
		t.Fatal("early replacement close destroyed original bridge session")
	}
	g.mu.Lock()
	active, inFlight, ownerCount := g.active, g.inFlight, g.activeByUser[1]
	g.mu.Unlock()
	if active != 1 || inFlight != 0 || ownerCount != 1 {
		t.Fatalf("active/in-flight/owner = %d/%d/%d, want 1/0/1", active, inFlight, ownerCount)
	}
	bridge.Close(oldID)
}

func TestVoiceGatewayCapacityAfterHangup(t *testing.T) {
	t.Parallel()
	bridge := &lifecycleVoiceBridge{sessions: make(map[string]struct{})}
	g := newVoiceGateway(bridge)
	var firstID string
	for i := 1; i <= voiceMaxGlobal; i++ {
		owner := ksid.ID(i)
		if !g.admit(owner) {
			t.Fatalf("user %d denied before capacity", i)
		}
		r := &voiceReservation{gateway: g, owner: owner}
		ctx := context.WithValue(t.Context(), voiceReservationKey{}, r)
		_, id, err := g.HandleOffer(ctx, "m=audio 9")
		if err != nil {
			t.Fatalf("user %d offer: %v", i, err)
		}
		if i == 1 {
			firstID = id
		}
	}
	if g.admit(ksid.ID(voiceMaxGlobal + 1)) {
		t.Fatal("offer admitted at full capacity")
	}
	bridge.Close(firstID)
	if !g.admit(ksid.ID(voiceMaxGlobal + 1)) {
		t.Fatal("offer denied after media hangup freed capacity")
	}
	g.release(ksid.ID(voiceMaxGlobal + 1))
}
