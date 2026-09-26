// Tests authenticated Go Mode MCP reads and writes and embedded voice gateway access.

package server

import (
	"bufio"
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
	"time"

	"github.com/maruel/gomode"
	"github.com/maruel/gomode/mcp"
	voiceapi "github.com/maruel/gomode/voicegateway/api"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/sse"
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

	// Every advertised tool carries no header-mirrored parameters; the empty
	// expectations catch an accidental x-mcp-header or a renamed tool.
	expectedHeaders := map[string]map[string]string{
		"nodes_list":  {},
		"node_read":   {},
		"node_update": {},
		"node_create": {},
		"node_append": {},
	}
	specs := registry.allSpecs()
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

// TestGoModeMCPResourceSubscription verifies that an MCP client subscribed to a
// node resource is notified over the SSE stream when that page changes.
func TestGoModeMCPResourceSubscription(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	// The router normally wires the broker; unit tests construct services directly.
	env.services.Broker = sse.NewBroker()
	token, wsID := setupGoModeWorkspace(t, env, "gomode-subscribe@example.com")

	created := callMCPTool(t, env, token, "node_create", map[string]any{"title": "Watched", "content": "Before"})
	if created.IsError {
		t.Fatalf("node_create = %#v", created)
	}
	id := created.StructuredContent.Node.ID
	uri := fmt.Sprintf("mddb://workspaces/%s/nodes/%s", wsID, id)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": "1", "method": "subscriptions/listen",
		"params": map[string]any{
			"notifications": map[string]any{"resourceSubscriptions": []string{uri}},
			"_meta": map[string]any{
				"io.modelcontextprotocol/protocolVersion":    mcp.ProtocolVersion,
				"io.modelcontextprotocol/clientInfo":         map[string]string{"name": "mddb-test", "version": "1"},
				"io.modelcontextprotocol/clientCapabilities": map[string]any{},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal subscriptions/listen: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, env.server.URL+goModeMCPEndpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create subscription request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Protocol-Version", mcp.ProtocolVersion)
	req.Header.Set("Mcp-Method", "subscriptions/listen")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open subscription: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("subscription status = %d", resp.StatusCode)
	}

	// The handler captures its dedup baseline before acknowledging, so a write
	// that lands after the acknowledgment is reported as a change.
	reader := bufio.NewReader(resp.Body)
	readSSEUntil(t, reader, "subscriptions/acknowledged")

	updated := callMCPTool(t, env, token, "node_update", map[string]any{"nodeId": id, "title": "Watched", "content": "After"})
	if updated.IsError {
		t.Fatalf("node_update = %#v", updated)
	}
	frame := readSSEUntil(t, reader, "notifications/resources/updated")
	if !strings.Contains(frame, uri) {
		t.Fatalf("update notification = %q, want uri %q", frame, uri)
	}
}

// readSSEUntil reads the stream line by line until one contains substr.
func readSSEUntil(t *testing.T, r *bufio.Reader, substr string) string {
	t.Helper()
	var seen strings.Builder
	for {
		line, err := r.ReadString('\n')
		seen.WriteString(line)
		if strings.Contains(line, substr) {
			return line
		}
		if err != nil {
			t.Fatalf("SSE stream ended before %q: %v\n%s", substr, err, seen.String())
		}
	}
}

// mcpToolOutcome is the decoded tools/call result shared by the MCP write tests.
type mcpToolOutcome struct {
	IsError           bool `json:"isError"`
	StructuredContent struct {
		Error string     `json:"error"`
		Node  nodeDetail `json:"node"`
	} `json:"structuredContent"`
}

// setupGoModeWorkspace registers a user who owns an organization and workspace,
// making them an editor, and switches to it. It returns the bearer token and the
// workspace ID.
func setupGoModeWorkspace(t *testing.T, env *testEnv, email string) (string, ksid.ID) {
	t.Helper()
	var auth dto.AuthResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Email: email, Password: "Pass1234", Name: "Go Mode",
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
	var switched dto.SwitchWorkspaceResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/switch-workspace", dto.SwitchWorkspaceRequest{
		WsID: ws.ID,
	}, &switched, auth.Token); status != http.StatusOK {
		t.Fatalf("switch workspace status = %d", status)
	}
	return switched.Token, ws.ID
}

// TestGoModeMCPWriteTools verifies that an editor can create, append to, and
// replace a page through the MCP tools, and that the edits persist for reads.
func TestGoModeMCPWriteTools(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	token, _ := setupGoModeWorkspace(t, env, "gomode-write@example.com")

	listed := callMCP(t, env, token, "tools/list", map[string]any{})
	var tools mcp.ToolsListResult
	if err := json.Unmarshal(listed, &tools); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	names := make(map[string]bool, len(tools.Tools))
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"node_update", "node_create", "node_append"} {
		if !names[want] {
			t.Fatalf("tools/list missing %q: %v", want, names)
		}
	}

	created := callMCPTool(t, env, token, "node_create", map[string]any{"title": "Agent page", "content": "First"})
	if created.IsError || created.StructuredContent.Node.Content != "First" {
		t.Fatalf("node_create = %#v, want created page", created)
	}
	id := created.StructuredContent.Node.ID

	appended := callMCPTool(t, env, token, "node_append", map[string]any{"nodeId": id, "content": "Second"})
	if got := appended.StructuredContent.Node.Content; got != "First\n\nSecond" {
		t.Fatalf("node_append content = %q, want %q", got, "First\n\nSecond")
	}

	updated := callMCPTool(t, env, token, "node_update", map[string]any{"nodeId": id, "title": "Renamed", "content": "Replaced"})
	if node := updated.StructuredContent.Node; node.Title != "Renamed" || node.Content != "Replaced" {
		t.Fatalf("node_update node = %#v, want renamed and replaced", node)
	}

	read := callMCPTool(t, env, token, "node_read", map[string]any{"nodeId": id})
	if node := read.StructuredContent.Node; node.Title != "Renamed" || node.Content != "Replaced" {
		t.Fatalf("node_read node = %#v, want the update to persist", node)
	}

	missing := callMCPTool(t, env, token, "node_update", map[string]any{"nodeId": ksid.ID(0).String(), "title": "x", "content": "y"})
	if !missing.IsError {
		t.Fatalf("node_update on a missing node = %#v, want a tool error", missing)
	}
}

// TestGoModeMCPWriteToolsRequireEditor guards that viewers neither see nor can
// call the mutating tools.
func TestGoModeMCPWriteToolsRequireEditor(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	_, wsID := setupGoModeWorkspace(t, env, "gomode-viewer@example.com")

	viewer := &workspaceRegistry{svc: env.services, wsID: wsID, role: identity.WSRoleViewer}
	tools, err := viewer.Tools(t.Context())
	if err != nil {
		t.Fatalf("Tools() error: %v", err)
	}
	for _, tool := range tools {
		switch tool.Name {
		case "node_update", "node_create", "node_append":
			t.Fatalf("viewer advertised write tool %q", tool.Name)
		}
	}
	args := json.RawMessage(`{"nodeId":"` + ksid.ID(0).String() + `","title":"x","content":"y"}`)
	if _, err := viewer.CallTool(t.Context(), "node_update", args); err == nil {
		t.Fatal("viewer CallTool(node_update) succeeded, want an error")
	}

	editor := &workspaceRegistry{svc: env.services, wsID: wsID, role: identity.WSRoleEditor}
	tools, err = editor.Tools(t.Context())
	if err != nil {
		t.Fatalf("Tools() error: %v", err)
	}
	found := false
	for _, tool := range tools {
		if tool.Name == "node_update" {
			found = true
		}
	}
	if !found {
		t.Fatal("editor did not see node_update")
	}
}

func callMCP(t *testing.T, e *testEnv, token, method string, params map[string]any) json.RawMessage {
	return callMCPNamed(t, e, token, method, params, "")
}

// callMCPTool calls a tool and decodes its structured result.
func callMCPTool(t *testing.T, e *testEnv, token, name string, args map[string]any) mcpToolOutcome {
	t.Helper()
	raw := callMCPNamed(t, e, token, "tools/call", map[string]any{"name": name, "arguments": args}, name)
	var out mcpToolOutcome
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode tools/call %s: %v", name, err)
	}
	return out
}

func callMCPNamed(t *testing.T, e *testEnv, token, method string, params map[string]any, name string) json.RawMessage {
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
	if name != "" {
		req.Header.Set("Mcp-Name", name)
	}
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
