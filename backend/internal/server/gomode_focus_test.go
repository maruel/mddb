// Tests bounded conversation focus leases, isolation, eviction, and cancellation.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/maruel/gomode/mcp"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/mcpschema"
)

func TestVoiceFocusHTTPAndConversationIsolation(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	token, ws := setupGoModeWorkspace(t, env, "focus-context@example.com")
	var a, b dto.CreatePageResponse
	for title, out := range map[string]*dto.CreatePageResponse{"A": &a, "B": &b} {
		if status := env.doJSON(t, http.MethodPost, "/api/v1/workspaces/"+ws.String()+"/nodes/0/page/create", dto.CreatePageRequest{Title: title, Content: title}, out, token); status != http.StatusOK {
			t.Fatalf("create = %d", status)
		}
	}
	endpoint := goModeMCPEndpoint + "?workspaceId=" + ws.String()
	publish := func(session string, node ksid.ID) dto.UpdateVoiceFocusResponse {
		in := dto.UpdateVoiceFocusRequest{Session: session, NodeID: node, BrowserSelection: true}
		var out dto.UpdateVoiceFocusResponse
		for range 2 {
			if status := env.doJSON(t, http.MethodPost, "/api/v1/workspaces/"+ws.String()+"/gomode/focus", in, &out, token); status != http.StatusOK {
				t.Fatalf("focus = %d", status)
			}
			if out.Applied {
				return out
			}
			in.Version = out.Version
		}
		t.Fatal("focus did not apply")
		return out
	}
	read := func(session string) mcpschema.ServiceItemsOutput {
		raw := callMCPNamedAt(t, env, token, endpoint, "resources/read", map[string]any{"uri": voiceItemsURI, "_meta": map[string]any{"openai/session": session}}, voiceItemsURI)
		var resourceRead mcp.ResourcesReadResult
		if err := json.Unmarshal(raw, &resourceRead); err != nil {
			t.Fatal(err)
		}
		var out mcpschema.ServiceItemsOutput
		if err := json.Unmarshal([]byte(resourceRead.Contents[0].Text), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	va := publish("a", a.ID)
	publish("b", b.ID)
	for session, want := range map[string]ksid.ID{"a": a.ID, "b": b.ID} {
		items := read(session)
		if len(items.Items) == 0 || !items.Items[0].Active || items.Items[0].ID != want.String() {
			t.Fatalf("session %s items = %+v", session, items)
		}
		raw := callMCPNamedAt(t, env, token, endpoint, "server/discover", map[string]any{"_meta": map[string]any{"openai/session": session}}, "")
		var discovered mcp.ServerDiscoverResult
		if err := json.Unmarshal(raw, &discovered); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(discovered.Instructions, "Selected node: mddb://workspaces/"+ws.String()+"/nodes/"+want.String()) {
			t.Fatalf("session %s instructions = %s", session, discovered.Instructions)
		}
	}
	for _, item := range read("").Items {
		if item.Active {
			t.Fatal("ambiguous sessionless focus marked active")
		}
	}
	// Reading another node and listing the root are useful MCP operations, but
	// cannot move a browser's authoritative visible selection.
	for name, args := range map[string]map[string]string{"node_read": {"nodeId": b.ID.String()}, "nodes_list": {}} {
		callMCPNamedAt(t, env, token, endpoint, "tools/call", map[string]any{"name": name, "arguments": args, "_meta": map[string]any{"openai/session": "a"}}, name)
		if got := read("a").Items[0]; got.ID != a.ID.String() || !got.Active {
			t.Fatalf("%s moved browser focus: %+v", name, got)
		}
	}
	var out dto.UpdateVoiceFocusResponse
	url := "/api/v1/workspaces/" + ws.String() + "/gomode/focus"
	if status := env.doJSON(t, http.MethodPost, url, dto.UpdateVoiceFocusRequest{Session: "a", NodeID: b.ID, Version: 0}, &out, token); status != http.StatusOK || out.Applied {
		t.Fatalf("stale publication = %d,%+v", status, out)
	}
	if read("a").Items[0].ID != a.ID.String() {
		t.Fatal("stale update changed focus")
	}
	if status := env.doJSON(t, http.MethodPost, url, dto.UpdateVoiceFocusRequest{Session: "a", NodeID: ksid.ID(999), Version: va.Version}, nil, token); status != http.StatusNotFound {
		t.Fatalf("missing node = %d", status)
	}
	if status := env.doJSON(t, http.MethodPost, url, dto.UpdateVoiceFocusRequest{Session: strings.Repeat("s", 257)}, nil, token); status != http.StatusBadRequest {
		t.Fatalf("oversized session = %d", status)
	}
	if status := env.doJSON(t, http.MethodPost, url, dto.UpdateVoiceFocusRequest{Session: "a"}, nil, ""); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d", status)
	}
	otherToken, _ := setupGoModeWorkspace(t, env, "focus-other@example.com")
	if status := env.doJSON(t, http.MethodPost, url, dto.UpdateVoiceFocusRequest{Session: "a", NodeID: b.ID}, nil, otherToken); status != http.StatusForbidden {
		t.Fatalf("unauthorized workspace = %d", status)
	}
	publish("a", 0)
	for _, item := range read("a").Items {
		if item.Active {
			t.Fatal("clear retained active node")
		}
	}
}

func TestVoiceFocusIsolationAndLease(t *testing.T) {
	t.Parallel()
	f := newVoiceFocusState()
	a := voiceFocusKey{user: 1, session: "a"}
	b := voiceFocusKey{user: 1, session: "b"}
	zero := uint64(0)
	claim, applied, err := f.set(t.Context(), a, 2, 3, &zero, voiceFocusTool)
	if err != nil || applied || claim.version == 0 || claim.workspace != 0 {
		t.Fatalf("claim = %+v,%v,%v", claim, applied, err)
	}
	v, applied, err := f.set(t.Context(), a, 2, 3, &claim.version, voiceFocusTool)
	if err != nil || !applied {
		t.Fatalf("publish = %+v,%v,%v", v, applied, err)
	}
	if _, applied, _ := f.set(t.Context(), a, 2, 4, &claim.version, voiceFocusTool); applied {
		t.Fatal("stale publication applied")
	}
	refreshed, _, _ := f.set(t.Context(), a, 2, 3, nil, voiceFocusTool)
	if refreshed.version == v.version {
		t.Fatal("same selection reused another owner's publication lease")
	}
	if _, applied, _ := f.set(t.Context(), a, 2, 0, &v.version, voiceFocusTool); applied {
		t.Fatal("retired owner cleared renewed selection")
	}
	if got := f.lookup(b, 0); got.workspace != 0 {
		t.Fatalf("conversation leaked: %+v", got)
	}
	_, _, _ = f.set(t.Context(), b, 2, 4, nil, voiceFocusTool)
	if got := f.lookup(voiceFocusKey{user: 1}, 2); got.node != 0 {
		t.Fatalf("ambiguous node = %+v", got)
	}
	if got := f.workspace(voiceFocusKey{user: 1}); got != 0 {
		t.Fatalf("sessionless workspace borrowed retained focus = %s", got)
	}
	_, _, _ = f.set(t.Context(), b, 5, 6, nil, voiceFocusTool)
	if got := f.workspace(voiceFocusKey{user: 1}); got != 0 {
		t.Fatalf("ambiguous workspace = %s", got)
	}
	if got := f.lookup(voiceFocusKey{user: 7, session: "a"}, 0); got.workspace != 0 {
		t.Fatal("user focus leaked")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := f.set(ctx, a, 2, 9, nil, voiceFocusTool); err == nil {
		t.Fatal("canceled write succeeded")
	}
	if got := f.lookup(a, 2); got.node != 3 {
		t.Fatalf("canceled/stale write changed focus: %+v", got)
	}
}

func TestVoiceWorkspaceExplicitRouting(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	token, first := setupGoModeWorkspace(t, env, "focus-workspaces@example.com")
	ws, err := env.wsService.Get(first)
	if err != nil {
		t.Fatal(err)
	}
	var second dto.WorkspaceResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/organizations/"+ws.OrganizationID.String()+"/workspaces", dto.CreateWorkspaceRequest{Name: "Second workspace"}, &second, token); status != http.StatusOK {
		t.Fatalf("create second workspace = %d", status)
	}
	user, err := env.userService.GetByEmail("focus-workspaces@example.com")
	if err != nil {
		t.Fatal(err)
	}
	focus := newVoiceFocusState()
	registry := &workspaceRegistry{svc: env.services, user: user, focus: focus}
	if _, err := registry.bindWorkspace(t.Context()); err == nil {
		t.Fatal("ambiguous native workspace borrowed shared history")
	}
	for session, id := range map[string]ksid.ID{"first-conversation": first, "second-conversation": second.ID} {
		var page dto.CreatePageResponse
		if status := env.doJSON(t, http.MethodPost, "/api/v1/workspaces/"+id.String()+"/nodes/0/page/create", dto.CreatePageRequest{Title: session}, &page, token); status != http.StatusOK {
			t.Fatalf("create scoped page = %d", status)
		}
		result := callMCPNamedAt(t, env, token, goModeMCPEndpoint, "tools/call", map[string]any{"name": "workspace_select", "arguments": map[string]string{"workspaceId": id.String()}, "_meta": map[string]any{"openai/session": session}}, "workspace_select")
		var out mcpToolOutcome
		if err := json.Unmarshal(result, &out); err != nil || out.IsError {
			t.Fatalf("select = %s,%v", result, err)
		}
		nativeRead := callMCPNamedAt(t, env, token, goModeMCPEndpoint, "tools/call", map[string]any{"name": "node_read", "arguments": map[string]string{"nodeId": page.ID.String()}, "_meta": map[string]any{"openai/session": session}}, "node_read")
		if err := json.Unmarshal(nativeRead, &out); err != nil || out.IsError {
			t.Fatalf("native node read = %s,%v", nativeRead, err)
		}
		// Each conversation's instructions resolve its own workspace, without a
		// shared workspace default. The HTTP resource read proves scoped routing.
		raw := callMCPNamedAt(t, env, token, goModeMCPEndpoint, "resources/read", map[string]any{"uri": voiceItemsURI, "_meta": map[string]any{"openai/session": session}}, voiceItemsURI)
		var resourceRead mcp.ResourcesReadResult
		if err := json.Unmarshal(raw, &resourceRead); err != nil {
			t.Fatal(err)
		}
		if len(resourceRead.Contents) != 1 {
			t.Fatalf("resource count = %d", len(resourceRead.Contents))
		}
		var items mcpschema.ServiceItemsOutput
		if err := json.Unmarshal([]byte(resourceRead.Contents[0].Text), &items); err != nil {
			t.Fatal(err)
		}
		if len(items.Items) != 1 || !items.Items[0].Active || items.Items[0].ID != page.ID.String() || items.Items[0].Reference != "mddb://workspaces/"+id.String()+"/nodes/"+page.ID.String() {
			t.Fatalf("workspace %s items = %+v", id, items)
		}
		// Resource catalogs without explicit workspace routing remain generic,
		// because shared ListResources has no conversation metadata seam.
		raw = callMCPNamedAt(t, env, token, goModeMCPEndpoint, "resources/list", map[string]any{"_meta": map[string]any{"openai/session": session}}, "")
		var listed mcp.ResourcesListResult
		if err := json.Unmarshal(raw, &listed); err != nil {
			t.Fatal(err)
		}
		if len(listed.Resources) != 1 || listed.Resources[0].URI != voiceItemsURI {
			t.Fatalf("unbound catalog = %+v", listed.Resources)
		}
	}
	_, _, _ = focus.set(t.Context(), voiceFocusKey{user: user.ID, session: "another-conversation"}, first, 0, nil, voiceFocusTool)
	if _, err := registry.bindWorkspace(t.Context()); err == nil {
		t.Fatal("sessionless operation borrowed a retained workspace")
	}
	// A scoped connection cannot select a different otherwise accessible workspace.
	raw := callMCPNamedAt(t, env, token, goModeMCPEndpoint+"?workspaceId="+first.String(), "tools/call", map[string]any{"name": "workspace_select", "arguments": map[string]string{"workspaceId": second.ID.String()}, "_meta": map[string]any{"openai/session": "first-conversation"}}, "workspace_select")
	var out mcpToolOutcome
	if err := json.Unmarshal(raw, &out); err != nil || !out.IsError {
		t.Fatalf("cross-scope selection = %s,%v", raw, err)
	}
}

func TestVoiceFocusBoundedAndNoABA(t *testing.T) {
	t.Parallel()
	f := newVoiceFocusState()
	key := voiceFocusKey{user: 1, session: "original"}
	old, _, _ := f.set(t.Context(), key, 2, 3, nil, voiceFocusTool)
	for i := range maxVoiceFocusEntries + 10 {
		_, _, _ = f.set(t.Context(), voiceFocusKey{user: 1, session: strconv.Itoa(i)}, 2, 3, nil, voiceFocusTool)
	}
	if len(f.entries) != maxVoiceFocusEntries {
		t.Fatalf("retained %d entries", len(f.entries))
	}
	claim, applied, _ := f.set(t.Context(), key, 2, 9, &old.version, voiceFocusTool)
	if applied || claim.version <= old.version || claim.workspace != 0 {
		t.Fatalf("evicted lease reused: %+v,%v", claim, applied)
	}
	for i := range maxVoiceFocusEntries * 2 {
		f.lookup(voiceFocusKey{user: 1, session: fmt.Sprintf("read-%d", i)}, 0)
	}
	if len(f.entries) != maxVoiceFocusEntries {
		t.Fatal("reads grew retained focus")
	}
}

func TestVoiceFocusBrowserAuthority(t *testing.T) {
	t.Parallel()
	f := newVoiceFocusState()
	key := voiceFocusKey{user: 1, session: "conversation"}
	v, _, _ := f.set(t.Context(), key, 2, 3, nil, voiceFocusBrowser)
	if _, applied, _ := f.set(t.Context(), key, 2, 4, nil, voiceFocusTool); applied {
		t.Fatal("tool replaced browser selection")
	}
	cleared, applied, err := f.set(t.Context(), key, 2, 0, &v.version, voiceFocusBrowser)
	if err != nil || !applied {
		t.Fatalf("browser clear = %+v,%v,%v", cleared, applied, err)
	}
	if _, applied, _ := f.set(t.Context(), key, 2, 4, nil, voiceFocusTool); applied {
		t.Fatal("tool replaced browser's empty selection")
	}
	released, applied, err := f.set(t.Context(), key, 2, 0, &cleared.version, voiceFocusRelease)
	if err != nil || !applied || released.browser {
		t.Fatalf("release = %+v,%v,%v", released, applied, err)
	}
	if _, applied, err := f.set(t.Context(), key, 2, 4, nil, voiceFocusTool); err != nil || !applied {
		t.Fatalf("native tool focus = %v,%v", applied, err)
	}
	if got := f.lookup(key, 2); got.node != 4 {
		t.Fatalf("native focus = %+v", got)
	}
}

func TestVoiceFocusConcurrentNotificationsAndSessionLimit(t *testing.T) {
	t.Parallel()
	f := newVoiceFocusState()
	ctx, cancel := context.WithCancel(t.Context())
	ch, cleanup := f.subscribe(ctx, voiceFocusKey{user: 1, session: "a"})
	var wg sync.WaitGroup
	for i := range uint64(16) {
		wg.Go(func() {
			for n := range uint64(100) {
				_, _, _ = f.set(t.Context(), voiceFocusKey{user: 1, session: "a"}, 2, ksid.ID(i*100+n+1), nil, voiceFocusTool)
				f.lookup(voiceFocusKey{user: 1, session: "a"}, 2)
			}
		})
	}
	wg.Wait()
	select {
	case <-ch:
	default:
		t.Fatal("focus did not notify")
	}
	cancel()
	cleanup()
	cleanup()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.subscribers) != 0 {
		t.Fatal("subscriber retained after cancellation")
	}
	meta := &mcp.RequestMeta{}
	meta.Session = string(make([]byte, 257))
	if _, err := withVoiceSession(t.Context(), meta); err == nil {
		t.Fatal("oversized conversation accepted")
	}
}
