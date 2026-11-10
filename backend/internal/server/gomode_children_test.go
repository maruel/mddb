// Tests bounded offscreen child-list windows, represented summary dependencies, and pre-read handoff.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/maruel/gomode"
	"github.com/maruel/gomode/mcp"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/mcpschema"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestChildrenResources(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	token, ws := setupGoModeWorkspace(t, env, "children-context@example.com")
	user, err := env.services.User.GetByEmail("children-context@example.com")
	if err != nil {
		t.Fatal(err)
	}
	var login *identity.Session
	for value := range env.services.Session.GetActiveByUserID(user.ID) {
		login = value
		break
	}
	if login == nil {
		t.Fatal("missing login")
	}
	ctx := reqctx.WithSession(reqctx.WithSessionID(t.Context(), login.ID), login)
	ctx = context.WithValue(ctx, voiceSessionKey{}, "child-handoff")
	store, err := env.services.FileStore.GetWorkspaceStore(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	nodeHandler := &handlers.NodeHandler{Svc: env.services}
	create := func(t *testing.T, parent ksid.ID, title string) ksid.ID {
		out, err := nodeHandler.CreatePage(ctx, ws, user, &dto.CreatePageRequest{ParentID: parent, Title: title, Content: "body"})
		if err != nil {
			t.Fatal(err)
		}
		return out.ID
	}
	parent := create(t, 0, "Parent")
	child := create(t, parent, "Child")
	other := create(t, 0, "Other")
	endpoint := goModeMCPEndpoint + "?workspaceId=" + ws.String()
	meta := map[string]any{"openai/session": "children-conversation"}
	list := func(t *testing.T, cursor string) mcpschema.ListNodesOutput {
		raw := callMCPNamedAt(t, env, token, endpoint, "tools/call", map[string]any{"name": "nodes_list", "arguments": map[string]any{"parentId": parent.String(), "cursor": cursor}, "_meta": meta}, "nodes_list")
		var result struct {
			Snapshot mcpschema.ListNodesOutput `json:"structuredContent"`
			IsError  bool                      `json:"isError"`
		}
		if err := json.Unmarshal(raw, &result); err != nil || result.IsError {
			t.Fatalf("list: %s, %v", raw, err)
		}
		return result.Snapshot
	}
	state := func(t *testing.T, uri string) mcpschema.ResourceContext {
		raw := callMCPNamedAt(t, env, token, endpoint, "resources/read", map[string]any{"uri": voiceContextURI, "_meta": meta}, voiceContextURI)
		var resource mcp.ResourcesReadResult
		if err := json.Unmarshal(raw, &resource); err != nil || len(resource.Contents) != 1 {
			t.Fatalf("context: %s,%v", raw, err)
		}
		var projection mcpschema.ConversationContext
		if err := json.Unmarshal([]byte(resource.Contents[0].Text), &projection); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(resource.Meta[gomode.ConversationContextMetaKey])
		if err != nil {
			t.Fatal(err)
		}
		var transport gomode.ConversationContext
		if err := json.Unmarshal(encoded, &transport); err != nil {
			t.Fatal(err)
		}
		for _, watched := range projection.Resources {
			if watched.URI == uri {
				// Routing is a private union, separate from public mddb freshness.
				watched.Dependencies = transport.Dependencies
				return watched
			}
		}
		t.Fatalf("list dependency missing: %+v", projection)
		return mcpschema.ResourceContext{}
	}
	initial := list(t, "")
	if len(initial.Nodes) != 1 || initial.Nodes[0].ID != child.String() || initial.Nodes[0].HasChildren || initial.URI == "" || initial.Revision == "" {
		t.Fatalf("list: %+v", initial)
	}
	raw := callMCPNamedAt(t, env, token, endpoint, "resources/read", map[string]any{"uri": initial.URI, "_meta": meta}, initial.URI)
	var resource mcp.ResourcesReadResult
	if err := json.Unmarshal(raw, &resource); err != nil || len(resource.Contents) != 1 {
		t.Fatalf("resource: %s,%v", raw, err)
	}
	var snapshot mcpschema.ListNodesOutput
	if err := json.Unmarshal([]byte(resource.Contents[0].Text), &snapshot); err != nil || !reflect.DeepEqual(initial, snapshot) {
		t.Fatalf("tool/resource differ: %+v,%v", snapshot, err)
	}
	// Child body edits change no represented field. An unrelated hierarchy change
	// must also remain quiet while this offscreen window is watched.
	node, err := store.ReadNode(ctx, child)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdatePage(ctx, child, node.Title, "changed body", node.Revision, handlers.GitAuthor(user))
	if err != nil {
		t.Fatal(err)
	}
	env.services.PublishNodeEvent(ctx, ws, dto.EventNodeUpdated, updated, user.ID)
	create(t, other, "Unrelated")
	if watched := state(t, initial.URI); watched.State != mcpschema.ResourceContextRead {
		t.Fatalf("unrepresented effects: %+v", watched)
	}
	// A grandchild changes the returned child's HasChildren, even though the
	// queried parent's direct membership remains identical.
	grandchild := create(t, child, "Grandchild")
	if watched := state(t, initial.URI); watched.State != mcpschema.ResourceContextChanged {
		t.Fatalf("HasChildren addition lost: %+v", watched)
	}
	current := list(t, "")
	if !current.Nodes[0].HasChildren || current.Revision == initial.Revision {
		t.Fatalf("presence snapshot: %+v", current)
	}
	if status := env.doJSON(t, http.MethodPost, "/api/v1/workspaces/"+ws.String()+"/nodes/"+grandchild.String()+"/delete", nil, nil, token); status != http.StatusOK {
		t.Fatalf("delete: %d", status)
	}
	if watched := state(t, initial.URI); watched.State != mcpschema.ResourceContextChanged {
		t.Fatalf("HasChildren removal lost: %+v", watched)
	}
	list(t, "")
	node, err = store.ReadNode(ctx, child)
	if err != nil {
		t.Fatal(err)
	}
	updated, err = store.UpdatePage(ctx, child, "Renamed", node.Content, node.Revision, handlers.GitAuthor(user))
	if err != nil {
		t.Fatal(err)
	}
	env.services.PublishNodeEvent(ctx, ws, dto.EventNodeUpdated, updated, user.ID)
	if watched := state(t, initial.URI); watched.State != mcpschema.ResourceContextChanged {
		t.Fatalf("title change lost: %+v", watched)
	}
	list(t, "")
	added := create(t, parent, "Added")
	if watched := state(t, initial.URI); watched.State != mcpschema.ResourceContextChanged {
		t.Fatalf("membership addition lost: %+v", watched)
	}
	list(t, "")
	move, err := store.MoveNode(ctx, added, other, handlers.GitAuthor(user))
	if err != nil {
		t.Fatal(err)
	}
	env.services.PublishNodeMove(ctx, ws, added, move, user.ID)
	if watched := state(t, initial.URI); watched.State != mcpschema.ResourceContextChanged {
		t.Fatalf("membership move lost: %+v", watched)
	}

	for _, retirement := range []string{"Release", "Eviction"} {
		t.Run("SuccessfulReadSurvives"+retirement, func(t *testing.T) {
			reg := &workspaceRegistry{svc: env.services, user: user, wsID: ws, role: identity.WSRoleAdmin, focus: newVoiceFocusState()}
			key := voiceFocusKey{user: user.ID, login: login.ID, session: conversationSession(ctx)}
			changes, unsubscribe := reg.focus.subscribe(ctx, key)
			t.Cleanup(unsubscribe)
			entered, unblock := make(chan struct{}), make(chan struct{})
			release := sync.OnceFunc(func() { close(unblock) })
			t.Cleanup(release)
			locked := make(chan error, 1)
			go func() {
				locked <- store.Repo().WriteTx(ctx, func() error {
					close(entered)
					<-unblock
					return nil
				})
			}()
			<-entered
			type outcome struct {
				snapshot mcpschema.ListNodesOutput
				err      error
			}
			result := make(chan outcome, 1)
			go func() {
				snapshot, err := reg.childrenSnapshot(ctx, &childrenResource{workspace: ws, parent: parent})
				result <- outcome{snapshot, err}
			}()
			var owner *voiceContext
			deadline := time.NewTimer(5 * time.Second)
			t.Cleanup(func() { deadline.Stop() })
			for owner == nil {
				select {
				case <-changes:
					reg.focus.mu.Lock()
					if el := reg.focus.entries[key]; el != nil {
						candidate := el.Value.(voiceFocus).context
						if candidate != nil && len(candidate.resources) == 1 && candidate.resources[0].readingChildren {
							owner = candidate
						}
					}
					reg.focus.mu.Unlock()
				case early := <-result:
					t.Fatalf("listing bypassed held repository gate: %+v", early)
				case <-deadline.C:
					t.Fatal("listing did not register before its repository read")
				}
			}
			t.Cleanup(owner.cancel)
			if retirement == "Release" {
				if _, _, err := reg.focus.set(ctx, key, ws, 0, nil, voiceFocusRelease); err != nil {
					t.Fatal(err)
				}
			} else {
				reg.focus.mu.Lock()
				for i := range maxVoiceFocusEntries {
					reg.focus.putLocked(voiceFocus{key: voiceFocusKey{user: user.ID, login: login.ID, session: fmt.Sprintf("replacement-%d", i)}})
				}
				reg.focus.mu.Unlock()
			}
			// Cancellation is observed while the data read is still gated, not by cleanup.
			for range owner.feed.Events {
			}
			release()
			if err := <-locked; err != nil {
				t.Fatal(err)
			}
			got := <-result
			if got.err != nil || len(got.snapshot.Nodes) == 0 || got.snapshot.Revision == "" {
				t.Fatalf("authorized completed listing failed after %s: %+v", retirement, got)
			}
			if current := reg.focus.lookup(key, 0); current.context != nil {
				t.Fatal("completed obsolete read resurrected its watch")
			}
		})
	}

	t.Run("CanceledRegistrationCannotComplete", func(t *testing.T) {
		reg := &workspaceRegistry{svc: env.services, user: user, wsID: ws, role: identity.WSRoleAdmin, focus: newVoiceFocusState()}
		query := childrenResource{workspace: ws, parent: parent}
		read, err := reg.beginResourceRead(ctx, query.uri(), parent, 0, watchChildren)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(read.context.cancel)
		read.context.cancel()
		if err := reg.completeChildrenRead(ctx, read, []ksid.ID{child}); !errors.Is(err, context.Canceled) {
			t.Fatalf("dead interest completion: %v", err)
		}
		if read.watch.resource.State == mcpschema.ResourceContextRead {
			t.Fatal("dead interest reported a finished read")
		}
	})

	t.Run("BoundedContinuation", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			create(t, parent, "Window child")
		}
		first := list(t, "")
		if len(first.Nodes) != 50 || first.NextCursor == "" || first.NextURI == "" {
			t.Fatalf("bounded first page: %+v", first)
		}
		watched := state(t, first.URI)
		if len(watched.Dependencies) != 2 || len(watched.Dependencies[1].URIs) != 50 {
			t.Fatalf("lost represented members: %+v", watched)
		}
		second := list(t, first.NextCursor)
		if len(second.Nodes) != 1 || second.NextCursor != "" || second.URI == first.URI {
			t.Fatalf("continuation: %+v", second)
		}
		if firstWatch := state(t, first.URI); firstWatch.State != mcpschema.ResourceContextRead {
			t.Fatalf("continuation evicted first page: %+v", firstWatch)
		}
	})

	t.Run("PreReadReconciliation", func(t *testing.T) {
		reg := &workspaceRegistry{svc: env.services, nodeH: &handlers.NodeHandler{Svc: env.services}, user: user, wsID: ws, role: identity.WSRoleAdmin, focus: newVoiceFocusState()}
		query := childrenResource{workspace: ws, parent: parent}
		ticket, err := reg.beginResourceRead(ctx, query.uri(), parent, 0, watchChildren)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(ticket.context.cancel)
		// A summary for a child outside the returned window can arrive during the
		// read; it must not be retained as an unread change after exact membership.
		queued := create(t, parent, "Outside returned window")
		if err := reg.completeChildrenRead(ctx, ticket, []ksid.ID{child}); err != nil {
			t.Fatal(err)
		}
		reg.finishResourceRead(ctx, ticket, "snapshot", false)
		projection, err := reg.conversationState(ctx)
		if err != nil || projection.Resources[0].State != mcpschema.ResourceContextChanged {
			t.Fatalf("membership handoff lost: %+v,%v", projection, err)
		}
		// Membership additions affect order/range, even if not returned, so reread.
		second, err := reg.beginResourceRead(ctx, query.uri(), parent, 0, watchChildren)
		if err != nil {
			t.Fatal(err)
		}
		node, err := store.ReadNode(ctx, queued)
		if err != nil {
			t.Fatal(err)
		}
		update, err := store.UpdatePage(ctx, queued, "Outside renamed", node.Content, node.Revision, handlers.GitAuthor(user))
		if err != nil {
			t.Fatal(err)
		}
		env.services.PublishNodeEvent(ctx, ws, dto.EventNodeUpdated, update, user.ID)
		if err := reg.completeChildrenRead(ctx, second, []ksid.ID{child}); err != nil {
			t.Fatal(err)
		}
		reg.finishResourceRead(ctx, second, "current", false)
		projection, err = reg.conversationState(ctx)
		if err != nil || len(projection.Resources) != 1 || projection.Resources[0].State != mcpschema.ResourceContextRead {
			t.Fatalf("unreturned summary leaked: %+v,%v", projection, err)
		}
		third, err := reg.beginResourceRead(ctx, query.uri(), parent, 0, watchChildren)
		if err != nil {
			t.Fatal(err)
		}
		create(t, child, "Concurrent grandchild")
		if err := reg.completeChildrenRead(ctx, third, []ksid.ID{child}); err != nil {
			t.Fatal(err)
		}
		reg.finishResourceRead(ctx, third, "older", false)
		projection, err = reg.conversationState(ctx)
		if err != nil || projection.Resources[0].State != mcpschema.ResourceContextChanged {
			t.Fatalf("handoff lost represented summary: %+v,%v", projection, err)
		}
		if !slices.Contains(projection.Resources[0].Dependencies[1].URIs, reg.nodeURI(child)) {
			t.Fatalf("member routing missing: %+v", projection)
		}
		projection.Resources[0].Dependencies[1].URIs[0] = "host://caller-reused"
		projection.Resources[0].Dependencies[1].Aspects[0] = "caller-reused"
		latest, err := reg.conversationState(ctx)
		if err != nil || latest.Resources[0].Dependencies[1].URIs[0] != reg.nodeURI(child) || latest.Resources[0].Dependencies[1].Aspects[0] != "summary" {
			t.Fatalf("projection aliases live routing: %+v,%v", latest, err)
		}
	})
}
