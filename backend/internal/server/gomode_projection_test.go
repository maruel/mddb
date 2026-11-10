// Tests bounded MCP inventory, complete document continuation, wire compatibility, and subscription retirement.

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/maruel/gomode/mcp"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/mcpschema"
	"github.com/maruel/mddb/backend/internal/server/sse"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestWorkspaceRegistry(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	env.services.Broker = sse.NewBroker()
	token, wsID := setupGoModeWorkspace(t, env, "bounded-mcp@example.com")
	user, err := env.services.User.GetByEmail("bounded-mcp@example.com")
	if err != nil {
		t.Fatal(err)
	}
	reg := &workspaceRegistry{svc: env.services, user: user, wsID: wsID, role: identity.WSRoleAdmin, focus: newVoiceFocusState(), nodeH: &handlers.NodeHandler{Svc: env.services}}
	store, err := env.services.FileStore.GetWorkspaceStore(t.Context(), wsID)
	if err != nil {
		t.Fatal(err)
	}
	author := git.Author{Name: "Test", Email: user.Email}
	var active ksid.ID
	for i := range 55 {
		n, err := store.CreatePageUnderParent(t.Context(), 0, fmt.Sprintf("Node %02d", i), "body", author)
		if err != nil {
			t.Fatal(err)
		}
		active = n.ID
	}
	endpoint := goModeMCPEndpoint + "?workspaceId=" + wsID.String()
	t.Run("ListResources", func(t *testing.T) {
		var ids []string
		cursor := ""
		for {
			raw := callMCPNamedAt(t, env, token, endpoint, "resources/list", map[string]any{"cursor": cursor}, "")
			var out mcp.ResourcesListResult
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			if len(out.Resources) > 52 {
				t.Fatalf("unbounded resources: %d", len(out.Resources))
			}
			for _, r := range out.Resources {
				if r.URI != voiceItemsURI && r.URI != voiceContextURI {
					ids = append(ids, r.Name)
				}
			}
			if out.NextCursor == "" {
				break
			}
			cursor = out.NextCursor
		}
		if len(ids) != 55 {
			t.Fatalf("inventory = %d", len(ids))
		}
		for i := 1; i < len(ids); i++ {
			a, _ := ksid.Parse(ids[i-1])
			b, _ := ksid.Parse(ids[i])
			if a >= b {
				t.Fatal("unstable inventory traversal")
			}
		}
		reg.Resources(t.Context())(func(r mcp.ResourceDescriptor, err error) bool {
			if err != nil || r.URI != voiceItemsURI {
				t.Fatalf("first resource = %#v, %v", r, err)
			}
			return false
		})
	})
	t.Run("ListNodes", func(t *testing.T) {
		cursor := ""
		count := 0
		for {
			raw := callMCPNamedAt(t, env, token, endpoint, "tools/call", map[string]any{"name": "nodes_list", "arguments": map[string]string{"cursor": cursor}}, "nodes_list")
			var result struct {
				StructuredContent mcpschema.ListNodesOutput `json:"structuredContent"`
			}
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			out := result.StructuredContent
			if out.Nodes == nil || len(out.Nodes) > 50 {
				t.Fatalf("invalid child page: %#v", out)
			}
			count += len(out.Nodes)
			if out.NextCursor == "" {
				break
			}
			cursor = out.NextCursor
		}
		if count != 55 {
			t.Fatalf("children = %d", count)
		}
		raw := callMCPNamedAt(t, env, token, endpoint, "tools/call", map[string]any{"name": "nodes_list", "arguments": map[string]string{"parentId": active.String()}}, "nodes_list")
		var result struct {
			StructuredContent mcpschema.ListNodesOutput `json:"structuredContent"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if result.StructuredContent.Nodes == nil || len(result.StructuredContent.Nodes) != 0 {
			t.Fatalf("empty children must be an array: %s", raw)
		}
	})

	t.Run("Cursors", func(t *testing.T) {
		page, err := reg.ListResources(t.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range []string{"!", strings.Repeat("x", 513), encodeMCPCursor(&mcpschema.Cursor{Version: 1, Workspace: ksid.NewID(), Kind: "resources", Last: active, Limit: 50})} {
			if _, err := reg.ListResources(t.Context(), raw); err == nil {
				t.Fatalf("accepted cursor %q", raw)
			}
		}
		if _, err := reg.decodeCursor(page.NextCursor, "children", 0, 0, 50); err == nil {
			t.Fatal("accepted cursor from another query")
		}
		child, err := reg.decodeCursor("", "children", active, 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		child.Last = active
		if _, err := reg.decodeCursor(encodeMCPCursor(&child), "children", 0, 0, 50); err == nil {
			t.Fatal("accepted another parent's cursor")
		}
	})
	t.Run("ServiceItems", func(t *testing.T) {
		ctx := context.WithValue(t.Context(), voiceSessionKey{}, "beyond-page")
		if _, _, err := reg.focus.set(ctx, voiceFocusKey{user: user.ID, session: "beyond-page"}, wsID, active, nil, voiceFocusBrowser); err != nil {
			t.Fatal(err)
		}
		out, err := reg.serviceItems(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Items) != 20 || out.OmittedCount != 35 {
			t.Fatalf("items = %#v", out)
		}
	})
	t.Run("NestedFocusIndependentOfCatalog", func(t *testing.T) {
		n, err := store.CreatePageUnderParent(t.Context(), active, "Nested", "nested", author)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.WithValue(t.Context(), voiceSessionKey{}, "nested")
		if _, _, err := reg.focus.set(ctx, voiceFocusKey{user: user.ID, session: "nested"}, wsID, n.ID, nil, voiceFocusBrowser); err != nil {
			t.Fatal(err)
		}
		out, err := reg.serviceItems(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Items) != 20 || slices.ContainsFunc(out.Items, func(item mcpschema.ServiceItem) bool { return item.ID == n.ID.String() }) || out.OmittedCount != 35 {
			t.Fatalf("nested active projection: %#v", out)
		}
		current, e := reg.conversationState(ctx)
		if e != nil || current.Focus == nil || current.Focus.ID != n.ID.String() {
			t.Fatalf("offscreen focus missing: %+v %v", current, e)
		}
	})

	t.Run("CompleteRead", func(t *testing.T) {
		body := strings.Repeat("🙂 markdown\n", 10000)
		if _, err := store.UpdatePage(t.Context(), active, "Long", body, pageRevision(t, t.Context(), store, active), author); err != nil {
			t.Fatal(err)
		}
		var all strings.Builder
		cursor := ""
		revision := ""
		stale := ""
		for {
			snap, next, err := reg.readSnapshot(t.Context(), active, cursor)
			if err != nil {
				t.Fatal(err)
			}
			if len(snap.Node.Content) > 32<<10 {
				t.Fatal("unbounded chunk")
			}
			if revision != "" && revision != snap.Revision {
				t.Fatal("mixed revisions")
			}
			revision = snap.Revision
			all.WriteString(snap.Node.Content)
			if next == "" {
				break
			}
			cursor = next
			stale = next
		}
		if all.String() != body {
			t.Fatal("continuation lost document text")
		}
		snap, next, err := reg.readSnapshot(t.Context(), active, "")
		if err != nil {
			t.Fatal(err)
		}
		resource := reg.snapshotDetail(&snap, next)
		raw, err := reg.ReadResource(t.Context(), &mcp.ResourcesReadParams{URI: resource.NextContentURI})
		if err != nil {
			t.Fatal(err)
		}
		var resourceChunk mcpschema.NodeDetail
		if err := json.Unmarshal([]byte(raw.Contents[0].Text), &resourceChunk); err != nil {
			t.Fatal(err)
		}
		if resourceChunk.Content == "" || resourceChunk.Revision != revision {
			t.Fatal("resource continuation failed")
		}
		if _, err := store.UpdatePage(t.Context(), active, "Changed", body+"new", pageRevision(t, t.Context(), store, active), author); err != nil {
			t.Fatal(err)
		}
		if _, _, err := reg.readSnapshot(t.Context(), active, stale); err == nil {
			t.Fatal("accepted stale content cursor")
		}
		other := *reg
		other.wsID = ksid.NewID()
		if _, _, err := other.readSnapshot(t.Context(), active, stale); err == nil {
			t.Fatal("accepted cross-workspace content cursor")
		}
	})
	t.Run("ResourceWire", func(t *testing.T) {
		if _, err := store.UpdatePage(t.Context(), active, "Small", "wire body", pageRevision(t, t.Context(), store, active), author); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(env.services.FileStore.RootDir(), wsID.String(), active.String(), "metadata.json")
		metadata := `{"title":"Table","properties":[{"name":"Link","type":"relation","relation_config":{"target_node_id":"1","is_dual_link":true}},{"name":"Total","type":"rollup","rollup_config":{"relation_property":"Link","target_property":"Value","aggregation":"sum"}},{"name":"Formula","type":"formula","formula_config":{"expression":"Value+1"}}],"views":[{"id":"1","name":"View","type":"table","filters":[{"and":[{"property":"Name","operator":"equals","value":"a"}]}]}]}`
		if err := os.WriteFile(path, []byte(metadata), 0o600); err != nil {
			t.Fatal(err)
		}
		original, err := store.ReadNode(t.Context(), active)
		if err != nil {
			t.Fatal(err)
		}
		snap, _, err := reg.readSnapshot(t.Context(), active, "")
		if err != nil {
			t.Fatal(err)
		}
		snap.Node.Content = original.Content
		next := "" // Compare the pre-existing fields independently of chunk additions.
		got := reg.snapshotDetail(&snap, next)
		tool := reg.readNode(t.Context(), &mcp.ToolRequest[mcpschema.ReadNodeInput]{Arguments: mcpschema.ReadNodeInput{NodeID: active.String()}})
		if tool.IsError {
			t.Fatalf("node_read failed: %+v", tool)
		}
		toolJSON, err := json.Marshal(tool.Structured.(mcpschema.ReadNodeOutput).Node)
		if err != nil {
			t.Fatal(err)
		}
		resourceJSON, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(toolJSON, resourceJSON) {
			t.Fatalf("tool and resource disagree on the same canonical snapshot:\ntool %s\nresource %s", toolJSON, resourceJSON)
		}
		if len(got.Properties) != 3 || got.Properties[0].RelationConfig == nil || !got.Properties[0].RelationConfig.IsDualLink || got.Properties[1].RollupConfig.Aggregation != "sum" || got.Properties[2].FormulaConfig.Expression != "Value+1" {
			t.Fatalf("incomplete canonical schema: %+v", got.Properties)
		}
		if len(got.Views) != 1 || len(got.Views[0].Filters) != 1 || got.Views[0].Filters[0].And[0].Value != "a" || got.Created == 0 || got.Modified == 0 {
			t.Fatalf("incomplete canonical metadata: %+v", got)
		}
	})
	t.Run("ChildHierarchyProjection", func(t *testing.T) {
		uri := reg.nodeURI(active)
		seq, err := reg.SubscribeResourceUpdates(t.Context(), &mcp.SubscriptionsListenParams{Notifications: mcp.SubscriptionFilter{ResourceSubscriptions: []string{uri, voiceItemsURI}}})
		if err != nil {
			t.Fatal(err)
		}
		var child dto.CreatePageResponse
		base := "/api/v1/workspaces/" + wsID.String() + "/nodes/"
		if code := env.doJSON(t, "POST", base+active.String()+"/page/create", dto.CreatePageRequest{Title: "child", Content: "child"}, &child, token); code != 200 {
			t.Fatal(code)
		}
		for update, err := range seq {
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(update.ResourceURIs, []string{voiceItemsURI}) {
				t.Fatalf("child created invalidates unchanged parent=%+v", update)
			}
			break
		}
		// A move changes the watched resource's own ParentID even though a
		// child's ancestry does not change this parent's representation.
		seq, err = reg.SubscribeResourceUpdates(t.Context(), &mcp.SubscriptionsListenParams{Notifications: mcp.SubscriptionFilter{ResourceSubscriptions: []string{uri}}})
		if err != nil {
			t.Fatal(err)
		}
		before, err := reg.ReadResource(t.Context(), &mcp.ResourcesReadParams{URI: uri})
		if err != nil {
			t.Fatal(err)
		}

		other, err := store.CreatePageUnderParent(t.Context(), 0, "new parent", "", author)
		if err != nil {
			t.Fatal(err)
		}
		if code := env.doJSON(t, "POST", base+active.String()+"/move", dto.MoveNodeRequest{NewParentID: other.ID}, nil, token); code != 200 {
			t.Fatal(code)
		}
		for update, err := range seq {
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(update.ResourceURIs, []string{uri}) {
				t.Fatalf("own move missing=%+v", update)
			}
			break
		}
		after, err := reg.ReadResource(t.Context(), &mcp.ResourcesReadParams{URI: uri})
		if err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(before, after) {
			t.Fatal("own move did not actually change resource representation")
		}
		var moved mcpschema.NodeDetail
		if err := json.Unmarshal([]byte(after.Contents[0].Text), &moved); err != nil {
			t.Fatal(err)
		}
		if moved.ParentID != other.ID.String() {
			t.Fatalf("moved resource ParentID=%v", moved.ParentID)
		}

		// Restore fixture ancestry for subsequent tests.
		if _, err := store.MoveNode(t.Context(), active, 0, author); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("SubscriptionReadError", func(t *testing.T) {
		uri := reg.nodeURI(active)
		seq, err := reg.SubscribeResourceUpdates(t.Context(), &mcp.SubscriptionsListenParams{Notifications: mcp.SubscriptionFilter{ResourceSubscriptions: []string{uri}}})
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(env.services.FileStore.RootDir(), wsID.String(), active.String(), "metadata.json")
		before, err := os.ReadFile(path) //nolint:gosec // Test-owned temporary workspace path.
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Error(err)
			}
		})
		if err := os.WriteFile(path, []byte(strings.Repeat("x", content.MaxSnapshotMetadataBytes+1)), 0o600); err != nil {
			t.Fatal(err)
		}
		reg.svc.Broker.Publish(wsID, &dto.WorkspaceEvent{Type: dto.EventNodeUpdated, NodeID: active, Aspects: []dto.EventAspect{dto.EventAspectBody}})
		for update, err := range seq {
			if err != nil || !slices.Equal(update.ResourceURIs, []string{uri}) {
				t.Fatalf("invalidation = %#v %v", update, err)
			}
			// Invalidation does not probe the body. The caller's subsequent read
			// surfaces storage failure instead of treating it as a disappeared node.
			if _, err := reg.ReadResource(t.Context(), &mcp.ResourcesReadParams{URI: uri}); err == nil {
				t.Fatal("corrupt metadata read succeeded")
			}
			break
		}
	})

	t.Run("SubscriptionWithdrawal", func(t *testing.T) {
		n, err := store.CreatePageUnderParent(t.Context(), active, "Withdrawn", "child", author)
		if err != nil {
			t.Fatal(err)
		}
		uri := reg.nodeURI(n.ID)
		seq, err := reg.SubscribeResourceUpdates(t.Context(), &mcp.SubscriptionsListenParams{Notifications: mcp.SubscriptionFilter{ResourceSubscriptions: []string{uri}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.DeletePage(t.Context(), n.ID, author); err != nil {
			t.Fatal(err)
		}
		reg.svc.Broker.Publish(wsID, &dto.WorkspaceEvent{Type: dto.EventNodeDeleted, NodeID: n.ID})
		got := false
		for update, err := range seq {
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(update.ResourceURIs, uri) {
				t.Fatalf("withdrawal missing: %#v", update)
			}
			got = true
			break
		}
		if !got {
			t.Fatal("stream closed before withdrawal")
		}
		reg.focus.mu.Lock()
		count := len(reg.focus.subscribers)
		reg.focus.mu.Unlock()
		if count != 0 {
			t.Fatal("stopped iterator retained focus subscriber")
		}
	})

	t.Run("SubscriptionCancellation", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			_, err := reg.SubscribeResourceUpdates(ctx, &mcp.SubscriptionsListenParams{Meta: mcp.RequestMeta{Session: "never-started"}, Notifications: mcp.SubscriptionFilter{ResourceSubscriptions: []string{voiceItemsURI}}})
			if err != nil {
				t.Fatal(err)
			}
			cancel()
			synctest.Wait()
			reg.focus.mu.Lock()
			n := len(reg.focus.subscribers)
			reg.focus.mu.Unlock()
			if n != 0 {
				t.Fatalf("retained subscribers = %d", n)
			}
		})
	})
	t.Run("SubscriptionWorkspaceWithdrawal", func(t *testing.T) {
		uri := reg.nodeURI(active)
		seq, err := reg.SubscribeResourceUpdates(t.Context(), &mcp.SubscriptionsListenParams{Notifications: mcp.SubscriptionFilter{ResourceSubscriptions: []string{uri, voiceItemsURI}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := reg.svc.Workspace.Delete(wsID); err != nil {
			t.Fatal(err)
		}
		reg.svc.Broker.Publish(wsID, &dto.WorkspaceEvent{Type: dto.EventNodeUpdated, NodeID: active, Aspects: []dto.EventAspect{dto.EventAspectBody}})
		count := 0
		for update, err := range seq {
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(update.ResourceURIs, uri) || !slices.Contains(update.ResourceURIs, voiceItemsURI) {
				t.Fatalf("workspace withdrawal: %#v", update)
			}
			count++
		}
		if count != 1 {
			t.Fatalf("withdrawal notifications = %d", count)
		}
		reg.focus.mu.Lock()
		retained := len(reg.focus.subscribers)
		reg.focus.mu.Unlock()
		if retained != 0 {
			t.Fatal("withdrawn workspace retained subscriptions")
		}
	})
}

func TestGoModeSubscriptionSetupRoleChange(t *testing.T) {
	for _, beforeSubscribe := range []bool{true, false} {
		t.Run(fmt.Sprintf("beforeSubscribe=%t", beforeSubscribe), func(t *testing.T) {
			t.Parallel()
			env := setupTestEnv(t)
			env.services.Broker = sse.NewBroker()
			_, wsID := setupGoModeWorkspace(t, env, "setup-role@example.com")
			u, err := env.userService.GetByEmail("setup-role@example.com")
			if err != nil {
				t.Fatal(err)
			}
			reg := &workspaceRegistry{svc: env.services, user: u, wsID: wsID, role: identity.WSRoleAdmin, focus: newVoiceFocusState()}
			ws, err := env.wsService.Get(wsID)
			if err != nil {
				t.Fatal(err)
			}
			membership, err := env.orgMemService.Get(u.ID, ws.OrganizationID)
			if err != nil {
				t.Fatal(err)
			}
			changeRole := func() {
				if _, err := env.orgMemService.Modify(membership.ID, func(m *identity.OrganizationMembership) error { m.Role = identity.OrgRoleMember; return nil }); err != nil {
					t.Fatal(err)
				}
				wm, err := env.wsMemService.Get(u.ID, wsID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := env.wsMemService.Modify(wm.ID, func(m *identity.WorkspaceMembership) error { m.Role = identity.WSRoleViewer; return nil }); err != nil {
					t.Fatal(err)
				}
			}
			if beforeSubscribe {
				changeRole()
			}
			// No broker event: the change may precede event registration.
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				seq, err := reg.SubscribeResourceUpdates(ctx, &mcp.SubscriptionsListenParams{})
				if err != nil {
					cancel()
					t.Fatal(err)
				}
				if !beforeSubscribe {
					changeRole()
				}
				var wg sync.WaitGroup
				done := make(chan struct{})
				wg.Go(func() {
					defer close(done)
					for _, err := range seq {
						if err != nil {
							t.Error(err)
						}
					}
				})
				synctest.Wait()
				select {
				case <-done:
				default:
					t.Error("role change during setup did not stop stream")
				}
				cancel()
				wg.Wait()
			})
		})
	}
}

func pageRevision(t *testing.T, ctx context.Context, ws *content.WorkspaceFileStore, id ksid.ID) string {
	node, err := ws.ReadNode(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return node.Revision
}
