// Bounds embedded voice sessions and binds their signaling routes to the mddb user.

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/maruel/gomode/voicegateway"
	voiceapi "github.com/maruel/gomode/voicegateway/api"
	voicev1 "github.com/maruel/gomode/voicegateway/api/v1"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
)

const (
	voiceMaxPerUser = 1
	voiceMaxGlobal  = 8
	voiceOffersUser = 3
	voiceOffersAll  = 20
	voiceMaxOffer   = 256 << 10
	voiceSessionTTL = 2 * time.Hour
)

type voiceSession struct {
	owner ksid.ID
	timer *time.Timer
}

type voiceSessionLifecycle interface {
	SetOnSessionClosed(func(string))
	HasSession(string) bool
}

type voicePendingSession struct {
	closed bool
}

type voiceGateway struct {
	bridge  voicegateway.MediaBridge
	handler http.Handler

	mu           sync.Mutex
	sessions     map[string]voiceSession
	pending      map[string]*voicePendingSession
	activeByUser map[ksid.ID]int
	inFlightUser map[ksid.ID]bool
	offersByUser map[ksid.ID][]time.Time
	globalOffers []time.Time
	active       int
	inFlight     int
}

func newVoiceGateway(bridge voicegateway.MediaBridge) *voiceGateway {
	g := &voiceGateway{
		bridge:       bridge,
		sessions:     make(map[string]voiceSession),
		pending:      make(map[string]*voicePendingSession),
		activeByUser: make(map[ksid.ID]int),
		inFlightUser: make(map[ksid.ID]bool),
		offersByUser: make(map[ksid.ID][]time.Time),
	}
	g.handler = voicegateway.NewEmbeddedHandler(func() voicegateway.MediaBridge { return g })
	if lifecycle, ok := bridge.(voiceSessionLifecycle); ok {
		lifecycle.SetOnSessionClosed(g.onSessionClosed)
	}
	return g
}

func (g *voiceGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u := reqctx.User(r.Context())
	if u == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if r.URL.Path == "/api/voicegateway/v1/voice/rtc/offer" {
		// Validate before replacing an existing session. A malformed retry must
		// not evict a call that is still working.
		body, err := io.ReadAll(io.LimitReader(r.Body, voiceMaxOffer+1))
		if err != nil {
			writeVoiceError(w, http.StatusBadRequest, "cannot read voice offer")
			return
		}
		if len(body) > voiceMaxOffer {
			writeVoiceError(w, http.StatusRequestEntityTooLarge, "voice offer is too large")
			return
		}
		var offer voicev1.VoiceRTCOfferReq
		if err := json.Unmarshal(body, &offer); err != nil || offer.SDP == "" {
			writeVoiceError(w, http.StatusBadRequest, "invalid voice offer")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		if !g.admit(u.ID) {
			w.Header().Set("Retry-After", "60")
			writeVoiceError(w, http.StatusTooManyRequests, "voice session limit reached")
			return
		}
		reservation := &voiceReservation{gateway: g, owner: u.ID}
		defer reservation.release()
		g.handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), voiceReservationKey{}, reservation)))
		return
	}
	id := r.PathValue("sessionID")
	g.mu.Lock()
	session, ok := g.sessions[id]
	g.mu.Unlock()
	if !ok || session.owner != u.ID {
		writeVoiceError(w, http.StatusNotFound, "voice session not found")
		return
	}
	g.handler.ServeHTTP(w, r)
}

func writeVoiceError(w http.ResponseWriter, status int, message string) {
	code := voiceapi.CodeBadRequest
	if status == http.StatusNotFound {
		code = voiceapi.CodeNotFound
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(voiceapi.ErrorResponse{
		Error: voiceapi.ErrorDetails{Code: code, Message: message},
	}); err != nil {
		slog.Error("encode voice error", "err", err)
	}
}

func (g *voiceGateway) admit(id ksid.ID) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.admitLocked(id, time.Now())
}

func (g *voiceGateway) admitLocked(id ksid.ID, now time.Time) bool {
	if g.inFlightUser[id] || g.active+g.inFlight >= voiceMaxGlobal {
		return false
	}
	g.globalOffers = recentVoiceOffers(g.globalOffers, now)
	for user, offers := range g.offersByUser {
		offers = recentVoiceOffers(offers, now)
		if len(offers) == 0 {
			delete(g.offersByUser, user)
		} else {
			g.offersByUser[user] = offers
		}
	}
	if len(g.offersByUser[id]) >= voiceOffersUser || len(g.globalOffers) >= voiceOffersAll {
		return false
	}
	g.offersByUser[id] = append(g.offersByUser[id], now)
	g.globalOffers = append(g.globalOffers, now)
	g.inFlight++
	g.inFlightUser[id] = true
	return true
}

func recentVoiceOffers(offers []time.Time, now time.Time) []time.Time {
	cutoff := now.Add(-time.Minute)
	for len(offers) > 0 && !offers[0].After(cutoff) {
		offers = offers[1:]
	}
	return offers
}

func (g *voiceGateway) release(id ksid.ID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inFlight--
	delete(g.inFlightUser, id)
}

func (g *voiceGateway) HandleOffer(ctx context.Context, sdp string) (answer, sessionID string, err error) {
	r := ctx.Value(voiceReservationKey{}).(*voiceReservation)
	answer, sessionID, err = g.bridge.HandleOffer(ctx, sdp)
	if err != nil {
		return "", "", err
	}
	// A peer may close while HandleOffer is returning. Track this ID before
	// probing bridge state so a concurrent callback cannot be missed.
	g.mu.Lock()
	g.pending[sessionID] = &voicePendingSession{}
	g.mu.Unlock()
	if lifecycle, ok := g.bridge.(voiceSessionLifecycle); ok && !lifecycle.HasSession(sessionID) {
		g.mu.Lock()
		g.pending[sessionID].closed = true
		g.mu.Unlock()
	}
	g.mu.Lock()
	pending := g.pending[sessionID]
	delete(g.pending, sessionID)
	if pending.closed {
		g.mu.Unlock()
		return "", "", errors.New("voice session closed during offer")
	}
	var oldID string
	if g.activeByUser[r.owner] >= voiceMaxPerUser {
		for id, session := range g.sessions {
			if session.owner != r.owner {
				continue
			}
			oldID = id
			session.timer.Stop()
			delete(g.sessions, id)
			g.active--
			g.activeByUser[r.owner]--
			break
		}
	}
	r.used = true
	g.sessions[sessionID] = voiceSession{
		owner: r.owner,
		timer: time.AfterFunc(voiceSessionTTL, func() { g.Close(sessionID) }),
	}
	g.active++
	g.activeByUser[r.owner]++
	g.mu.Unlock()
	if oldID != "" {
		g.bridge.Close(oldID)
	}
	g.release(r.owner)
	return answer, sessionID, nil
}

func (g *voiceGateway) Close(id string) {
	if g.forget(id) {
		g.bridge.Close(id)
	}
}

func (g *voiceGateway) onSessionClosed(id string) {
	g.mu.Lock()
	if pending, ok := g.pending[id]; ok {
		pending.closed = true
		g.mu.Unlock()
		return
	}
	g.mu.Unlock()
	g.forget(id)
}

func (g *voiceGateway) forget(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	session, ok := g.sessions[id]
	if ok {
		delete(g.sessions, id)
		g.active--
		g.activeByUser[session.owner]--
		if g.activeByUser[session.owner] == 0 {
			delete(g.activeByUser, session.owner)
		}
		session.timer.Stop()
	}
	return ok
}

func (g *voiceGateway) DiagnoseVoiceRTC(ctx context.Context, sessionID string, client *voicev1.VoiceRTCClientDiagnostics) voicev1.VoiceRTCDiagnosticsResp {
	if diagnostic, ok := g.bridge.(voicegateway.DiagnosticMediaBridge); ok {
		return diagnostic.DiagnoseVoiceRTC(ctx, sessionID, client)
	}
	return voicev1.VoiceRTCDiagnosticsResp{
		SessionID: sessionID,
		Issue:     voicev1.VoiceRTCConnectivityIssueVoiceBridgeUnavailable,
		Side:      voicev1.VoiceRTCConnectivitySideServer,
		Message:   "voice diagnostics are unavailable on this server",
		Client:    *client,
	}
}

type voiceReservationKey struct{}

type voiceReservation struct {
	gateway *voiceGateway
	owner   ksid.ID
	used    bool
}

func (r *voiceReservation) release() {
	if !r.used {
		r.gateway.release(r.owner)
		r.used = true
	}
}

var _ voicegateway.MediaBridge = (*voiceGateway)(nil)
var _ voicegateway.DiagnosticMediaBridge = (*voiceGateway)(nil)
