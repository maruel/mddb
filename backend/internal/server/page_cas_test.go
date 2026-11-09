// Tests conditional HTTP/MCP page mutations, canonical responses, and rejected-write event isolation.

package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/maruel/gomode/mcp"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/mcpschema"
	"github.com/maruel/mddb/backend/internal/server/sse"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestWorkspaceInvalidationAspects(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	env.services.Broker = sse.NewBroker()
	token, wsID := setupGoModeWorkspace(t, env, "invalidation-aspects@example.com")
	base := "/api/v1/workspaces/" + wsID.String() + "/nodes/"
	var created dto.CreatePageResponse
	if code := env.doJSON(t, http.MethodPost, base+"0/page/create", dto.CreatePageRequest{Title: "Original", Content: "body"}, &created, token); code != http.StatusOK {
		t.Fatal(code)
	}
	all, stop := env.services.Broker.SubscribeEvents(wsID, sse.EventInterest{All: true})
	t.Cleanup(stop)
	catalog, stopCatalog := env.services.Broker.SubscribeEvents(wsID, sse.EventInterest{Catalog: true})
	t.Cleanup(stopCatalog)
	previous := pageRevisionHTTP(t, env, token, wsID, created.ID)
	var result dto.UpdatePageResponse
	for _, mutation := range []struct {
		title, body string
		catalog     bool
	}{
		{"Original", "changed body", false}, {"Renamed", "changed body", true},
	} {
		if code := env.doJSON(t, http.MethodPost, base+created.ID.String()+"/page", dto.UpdatePageRequest{Title: mutation.title, Content: mutation.body, ExpectedRevision: previous}, &result, token); code != http.StatusOK {
			t.Fatal(code)
		}
		event := <-all
		if event.ChangeID == "" || event.Revision != result.Node.Revision || !slices.Contains(event.Aspects, dto.EventAspectBody) {
			t.Fatalf("mutation event=%+v", event)
		}
		select {
		case event := <-catalog:
			if !mutation.catalog || !slices.Contains(event.Aspects, dto.EventAspectCatalog) {
				t.Fatalf("unexpected catalog effect=%+v", event)
			}
		default:
			if mutation.catalog {
				t.Fatal("rename failed to invalidate descriptor")
			}
		}
		previous = result.Node.Revision
		// An actual HTTP retry with identical canonical content is a source no-op.
		if code := env.doJSON(t, http.MethodPost, base+created.ID.String()+"/page", dto.UpdatePageRequest{Title: mutation.title, Content: mutation.body, ExpectedRevision: previous}, &result, token); code != http.StatusOK {
			t.Fatal(code)
		}
		select {
		case event := <-all:
			t.Fatalf("unchanged write published=%+v", event)
		default:
		}
	}
	if code := env.doJSON(t, http.MethodPost, base+created.ID.String()+"/page/frontmatter", dto.UpdatePageFrontmatterRequest{Icon: "icon"}, nil, token); code != http.StatusOK {
		t.Fatal(code)
	}
	event := <-all
	if !slices.Contains(event.Aspects, dto.EventAspectMetadata) || slices.Contains(event.Aspects, dto.EventAspectBody) || slices.Contains(event.Aspects, dto.EventAspectCatalog) {
		t.Fatalf("metadata effects=%+v", event)
	}
	if actual := pageRevisionHTTP(t, env, token, wsID, created.ID); actual != previous {
		t.Fatal("metadata changed canonical CAS")
	}
	// Table metadata on an existing page creates a hybrid. Removing one part
	// retains its resource; the remaining title determines descriptor effects.
	store, err := env.services.FileStore.GetWorkspaceStore(t.Context(), wsID)
	if err != nil {
		t.Fatal(err)
	}
	user, err := env.services.User.GetByEmail("invalidation-aspects@example.com")
	if err != nil {
		t.Fatal(err)
	}
	hybrid := &content.Node{ID: created.ID, Title: "Table title"}
	if err := store.WriteTable(t.Context(), hybrid, true, handlers.GitAuthor(user)); err != nil {
		t.Fatal(err)
	}
	if code := env.doJSON(t, http.MethodPost, base+created.ID.String()+"/table/delete", dto.DeleteTableRequest{}, nil, token); code != http.StatusOK {
		t.Fatal(code)
	}
	event = <-all
	if event.Type != dto.EventNodeUpdated || !slices.Contains(event.Aspects, dto.EventAspectSchema) || !slices.Contains(event.Aspects, dto.EventAspectCatalog) {
		t.Fatalf("hybrid retained effects=%+v", event)
	}
	if _, err := store.ReadNodeSummary(t.Context(), created.ID); err != nil {
		t.Fatal("hybrid resource withdrawn", err)
	}
}

// The resource contains the node's own page/schema/metadata and ParentID,
// but no asset listing, records, Children or HasChildren.
func TestWatchedResourceEffects(t *testing.T) {
	env := setupTestEnv(t)
	env.services.Broker = sse.NewBroker()
	token, wsID := setupGoModeWorkspace(t, env, "watched-effects@example.com")
	user, err := env.services.User.GetByEmail("watched-effects@example.com")
	if err != nil {
		t.Fatal(err)
	}
	reg := &workspaceRegistry{svc: env.services, user: user, wsID: wsID, role: identity.WSRoleAdmin, focus: newVoiceFocusState(), nodeH: &handlers.NodeHandler{Svc: env.services}}

	base := "/api/v1/workspaces/" + wsID.String() + "/nodes/"
	var parent, child dto.CreatePageResponse
	if status := env.doJSON(t, http.MethodPost, base+"0/page/create", dto.CreatePageRequest{Title: "parent", Content: "body"}, &parent, token); status != http.StatusOK {
		t.Fatalf("create parent: status=%d", status)
	}
	var table dto.CreateTableUnderParentResponse
	if status := env.doJSON(t, http.MethodPost, base+"0/table/create", dto.CreateTableRequest{Title: "table"}, &table, token); status != http.StatusOK {
		t.Fatalf("create table: status=%d", status)
	}
	for _, test := range []struct {
		name   string
		id     ksid.ID
		mutate func(t *testing.T)
	}{
		{"own record", table.ID, func(t *testing.T) {
			if code := env.doJSON(t, http.MethodPost, base+table.ID.String()+"/table/records/create", dto.CreateRecordRequest{Data: map[string]any{}}, nil, token); code != 200 {
				t.Fatal(code)
			}
		}},
		{"own asset", parent.ID, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, err := writer.CreateFormFile("file", "standalone.txt")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(part, "asset"); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, env.server.URL+base+parent.ID.String()+"/assets", &body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := env.server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusCreated {
				t.Fatal(resp.StatusCode)
			}
		}},
		{"child create", parent.ID, func(t *testing.T) {
			if code := env.doJSON(t, http.MethodPost, base+parent.ID.String()+"/page/create", dto.CreatePageRequest{Title: "child", Content: "child"}, &child, token); code != 200 {
				t.Fatal(code)
			}
		}},
		{"child move", parent.ID, func(t *testing.T) {
			if code := env.doJSON(t, http.MethodPost, base+child.ID.String()+"/move", dto.MoveNodeRequest{NewParentID: table.ID}, nil, token); code != 200 {
				t.Fatal(code)
			}
		}},
		{"child delete", table.ID, func(t *testing.T) {
			if code := env.doJSON(t, http.MethodPost, base+child.ID.String()+"/delete", dto.DeleteNodeRequest{}, nil, token); code != 200 {
				t.Fatal(code)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			events, stop := env.services.Broker.SubscribeEvents(wsID, sse.EventInterest{Nodes: []ksid.ID{test.id}})
			t.Cleanup(stop)
			before, err := reg.ReadResource(t.Context(), &mcp.ResourcesReadParams{URI: reg.nodeURI(test.id)})
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t)
			after, err := reg.ReadResource(t.Context(), &mcp.ResourcesReadParams{URI: reg.nodeURI(test.id)})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("test mutation actually changed resource: before=%+v after=%+v", before, after)
			}

			select {
			case event := <-events:
				t.Fatalf("unchanged resource invalidated: %+v", event)
			default:
			}
		})
	}
}

func TestConditionalPageWrites(t *testing.T) {
	env := setupTestEnv(t)
	env.services.Broker = sse.NewBroker()
	token, wsID := setupGoModeWorkspace(t, env, "conditional@example.com")
	base := "/api/v1/workspaces/" + wsID.String() + "/nodes/"
	var created dto.CreatePageResponse
	if status := env.doJSON(t, http.MethodPost, base+"0/page/create", dto.CreatePageRequest{Title: "Original", Content: "body"}, &created, token); status != http.StatusOK {
		t.Fatal(status)
	}
	revision := pageRevisionHTTP(t, env, token, wsID, created.ID)
	events, unsubscribe := env.services.Broker.SubscribeEvents(wsID, sse.EventInterest{All: true})
	t.Cleanup(unsubscribe)
	var saved dto.UpdatePageResponse
	if status := env.doJSON(t, http.MethodPost, base+created.ID.String()+"/page", dto.UpdatePageRequest{Title: "  Canonical  ", Content: "\n\nCanonical body", ExpectedRevision: revision}, &saved, token); status != http.StatusOK {
		t.Fatal(status)
	}
	if saved.Node.Title != "Canonical" || saved.Node.Content != "Canonical body" || saved.Node.Revision == revision || saved.Node.Revision != pageRevisionHTTP(t, env, token, wsID, created.ID) {
		t.Fatalf("saved=%+v", saved)
	}
	select {
	case event := <-events:
		if event.Type != dto.EventNodeUpdated || event.NodeID != created.ID || event.Revision != saved.Node.Revision {
			t.Fatalf("event=%+v", event)
		}
	default:
		t.Fatal("successful write did not publish")
	}
	for _, path := range []string{"/page", "/page/append"} {
		request := map[string]string{"content": "stale body", "expected_revision": revision}
		if path == "/page" {
			request["title"] = "Stale"
		}
		var conflict dto.ErrorResponse
		if status := env.doJSON(t, http.MethodPost, base+created.ID.String()+path, request, &conflict, token); status != http.StatusConflict {
			t.Fatalf("stale%s status=%d", path, status)
		}
		if conflict.Error.Code != dto.ErrorCodeConflict || conflict.Details["current_revision"] != saved.Node.Revision || conflict.Details["recovery"] != "read_current" {
			t.Fatalf("conflict is not recoverable: %+v", conflict)
		}
		if got := pageRevisionHTTP(t, env, token, wsID, created.ID); got != saved.Node.Revision {
			t.Fatal("stale mutation changed content")
		}
		select {
		case event := <-events:
			t.Fatalf("rejected write published=%+v", event)
		default:
		}
	}
	if status := env.doJSON(t, http.MethodPost, base+created.ID.String()+"/page", dto.UpdatePageRequest{Title: "Missing", Content: "missing revision"}, nil, token); status != http.StatusBadRequest {
		t.Fatalf("missing revision=%d", status)
	}
	old := saved.Node.Revision
	outcome := callMCPTool(t, env, token, "node_append", map[string]any{"nodeId": created.ID.String(), "content": "MCP append", "expectedRevision": old})
	if outcome.IsError || outcome.StructuredContent.Node.Content != "Canonical body\n\nMCP append" || outcome.StructuredContent.Node.Revision == old {
		t.Fatalf("append=%+v", outcome)
	}
	for _, tool := range []string{"node_update", "node_append"} {
		args := map[string]any{"nodeId": created.ID.String(), "content": "stale", "expectedRevision": old}
		if tool == "node_update" {
			args["title"] = "Stale"
		}
		stale := callMCPTool(t, env, token, tool, args)
		if !stale.IsError {
			t.Fatalf("stale %s succeeded", tool)
		}
		var conflict mcpschema.MutationError
		if err := json.Unmarshal(stale.Meta["com.fghbuild.mddb/mutationError"], &conflict); err != nil {
			t.Fatalf("decode MCP recovery: %v", err)
		}
		if conflict.Code != dto.ErrorCodeConflict || conflict.CurrentRevision != outcome.StructuredContent.Node.Revision || conflict.Recovery != "read_current" {
			t.Fatalf("MCP conflict is not recoverable: %+v", stale)
		}
	}
	// Large mutation results use the same bounded windows and revision-bound
	// continuation as node_read; every byte remains retrievable.
	large := strings.Repeat("🌳 body\n", 10000)
	expected := ""
	currentRevision := outcome.StructuredContent.Node.Revision
	// A client that loses an accepted append response retries the same request,
	// then recovers by reading the page instead of rebasing the append.
	replay := dto.AppendPageRequest{Content: "Lost response append", ExpectedRevision: currentRevision}
	if status := env.doJSON(t, http.MethodPost, base+created.ID.String()+"/page/append", replay, nil, token); status != http.StatusOK {
		t.Fatalf("accepted append status=%d", status)
	}
	var recovery dto.ErrorResponse
	if status := env.doJSON(t, http.MethodPost, base+created.ID.String()+"/page/append", replay, &recovery, token); status != http.StatusConflict {
		t.Fatalf("replayed append status=%d", status)
	}
	var recovered dto.NodeResponse
	if status := env.doJSON(t, http.MethodGet, base+created.ID.String(), nil, &recovered, token); status != http.StatusOK {
		t.Fatalf("recovery read status=%d", status)
	}
	if recovered.Content != "Canonical body\n\nMCP append\n\nLost response append" || recovery.Details["current_revision"] != recovered.Revision || recovery.Details["recovery"] != "read_current" {
		t.Fatalf("ambiguous append recovery: %+v %+v", recovery, recovered)
	}
	// Released MCP clients receive the same machine-facing recovery metadata,
	// outside the success-only structuredContent schema.
	var released struct {
		Result mcpToolOutcome `json:"result"`
	}
	if status := env.doJSON(t, http.MethodPost, goModeMCPEndpoint, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "node_append", "arguments": map[string]any{
			"nodeId": created.ID.String(), "content": replay.Content, "expectedRevision": replay.ExpectedRevision,
		}},
	}, &released, token); status != http.StatusOK {
		t.Fatalf("released replay status=%d", status)
	}
	var releasedConflict mcpschema.MutationError
	if err := json.Unmarshal(released.Result.Meta["com.fghbuild.mddb/mutationError"], &releasedConflict); err != nil {
		t.Fatalf("decode released recovery: %v", err)
	}
	if !released.Result.IsError || releasedConflict.Code != dto.ErrorCodeConflict || releasedConflict.CurrentRevision != recovered.Revision || releasedConflict.Recovery != "read_current" {
		t.Fatalf("released recovery=%+v", releasedConflict)
	}
	currentRevision = recovered.Revision
	for _, tool := range []string{"node_update", "node_append"} {
		args := map[string]any{"nodeId": created.ID.String(), "content": large, "expectedRevision": currentRevision}
		if tool == "node_update" {
			args["title"] = "Large"
			expected = large
		} else {
			expected = strings.TrimRight(expected, "\n") + "\n\n" + large
		}
		written := callMCPTool(t, env, token, tool, args)
		first := written.StructuredContent.Node
		if written.IsError || !first.ContentTruncated || first.NextContentCursor == "" || first.ContentBytes != int64(len(expected)) || len(first.Content) > mcpContentPageSize || !utf8.ValidString(first.Content) {
			t.Fatalf("unbounded %s result: %+v", tool, first)
		}
		var all strings.Builder
		all.WriteString(first.Content)
		seen := map[string]bool{}
		cursor := first.NextContentCursor
		for cursor != "" {
			if seen[cursor] {
				t.Fatal("continuation made no progress")
			}
			seen[cursor] = true
			chunk := callMCPTool(t, env, token, "node_read", map[string]any{"nodeId": created.ID.String(), "contentCursor": cursor})
			next := chunk.StructuredContent.Node
			if chunk.IsError || next.Revision != first.Revision || next.ContentBytes != first.ContentBytes || len(next.Content) > mcpContentPageSize || !utf8.ValidString(next.Content) {
				t.Fatalf("invalid continuation %+v", next)
			}
			all.WriteString(next.Content)
			cursor = next.NextContentCursor
		}
		if all.String() != expected {
			t.Fatalf("%s continuation lost content: %d/%d", tool, all.Len(), len(expected))
		}
		currentRevision = first.Revision
	}
}
