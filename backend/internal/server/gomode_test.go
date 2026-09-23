// Tests authenticated Go Mode MCP reads across nested workspace content.

package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/maruel/gomode/mcp"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
)

func TestGoModeMCP(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	var auth dto.AuthResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Email: "gomode@example.com", Password: "Pass1234", Name: "Go Mode",
	}, &auth, ""); status != http.StatusOK {
		t.Fatalf("register status = %d", status)
	}
	var org dto.OrganizationResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/organizations", dto.CreateOrganizationRequest{
		Name: "Go Mode Org",
	}, &org, auth.Token); status != http.StatusOK {
		t.Fatalf("create organization status = %d", status)
	}
	var ws dto.WorkspaceResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/organizations/"+org.ID.String()+"/workspaces", dto.CreateWorkspaceRequest{
		Name: "Go Mode Workspace",
	}, &ws, auth.Token); status != http.StatusOK {
		t.Fatalf("create workspace status = %d", status)
	}
	createPage := func(parentID, title string) dto.CreatePageResponse {
		var page dto.CreatePageResponse
		path := "/api/v1/workspaces/" + ws.ID.String() + "/nodes/" + parentID + "/page/create"
		if status := env.doJSON(t, http.MethodPost, path, dto.CreatePageRequest{Title: title}, &page, auth.Token); status != http.StatusOK {
			t.Fatalf("create %q status = %d", title, status)
		}
		return page
	}
	root := createPage("0", "Root")
	child := createPage(root.ID.String(), "Child")
	grandchild := createPage(child.ID.String(), "Grandchild")
	var switched dto.SwitchWorkspaceResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/switch-workspace", dto.SwitchWorkspaceRequest{
		WsID: ws.ID,
	}, &switched, auth.Token); status != http.StatusOK {
		t.Fatalf("switch workspace status = %d", status)
	}

	// MCP is a POST transport for reads; no database commit is allowed.
	env.services.RootRepo = nil
	result := callMCP(t, env, switched.Token, "resources/list", map[string]any{})
	var listed mcp.ResourcesListResult
	if err := json.Unmarshal(result, &listed); err != nil {
		t.Fatalf("decode resources/list: %v", err)
	}
	want := map[string]string{
		root.ID.String():       "Root",
		child.ID.String():      "Child",
		grandchild.ID.String(): "Grandchild",
	}
	if len(listed.Resources) != len(want) {
		t.Fatalf("resources/list returned %d resources, want %d", len(listed.Resources), len(want))
	}
	for _, resource := range listed.Resources {
		id := resource.Name
		if title, ok := want[id]; !ok || resource.Title != title {
			t.Errorf("unexpected resource %q (%q)", id, resource.Title)
		}
	}

	// A client can declare any content type, including multipart, for the MCP
	// transport. That declaration must not bypass the read-only body limit.
	bodyLimit := storage.DefaultServerQuotas().MaxRequestBodyBytes
	params := map[string]any{"_meta": map[string]any{
		"io.modelcontextprotocol/protocolVersion":    mcp.ProtocolVersion,
		"io.modelcontextprotocol/clientInfo":         map[string]string{"name": "mddb-test", "version": "1"},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "1", "method": "resources/list", "params": params})
	if err != nil {
		t.Fatalf("marshal MCP request: %v", err)
	}
	body = append(body, strings.Repeat(" ", int(bodyLimit))...)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, env.server.URL+goModeMCPEndpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create oversized MCP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+switched.Token)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	req.Header.Set("Mcp-Protocol-Version", mcp.ProtocolVersion)
	req.Header.Set("Mcp-Method", "resources/list")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send oversized MCP request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized multipart MCP status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func callMCP(t *testing.T, e *testEnv, token, method string, params map[string]any) json.RawMessage {
	params["_meta"] = map[string]any{
		"io.modelcontextprotocol/protocolVersion":    mcp.ProtocolVersion,
		"io.modelcontextprotocol/clientInfo":         map[string]string{"name": "mddb-test", "version": "1"},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "1", "method": method, "params": params})
	if err != nil {
		t.Fatalf("marshal MCP request: %v", err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, e.server.URL+goModeMCPEndpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create MCP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Protocol-Version", mcp.ProtocolVersion)
	req.Header.Set("Mcp-Method", method)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send MCP request: %v", err)
	}
	data, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read MCP response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("MCP status = %d: %s", resp.StatusCode, data)
	}
	var result struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode MCP response: %v", err)
	}
	if len(result.Error) != 0 {
		t.Fatalf("MCP error: %s", result.Error)
	}
	return result.Result
}
