// Tests HttpOnly browser transport, native-shaped cookie requests, CSRF protection, and authoritative credential selection.

package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/maruel/gomode/mcp"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
)

func TestBrowserSessions(t *testing.T) {
	e := setupTestEnv(t)
	request := func(method, path string, body any, cookie *http.Cookie, bearer, origin, site string) (int, http.Header, map[string]json.RawMessage) {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r, err := http.NewRequestWithContext(t.Context(), method, e.server.URL+path, bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if site != "" {
			r.Header.Set("Sec-Fetch-Site", site)
		}
		if path == goModeMCPEndpoint {
			r.Header.Set("Mcp-Protocol-Version", mcp.ProtocolVersion)
			r.Header.Set("Mcp-Method", "server/discover")
		}
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(res.Body)
		if closeErr := res.Body.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]json.RawMessage
		if res.Header.Get("Content-Type") == "application/json" {
			if err := json.Unmarshal(data, &out); err != nil {
				t.Fatal(err)
			}
		}
		return res.StatusCode, res.Header, out
	}
	body := dto.RegisterRequest{Email: "cookie@example.com", Password: "password123", Name: "Browser"}
	status, headers, out := request("POST", "/api/v1/auth/browser/register", body, nil, "", "", "")
	if status != 200 {
		t.Fatalf("register: %d", status)
	}
	if _, exists := out["token"]; exists {
		t.Fatal("browser JSON exposes credential")
	}
	if len(out["id"]) == 0 {
		t.Fatal("browser registration omitted account")
	}
	cookies := (&http.Response{Header: headers}).Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies: %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != handlers.SessionCookieName || !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge <= 0 {
		t.Fatal("unsafe cookie attributes")
	}
	token := cookie.Value
	if got, _, _ := request("GET", "/api/v1/auth/me", nil, cookie, "", "", ""); got != 200 {
		t.Fatalf("cookie /me: %d", got)
	}
	var org dto.OrganizationResponse
	if got := e.doJSON(t, "POST", "/api/v1/organizations", dto.CreateOrganizationRequest{Name: "Cookie organization"}, &org, token); got != 200 {
		t.Fatal("create org failed")
	}
	var ws dto.WorkspaceResponse
	if got := e.doJSON(t, "POST", "/api/v1/organizations/"+org.ID.String()+"/workspaces", dto.CreateWorkspaceRequest{Name: "Cookie workspace"}, &ws, token); got != 200 {
		t.Fatal("create workspace failed")
	}
	if got, _, response := request("POST", "/api/v1/auth/switch-workspace", dto.SwitchWorkspaceRequest{WsID: ws.ID}, cookie, "", "", ""); got != 200 {
		t.Fatal("switch failed")
	} else if _, ok := response["token"]; ok {
		t.Fatal("workspace response exposed HttpOnly credential")
	}
	// Native requests have no browser Origin, fetch metadata or JS nonce.
	native := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "server/discover", "params": map[string]any{"_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": mcp.ProtocolVersion, "io.modelcontextprotocol/clientInfo": map[string]string{"name": "gomode-android", "version": "1"}, "io.modelcontextprotocol/clientCapabilities": map[string]any{}}}}
	if got, _, _ := request("POST", goModeMCPEndpoint, native, cookie, "", "", ""); got != 200 {
		t.Fatalf("native cookie MCP: %d", got)
	}
	if got, _, _ := request("GET", "/api/v1/auth/me", nil, cookie, token, "", ""); got != 200 {
		t.Fatalf("same credential rejected: %d", got)
	}
	var login dto.AuthResponse
	if got := e.doJSON(t, "POST", "/api/v1/auth/login", dto.LoginRequest{Email: body.Email, Password: body.Password}, &login, ""); got != 200 {
		t.Fatal("API login failed")
	}
	for _, bearer := range []string{login.Token, "forged"} {
		if got, _, _ := request("GET", "/api/v1/auth/me", nil, cookie, bearer, "", ""); got != 401 {
			t.Fatalf("conflicting credential: %d", got)
		}
	}
	// A stale browser operation must not act on a replacement session, including
	// a different session for the same account. The pin is public, not authority.
	var current dto.UserResponse
	if err := json.Unmarshal(out["session_id"], &current.SessionID); err != nil {
		t.Fatal(err)
	}
	if current.SessionID.IsZero() || login.User.SessionID == current.SessionID {
		t.Fatal("normal logins did not receive independent session identities")
	}
	for _, pin := range []string{current.SessionID.String(), "invalid"} {
		if got, _, _ := request("POST", "/api/v1/auth/browser/logout?session_id="+pin, nil, nil, login.Token, "", ""); got != 401 {
			t.Fatalf("stale session pin accepted: %d", got)
		}
	}
	if got, _, _ := request("GET", "/api/v1/auth/me?session_id="+login.User.SessionID.String(), nil, nil, login.Token, "", ""); got != 200 {
		t.Fatalf("matching session pin rejected: %d", got)
	}
	for _, bearer := range []string{"", token, "forged"} {
		for _, site := range []string{"", "cross-site"} {
			if got, _, _ := request("POST", "/api/v1/auth/browser/logout", nil, cookie, bearer, "https://attacker.example", site); got != 403 {
				t.Fatalf("cross-site mutation: %d", got)
			}
		}
	}
	if got, _, _ := request("POST", "/api/v1/auth/device/start", map[string]any{}, nil, "", "https://attacker.example", "cross-site"); got != 403 {
		t.Fatalf("new unauth endpoint bypasses CSRF classifier: %d", got)
	}
	if got, _, _ := request("POST", "/api/v1/auth/browser/logout", nil, cookie, "", e.server.URL, "same-origin"); got != 200 {
		t.Fatalf("logout: %d", got)
	}
	if got, _, _ := request("GET", "/api/v1/auth/me", nil, cookie, "", "", ""); got != 401 {
		t.Fatalf("revoked cookie accepted: %d", got)
	}
	if got, _, _ := request("GET", "/api/v1/auth/me", nil, nil, token, "", ""); got != 401 {
		t.Fatalf("revoked bearer accepted: %d", got)
	}
	if got, _, _ := request("GET", "/api/v1/auth/me", nil, nil, login.Token, "", ""); got != 200 {
		t.Fatalf("independent API session revoked: %d", got)
	}
}

func TestBrowserAuthenticationRateLimit(t *testing.T) {
	e := setupTestEnv(t)
	registration := dto.RegisterRequest{Email: "rate@example.com", Password: "password123", Name: "Rate"}
	if got := e.doJSON(t, "POST", "/api/v1/auth/browser/register", registration, nil, ""); got != 200 {
		t.Fatalf("register: %d", got)
	}
	for i := range 4 {
		path := "/api/v1/auth/browser/login"
		if i%2 == 0 {
			path = "/api/v1/auth/login"
		}
		if got := e.doJSON(t, "POST", path, dto.LoginRequest{Email: registration.Email, Password: "wrongpassword"}, nil, ""); got != 401 {
			t.Fatalf("login attempt %d: %d", i, got)
		}
	}
	// Both transports exhaust the same production five-attempt IP budget.
	for _, path := range []string{"/api/v1/auth/browser/login", "/api/v1/auth/browser/register", "/api/v1/auth/login", "/api/v1/auth/register"} {
		if got := e.doJSON(t, "POST", path, registration, nil, ""); got != 429 {
			t.Fatalf("unlimited authentication route %s: %d", path, got)
		}
	}
}
