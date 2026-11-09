// Bounds embedded voice sessions and binds their signaling routes to the mddb user.

package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/storage/identity"

	"github.com/maruel/gomode/voicegateway"
	voiceapi "github.com/maruel/gomode/voicegateway/api"
	voicev2 "github.com/maruel/gomode/voicegateway/api/v2"
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
	owner  ksid.ID
	timer  *time.Timer
	cancel context.CancelFunc
}

type voiceSessionLifecycle interface {
	SetOnSessionClosed(callback func(string))
	HasSession(sessionID string) bool
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
	tickets      map[string]voiceTextTicket
}

func newVoiceGateway(bridge voicegateway.MediaBridge) *voiceGateway {
	g := &voiceGateway{
		bridge:       bridge,
		sessions:     make(map[string]voiceSession),
		pending:      make(map[string]*voicePendingSession),
		activeByUser: make(map[ksid.ID]int),
		inFlightUser: make(map[ksid.ID]bool),
		offersByUser: make(map[ksid.ID][]time.Time),
		tickets:      make(map[string]voiceTextTicket),
	}
	// Both transports pass through mddb admission and ownership checks.
	var media voicegateway.MediaBridge = g
	if choices, ok := bridge.(voicegateway.ChoiceMediaBridge); ok {
		media = &voiceChoiceGateway{voiceGateway: g, choices: choices}
	}
	var text voicegateway.TextSessionServer
	if backend, ok := bridge.(voiceTextBackend); ok {
		text = &voiceTextGateway{gateway: g, backend: backend}
	}
	g.handler = voicegateway.NewEmbeddedHandler(func() voicegateway.MediaBridge { return media }, text)
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
	prefix := "/api/voicegateway/" + voiceProtocol(r).String() + "/voice"
	if r.URL.Path == prefix+"/text/ticket" {
		g.serveTextTicket(w, r, u)
		return
	}
	if r.URL.Path == prefix+"/text" {
		g.handler.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == prefix+"/choices" {
		g.handler.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == prefix+"/rtc/offer" {
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
		var offer voicev2.VoiceRTCOfferReq
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

// serveTextTicket binds a single-use browser ticket to the authenticated owner.
func (g *voiceGateway) serveTextTicket(w http.ResponseWriter, r *http.Request, u *identity.User) {
	var in voicev2.VoiceTextTicketReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		writeVoiceError(w, http.StatusBadRequest, "invalid text ticket request")
		return
	}
	choices, ok := g.bridge.(voicegateway.ChoiceTextSessionServer)
	if !ok {
		writeVoiceError(w, http.StatusServiceUnavailable, "text voice sessions unavailable")
		return
	}
	catalog := choices.VoiceChoices()
	choice := in.ChoiceID
	if choice == "" {
		choice = catalog.DefaultChoiceID(voicev2.VoiceTransportText)
	}
	valid := false
	for _, ch := range catalog.Choices {
		if ch.ID == choice {
			for _, transport := range ch.Transports {
				if transport == voicev2.VoiceTransportText {
					valid = true
				}
			}
		}
	}
	if !valid {
		writeVoiceError(w, http.StatusBadRequest, "choice does not support text sessions")
		return
	}
	now := time.Now()
	g.mu.Lock()
	for id, entry := range g.tickets {
		if !now.Before(entry.expiry) {
			delete(g.tickets, id)
		}
	}
	if len(g.tickets) >= 1024 {
		g.mu.Unlock()
		writeVoiceError(w, http.StatusTooManyRequests, "too many pending voice tickets")
		return
	}
	ticket := rand.Text()
	g.tickets[ticket] = voiceTextTicket{session: reqctx.Session(r.Context()), owner: u.ID, origin: r.Header.Get("Origin"), choice: choice, protocol: voiceProtocol(r), expiry: now.Add(30 * time.Second)}
	g.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(voicev2.VoiceTextTicketResp{Ticket: ticket}); err != nil {
		slog.WarnContext(r.Context(), "write voice ticket", "err", err)
	}
}

// serveBrowserText redeems an owner-bound ticket without a login bearer in the URL.
func (g *voiceGateway) serveBrowserText(w http.ResponseWriter, r *http.Request) {
	version := voiceProtocol(r)
	ticket := ""
	valid := false
	for _, header := range r.Header.Values("Sec-WebSocket-Protocol") {
		for protocol := range strings.SplitSeq(header, ",") {
			protocol = strings.TrimSpace(protocol)
			if protocol == version.TextSubprotocol() {
				valid = true
			}
			if value, ok := strings.CutPrefix(protocol, "gomode.ticket."); ok {
				if ticket != "" {
					writeVoiceError(w, http.StatusUnauthorized, "invalid voice ticket")
					return
				}
				ticket = value
			}
		}
	}
	g.mu.Lock()
	entry, ok := g.tickets[ticket]
	delete(g.tickets, ticket)
	g.mu.Unlock()
	if !valid || !ok || entry.protocol != version || !time.Now().Before(entry.expiry) || entry.origin != r.Header.Get("Origin") {
		writeVoiceError(w, http.StatusUnauthorized, "invalid or expired voice ticket")
		return
	}
	ctx := reqctx.WithUser(r.Context(), &identity.User{ID: entry.owner})
	if entry.session != nil {
		ctx = reqctx.WithSession(ctx, entry.session)
		ctx = reqctx.WithSessionID(ctx, entry.session.ID)
	}
	r = r.Clone(ctx)
	r.URL.Path = "/api/voicegateway/" + version.String() + "/voice/text"
	q := r.URL.Query()
	q.Set("choiceID", entry.choice)
	r.URL.RawQuery = q.Encode()
	r.Header.Del("Origin")
	r.Header.Set("Sec-WebSocket-Protocol", version.TextSubprotocol())
	g.handler.ServeHTTP(w, r)
}

func voiceProtocol(r *http.Request) voiceapi.ProtocolVersion {
	if strings.HasPrefix(r.URL.Path, "/api/voicegateway/v1/") {
		return voiceapi.ProtocolV1
	}
	return voiceapi.ProtocolV2
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
	answer, sessionID, err = g.bridge.HandleOffer(ctx, sdp)
	return g.acceptOffer(ctx, answer, sessionID, err)
}

func (g *voiceGateway) acceptOffer(ctx context.Context, answer, sessionID string, err error) (sdpAnswer, id string, offerErr error) {
	if err != nil {
		return "", "", err
	}
	r := ctx.Value(voiceReservationKey{}).(*voiceReservation)
	// A peer may close while HandleOffer is returning. Track this ID before
	// probing bridge state so a concurrent callback cannot be missed.
	g.mu.Lock()
	g.pending[sessionID] = &voicePendingSession{}
	g.mu.Unlock()
	textCancel, text := ctx.Value(voiceTextCancelKey{}).(context.CancelFunc)
	if lifecycle, ok := g.bridge.(voiceSessionLifecycle); ok && !text && !lifecycle.HasSession(sessionID) {
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
	var oldCancel context.CancelFunc
	if g.activeByUser[r.owner] >= voiceMaxPerUser {
		for id, session := range g.sessions {
			if session.owner != r.owner {
				continue
			}
			oldID = id
			oldCancel = session.cancel
			session.timer.Stop()
			delete(g.sessions, id)
			g.active--
			g.activeByUser[r.owner]--
			break
		}
	}
	r.used = true
	g.sessions[sessionID] = voiceSession{
		owner:  r.owner,
		cancel: textCancel,
		timer:  time.AfterFunc(voiceSessionTTL, func() { g.Close(sessionID) }),
	}
	g.active++
	g.activeByUser[r.owner]++
	g.mu.Unlock()
	if oldID != "" {
		if oldCancel != nil {
			oldCancel()
		} else {
			g.bridge.Close(oldID)
		}
	}
	g.release(r.owner)
	return answer, sessionID, nil
}

func (g *voiceGateway) Close(id string) {
	g.mu.Lock()
	cancel := g.sessions[id].cancel
	g.mu.Unlock()
	if g.forget(id) {
		if cancel != nil {
			cancel()
		} else {
			g.bridge.Close(id)
		}
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

// VoiceRTCError forwards retained session errors for the published v1 diagnostics.
func (g *voiceGateway) VoiceRTCError(sessionID string) string {
	if provider, ok := g.bridge.(voicegateway.VoiceRTCErrorProvider); ok {
		return provider.VoiceRTCError(sessionID)
	}
	return ""
}

func (g *voiceGateway) DiagnoseVoiceRTC(ctx context.Context, sessionID string, client *voicev2.VoiceRTCClientDiagnostics) voicev2.VoiceRTCDiagnosticsResp {
	if diagnostic, ok := g.bridge.(voicegateway.DiagnosticMediaBridge); ok {
		return diagnostic.DiagnoseVoiceRTC(ctx, sessionID, client)
	}
	return voicev2.VoiceRTCDiagnosticsResp{
		SessionID: sessionID,
		Issue:     voicev2.VoiceRTCConnectivityIssueVoiceBridgeUnavailable,
		Side:      voicev2.VoiceRTCConnectivitySideServer,
		Message:   "voice diagnostics are unavailable on this server",
		Client:    *client,
	}
}

type voiceReservationKey struct{}

// voiceChoiceGateway preserves optional choice routing through session admission.
type voiceChoiceGateway struct {
	*voiceGateway

	choices voicegateway.ChoiceMediaBridge
}

func (g *voiceChoiceGateway) VoiceChoices() voicev2.VoiceChoicesResp {
	return g.choices.VoiceChoices()
}

func (g *voiceChoiceGateway) HandleChoiceOffer(ctx context.Context, sdp, choiceID string) (answer, sessionID string, err error) {
	answer, sessionID, err = g.choices.HandleChoiceOffer(ctx, sdp, choiceID)
	return g.acceptOffer(ctx, answer, sessionID, err)
}

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

type voiceTextTicket struct {
	session  *identity.Session
	owner    ksid.ID
	origin   string
	choice   string
	protocol voiceapi.ProtocolVersion
	expiry   time.Time
}

type voiceTextCancelKey struct{}

// voiceTextGateway applies the shared capacity and replacement limits to text sessions.
type voiceTextGateway struct {
	gateway *voiceGateway
	backend voiceTextBackend
}

func (g *voiceTextGateway) VoiceChoices() voicev2.VoiceChoicesResp {
	return g.backend.VoiceChoices()
}

func (g *voiceTextGateway) ServeTextSession(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	return g.ServeTextChoice(ctx, w, r, "")
}

func (g *voiceTextGateway) ServeTextChoice(ctx context.Context, w http.ResponseWriter, r *http.Request, choice string) error {
	// Reject malformed upgrade requests before admission can replace a live call.
	upgrade := false
	for _, header := range r.Header.Values("Connection") {
		for token := range strings.SplitSeq(header, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				upgrade = true
			}
		}
	}
	protocol := false
	for _, header := range r.Header.Values("Sec-WebSocket-Protocol") {
		for token := range strings.SplitSeq(header, ",") {
			if strings.TrimSpace(token) == voiceapi.SessionProtocolFromContext(ctx).Version.TextSubprotocol() {
				protocol = true
			}
		}
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key")))
	origin, originErr := url.Parse(r.Header.Get("Origin"))
	originAllowed := r.Header.Get("Origin") == "" || (originErr == nil && strings.EqualFold(origin.Host, r.Host))
	if !r.ProtoAtLeast(1, 1) || !originAllowed || len(r.Header.Values("Sec-WebSocket-Key")) != 1 || r.Method != http.MethodGet || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || !upgrade || r.Header.Get("Sec-WebSocket-Version") != "13" || err != nil || len(key) != 16 || !protocol {
		writeVoiceError(w, http.StatusBadRequest, "invalid text WebSocket upgrade")
		return nil
	}
	owner := reqctx.User(ctx).ID
	if !g.gateway.admit(owner) {
		writeVoiceError(w, http.StatusTooManyRequests, "voice session limit reached")
		return nil
	}
	reservation := &voiceReservation{gateway: g.gateway, owner: owner}
	defer reservation.release()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx = context.WithValue(ctx, voiceReservationKey{}, reservation)
	ctx = context.WithValue(ctx, voiceTextCancelKey{}, cancel)
	id := rand.Text()
	if _, _, err := g.gateway.acceptOffer(ctx, "", id, nil); err != nil {
		writeVoiceError(w, http.StatusServiceUnavailable, "text session unavailable")
		return err
	}
	defer g.gateway.Close(id)
	return g.backend.ServeTextChoice(ctx, w, r, choice)
}

// voiceTextBackend supports named text sessions and the default text session.
type voiceTextBackend interface {
	voicegateway.TextSessionServer
	voicegateway.ChoiceTextSessionServer
}
