// Tests actual record/query dependencies, saved-view windows, and canonical MCP tool/resource projections.

package server

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/maruel/gomode/mcp"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/mcpschema"
)

func TestRecordResources(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	token, ws := setupGoModeWorkspace(t, env, "record-context@example.com")
	base := "/api/v1/workspaces/" + ws.String() + "/nodes/"
	var table dto.CreateTableUnderParentResponse
	if status := env.doJSON(t, http.MethodPost, base+"0/table/create", dto.CreateTableRequest{Title: "Scores", Properties: []dto.Property{{Name: "Score", Type: dto.PropertyTypeNumber}, {Name: "Keep", Type: dto.PropertyTypeCheckbox}}}, &table, token); status != http.StatusOK {
		t.Fatalf("create table: %d", status)
	}
	var first, second dto.CreateRecordResponse
	for i, target := range []*dto.CreateRecordResponse{&first, &second} {
		if status := env.doJSON(t, http.MethodPost, base+table.ID.String()+"/table/records/create", dto.CreateRecordRequest{Data: map[string]any{"Score": (i + 1) * 10, "Keep": true}}, target, token); status != http.StatusOK {
			t.Fatalf("create record: %d", status)
		}
	}
	var view dto.CreateViewResponse
	if status := env.doJSON(t, http.MethodPost, base+table.ID.String()+"/views/create", dto.CreateViewRequest{Name: "Ranked", Type: dto.ViewTypeTable}, &view, token); status != http.StatusOK {
		t.Fatalf("create view: %d", status)
	}
	if status := env.doJSON(t, http.MethodPost, base+table.ID.String()+"/views/"+view.View.ID.String(), dto.UpdateViewRequest{ExpectedRevision: view.View.Revision, Filters: []dto.Filter{{Property: "Keep", Operator: dto.FilterOpEquals, Value: true}}, Sorts: []dto.Sort{{Property: "Score", Direction: dto.SortAsc}}}, nil, token); status != http.StatusOK {
		t.Fatalf("configure view: %d", status)
	}
	var node dto.NodeResponse
	if status := env.doJSON(t, http.MethodGet, base+table.ID.String(), nil, &node, token); status != http.StatusOK {
		t.Fatalf("read table schema: %d", status)
	}
	var originalFirst, originalSecond dto.GetRecordResponse
	for i, target := range []*dto.GetRecordResponse{&originalFirst, &originalSecond} {
		id := []dto.CreateRecordResponse{first, second}[i].ID
		if status := env.doJSON(t, http.MethodGet, base+table.ID.String()+"/table/records/"+id.String(), nil, target, token); status != http.StatusOK {
			t.Fatalf("read original record: %d", status)
		}
	}
	endpoint := goModeMCPEndpoint + "?workspaceId=" + ws.String()
	meta := map[string]any{"openai/session": "record-conversation"}
	call := func(name string, args map[string]any) mcpschema.RecordsSnapshot {
		raw := callMCPNamedAt(t, env, token, endpoint, "tools/call", map[string]any{"name": name, "arguments": args, "_meta": meta}, name)
		var result struct {
			Snapshot mcpschema.RecordsSnapshot `json:"structuredContent"`
			IsError  bool                      `json:"isError"`
		}
		if err := json.Unmarshal(raw, &result); err != nil || result.IsError {
			t.Fatalf("%s: %s, %v", name, raw, err)
		}
		return result.Snapshot
	}
	readContext := func() mcpschema.ConversationContext {
		raw := callMCPNamedAt(t, env, token, endpoint, "resources/read", map[string]any{"uri": voiceContextURI, "_meta": meta}, voiceContextURI)
		var resource mcp.ResourcesReadResult
		if err := json.Unmarshal(raw, &resource); err != nil {
			t.Fatal(err)
		}
		var context mcpschema.ConversationContext
		if len(resource.Contents) != 1 {
			t.Fatalf("context contents: %+v", resource)
		}
		if err := json.Unmarshal([]byte(resource.Contents[0].Text), &context); err != nil {
			t.Fatal(err)
		}
		return context
	}
	// Schema reads, one offscreen record, and a query range establish distinct
	// dependencies. Another row can enter the query without changing the schema
	// or the directly watched record.
	callMCPNamedAt(t, env, token, endpoint, "tools/call", map[string]any{"name": "node_read", "arguments": map[string]any{"nodeId": table.ID.String()}, "_meta": meta}, "node_read")
	record := call("record_read", map[string]any{"nodeId": table.ID.String(), "recordId": first.ID.String()})
	query := call("records_query", map[string]any{"nodeId": table.ID.String(), "viewId": view.View.ID.String(), "limit": 1})
	if len(query.Records) != 1 || query.Records[0].ID != first.ID || query.NextURI == "" || query.Revision == "" {
		t.Fatalf("saved-view range: %+v", query)
	}
	raw := callMCPNamedAt(t, env, token, endpoint, "resources/read", map[string]any{"uri": query.URI, "_meta": meta}, query.URI)
	var resource mcp.ResourcesReadResult
	if err := json.Unmarshal(raw, &resource); err != nil {
		t.Fatal(err)
	}
	var snapshot mcpschema.RecordsSnapshot
	if err := json.Unmarshal([]byte(resource.Contents[0].Text), &snapshot); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(query, snapshot) {
		t.Fatalf("tool/resource data differ: %+v / %+v", query, snapshot)
	}
	if status := env.doJSON(t, http.MethodPost, base+table.ID.String()+"/table/records/"+second.ID.String(), dto.UpdateRecordRequest{Data: map[string]any{"Score": 5, "Keep": true}, ExpectedRevision: originalSecond.Revision, ExpectedSchemaRevision: node.SchemaRevision}, nil, token); status != http.StatusOK {
		t.Fatalf("change query ordering: %d", status)
	}
	context := readContext()
	if len(context.Resources) != 3 || !slices.ContainsFunc(context.Resources, func(w mcpschema.ResourceContext) bool { return w.URI == query.URI }) {
		t.Fatalf("actual read dependencies missing: %+v", context)
	}
	for _, watched := range context.Resources {
		if watched.URI == query.URI {
			if watched.State != mcpschema.ResourceContextChanged || !slices.Contains(watched.Aspects, "records") {
				t.Fatalf("query did not watch membership/order: %+v", watched)
			}
		} else if watched.State != mcpschema.ResourceContextRead {
			t.Fatalf("unrelated schema/record invalidated: %+v", watched)
		}
	}
	current := call("records_query", map[string]any{"nodeId": table.ID.String(), "viewId": view.View.ID.String(), "limit": 1})
	if current.Records[0].ID != second.ID || current.Revision == query.Revision {
		t.Fatalf("query order did not change: %+v", current)
	}
	if status := env.doJSON(t, http.MethodPost, base+table.ID.String()+"/table/records/"+first.ID.String(), dto.UpdateRecordRequest{Data: map[string]any{"Score": 40, "Keep": true}, ExpectedRevision: originalFirst.Revision, ExpectedSchemaRevision: node.SchemaRevision}, nil, token); status != http.StatusOK {
		t.Fatalf("change watched record: %d", status)
	}
	context = readContext()
	index := slices.IndexFunc(context.Resources, func(w mcpschema.ResourceContext) bool { return w.URI == record.URI })
	if index < 0 || context.Resources[index].State != mcpschema.ResourceContextChanged {
		t.Fatalf("actual record edit missed: %+v", context)
	}
}
