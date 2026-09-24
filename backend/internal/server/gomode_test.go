// Tests authenticated Go Mode MCP reads and embedded voice gateway access.

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/maruel/gomode"
	"github.com/maruel/gomode/mcp"
	voiceapi "github.com/maruel/gomode/voicegateway/api"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

type testVoiceBridge struct {
	mu     sync.Mutex
	next   int
	closed []string
}

func (b *testVoiceBridge) HandleOffer(_ context.Context, sdp string) (answer, sessionID string, err error) {
	if sdp == "invalid" {
		return "", "", errors.New("invalid SDP")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.next++
	return "answer", fmt.Sprintf("session-%d", b.next), nil
}

func (b *testVoiceBridge) Close(id string) {
	b.mu.Lock()
	b.closed = append(b.closed, id)
	b.mu.Unlock()
}

func TestGoModeVoiceGateway(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	var auth dto.AuthResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Email: "voice@example.com", Password: "Pass1234", Name: "Voice",
	}, &auth, ""); status != http.StatusOK {
		t.Fatalf("register status = %d", status)
	}
	serverCfg := &storage.ServerConfig{JWTSecret: testJWTSecret, Quotas: storage.DefaultServerQuotas(), RateLimits: storage.DefaultRateLimits()}
	bridge := &testVoiceBridge{}
	server := httptest.NewServer(NewRouter(env.services, &Config{ServerConfig: serverCfg, Version: "test", VoiceBridge: bridge}))
	t.Cleanup(server.Close)

	discoveryReq, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/.well-known/gomode.json", http.NoBody)
	if err != nil {
		t.Fatalf("new discovery request: %v", err)
	}
	resp, err := http.DefaultClient.Do(discoveryReq)
	if err != nil {
		t.Fatalf("get discovery: %v", err)
	}
	var settings gomode.Settings
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatalf("decode discovery: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close discovery: %v", err)
	}
	if settings.WebShell.VoiceGateway.URL != "/" || !settings.WebShell.VoiceGateway.AuthRequired {
		t.Fatalf("voice discovery = %+v", settings.WebShell.VoiceGateway)
	}

	for _, tc := range []struct {
		name, path, body string
		want             int
	}{
		{name: "offer", path: "/api/voicegateway/v1/voice/rtc/offer", body: `{}`, want: http.StatusBadRequest},
		{name: "diagnostics", path: "/api/voicegateway/v1/voice/rtc/session/diagnostics", body: `{}`, want: http.StatusNotFound},
		{name: "close", path: "/api/voicegateway/v1/voice/rtc/session", want: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, authCase := range []struct {
				name, token string
				want        int
			}{
				{name: "anonymous", want: http.StatusUnauthorized},
				{name: "authenticated", token: auth.Token, want: tc.want},
			} {
				t.Run(authCase.name, func(t *testing.T) {
					req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+tc.path, strings.NewReader(tc.body))
					if err != nil {
						t.Fatalf("new request: %v", err)
					}
					if authCase.token != "" {
						req.Header.Set("Authorization", "Bearer "+authCase.token)
					}
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatalf("voice request: %v", err)
					}
					if err := resp.Body.Close(); err != nil {
						t.Fatalf("close voice response: %v", err)
					}
					if resp.StatusCode != authCase.want {
						t.Fatalf("voice status = %d, want %d", resp.StatusCode, authCase.want)
					}
				})
			}
		})
	}

	var other dto.AuthResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Email: "other-voice@example.com", Password: "Pass1234", Name: "Other Voice",
	}, &other, ""); status != http.StatusOK {
		t.Fatalf("register second user status = %d", status)
	}
	voiceRequest := func(path, body, token string) int {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatalf("new voice request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("voice request: %v", err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close voice response: %v", err)
		}
		return resp.StatusCode
	}
	offer := "/api/voicegateway/v1/voice/rtc/offer"
	if status := voiceRequest(offer, `{"sdp":"m=audio 9"}`, auth.Token); status != http.StatusOK {
		t.Fatalf("valid offer status = %d", status)
	}
	if status := voiceRequest(offer, `{}`, auth.Token); status != http.StatusBadRequest {
		t.Fatalf("malformed reconnect status = %d", status)
	}
	if status := voiceRequest(offer, `{"sdp":"invalid"}`, auth.Token); status != http.StatusInternalServerError {
		t.Fatalf("bridge-rejected reconnect status = %d", status)
	}
	bridge.mu.Lock()
	closed := append([]string(nil), bridge.closed...)
	bridge.mu.Unlock()
	if len(closed) != 0 {
		t.Fatalf("rejected reconnect closed active session: %v", closed)
	}
	if status := voiceRequest("/api/voicegateway/v1/voice/rtc/session-1/diagnostics", `{}`, auth.Token); status != http.StatusOK {
		t.Fatalf("original session after rejected reconnect status = %d", status)
	}
	if status := voiceRequest(offer, `{"sdp":"m=audio 9"}`, auth.Token); status != http.StatusOK {
		t.Fatalf("replacement offer status = %d", status)
	}
	bridge.mu.Lock()
	if len(bridge.closed) != 1 || bridge.closed[0] != "session-1" {
		t.Fatalf("replaced session closes = %v", bridge.closed)
	}
	bridge.mu.Unlock()
	if status := voiceRequest("/api/voicegateway/v1/voice/rtc/session-1/diagnostics", `{}`, auth.Token); status != http.StatusNotFound {
		t.Fatalf("replaced session diagnostics status = %d", status)
	}
	diagnostics := "/api/voicegateway/v1/voice/rtc/session-2/diagnostics"
	closeSession := "/api/voicegateway/v1/voice/rtc/session-2"
	for _, path := range []string{diagnostics, closeSession} {
		if status := voiceRequest(path, `{}`, other.Token); status != http.StatusNotFound {
			t.Fatalf("cross-user request %s status = %d", path, status)
		}
	}
	if status := voiceRequest(diagnostics, `{}`, auth.Token); status != http.StatusOK {
		t.Fatalf("owner diagnostics status = %d", status)
	}
	if status := voiceRequest(closeSession, `{}`, auth.Token); status != http.StatusOK {
		t.Fatalf("owner close status = %d", status)
	}
	if status := voiceRequest(diagnostics, `{}`, auth.Token); status != http.StatusNotFound {
		t.Fatalf("closed session diagnostics status = %d", status)
	}
	if status := voiceRequest(offer, `{"sdp":"m=audio 9"}`, auth.Token); status != http.StatusTooManyRequests {
		t.Fatalf("offer rate limit status = %d", status)
	}
	for _, tc := range []struct {
		path, token string
		status      int
		code        voiceapi.ErrorCode
	}{
		{path: offer, token: auth.Token, status: http.StatusTooManyRequests, code: voiceapi.CodeBadRequest},
		{path: diagnostics, token: other.Token, status: http.StatusNotFound, code: voiceapi.CodeNotFound},
	} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+tc.path, strings.NewReader(`{"sdp":"m=audio 9"}`))
		if err != nil {
			t.Fatalf("new error request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+tc.token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("voice error request: %v", err)
		}
		if resp.StatusCode != tc.status || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
			t.Fatalf("error response status/content-type = %d/%q", resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		var got voiceapi.ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode voice error: %v", err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close error response: %v", err)
		}
		if got.Error.Code != tc.code || got.Error.Message == "" {
			t.Fatalf("voice error = %+v, want code %s", got.Error, tc.code)
		}
	}
}

// TestGoModeMCPUsesDefaultWorkspaceAfterLRUClear guards that the MCP surface
// resolves the same default workspace the UI shows when the account has not
// recorded a recent workspace yet, instead of rejecting the request.
func TestGoModeMCPUsesDefaultWorkspaceAfterLRUClear(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	var auth dto.AuthResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Email: "gomode-no-lru@example.com", Password: "Pass1234", Name: "No LRU",
	}, &auth, ""); status != http.StatusOK {
		t.Fatalf("register status = %d", status)
	}
	var org dto.OrganizationResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/organizations", dto.CreateOrganizationRequest{
		Name: "No LRU Org",
	}, &org, auth.Token); status != http.StatusOK {
		t.Fatalf("create organization status = %d", status)
	}
	var ws dto.WorkspaceResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/organizations/"+org.ID.String()+"/workspaces", dto.CreateWorkspaceRequest{
		Name: "No LRU Workspace",
	}, &ws, auth.Token); status != http.StatusOK {
		t.Fatalf("create workspace status = %d", status)
	}
	var page dto.CreatePageResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/workspaces/"+ws.ID.String()+"/nodes/0/page/create", dto.CreatePageRequest{Title: "No LRU page"}, &page, auth.Token); status != http.StatusOK {
		t.Fatalf("create page status = %d", status)
	}

	// An account that never switched a workspace has no recorded recent one.
	if _, err := env.services.User.Modify(auth.User.ID, func(u *identity.User) error {
		u.Settings.LastActiveWorkspaces = nil
		return nil
	}); err != nil {
		t.Fatalf("clear active workspaces: %v", err)
	}

	result := callMCP(t, env, auth.Token, "resources/list", map[string]any{})
	var listed mcp.ResourcesListResult
	if err := json.Unmarshal(result, &listed); err != nil {
		t.Fatalf("decode resources/list: %v", err)
	}
	if len(listed.Resources) != 1 || listed.Resources[0].Name != page.ID.String() {
		t.Fatalf("resources = %#v, want the default workspace page", listed.Resources)
	}
}

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

// TestMCPToolSchemas validates every advertised workspace tool schema and
// guards the parameter header mapping, so a malformed read-only catalog fails
// the build instead of the request path.
func TestMCPToolSchemas(t *testing.T) {
	t.Parallel()

	registry := &workspaceRegistry{}
	if err := registry.validateToolSchemas(); err != nil {
		t.Fatalf("validateToolSchemas() error: %v", err)
	}

	// The read-only workspace tools take no header-mirrored parameters; the
	// empty expectations catch an accidental x-mcp-header or a renamed tool.
	expectedHeaders := map[string]map[string]string{
		"nodes_list": {},
		"node_read":  {},
	}
	specs := registry.specs()
	if len(specs) != len(expectedHeaders) {
		t.Fatalf("specs = %d, want %d", len(specs), len(expectedHeaders))
	}
	for _, spec := range specs {
		t.Run(spec.Name, func(t *testing.T) {
			t.Parallel()

			want, ok := expectedHeaders[spec.Name]
			if !ok {
				t.Fatalf("unexpected tool %q", spec.Name)
			}
			params, err := mcp.HeaderParams(spec.InputSchema)
			if err != nil {
				t.Fatalf("HeaderParams() error: %v", err)
			}
			got := make(map[string]string, len(params))
			for _, param := range params {
				if len(param.Path) != 1 {
					t.Fatalf("header %q path = %v, want one property", param.Header, param.Path)
				}
				got[param.Path[0]] = param.Header
			}
			if len(got) != len(want) {
				t.Fatalf("headers = %v, want %v", got, want)
			}
			for property, header := range want {
				if got[property] != header {
					t.Fatalf("header for %q = %q, want %q", property, got[property], header)
				}
			}
		})
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
