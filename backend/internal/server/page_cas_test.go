// Tests conditional HTTP/MCP page mutations, canonical responses, and rejected-write event isolation.

package server

import (
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/sse"
)

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
	events, unsubscribe := env.services.Broker.SubscribeEvents(wsID)
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
		if status := env.doJSON(t, http.MethodPost, base+created.ID.String()+path, request, nil, token); status != http.StatusConflict {
			t.Fatalf("stale%s status=%d", path, status)
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
	}
	// Large mutation results use the same bounded windows and revision-bound
	// continuation as node_read; every byte remains retrievable.
	large := strings.Repeat("🌳 body\n", 10000)
	expected := ""
	currentRevision := outcome.StructuredContent.Node.Revision
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
