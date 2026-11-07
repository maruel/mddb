// Retains bounded conversation workspace/node focus and rejects stale frontend selection publications.

package server

import (
	"container/list"
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/maruel/gomode/mcp"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
)

const maxVoiceFocusEntries = 1024

type voiceFocusKey struct {
	user    ksid.ID
	session string
}

type voiceFocus struct {
	key       voiceFocusKey
	workspace ksid.ID
	node      ksid.ID
	version   uint64
	browser   bool
}

type voiceFocusSource uint8

const (
	voiceFocusTool voiceFocusSource = iota
	voiceFocusBrowser
	voiceFocusRelease
)

// voiceFocusState is ephemeral. Versions never repeat during this server lifetime,
// including when a key is evicted and later recreated.
type voiceFocusState struct {
	mu          sync.Mutex
	entries     map[voiceFocusKey]*list.Element
	recent      list.List
	nextVersion uint64
	subscribers map[chan struct{}]voiceFocusKey
}

func newVoiceFocusState() *voiceFocusState {
	return &voiceFocusState{entries: make(map[voiceFocusKey]*list.Element), subscribers: make(map[chan struct{}]voiceFocusKey)}
}

func (f *voiceFocusState) workspace(key voiceFocusKey) ksid.ID {
	f.mu.Lock()
	defer f.mu.Unlock()
	if key.session == "" {
		return 0
	}
	if el := f.entries[key]; el != nil {
		return el.Value.(voiceFocus).workspace
	}
	return 0
}

func (f *voiceFocusState) lookup(key voiceFocusKey, workspace ksid.ID) voiceFocus {
	f.mu.Lock()
	defer f.mu.Unlock()
	if key.session != "" {
		if el := f.entries[key]; el != nil {
			v := el.Value.(voiceFocus)
			f.recent.MoveToFront(el)
			if workspace == 0 || v.workspace == workspace {
				return v
			}
		}
		return voiceFocus{}
	}
	var found voiceFocus
	for k, el := range f.entries {
		v := el.Value.(voiceFocus)
		if k.user != key.user || v.workspace == 0 || (workspace != 0 && v.workspace != workspace) {
			continue
		}
		if found.workspace != 0 && (found.workspace != v.workspace || found.node != v.node) {
			return voiceFocus{}
		}
		found = v
	}
	return found
}

func (f *voiceFocusState) set(ctx context.Context, key voiceFocusKey, workspace, node ksid.ID, expected *uint64, source voiceFocusSource) (voiceFocus, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return voiceFocus{}, false, err
	}
	var current voiceFocus
	if el := f.entries[key]; el != nil {
		current = el.Value.(voiceFocus)
	}
	if expected != nil && (current.version == 0 || *expected != current.version) {
		// Initial claims do not publish a selection. This prevents an old initial
		// request from acting after its entry was evicted and recreated.
		if current.version == 0 {
			f.nextVersion++
			current = voiceFocus{key: key, version: f.nextVersion}
			f.putLocked(current)
		}
		return current, false, nil
	}
	// MCP access can inspect another node without changing the node displayed
	// by a browser. A browser clear retains authority until its publisher retires.
	if current.browser && source == voiceFocusTool {
		return current, false, nil
	}
	// Clearing a retired workspace must not clear a newer workspace selection.
	if node == 0 && expected != nil && current.workspace != 0 && current.workspace != workspace {
		return current, true, nil
	}
	f.nextVersion++
	changed := current.workspace != workspace || current.node != node
	current = voiceFocus{key: key, workspace: workspace, node: node, version: f.nextVersion, browser: source == voiceFocusBrowser}
	f.putLocked(current)
	if changed {
		f.notifyLocked(key)
	}
	return current, true, nil
}

func (f *voiceFocusState) putLocked(v voiceFocus) {
	if el := f.entries[v.key]; el != nil {
		el.Value = v
		f.recent.MoveToFront(el)
		return
	}
	if len(f.entries) == maxVoiceFocusEntries {
		el := f.recent.Back()
		old := el.Value.(voiceFocus)
		delete(f.entries, old.key)
		f.recent.Remove(el)
		f.notifyLocked(old.key)
	}
	f.entries[v.key] = f.recent.PushFront(v)
}

func (f *voiceFocusState) notifyLocked(key voiceFocusKey) {
	for ch, sub := range f.subscribers {
		if sub.user != key.user || (sub.session != "" && sub.session != key.session) {
			continue
		}
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (f *voiceFocusState) subscribe(ctx context.Context, key voiceFocusKey) (events <-chan struct{}, unsubscribe func()) {
	ch := make(chan struct{}, 1)
	f.mu.Lock()
	f.subscribers[ch] = key
	f.mu.Unlock()
	cleanup := sync.OnceFunc(func() { f.mu.Lock(); delete(f.subscribers, ch); f.mu.Unlock() })
	stop := context.AfterFunc(ctx, cleanup)
	return ch, func() { stop(); cleanup() }
}

type voiceSessionKey struct{}

func withVoiceSession(ctx context.Context, meta *mcp.RequestMeta) (context.Context, error) {
	if meta == nil {
		return ctx, nil
	}
	if len(meta.Session) > 256 || strings.TrimSpace(meta.Session) != meta.Session {
		return ctx, mcp.ErrInvalidParams("openai/session must be at most 256 bytes")
	}
	return context.WithValue(ctx, voiceSessionKey{}, meta.Session), nil
}

func conversationSession(ctx context.Context) string {
	v, _ := ctx.Value(voiceSessionKey{}).(string)
	return v
}

func goModeFocus(svc *handlers.Services, cfg *handlers.Config, focus *voiceFocusState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var in dto.UpdateVoiceFocusRequest
		if !readAndDecodeBody(ctx, w, r, &in, cfg) {
			return
		}
		ws, err := ksid.Parse(r.PathValue("wsID"))
		if err != nil {
			writeJSONResponse[dto.UpdateVoiceFocusResponse](ctx, w, nil, dto.BadRequest("invalid workspace ID"))
			return
		}
		in.WsID = ws
		if err := in.Validate(); err != nil {
			writeJSONResponse[dto.UpdateVoiceFocusResponse](ctx, w, nil, err)
			return
		}
		if !in.NodeID.IsZero() {
			store, err := svc.FileStore.GetWorkspaceStore(ctx, ws)
			if err != nil {
				writeJSONResponse[dto.UpdateVoiceFocusResponse](ctx, w, nil, err)
				return
			}
			if _, err := store.ReadNodeSummary(ctx, in.NodeID); err != nil {
				writeJSONResponse[dto.UpdateVoiceFocusResponse](ctx, w, nil, dto.NotFound("node"))
				return
			}
		}
		user := reqctx.User(ctx)
		source := voiceFocusTool
		if in.BrowserSelection {
			source = voiceFocusBrowser
		}
		if in.Release {
			source = voiceFocusRelease
		}
		v, applied, err := focus.set(ctx, voiceFocusKey{user: user.ID, session: in.Session}, ws, in.NodeID, &in.Version, source)
		if errors.Is(err, context.Canceled) {
			return
		}
		writeJSONResponse(ctx, w, &dto.UpdateVoiceFocusResponse{Version: v.version, Applied: applied}, err)
	}
}
