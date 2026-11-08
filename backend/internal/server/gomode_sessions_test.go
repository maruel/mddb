// Tests authenticated MCP session persistence, owner isolation, and stream revocation over HTTP.

package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maruel/gomode/mcp"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/sse"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestGoModeMCPSessions(t *testing.T) {
	for _, version := range []string{"2025-06-18", "2025-11-25"} {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			env := setupTestEnv(t)
			env.services.Broker = sse.NewBroker()
			token, ws := setupGoModeWorkspace(t, env, "owner@example.com")
			endpoint := env.server.URL + goModeMCPEndpoint + "?workspaceId=" + ws.String()
			post := func(session, method string, params any) (int, http.Header, mcp.JSONRPCResponse) {
				body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
				if err != nil {
					t.Fatal(err)
				}
				req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Mcp-Protocol-Version", version)
				if session != "" {
					req.Header.Set("Mcp-Session-Id", session)
				}
				resp, err := env.server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(resp.Body)
				closeErr := resp.Body.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("read=%v close=%v", err, closeErr)
				}
				var out mcp.JSONRPCResponse
				if resp.StatusCode == http.StatusOK {
					if err := json.Unmarshal(data, &out); err != nil {
						t.Fatalf("decode %s: %v: %s", method, err, data)
					}
				}
				return resp.StatusCode, resp.Header, out
			}
			status, headers, out := post("", "initialize", map[string]any{"protocolVersion": version, "clientInfo": map[string]string{"name": "session-test", "version": "1"}, "capabilities": map[string]any{}})
			if status != http.StatusOK || out.Error != nil || headers.Get("Mcp-Session-Id") == "" {
				t.Fatalf("initialize status=%d session=%q error=%v", status, headers.Get("Mcp-Session-Id"), out.Error)
			}
			session := headers.Get("Mcp-Session-Id")
			firstWorkspace, err := env.wsService.Get(ws)
			if err != nil {
				t.Fatal(err)
			}
			var otherWorkspace dto.WorkspaceResponse
			if status := env.doJSON(t, http.MethodPost, "/api/v1/organizations/"+firstWorkspace.OrganizationID.String()+"/workspaces", dto.CreateWorkspaceRequest{Name: "Other workspace"}, &otherWorkspace, token); status != http.StatusOK {
				t.Fatalf("create second workspace status=%d", status)
			}
			var switched dto.SwitchWorkspaceResponse
			if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/switch-workspace", dto.SwitchWorkspaceRequest{WsID: otherWorkspace.ID}, &switched, token); status != http.StatusOK {
				t.Fatalf("switch global workspace status=%d", status)
			}
			scopedEndpoint := endpoint
			endpoint = env.server.URL + goModeMCPEndpoint
			genericStatus, genericHeaders, genericOut := post("", "initialize", map[string]any{"protocolVersion": version, "clientInfo": map[string]string{"name": "generic", "version": "1"}, "capabilities": map[string]any{}})
			if genericStatus != http.StatusOK || genericOut.Error != nil {
				t.Fatalf("generic initialize status=%d error=%v", genericStatus, genericOut.Error)
			}
			_, _, guidance := post(genericHeaders.Get("Mcp-Session-Id"), "resources/subscribe", map[string]any{"uri": voiceItemsURI, "_meta": map[string]string{"openai/session": "conversation-a"}})
			if guidance.Error == nil || !strings.Contains(guidance.Error.Message, "?workspaceId=") {
				t.Fatalf("unscoped conversation subscription did not explain scope: %+v", guidance)
			}
			endpoint = scopedEndpoint
			if status, _, out := post(session, "resources/subscribe", map[string]string{"uri": voiceItemsURI}); status != http.StatusOK || out.Error != nil {
				t.Fatalf("subscribe status=%d error=%v", status, out.Error)
			}
			var login dto.AuthResponse
			if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/login", dto.LoginRequest{Email: "owner@example.com", Password: "Pass1234"}, &login, ""); status != http.StatusOK {
				t.Fatalf("second login status=%d", status)
			}
			ownerToken := token
			token = login.Token
			if status, _, _ := post(session, "resources/read", map[string]string{"uri": voiceItemsURI}); status != http.StatusNotFound {
				t.Fatalf("another login reused session: status=%d", status)
			}
			token = ownerToken
			scoped := endpoint
			endpoint = env.server.URL + goModeMCPEndpoint
			if status, _, _ := post(session, "resources/read", map[string]string{"uri": voiceItemsURI}); status != http.StatusNotFound {
				t.Fatalf("another workspace scope reused session: status=%d", status)
			}
			endpoint = scoped
			open := func(session string) (<-chan string, <-chan error) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				t.Cleanup(cancel)
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Mcp-Session-Id", session)
				req.Header.Set("Mcp-Protocol-Version", version)
				req.Header.Set("Accept", "text/event-stream")
				resp, err := env.server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				if resp.StatusCode != http.StatusOK {
					_ = resp.Body.Close()
					t.Fatalf("stream status=%d", resp.StatusCode)
				}
				var wg sync.WaitGroup
				done := make(chan error, 1)
				updates := make(chan string, 16)
				wg.Go(func() {
					reader := bufio.NewScanner(resp.Body)
					for reader.Scan() {
						data, ok := strings.CutPrefix(reader.Text(), "data: ")
						if !ok {
							continue
						}
						var event struct {
							Method string `json:"method"`
							Params struct {
								URI string `json:"uri"`
							} `json:"params"`
						}
						if err := json.Unmarshal([]byte(data), &event); err != nil {
							done <- err
							return
						}
						if event.Method == string(mcp.NotificationMethodResourcesUpdated) {
							select {
							case updates <- event.Params.URI:
							case <-ctx.Done():
								done <- ctx.Err()
								return
							}
						}
					}
					done <- reader.Err()
				})
				t.Cleanup(func() { _ = resp.Body.Close(); wg.Wait() })
				return updates, done
			}
			updates, stopped := open(session)
			waitUpdate := func() {
				select {
				case uri := <-updates:
					if uri != voiceItemsURI {
						t.Fatalf("update uri=%q", uri)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("resource update was not delivered")
				}
			}
			waitUpdate()
			if status, _, result := post(session, "tools/call", map[string]any{"name": "page_create", "arguments": map[string]string{"title": "Released update", "content": "changed"}}); status != http.StatusOK || result.Error != nil {
				t.Fatalf("scoped page create status=%d error=%v", status, result.Error)
			}
			waitUpdate()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, endpoint, http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Mcp-Session-Id", session)
			req.Header.Set("Mcp-Protocol-Version", version)
			resp, err := env.server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("delete status=%d", resp.StatusCode)
			}
			select {
			case err := <-stopped:
				if err != nil {
					t.Fatalf("stream ended with error: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("DELETE did not stop the existing stream")
			}
			if status, _, _ := post(session, "resources/read", map[string]string{"uri": voiceItemsURI}); status != http.StatusNotFound {
				t.Fatalf("deleted session status=%d", status)
			}
			status, headers, out = post("", "initialize", map[string]any{"protocolVersion": version, "clientInfo": map[string]string{"name": "session-test", "version": "1"}, "capabilities": map[string]any{}})
			if status != http.StatusOK || out.Error != nil {
				t.Fatalf("initialize status=%d error=%v", status, out.Error)
			}
			session = headers.Get("Mcp-Session-Id")
			_, stopped = open(session)
			u, err := env.userService.GetByEmail("owner@example.com")
			if err != nil {
				t.Fatal(err)
			}
			workspace, err := env.wsService.Get(ws)
			if err != nil {
				t.Fatal(err)
			}
			membership, err := env.orgMemService.Get(u.ID, workspace.OrganizationID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := env.orgMemService.Modify(membership.ID, func(m *identity.OrganizationMembership) error { m.Role = identity.OrgRoleMember; return nil }); err != nil {
				t.Fatal(err)
			}
			wm, err := env.wsMemService.Get(u.ID, ws)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := env.wsMemService.Modify(wm.ID, func(m *identity.WorkspaceMembership) error { m.Role = identity.WSRoleViewer; return nil }); err != nil {
				t.Fatal(err)
			}
			env.services.Broker.Publish(ws, &dto.WorkspaceEvent{Type: dto.EventNodeUpdated})
			select {
			case err := <-stopped:
				if err != nil {
					t.Fatalf("empty-filter stream ended with error: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("role change did not stop empty-filter stream")
			}
			status, headers, out = post("", "initialize", map[string]any{"protocolVersion": version, "clientInfo": map[string]string{"name": "session-test", "version": "1"}, "capabilities": map[string]any{}})
			if status != http.StatusOK || out.Error != nil {
				t.Fatalf("initialize status=%d error=%v", status, out.Error)
			}
			session = headers.Get("Mcp-Session-Id")
			_, stopped = open(session)
			if _, err := env.services.Session.RevokeAllForUser(u.ID); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-stopped:
				if err != nil {
					t.Fatalf("revoked stream ended with error: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("revocation did not stop the stream")
			}
			if status, _, _ := post(session, "resources/read", map[string]string{"uri": voiceItemsURI}); status != http.StatusUnauthorized {
				t.Fatalf("revoked login accepted: status=%d", status)
			}
		})
	}
}

func TestGoModeMCPStreamLifetime(t *testing.T) {
	for _, method := range []string{"missing-header", "mismatched-header", "revoke", "role", "membership"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			env := setupTestEnv(t)
			env.services.Broker = sse.NewBroker()
			token, wsID := setupGoModeWorkspace(t, env, "native-lifetime@example.com")
			body, err := json.Marshal(map[string]any{
				"jsonrpc": "2.0", "id": 1, "method": "subscriptions/listen",
				"params": map[string]any{
					"notifications": map[string]any{"resourceSubscriptions": []string{voiceItemsURI}},
					"_meta": map[string]any{
						"io.modelcontextprotocol/protocolVersion":    mcp.ProtocolVersion,
						"io.modelcontextprotocol/clientInfo":         map[string]string{"name": "lifetime-test", "version": "1"},
						"io.modelcontextprotocol/clientCapabilities": map[string]any{},
						"openai/session": "native-lifetime",
					},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			t.Cleanup(cancel)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, env.server.URL+goModeMCPEndpoint+"?workspaceId="+wsID.String(), bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Mcp-Protocol-Version", mcp.ProtocolVersion)
			if method == "mismatched-header" {
				req.Header.Set("Mcp-Method", "resources/list")
			} else if method != "missing-header" {
				req.Header.Set("Mcp-Method", "subscriptions/listen")
			}
			resp, err := env.server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = resp.Body.Close() })
			if method == "missing-header" || method == "mismatched-header" {
				var failure mcp.JSONRPCResponse
				if err := json.NewDecoder(resp.Body).Decode(&failure); err != nil {
					t.Fatal(err)
				}
				if failure.Error == nil || resp.Header.Get("Content-Type") == "text/event-stream" ||
					(method == "mismatched-header" && resp.StatusCode != http.StatusBadRequest) {
					t.Fatalf("invalid header opened a stream: status=%d type=%q", resp.StatusCode, resp.Header.Get("Content-Type"))
				}
				return
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("stream status=%d", resp.StatusCode)
			}
			readSSEUntil(t, bufio.NewReader(resp.Body), "subscriptions/acknowledged")
			u, err := env.userService.GetByEmail("native-lifetime@example.com")
			if err != nil {
				t.Fatal(err)
			}
			switch method {
			case "revoke":
				if _, err := env.services.Session.RevokeAllForUser(u.ID); err != nil {
					t.Fatal(err)
				}
			case "role", "membership":
				ws, err := env.wsService.Get(wsID)
				if err != nil {
					t.Fatal(err)
				}
				membership, err := env.orgMemService.Get(u.ID, ws.OrganizationID)
				if err != nil {
					t.Fatal(err)
				}
				if method == "membership" {
					if err := env.orgMemService.Delete(membership.ID); err != nil {
						t.Fatal(err)
					}
				} else {
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
				env.services.Broker.Publish(wsID, &dto.WorkspaceEvent{Type: dto.EventNodeUpdated})
			}
			if _, err := io.Copy(io.Discard, resp.Body); err != nil {
				t.Fatalf("revoked stream did not end cleanly before its deadline: %v", err)
			}
		})
	}
}

func TestGoModeMCPOrigin(t *testing.T) {
	env := setupTestEnv(t)
	token, _ := setupGoModeWorkspace(t, env, "origin@example.com")
	for _, origin := range []string{"", "http://localhost:8080", "https://foreign.example", "null"} {
		t.Run(origin, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, env.server.URL+goModeMCPEndpoint, http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			resp, err := env.server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			forbidden := origin != "" && origin != "http://localhost:8080"
			if (resp.StatusCode == http.StatusForbidden) != forbidden {
				t.Fatalf("origin=%q status=%d", origin, resp.StatusCode)
			}
		})
	}
}

func TestValidGoModeOrigin(t *testing.T) {
	for _, tc := range []struct {
		origin []string
		want   bool
	}{
		{nil, true}, {[]string{"https://example.com"}, true}, {[]string{"https://EXAMPLE.com"}, true},
		{[]string{"http://example.com"}, false}, {[]string{"https://example.com:443"}, false},
		{[]string{"https://example.com/"}, false}, {[]string{"https://example.com?x=y"}, false},
		{[]string{"https://user@example.com"}, false}, {[]string{"null"}, false},
		{[]string{"https://example.com", "https://example.com"}, false},
	} {
		req := &http.Request{Header: http.Header{"Origin": tc.origin}}
		if got := validGoModeOrigin(req, "https://example.com/reverse/proxy"); got != tc.want {
			t.Errorf("origin=%q got=%v want=%v", tc.origin, got, tc.want)
		}
	}
}
