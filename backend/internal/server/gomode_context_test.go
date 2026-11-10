// Tests pre-read dependency handoff, snapshot reconciliation, scope isolation, and lease-owned feed teardown.

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/maruel/gomode"
	"github.com/maruel/gomode/mcp"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/mcpschema"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestVoiceContext(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	_, ws := setupGoModeWorkspace(t, env, "watched-context@example.com")
	user, err := env.services.User.GetByEmail("watched-context@example.com")
	if err != nil {
		t.Fatal(err)
	}
	var login *identity.Session
	for value := range env.services.Session.GetActiveByUserID(user.ID) {
		login = value
		break
	}
	if login == nil {
		t.Fatal("workspace setup did not create an authenticated session")
	}
	store, err := env.services.FileStore.GetWorkspaceStore(t.Context(), ws)
	if err != nil {
		t.Fatal(err)
	}
	page, err := store.CreatePageUnderParent(t.Context(), 0, "Watched", "original", handlers.GitAuthor(user))
	if err != nil {
		t.Fatal(err)
	}
	reg := &workspaceRegistry{svc: env.services, nodeH: &handlers.NodeHandler{Svc: env.services}, user: user, wsID: ws, role: identity.WSRoleAdmin, focus: newVoiceFocusState()}
	ctx := reqctx.WithSession(reqctx.WithSessionID(t.Context(), login.ID), login)
	ctx = context.WithValue(ctx, voiceSessionKey{}, "conversation")
	uri := reg.nodeURI(page.ID)

	t.Run("PublicResourceAndToolAcknowledgment", func(t *testing.T) {
		result, err := reg.CallTool(ctx, &mcp.ToolsCallParams{Meta: mcp.RequestMeta{Session: "conversation"}, Name: "node_read", Arguments: json.RawMessage(`{"nodeId":"` + page.ID.String() + `"}`)})
		if err != nil || result.IsError {
			t.Fatalf("read: %+v, %v", result, err)
		}
		encoded, err := json.Marshal(result.Meta[gomode.ConversationContextMetaKey])
		if err != nil {
			t.Fatal(err)
		}
		var update gomode.ConversationContext
		if err := json.Unmarshal(encoded, &update); err != nil {
			t.Fatal(err)
		}
		if update.Content == nil || len(update.Dependencies) == 0 {
			t.Fatalf("missing reconciliation: %+v", update)
		}
		resource, err := reg.ReadResource(ctx, &mcp.ResourcesReadParams{URI: voiceContextURI, Meta: mcp.RequestMeta{Session: "conversation"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(resource.Contents) != 1 || resource.Contents[0].Text != *update.Content {
			t.Fatalf("different public projections: %+v versus %+v", resource, update)
		}
		var public map[string]json.RawMessage
		if err := json.Unmarshal([]byte(resource.Contents[0].Text), &public); err != nil {
			t.Fatal(err)
		}
		if public["epoch"] != nil || public["dependencies"] != nil || public["version"] != nil {
			t.Fatalf("private transport in public context: %s", resource.Contents[0].Text)
		}
		var model mcpschema.ModelContext
		if err := json.Unmarshal([]byte(*update.Content), &model); err != nil {
			t.Fatal(err)
		}
		if len(model.Resources) != 1 || model.Resources[0].URI != uri || model.Resources[0].State != mcpschema.ResourceContextRead {
			t.Fatalf("host projection lost actual watched read: %+v", model)
		}
	})

	t.Run("MutationBetweenReadAndAcknowledgement", func(t *testing.T) {
		updates, unsubscribe := reg.focus.subscribe(ctx, voiceFocusKey{user: user.ID, login: login.ID, session: "conversation"})
		t.Cleanup(unsubscribe)
		read, err := reg.beginResourceRead(ctx, uri, page.ID, 0, watchNode)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(read.context.cancel)
		<-updates
		snap, _, err := reg.readSnapshot(ctx, page.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		// The canonical body revision is unchanged, but a real metadata
		// mutation after the read makes its rich snapshot stale.
		if _, err := reg.nodeH.UpdatePageFrontmatter(ctx, ws, user, &dto.UpdatePageFrontmatterRequest{ID: page.ID, Icon: "new-icon"}); err != nil {
			t.Fatal(err)
		}
		<-updates
		reg.finishResourceRead(ctx, read, snap.Revision, false)
		out, err := reg.conversationState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Resources) != 1 || out.Resources[0].State != mcpschema.ResourceContextChanged || out.Resources[0].Revision != snap.Revision || out.Resources[0].ChangeID == "" {
			t.Fatalf("late read lost a same-CAS metadata mutation: %+v", out)
		}
		// Rereading after that commit reconciles the invalidation without
		// another body/catalog scan on the event path.
		next, err := reg.beginResourceRead(ctx, uri, page.ID, 0, watchNode)
		if err != nil {
			t.Fatal(err)
		}
		latest, _, err := reg.readSnapshot(ctx, page.ID, "")
		if err != nil || latest.Node.Icon != "new-icon" {
			t.Fatalf("metadata read: %+v, %v", latest.Node, err)
		}
		reg.finishResourceRead(ctx, next, latest.Revision, false)
		reg.finishResourceRead(ctx, read, "superseded", false)
		out, err = reg.conversationState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if out.Resources[0].State != mcpschema.ResourceContextRead || out.Resources[0].Revision != latest.Revision {
			t.Fatalf("superseded read replaced current reconciliation: %+v", out)
		}
		if _, _, err := reg.focus.set(ctx, voiceFocusKey{user: user.ID, login: login.ID, session: "conversation"}, ws, 0, nil, voiceFocusRelease); err != nil {
			t.Fatal(err)
		}
		if _, ok := <-read.context.feed.Events; ok {
			t.Fatal("retired context retained its broker registration")
		}
	})
	t.Run("LoginIsolationAndRelease", func(t *testing.T) {
		read, err := reg.beginResourceRead(ctx, uri, page.ID, 0, watchNode)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(read.context.cancel)
		other := reqctx.WithSessionID(ctx, ksid.NewID())
		if out, err := reg.conversationState(other); err != nil || len(out.Resources) != 0 || out.Focus != nil {
			t.Fatalf("another login inherited conversation context: %+v", out)
		}
		key := voiceFocusKey{user: user.ID, login: login.ID, session: "conversation"}
		if _, _, err := reg.focus.set(ctx, key, ws, 0, nil, voiceFocusRelease); err != nil {
			t.Fatal(err)
		}
		if _, ok := <-read.context.feed.Events; ok {
			t.Fatal("lease release did not unsubscribe before test cleanup")
		}
		reg.finishResourceRead(ctx, read, "late", false)
		if out, err := reg.conversationState(ctx); err != nil || len(out.Resources) != 0 {
			t.Fatalf("late result resurrected a retired dependency: %+v", out)
		}
	})
	t.Run("ReadBeforeInitialFocusClaim", func(t *testing.T) {
		local := *reg
		local.focus = newVoiceFocusState()
		read, err := local.beginResourceRead(ctx, uri, page.ID, 0, watchNode)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(read.context.cancel)
		handler := goModeFocus(env.services, nil, local.focus)
		post := func(in dto.UpdateVoiceFocusRequest) dto.UpdateVoiceFocusResponse {
			body, err := json.Marshal(in)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/focus", bytes.NewReader(body)).WithContext(reqctx.WithUser(ctx, user))
			req.SetPathValue("wsID", ws.String())
			response := httptest.NewRecorder()
			handler(response, req)
			if response.Code != http.StatusOK {
				t.Fatalf("focus response: %d %s", response.Code, response.Body.String())
			}
			var out dto.UpdateVoiceFocusResponse
			if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			return out
		}
		claim := post(dto.UpdateVoiceFocusRequest{Session: "conversation", NodeID: page.ID, BrowserSelection: true})
		if claim.Applied || claim.Version == 0 {
			t.Fatalf("initial claim applied selection: %+v", claim)
		}
		key := voiceFocusKey{user: user.ID, login: login.ID, session: "conversation"}
		if current := local.focus.lookup(key, ws); current.context != read.context || current.node != 0 {
			t.Fatalf("read-first claim orphaned feed or applied focus: %+v", current)
		}
		applied := post(dto.UpdateVoiceFocusRequest{Session: "conversation", NodeID: page.ID, BrowserSelection: true, Version: claim.Version})
		if !applied.Applied || applied.Version <= claim.Version {
			t.Fatalf("claim retry: %+v", applied)
		}
		next, err := local.beginResourceRead(ctx, uri, page.ID, 0, watchNode)
		if err != nil || next.context != read.context {
			t.Fatalf("duplicate context after claim: %p vs %p,%v", next.context, read.context, err)
		}
		released := post(dto.UpdateVoiceFocusRequest{Session: "conversation", Release: true, Version: applied.Version})
		if !released.Applied {
			t.Fatalf("release: %+v", released)
		}
		for range read.context.feed.Events {
		}
		if _, ok := <-read.context.feed.Events; ok {
			t.Fatal("released read-first feed remains open before cleanup")
		}
	})

	t.Run("ContextReadDoesNotInvalidateItems", func(t *testing.T) {
		streamCtx, cancel := context.WithCancel(ctx)
		changes, err := reg.SubscribeResourceUpdates(streamCtx, &mcp.SubscriptionsListenParams{Notifications: mcp.SubscriptionFilter{ResourceSubscriptions: []string{voiceItemsURI, voiceContextURI}}})
		if err != nil {
			t.Fatal(err)
		}
		updates := make(chan mcp.ResourceUpdate, 1)
		var worker sync.WaitGroup
		worker.Go(func() {
			for update, err := range changes {
				if err != nil {
					t.Error(err)
					return
				}
				updates <- update
				return
			}
		})
		t.Cleanup(func() { cancel(); worker.Wait() })
		ticket, err := reg.beginResourceRead(ctx, uri, page.ID, 0, watchNode)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(ticket.context.cancel)
		update := <-updates
		if !slices.Equal(update.ResourceURIs, []string{voiceContextURI}) || update.ResourcesListChanged {
			t.Fatalf("dependency read invalidated item catalog: %+v", update)
		}
	})

	t.Run("RecreationEvictionAndOverflow", func(t *testing.T) {
		first, err := reg.beginResourceRead(ctx, uri, page.ID, 0, watchNode)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(first.context.cancel)
		old, err := reg.conversationState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		key := voiceFocusKey{user: user.ID, login: login.ID, session: "conversation"}
		if _, _, err := reg.focus.set(ctx, key, ws, 0, nil, voiceFocusRelease); err != nil {
			t.Fatal(err)
		}
		for range first.context.feed.Events {
		}
		second, err := reg.beginResourceRead(ctx, uri, page.ID, 0, watchNode)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(second.context.cancel)
		reg.finishResourceRead(ctx, second, "current", false)
		reg.finishResourceRead(ctx, first, "retired", false)
		current, err := reg.conversationState(ctx)
		if err != nil || current.Epoch != old.Epoch || current.Version <= old.Version || current.Resources[0].Revision != "current" {
			t.Fatalf("recreated authority: %+v, %v", current, err)
		}
		// All reads share the same feed, while a bounded LRU evicts identities
		// without claiming their documents were removed.
		for i := 0; i < maxVoiceContextResources; i++ {
			id := ksid.NewID()
			ticket, err := reg.beginResourceRead(ctx, reg.nodeURI(id), id, 0, watchNode)
			if err != nil {
				t.Fatal(err)
			}
			if ticket.context != second.context {
				t.Fatal("one read created another feed")
			}
			reg.finishResourceRead(ctx, ticket, strconv.Itoa(i), false)
		}
		reg.finishResourceRead(ctx, second, "evicted", false)
		current, err = reg.conversationState(ctx)
		if err != nil || len(current.Resources) != maxVoiceContextResources || current.OmittedCount != 1 {
			t.Fatalf("bounded watches: %+v, %v", current, err)
		}
		for _, resource := range current.Resources {
			if resource.URI == uri {
				t.Fatal("late read reinserted evicted identity")
			}
		}
		// Freeze the actual consumer while publishing enough relevant mutations to
		// force a real scoped gap. No source body or catalog read is involved.
		reg.focus.mu.Lock()
		node := second.context.resources[0].node
		for range 65 {
			env.services.Broker.Publish(ws, &dto.WorkspaceEvent{Type: dto.EventNodeUpdated, NodeID: node, Aspects: []dto.EventAspect{dto.EventAspectBody}})
		}
		reg.focus.mu.Unlock()
		current, err = reg.conversationState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, resource := range current.Resources {
			if resource.State != mcpschema.ResourceContextChanged || resource.ChangeID == "" {
				t.Fatalf("gap did not invalidate watch: %+v", resource)
			}
		}
	})
	t.Run("LastListenerLifetime", func(t *testing.T) {
		key := voiceFocusKey{user: user.ID, login: login.ID, session: "conversation"}
		if _, _, err := reg.focus.set(ctx, key, ws, 0, nil, voiceFocusRelease); err != nil {
			t.Fatal(err)
		}
		read, err := reg.beginResourceRead(ctx, uri, page.ID, 0, watchNode)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(read.context.cancel)
		_, stop := reg.focus.subscribe(ctx, key)
		t.Cleanup(stop)
		stop()
		for range read.context.feed.Events {
		}
		out, err := reg.conversationState(ctx)
		if err != nil || len(out.Resources) != 0 {
			t.Fatalf("headless last listener retained watches: %+v %v", out, err)
		}
		if _, _, err := reg.focus.set(ctx, key, ws, page.ID, nil, voiceFocusBrowser); err != nil {
			t.Fatal(err)
		}
		hosted, err := reg.beginResourceRead(ctx, uri, page.ID, 0, watchNode)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(hosted.context.cancel)
		_, stopHosted := reg.focus.subscribe(ctx, key)
		t.Cleanup(stopHosted)
		stopHosted()
		out, err = reg.conversationState(ctx)
		if err != nil || len(out.Resources) != 1 {
			t.Fatalf("frontend lease lost on transport recovery: %+v %v", out, err)
		}
		if _, _, err := reg.focus.set(ctx, key, ws, 0, nil, voiceFocusRelease); err != nil {
			t.Fatal(err)
		}
		for range hosted.context.feed.Events {
		}
	})
	t.Run("OnlyAcceptedRequestEchoIsReconciled", func(t *testing.T) {
		writeCtx := reqctx.WithRequestID(ctx, ksid.NewID().String())
		snap, _, err := reg.readSnapshot(ctx, page.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		result := reg.updateNode(writeCtx, &mcp.ToolRequest[mcpschema.UpdateNodeInput]{Arguments: mcpschema.UpdateNodeInput{NodeID: page.ID.String(), Title: "Accepted", Content: "written body", ExpectedRevision: snap.Revision}})
		if result.IsError {
			t.Fatalf("write: %+v", result)
		}
		projection := result.Structured.(mcpschema.WriteNodeOutput).Node
		fresh, _, err := reg.readSnapshot(ctx, page.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(projection, reg.snapshotDetail(&fresh, reg.snapshotNext(&fresh))) {
			t.Fatal("mutation/resource snapshots differ")
		}
		out, err := reg.conversationState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		index := slices.IndexFunc(out.Resources, func(r mcpschema.ResourceContext) bool { return r.URI == uri })
		if index < 0 || out.Resources[index].State != mcpschema.ResourceContextRead || out.Resources[index].Revision != projection.Revision {
			t.Fatalf("accepted echo stayed unread: %+v", out)
		}
		// Same user, same canonical title/body revision, different request: metadata
		// remains an independent change, never an actor-wide suppressed echo.
		if _, err := reg.nodeH.UpdatePageFrontmatter(reqctx.WithRequestID(ctx, ksid.NewID().String()), ws, user, &dto.UpdatePageFrontmatterRequest{ID: page.ID, Icon: "independent"}); err != nil {
			t.Fatal(err)
		}
		out, err = reg.conversationState(ctx)
		if err != nil || out.Resources[index].State != mcpschema.ResourceContextChanged {
			t.Fatalf("same-actor edit was suppressed: %+v %v", out, err)
		}
	})

	t.Run("ReleasedFocusIsAuthoritativelyEmpty", func(t *testing.T) {
		claimCtx := context.WithValue(ctx, voiceSessionKey{}, "initial-claim")
		claimKey := voiceFocusKey{user: user.ID, login: login.ID, session: "initial-claim"}
		zero := uint64(0)
		claim, applied, err := reg.focus.set(claimCtx, claimKey, ws, page.ID, &zero, voiceFocusBrowser)
		if err != nil || applied || claim.workspace != 0 {
			t.Fatalf("initial claim: %+v %v %v", claim, applied, err)
		}
		empty, err := reg.conversationState(claimCtx)
		if err != nil || empty.Focus != nil || len(empty.Resources) != 0 {
			t.Fatalf("uncommitted claim inherited a workspace: %+v %v", empty, err)
		}
		key := voiceFocusKey{user: user.ID, login: login.ID, session: "conversation"}
		if _, _, err := reg.focus.set(ctx, key, ws, page.ID, nil, voiceFocusBrowser); err != nil {
			t.Fatal(err)
		}
		read, err := reg.beginResourceRead(ctx, uri, page.ID, 0, watchNode)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(read.context.cancel)
		before, err := reg.conversationState(ctx)
		if err != nil || before.Focus == nil {
			t.Fatalf("focus: %+v %v", before, err)
		}
		if _, _, err := reg.focus.set(ctx, key, ws, 0, nil, voiceFocusRelease); err != nil {
			t.Fatal(err)
		}
		for range read.context.feed.Events {
		}
		reg.finishResourceRead(ctx, read, "retired", false)
		after, err := reg.conversationState(ctx)
		if err != nil || after.Focus != nil || len(after.Resources) != 0 || after.Version <= before.Version {
			t.Fatalf("released authority: %+v %v", after, err)
		}
	})

	t.Run("LoginRevocationWithoutStreamConsumer", func(t *testing.T) {
		read, err := reg.beginResourceRead(ctx, uri, page.ID, 0, watchNode)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(read.context.cancel)
		if err := env.services.Session.Revoke(login.ID); err != nil {
			t.Fatal(err)
		}
		// The dependency owner was established by a read, without consuming an
		// MCP iterator. Persisted login revocation must unsubscribe it anyway.
		for range read.context.feed.Events {
		}
		out, err := reg.conversationState(ctx)
		if err != nil || len(out.Resources) != 0 || out.Focus != nil {
			t.Fatalf("revoked lease retained context: %+v %v", out, err)
		}
	})
}
