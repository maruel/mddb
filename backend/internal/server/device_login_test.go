// Tests device pairing session parity, individual revocation, eligibility, limits, and durable audit failure.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maruel/mddb/backend/internal/server/handlers"

	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/gomode/devicelogin"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestDeviceLogin(t *testing.T) {
	// KSID's process-wide generator must not interleave real and frozen clocks.
	// The real HTTP fixture owns sessions and background workers outside each
	// bubble. Align virtual time with those sessions before advancing the poll.
	afterPollInterval := func(t *testing.T, f func(*testing.T)) {
		now := time.Now()
		synctest.Test(t, func(t *testing.T) {
			time.Sleep(time.Until(now) + 5*time.Second)
			f(t)
		})
	}
	setup := func(t *testing.T) (*testEnv, http.Handler, string, dto.AuthResponse) {
		env := setupTestEnv(t)
		dir := t.TempDir()
		var auth dto.AuthResponse
		if status := env.doJSON(t, "POST", "/api/v1/auth/register", dto.RegisterRequest{Email: "pair@example.com", Password: "Password123", Name: "Pair"}, &auth, ""); status != 200 {
			t.Fatalf("register = %d", status)
		}
		cfg := &Config{ServerConfig: &storage.ServerConfig{JWTSecret: testJWTSecret, Quotas: storage.DefaultServerQuotas(false), RateLimits: storage.DefaultRateLimits()}, ConfigDir: dir, BaseURL: "https://mddb.example", FastRateLimits: true, VoiceBridge: &testVoiceBridge{}, OAuth: OAuthConfig{GoogleClientID: "client", GoogleClientSecret: "secret"}}
		h := NewRouter(env.services, cfg)
		t.Cleanup(h.Close)
		return env, h, dir, auth
	}
	t.Run("normal session user agent is UTF-8 bounded", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			in   string
			want string
		}{
			{"multibyte preserved", strings.Repeat("x", 199) + "é", strings.Repeat("x", 199) + "é"},
			{"byte limit", strings.Repeat("x", 255) + "é", strings.Repeat("x", 255)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				env, _, _, auth := setup(t)
				user, err := env.services.User.Get(auth.User.ID)
				if err != nil {
					t.Fatal(err)
				}
				cfg := &handlers.Config{ServerConfig: storage.ServerConfig{JWTSecret: testJWTSecret, Quotas: storage.DefaultServerQuotas(false)}}
				_, sid, err := cfg.IssueToken(env.services.Session, user, "192.0.2.123", tc.in, "")
				if err != nil {
					t.Fatal(err)
				}
				session, err := env.services.Session.Get(sid)
				if err != nil {
					t.Fatal(err)
				}
				if session.DeviceInfo != tc.want {
					t.Fatalf("stored user agent = %q, want %q", session.DeviceInfo, tc.want)
				}
			})
		}
	})
	post := func(h http.Handler, name, body, token, csrf string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, deviceLoginPrefix+name, strings.NewReader(body))
		r.RemoteAddr = "192.0.2.123:1234"
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("User-Agent", "Display browser")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if csrf != "" {
			r.Header.Set("X-Device-Login-CSRF", csrf)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	start := func(t *testing.T, h http.Handler) devicelogin.StartResponse {
		w := post(h, "start", "{}", "", "")
		if w.Code != 200 {
			t.Fatalf("start %d: %s", w.Code, w.Body)
		}
		var code devicelogin.StartResponse
		if err := json.Unmarshal(w.Body.Bytes(), &code); err != nil {
			t.Fatal(err)
		}
		return code
	}
	lookup := func(t *testing.T, h http.Handler, code, token string) devicelogin.LookupResponse {
		w := post(h, "lookup", `{"user_code":"`+code+`"}`, token, "")
		if w.Code != 200 {
			t.Fatalf("lookup %d: %s", w.Code, w.Body)
		}
		var d devicelogin.LookupResponse
		if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	t.Run("completion persistence", func(t *testing.T) {
		for _, failure := range []string{"none", "before creation", "after creation", "created audit", "response audit", "write", "concurrent", "cancel"} {
			t.Run(failure, func(t *testing.T) {
				t.Parallel()
				env, _, dir, auth := setup(t)
				hostcfg := &Config{ServerConfig: &storage.ServerConfig{JWTSecret: testJWTSecret}, ConfigDir: dir, BaseURL: "https://mddb.example"}
				hcfg := &handlers.Config{JWTSecret: testJWTSecret}
				ctx, cancel := context.WithCancel(t.Context())
				t.Cleanup(cancel)
				host := &deviceCompletionHost{Host: &deviceLoginHost{services: env.services, config: hostcfg, auth: handlers.NewAuthHandler(env.services, hcfg)}, failure: failure, cancel: cancel, ip: netip.MustParseAddr("192.0.2.123")}
				core := devicelogin.New(host)
				mux := http.NewServeMux()
				mux.HandleFunc("POST "+deviceLoginPrefix+"start", core.Start)
				mux.HandleFunc("POST "+deviceLoginPrefix+"lookup", core.Lookup)
				mux.HandleFunc("POST "+deviceLoginPrefix+"approve", core.Approve)
				mux.HandleFunc("POST "+deviceLoginPrefix+"poll", core.Poll)
				code := start(t, mux)
				detail := lookup(t, mux, code.UserCode, auth.Token)
				if w := post(mux, "approve", `{"user_code":"`+code.UserCode+`","approve":true}`, auth.Token, detail.CSRFToken); w.Code != 200 {
					t.Fatal(w.Body.String())
				}
				afterPollInterval(t, func(t *testing.T) {
					poll := func() {
						r := httptest.NewRequestWithContext(ctx, http.MethodPost, deviceLoginPrefix+"poll", strings.NewReader(`{"device_code":"`+code.DeviceCode+`"}`))
						r.Header.Set("Content-Type", "application/json")
						r.Header.Set("Accept-Encoding", "gzip")
						w := http.ResponseWriter(httptest.NewRecorder())
						if failure == "write" {
							w = &deviceFailedWriter{w}
						}
						compressMiddleware(http.HandlerFunc(core.Poll)).ServeHTTP(w, r)
					}
					if failure == "concurrent" {
						var wg sync.WaitGroup
						for range 12 {
							wg.Go(poll)
						}
						wg.Wait()
					} else {
						poll()
					}
					restarted, err := identity.NewSessionService(filepath.Join(env.dataDir, "db", "sessions.jsonl"))
					if err != nil {
						t.Fatal(err)
					}
					active := restarted.CountActive()
					// setup created the approver's independent normal session.
					if failure == "none" || failure == "concurrent" {
						if active != 2 || host.created.Load() != 1 {
							t.Fatalf("active %d created %d", active, host.created.Load())
						}
					} else if active != 1 {
						t.Fatalf("failed completion left %d active sessions", active)
					}
					if failure == "before creation" {
						if host.created.Load() != 0 {
							t.Fatal("creation attempted after prerequisite failed")
						}
					} else if host.created.Load() != 1 {
						t.Fatalf("created %d sessions", host.created.Load())
					}
					before := host.created.Load()
					poll()
					if host.created.Load() != before {
						t.Fatal("retry duplicated persistent session")
					}
					data, err := fs.ReadFile(os.DirFS(dir), "device-login-audit.jsonl")
					if err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(string(data), code.RequestID) || strings.Contains(string(data), code.DeviceCode) || strings.Contains(string(data), code.UserCode) {
						t.Fatal("audit correlation/secrecy")
					}
				})
			})
		}
	})
	t.Run("full middleware observes failed completion writes", func(t *testing.T) {
		env, h, dir, auth := setup(t)
		code := start(t, h)
		detail := lookup(t, h, code.UserCode, auth.Token)
		if w := post(h, "approve", `{"user_code":"`+code.UserCode+`","approve":true}`, auth.Token, detail.CSRFToken); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		afterPollInterval(t, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, deviceLoginPrefix+"poll", strings.NewReader(`{"device_code":"`+code.DeviceCode+`"}`))
			r.RemoteAddr = "192.0.2.123:1234"
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Accept-Encoding", "gzip")
			h.ServeHTTP(&deviceFailedWriter{httptest.NewRecorder()}, r)
			if env.services.Session.CountActive() != 1 {
				t.Fatal("failed compressed delivery left display session active")
			}
			data, err := fs.ReadFile(os.DirFS(dir), "device-login-audit.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), `"kind":"response_failed"`) || !strings.Contains(string(data), `"kind":"session_revoked"`) {
				t.Fatal("missing delivery cleanup audit")
			}
		})
	})
	t.Run("normal session and individual revocation", func(t *testing.T) {
		env, h, dir, auth := setup(t)
		code := start(t, h)
		if code.VerificationURI != "https://mddb.example/device" {
			t.Fatalf("verification URI %q", code.VerificationURI)
		}
		if w := post(h, "lookup", `{"user_code":"`+code.UserCode+`"}`, "", ""); w.Code != 401 {
			t.Fatalf("anonymous lookup %d", w.Code)
		}
		d := lookup(t, h, code.UserCode, auth.Token)
		if d.IP != "192.0.2.0/24" || d.UserAgent != "Display browser" {
			t.Fatalf("display metadata %+v", d)
		}
		body := `{"user_code":"` + code.UserCode + `","approve":true}`
		if w := post(h, "approve", body, auth.Token, ""); w.Code != 403 {
			t.Fatalf("missing CSRF %d", w.Code)
		}
		if w := post(h, "approve", body, auth.Token, d.CSRFToken); w.Code != 200 {
			t.Fatalf("approve %d: %s", w.Code, w.Body)
		}
		env.server.Client().Transport = deviceTransport{h}
		afterPollInterval(t, func(t *testing.T) {
			w := post(h, "poll", `{"device_code":"`+code.DeviceCode+`"}`, "", "")
			if w.Code != 200 {
				t.Fatalf("poll %d: %s", w.Code, w.Body)
			}
			var display dto.AuthResponse
			if err := json.Unmarshal(w.Body.Bytes(), &display); err != nil {
				t.Fatal(err)
			}
			if display.User.ID != auth.User.ID || display.Token == auth.Token {
				t.Fatal("session identity/independence mismatch")
			}
			parsed, err := jwt.Parse(display.Token, func(_ *jwt.Token) (any, error) { return testJWTSecret, nil })
			if err != nil {
				t.Fatal(err)
			}
			claims := parsed.Claims.(jwt.MapClaims)
			if claims["email"] != auth.User.Email || claims["sub"] != auth.User.ID.String() || claims["sid"] == nil || claims["exp"].(float64)-claims["iat"].(float64) != 31*24*3600 {
				t.Fatalf("unexpected claims %+v", claims)
			}
			sid, err := ksid.Parse(claims["sid"].(string))
			if err != nil {
				t.Fatal(err)
			}
			if status := env.doJSON(t, "GET", "/api/v1/auth/me", nil, &dto.UserResponse{}, display.Token); status != 200 {
				t.Fatalf("normal me %d", status)
			}

			var org dto.OrganizationResponse
			if status := env.doJSON(t, "POST", "/api/v1/organizations", dto.CreateOrganizationRequest{Name: "Pair org"}, &org, auth.Token); status != 200 {
				t.Fatalf("create org %d", status)
			}
			var ws dto.WorkspaceResponse
			if status := env.doJSON(t, "POST", "/api/v1/organizations/"+org.ID.String()+"/workspaces", dto.CreateWorkspaceRequest{Name: "Pair workspace"}, &ws, auth.Token); status != 200 {
				t.Fatalf("create workspace %d", status)
			}
			var switched dto.SwitchWorkspaceResponse
			if status := env.doJSON(t, "POST", "/api/v1/auth/switch-workspace", dto.SwitchWorkspaceRequest{WsID: ws.ID}, &switched, display.Token); status != 200 {
				t.Fatalf("switch workspace %d", status)
			}
			if switched.User.WorkspaceID != ws.ID || switched.User.WorkspaceRole != dto.WSRoleAdmin {
				t.Fatalf("switch changed session or permissions: %+v", switched.User)
			}
			// Pairing issues the same credential: writes, MCP, push, voice, OAuth linking
			// and approving another device use the account's ordinary permissions.
			var page dto.CreatePageResponse
			if status := env.doJSON(t, "POST", "/api/v1/workspaces/"+ws.ID.String()+"/nodes/0/page/create", dto.CreatePageRequest{Title: "Device write", Content: "Original"}, &page, display.Token); status != 200 {
				t.Fatal("device write", status)
			}
			outcome := callMCPTool(t, env, display.Token, "node_update", map[string]any{"nodeId": page.ID.String(), "title": "Device MCP", "content": "Updated by device MCP"})
			if outcome.IsError || outcome.StructuredContent.Error != "" {
				t.Fatalf("device MCP write: %+v", outcome)
			}
			var node dto.NodeResponse
			if status := env.doJSON(t, "GET", "/api/v1/workspaces/"+ws.ID.String()+"/nodes/"+page.ID.String(), nil, &node, display.Token); status != 200 || node.Content != "Updated by device MCP" {
				t.Fatalf("device MCP persistence %d %+v", status, node)
			}
			push, err := identity.NewPushSubscriptionService(filepath.Join(t.TempDir(), "push.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			env.services.PushSubscription = push
			if status := env.doJSON(t, "POST", "/api/v1/notifications/subscribe", dto.PushSubscribeRequest{Endpoint: "https://push.example.test/device", P256dh: "key", Auth: "auth"}, nil, display.Token); status != 200 {
				t.Fatal("device push", status)
			}
			for _, action := range []struct{ path, body string }{
				{"/api/voicegateway/v1/voice/rtc/offer", `{"sdp":"m=audio 9"}`},
				{"/api/v1/auth/oauth/link", `{"provider":"google"}`},
			} {
				r := httptest.NewRequest(http.MethodPost, action.path, strings.NewReader(action.body))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Authorization", "Bearer "+display.Token)
				response := httptest.NewRecorder()
				h.ServeHTTP(response, r)
				if response.Code != 200 {
					t.Fatalf("device action %s=%d %s", action.path, response.Code, response.Body)
				}
			}
			delegated := start(t, h)
			proof := lookup(t, h, delegated.UserCode, display.Token)
			if response := post(h, "approve", `{"user_code":"`+delegated.UserCode+`","approve":true}`, display.Token, proof.CSRFToken); response.Code != 200 {
				t.Fatalf("device delegation %d %s", response.Code, response.Body)
			}
			var other dto.AuthResponse
			if status := env.doJSON(t, "POST", "/api/v1/auth/login", dto.LoginRequest{Email: "pair@example.com", Password: "Password123"}, &other, ""); status != 200 {
				t.Fatal("normal login", status)
			}
			if status := env.doJSON(t, "POST", "/api/v1/auth/sessions/revoke", dto.RevokeSessionRequest{SessionID: other.User.SessionID}, nil, display.Token); status != 200 {
				t.Fatal("device revocation of another owned session", status)
			}
			if status := env.doJSON(t, "GET", "/api/v1/auth/me", nil, nil, other.Token); status != 401 {
				t.Fatal("other session survived revocation", status)
			}
			if err := env.services.Session.Revoke(sid); err != nil {
				t.Fatal(err)
			}
			if status := env.doJSON(t, "GET", "/api/v1/auth/me", nil, nil, display.Token); status != 401 {
				t.Fatalf("revoked me %d", status)
			}
			if status := env.doJSON(t, "GET", "/api/v1/auth/me", nil, &dto.UserResponse{}, auth.Token); status != 200 {
				t.Fatalf("approver revoked %d", status)
			}
			if w := post(h, "poll", `{"device_code":"`+code.DeviceCode+`"}`, "", ""); w.Code == 200 {
				t.Fatal("code reused")
			}
			data, err := fs.ReadFile(os.DirFS(dir), "device-login-audit.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), code.DeviceCode) || strings.Contains(string(data), code.UserCode) || !strings.Contains(string(data), `"approved":true`) {
				t.Fatalf("audit content %s", data)
			}
			info, err := os.Stat(filepath.Join(dir, "device-login-audit.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("audit permissions %v", info.Mode())
			}
		})
	})
	t.Run("same-account session cannot reuse decision proof", func(t *testing.T) {
		env, h, _, auth := setup(t)
		code := start(t, h)
		detail := lookup(t, h, code.UserCode, auth.Token)
		var replacement dto.AuthResponse
		if status := env.doJSON(t, "POST", "/api/v1/auth/login", dto.LoginRequest{Email: "pair@example.com", Password: "Password123"}, &replacement, ""); status != 200 {
			t.Fatalf("replacement login = %d", status)
		}
		body := `{"user_code":"` + code.UserCode + `","approve":true}`
		if w := post(h, "approve", body, replacement.Token, detail.CSRFToken); w.Code != 403 {
			t.Fatalf("stale decision proof = %d: %s", w.Code, w.Body)
		}
		fresh := lookup(t, h, code.UserCode, replacement.Token)
		if w := post(h, "approve", body, replacement.Token, fresh.CSRFToken); w.Code != 200 {
			t.Fatalf("fresh decision proof = %d: %s", w.Code, w.Body)
		}
	})
	t.Run("denial and audit failure", func(t *testing.T) {
		_, h, dir, auth := setup(t)
		code := start(t, h)
		d := lookup(t, h, code.UserCode, auth.Token)
		path := filepath.Join(dir, "device-login-audit.jsonl")
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		body := `{"user_code":"` + code.UserCode + `","approve":false}`
		if w := post(h, "approve", body, auth.Token, d.CSRFToken); w.Code != 500 {
			t.Fatalf("audit failure %d", w.Code)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if w := post(h, "approve", body, auth.Token, d.CSRFToken); w.Code != 200 {
			t.Fatalf("deny %d: %s", w.Code, w.Body)
		}
		afterPollInterval(t, func(t *testing.T) {
			if w := post(h, "poll", `{"device_code":"`+code.DeviceCode+`"}`, "", ""); !strings.Contains(w.Body.String(), "access_denied") {
				t.Fatalf("denial %d: %s", w.Code, w.Body)
			}
			data, err := fs.ReadFile(os.DirFS(dir), "device-login-audit.jsonl")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), `"approved":false`) {
				t.Fatalf("denial audit %s", data)
			}
		})
	})
	t.Run("revoked approver", func(t *testing.T) {
		env, h, _, auth := setup(t)
		code := start(t, h)
		d := lookup(t, h, code.UserCode, auth.Token)
		if _, err := env.services.Session.RevokeAllForUser(auth.User.ID); err != nil {
			t.Fatal(err)
		}
		if w := post(h, "approve", `{"user_code":"`+code.UserCode+`","approve":true}`, auth.Token, d.CSRFToken); w.Code != 401 {
			t.Fatalf("revoked approver %d", w.Code)
		}
	})
	t.Run("feature off", func(t *testing.T) {
		env, _, _, _ := setup(t)
		cfg := &Config{ServerConfig: &storage.ServerConfig{Quotas: storage.DefaultServerQuotas(false), RateLimits: storage.DefaultRateLimits()}, ConfigDir: t.TempDir()}
		for _, variant := range []string{"no secret", "no audit directory"} {
			t.Run(variant, func(t *testing.T) {
				if variant == "no audit directory" {
					cfg.JWTSecret = testJWTSecret
					cfg.ConfigDir = ""
				}
				h := NewRouter(env.services, cfg)
				t.Cleanup(h.Close)
				for _, name := range []string{"start", "poll", "lookup", "approve"} {
					if w := post(h, name, "{}", "", ""); w.Code != 404 {
						t.Fatalf("%s=%d", name, w.Code)
					}
				}
			})
		}
	})
	t.Run("host rate tiers", func(t *testing.T) {
		env, _, dir, _ := setup(t)
		cfg := &Config{ServerConfig: &storage.ServerConfig{JWTSecret: testJWTSecret, Quotas: storage.DefaultServerQuotas(false), RateLimits: storage.DefaultRateLimits()}, ConfigDir: dir}
		h := NewRouter(env.services, cfg)
		t.Cleanup(h.Close)
		for range 5 {
			start(t, h)
		}
		if w := post(h, "start", "{}", "", ""); w.Code != 429 {
			t.Fatalf("start limit %d", w.Code)
		}
		for range 15 {
			if w := post(h, "poll", `{"device_code":"unknown"}`, "", ""); w.Code == 429 {
				t.Fatal("poll incorrectly uses auth tier")
			}
		}
	})
}

type deviceFailedWriter struct{ http.ResponseWriter }

func (w *deviceFailedWriter) Write([]byte) (int, error) { return 0, errors.New("connection lost") }

// deviceCompletionHost exercises failures around the real host's durable issuer.
type deviceCompletionHost struct {
	devicelogin.Host
	failure string
	cancel  context.CancelFunc
	ip      netip.Addr
	created atomic.Int32
}

func (h *deviceCompletionHost) ClientIP(*http.Request) netip.Addr { return h.ip }
func (h *deviceCompletionHost) CreateSession(r *http.Request, subject string) (devicelogin.Session, error) {
	if h.failure == "before creation" {
		return nil, errors.New("creation unavailable")
	}
	session, err := h.Host.CreateSession(r, subject)
	if session != nil {
		h.created.Add(1)
	}
	if err != nil {
		return session, err
	}
	if h.failure == "after creation" {
		return session, errors.New("post-creation failure")
	}
	if h.failure == "cancel" {
		h.cancel()
	}
	return session, nil
}
func (h *deviceCompletionHost) Audit(ctx context.Context, e *devicelogin.AuditEvent) error {
	if h.failure == "created audit" && e.Kind == devicelogin.AuditSessionCreated || h.failure == "response audit" && e.Kind == devicelogin.AuditResponseWritten {
		return errors.New("audit unavailable")
	}
	return h.Host.Audit(ctx, e)
}

// deviceTransport keeps the real router and middleware in the virtual-time
// bubble, so normal API and MCP authentication see the session issuance clock.
type deviceTransport struct{ handler http.Handler }

func (d deviceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.RemoteAddr = "192.0.2.123:1234"
	if r.Body == nil {
		r.Body = http.NoBody
	}
	w := httptest.NewRecorder()
	d.handler.ServeHTTP(w, r)
	if err := r.Body.Close(); err != nil {
		return nil, err
	}
	return w.Result(), nil
}
