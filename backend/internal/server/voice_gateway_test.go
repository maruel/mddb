// Tests voice gateway admission limits and reservation cleanup.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"
	"github.com/maruel/mddb/backend/internal/testlog"

	voiceapi "github.com/gomodedev/gomode/voicegateway/api"
	voicev2 "github.com/gomodedev/gomode/voicegateway/api/v2"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
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
	_, ok := admitVoiceWait(g, id, now)
	return ok
}

func admitVoiceWait(g *voiceGateway, id ksid.ID, now time.Time) (time.Duration, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.admitLocked(id, now)
}

func admitVoice(g *voiceGateway, id ksid.ID) bool {
	_, ok := g.admit(id)
	return ok
}

type testTextVoiceBridge struct {
	testVoiceBridge

	started chan ksid.ID
}

func (b *testTextVoiceBridge) VoiceChoices() voicev2.VoiceChoicesResp {
	return voicev2.VoiceChoicesResp{Defaults: []voicev2.VoiceChoiceDefault{{Transport: voicev2.VoiceTransportText, ID: "text"}}, Choices: []voicev2.VoiceChoice{{ID: "text", Label: "Local text", Transports: []voicev2.VoiceTransport{voicev2.VoiceTransportText}}}}
}

func (b *testTextVoiceBridge) HandleChoiceOffer(ctx context.Context, sdp, id string) (answer, sessionID string, err error) {
	return b.HandleOffer(ctx, sdp)
}

func (b *testTextVoiceBridge) ServeTextSession(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	return b.ServeTextChoice(ctx, w, r, "text")
}

func (b *testTextVoiceBridge) ServeTextChoice(ctx context.Context, w http.ResponseWriter, r *http.Request, id string) error {
	if id != "text" {
		return fmt.Errorf("unexpected choice %s", id)
	}
	b.started <- reqctx.User(ctx).ID
	<-ctx.Done()
	return nil
}

func TestVoiceGateway(t *testing.T) {
	t.Run("admission", func(t *testing.T) {
		t.Parallel()
		g := newVoiceGateway(testlog.New(t), &testVoiceBridge{})
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
	})

	t.Run("concurrent offers", func(t *testing.T) {
		t.Parallel()
		g := newVoiceGateway(testlog.New(t), &testVoiceBridge{})
		var accepted atomic.Int32
		var wg sync.WaitGroup
		for range 32 {
			wg.Go(func() {
				if admitVoice(g, 1) {
					accepted.Add(1)
				}
			})
		}
		wg.Wait()
		if accepted.Load() != 1 || g.inFlight != 1 {
			t.Fatalf("accepted = %d, in-flight = %d; want one reservation", accepted.Load(), g.inFlight)
		}
		g.release(1)
	})

	t.Run("close during replacement", func(t *testing.T) {
		t.Parallel()
		bridge := &blockingVoiceBridge{started: make(chan struct{}), proceed: make(chan struct{})}
		g := newVoiceGateway(testlog.New(t), bridge)
		if !admitVoice(g, 1) {
			t.Fatal("initial offer denied")
		}
		first := &voiceReservation{gateway: g, owner: 1}
		ctx := context.WithValue(t.Context(), voiceReservationKey{}, first)
		_, oldID, err := g.HandleOffer(ctx, "m=audio 9")
		if err != nil {
			t.Fatalf("initial offer: %v", err)
		}
		if !admitVoice(g, 1) {
			t.Fatal("replacement offer denied")
		}
		second := &voiceReservation{gateway: g, owner: 1}
		ctx = context.WithValue(t.Context(), voiceReservationKey{}, second)
		type offerResult struct {
			id  string
			err error
		}
		result := make(chan offerResult, 1)
		var wg sync.WaitGroup
		wg.Go(func() {
			_, id, err := g.HandleOffer(ctx, "m=audio 9")
			result <- offerResult{id: id, err: err}
		})
		select {
		case <-bridge.started:
		case <-time.After(3 * time.Second):
			t.Fatal("replacement did not reach bridge")
		}
		if admitVoice(g, 1) {
			t.Fatal("concurrent offer admitted during replacement")
		}
		g.Close(oldID)
		close(bridge.proceed)
		select {
		case got := <-result:
			wg.Wait()
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
	})

	t.Run("rolling minute", func(t *testing.T) {
		t.Parallel()
		now := time.Unix(1000, 0)
		t.Run("per user", func(t *testing.T) {
			g := newVoiceGateway(testlog.New(t), &testVoiceBridge{})
			for range voiceOffersUser {
				if !admitVoiceAt(g, 1, now) {
					t.Fatal("offer denied before per-user minute cap")
				}
				g.release(1)
			}
			if wait, ok := admitVoiceWait(g, 1, now.Add(time.Minute-time.Nanosecond)); ok || wait != time.Nanosecond {
				t.Fatalf("got wait %v, ok %v; want a rejection until the oldest offer leaves the minute", wait, ok)
			}
			if !admitVoiceAt(g, 1, now.Add(time.Minute)) {
				t.Fatal("offer denied when per-user window expired")
			}
			g.release(1)
		})
		t.Run("global", func(t *testing.T) {
			g := newVoiceGateway(testlog.New(t), &testVoiceBridge{})
			for i := 1; i <= voiceOffersAll; i++ {
				id := ksid.ID(i)
				if !admitVoiceAt(g, id, now) {
					t.Fatalf("offer %d denied before global minute cap", i)
				}
				g.release(id)
			}
			nextID := ksid.ID(voiceOffersAll + 1)
			if wait, ok := admitVoiceWait(g, nextID, now.Add(time.Minute-time.Nanosecond)); ok || wait != time.Nanosecond {
				t.Fatalf("got wait %v, ok %v; want a rejection until the oldest offer leaves the minute", wait, ok)
			}
			if !admitVoiceAt(g, nextID, now.Add(time.Minute)) {
				t.Fatal("offer denied when global window expired")
			}
			g.release(nextID)
		})
	})

	t.Run("media hangup", func(t *testing.T) {
		t.Parallel()
		bridge := &lifecycleVoiceBridge{sessions: make(map[string]struct{})}
		g := newVoiceGateway(testlog.New(t), bridge)
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
	})

	t.Run("early media close", func(t *testing.T) {
		t.Parallel()
		bridge := &lifecycleVoiceBridge{sessions: make(map[string]struct{})}
		g := newVoiceGateway(testlog.New(t), bridge)
		if !admitVoice(g, 1) {
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
		if !admitVoice(g, 1) {
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
	})

	t.Run("capacity after hangup", func(t *testing.T) {
		t.Parallel()
		bridge := &lifecycleVoiceBridge{sessions: make(map[string]struct{})}
		g := newVoiceGateway(testlog.New(t), bridge)
		var firstID string
		for i := 1; i <= voiceMaxGlobal; i++ {
			owner := ksid.ID(i)
			if !admitVoice(g, owner) {
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
		if admitVoice(g, ksid.ID(voiceMaxGlobal+1)) {
			t.Fatal("offer admitted at full capacity")
		}
		bridge.Close(firstID)
		if !admitVoice(g, ksid.ID(voiceMaxGlobal+1)) {
			t.Fatal("offer denied after media hangup freed capacity")
		}
		g.release(ksid.ID(voiceMaxGlobal + 1))
	})

	t.Run("a full ticket table reports when the earliest ticket expires", func(t *testing.T) {
		t.Parallel()
		g := newVoiceGateway(testlog.New(t), &testTextVoiceBridge{})
		now := time.Now()
		for i := range 1024 {
			g.tickets[strconv.Itoa(i)] = voiceTextTicket{expiry: now.Add(30 * time.Second)}
		}
		g.tickets["0"] = voiceTextTicket{expiry: now.Add(5 * time.Second)}
		r := httptest.NewRequestWithContext(reqctx.WithUser(t.Context(), &identityv1.User{ID: ksid.NewID()}), http.MethodPost, "/api/voicegateway/v2/voice/text/ticket", strings.NewReader(`{"choiceID":"text"}`))
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		var body voiceapi.ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusTooManyRequests || body.Error.Code != voiceapi.CodeRateLimited || w.Header().Get("Retry-After") != "5" {
			t.Fatalf("got %d %s Retry-After %q, want 429 RATE_LIMITED after 5 s", w.Code, body.Error.Code, w.Header().Get("Retry-After"))
		}
	})

	t.Run("text sessions", func(t *testing.T) {
		t.Parallel()
		b := &testTextVoiceBridge{started: make(chan ksid.ID, 2)}
		g := newVoiceGateway(testlog.New(t), b)
		owner := &identityv1.User{ID: ksid.NewID()}
		ticket := func() string {
			r := httptest.NewRequestWithContext(reqctx.WithUser(t.Context(), owner), http.MethodPost, "/api/voicegateway/v2/voice/text/ticket", strings.NewReader(`{"choiceID":"text"}`))
			r.Header.Set("Origin", "https://mddb.example.com")
			w := httptest.NewRecorder()
			g.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("ticket: %d %s", w.Code, w.Body.String())
			}
			var out voicev2.VoiceTextTicketResp
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			return out.Ticket
		}
		browser := func(token, origin string) *http.Request {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/voicegateway/v2/voice/text/browser", http.NoBody)
			r.Header.Set("Connection", "Upgrade")
			r.Header.Set("Upgrade", "websocket")
			r.Header.Set("Sec-WebSocket-Version", "13")
			r.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			r.Header.Set("Sec-WebSocket-Protocol", "gomode.text.v2, gomode.ticket."+token)
			r.Header.Set("Origin", origin)
			return r
		}
		wrong := ticket()
		w := httptest.NewRecorder()
		g.serveBrowserText(w, browser(wrong, "https://evil.example.com"))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("wrong origin accepted: %d", w.Code)
		}
		for _, mismatch := range []string{"route", "subprotocol"} {
			t.Run("ticket rejects mismatched "+mismatch, func(t *testing.T) {
				token := ticket()
				r := browser(token, "https://mddb.example.com")
				if mismatch == "route" {
					r.URL.Path = "/api/voicegateway/v1/voice/text/browser"
				}
				r.Header.Set("Sec-WebSocket-Protocol", "gomode.text.v1, gomode.ticket."+token)
				w := httptest.NewRecorder()
				g.serveBrowserText(w, r)
				if w.Code != http.StatusUnauthorized {
					t.Fatalf("mismatched %s accepted: %d", mismatch, w.Code)
				}
			})
		}
		first := ticket()
		done := make(chan struct{})
		var wg sync.WaitGroup
		wg.Go(func() {
			g.serveBrowserText(httptest.NewRecorder(), browser(first, "https://mddb.example.com"))
			close(done)
		})
		select {
		case id := <-b.started:
			if id != owner.ID {
				t.Fatal("ticket owner lost")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("text session did not start")
		}
		g.mu.Lock()
		active := g.active
		var id string
		for sid := range g.sessions {
			id = sid
		}
		g.mu.Unlock()
		if active != 1 {
			t.Fatalf("active sessions: %d", active)
		}
		w = httptest.NewRecorder()
		g.serveBrowserText(w, browser(first, "https://mddb.example.com"))
		if w.Code != http.StatusUnauthorized {
			t.Fatal("ticket replay accepted")
		}
		stranger := &identityv1.User{ID: ksid.NewID()}
		r := httptest.NewRequestWithContext(reqctx.WithUser(t.Context(), stranger), http.MethodPost, "/api/voicegateway/v2/voice/rtc/"+id, http.NoBody)
		r.SetPathValue("sessionID", id)
		w = httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatal("another user accessed text session")
		}

		for _, protocol := range []string{"", "unexpected.protocol"} {
			badCtx, cancel := context.WithTimeout(t.Context(), time.Second)
			malformed := browser("unused", "")
			malformed = malformed.WithContext(reqctx.WithUser(badCtx, owner))
			malformed.URL.Path = "/api/voicegateway/v2/voice/text"
			malformed.URL.RawQuery = "choiceID=text"
			if protocol == "" {
				malformed.Header.Del("Upgrade")
			} else {
				malformed.Header.Set("Sec-WebSocket-Protocol", protocol)
			}
			w = httptest.NewRecorder()
			g.ServeHTTP(w, malformed)
			cancel()
			if w.Code != http.StatusBadRequest {
				t.Fatalf("malformed upgrade accepted: %d", w.Code)
			}
			g.mu.Lock()
			_, stillActive := g.sessions[id]
			g.mu.Unlock()
			if !stillActive {
				t.Fatal("malformed upgrade evicted the live session")
			}
		}

		g.Close(id)
		select {
		case <-done:
			wg.Wait()
		case <-time.After(5 * time.Second):
			t.Fatal("text session did not close")
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.active != 0 || g.inFlight != 0 || g.activeByUser[owner.ID] != 0 {
			t.Fatal("text session reservation leaked")
		}
	})
}

func TestWriteVoiceError(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status int
		v1, v2 voiceapi.ErrorCode
	}{
		{http.StatusBadRequest, voiceapi.CodeBadRequest, voiceapi.CodeBadRequest},
		{http.StatusUnauthorized, voiceapi.CodeBadRequest, voiceapi.CodeUnauthorized},
		{http.StatusNotFound, voiceapi.CodeNotFound, voiceapi.CodeNotFound},
		{http.StatusRequestEntityTooLarge, voiceapi.CodeBadRequest, voiceapi.CodeBadRequest},
		{http.StatusTooManyRequests, voiceapi.CodeBadRequest, voiceapi.CodeRateLimited},
		{http.StatusServiceUnavailable, voiceapi.CodeBadRequest, voiceapi.CodeVoiceBridgeUnavailable},
	} {
		for version, want := range map[voiceapi.ProtocolVersion]voiceapi.ErrorCode{voiceapi.ProtocolV1: tc.v1, voiceapi.ProtocolV2: tc.v2} {
			r := httptest.NewRequestWithContext(voiceapi.WithSessionProtocol(t.Context(), voiceapi.SessionProtocol{Version: version}), http.MethodPost, "/", http.NoBody)
			w := httptest.NewRecorder()
			writeVoiceError(w, r, tc.status, "message")
			var body voiceapi.ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || body.Error.Code != want {
				t.Errorf("%s %d: got %d %s, want %s", version, tc.status, w.Code, body.Error.Code, want)
			}
		}
	}
}
