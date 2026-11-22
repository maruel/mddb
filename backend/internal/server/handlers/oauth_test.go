// Tests for OAuth login, account-linking state binding, and email trust.

package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gomodedev/gomode/oauth/oauthserver"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto/v1"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/utils"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

// Provider user API endpoints that fakeProviders answers.
const (
	githubEmailsAPI = "api.github.com/user/emails"
	githubUserAPI   = "api.github.com/user"
	googleUserAPI   = "www.googleapis.com/oauth2/v2/userinfo"
	graphMeAPI      = "graph.microsoft.com/v1.0/me"
)

// fakeTenant is the Microsoft tenant of the fake ID token.
const fakeTenant = "11111111-2222-3333-4444-555555555555"

// fakeIDs holds the provider account ID that fakeProviders signs in.
var fakeIDs = map[identityv1.OAuthProvider]string{
	identityv1.OAuthProviderGitHub:    "4242",
	identityv1.OAuthProviderGoogle:    "g4242",
	identityv1.OAuthProviderMicrosoft: "m4242",
}

var providers = []identityv1.OAuthProvider{
	identityv1.OAuthProviderGitHub,
	identityv1.OAuthProviderGoogle,
	identityv1.OAuthProviderMicrosoft,
}

// fakeProviders answers the token endpoint of every provider and the user
// APIs in apis, and counts token exchanges.
type fakeProviders struct {
	exchanges atomic.Int32
	// apis maps an API host and path to its JSON response body.
	apis map[string]string
	// idClaims holds the claims of the ID token that Microsoft returns. A nil
	// map omits the ID token.
	idClaims map[string]any
}

// newFakeProviders returns providers that sign in person@example.com, which
// no account holds, and assert that the email is verified.
func newFakeProviders() *fakeProviders {
	return &fakeProviders{
		apis: map[string]string{
			githubEmailsAPI: `[{"email":"person@example.com","primary":true,"verified":true}]`,
			githubUserAPI:   `{"id":4242,"login":"octocat"}`,
			googleUserAPI:   `{"id":"g4242","email":"person@example.com","verified_email":true,"name":"Person"}`,
			graphMeAPI:      `{"id":"m4242","displayName":"Person","mail":"person@example.com","userPrincipalName":"person@example.com"}`,
		},
		idClaims: map[string]any{
			"aud":      "client",
			"iss":      "https://login.microsoftonline.com/" + fakeTenant + "/v2.0",
			"tid":      fakeTenant,
			"exp":      time.Now().Add(time.Hour).Unix(),
			"email":    "person@example.com",
			"xms_edov": true,
		},
	}
}

// setEmail makes every provider return email, verified or not. Google and
// Microsoft keep the fake account ID.
func (f *fakeProviders) setEmail(email string, verified bool) {
	v, _ := json.Marshal(verified)
	f.apis[githubEmailsAPI] = `[{"email":"` + email + `","primary":true,"verified":` + string(v) + `}]`
	f.apis[googleUserAPI] = `{"id":"g4242","email":"` + email + `","verified_email":` + string(v) + `}`
	f.idClaims["email"] = email
	f.idClaims["xms_edov"] = verified
}

func (f *fakeProviders) RoundTrip(r *http.Request) (*http.Response, error) {
	key := r.URL.Host + r.URL.Path
	body, ok := f.apis[key]
	switch key {
	case "github.com/login/oauth/access_token", "oauth2.googleapis.com/token":
		f.exchanges.Add(1)
		body, ok = `{"access_token":"tok","token_type":"bearer"}`, true
	case "login.microsoftonline.com/common/oauth2/v2.0/token":
		f.exchanges.Add(1)
		resp := map[string]any{"access_token": "tok", "token_type": "bearer"}
		if f.idClaims != nil {
			claims, err := json.Marshal(f.idClaims)
			if err != nil {
				return nil, err
			}
			enc := base64.RawURLEncoding
			resp["id_token"] = enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + enc.EncodeToString(claims) + "." + enc.EncodeToString([]byte("unsigned"))
		}
		b, err := json.Marshal(resp)
		if err != nil {
			return nil, err
		}
		body, ok = string(b), true
	}
	if !ok {
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: http.Header{}, Request: r}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}, nil
}

type oauthTestEnv struct {
	dir  string
	h    *OAuthHandler
	svc  *Services
	fake *fakeProviders
	// caller holds the session that makes the requests.
	caller *identityv1.User
	// victim is another account the caller must not reach. It holds
	// victim@example.com, which it never verified.
	victim *identityv1.User
}

func newOAuthTestEnv(t *testing.T, baseURL string) *oauthTestEnv {
	dir := t.TempDir()
	db := filepath.Join(dir, "db")
	if err := os.Mkdir(db, 0o700); err != nil {
		t.Fatal(err)
	}
	users, err := identity.NewUserService(filepath.Join(db, "users.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	users.SetBcryptCostForTesting(bcrypt.MinCost)
	sessions, err := identity.NewSessionService(filepath.Join(db, "sessions.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := git.NewRootRepo(t.Context(), dir, "test", "test@test.com")
	if err != nil {
		t.Fatal(err)
	}
	serverCfg, err := storage.LoadServerConfig(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	svc := &Services{User: users, Session: sessions, RootRepo: root}
	h := NewOAuthHandler(svc, &Config{ServerSettings: storage.NewServerSettings(serverCfg), BaseURL: baseURL})
	for _, p := range providers {
		h.AddProvider(p, "client", "secret", baseURL+"/api/v1/auth/oauth/"+string(p)+"/callback")
	}
	caller, err := users.Create("caller@example.com", "password123", "Caller")
	if err != nil {
		t.Fatal(err)
	}
	victim, err := users.Create("victim@example.com", "password123", "Victim")
	if err != nil {
		t.Fatal(err)
	}
	return &oauthTestEnv{dir: dir, h: h, svc: svc, fake: newFakeProviders(), caller: caller, victim: victim}
}

// loginRedirect starts a login with provider and returns the state sent to
// the provider and the state cookie.
func (e *oauthTestEnv) loginRedirect(t *testing.T, p identityv1.OAuthProvider) (string, *http.Cookie) {
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/auth/oauth/"+string(p), http.NoBody)
	r.SetPathValue("provider", string(p))
	w := httptest.NewRecorder()
	e.h.LoginRedirect(w, r)
	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("LoginRedirect status = %d; body %s", w.Code, w.Body)
	}
	return authorizeState(t, w.Header().Get("Location")), stateCookie(t, w)
}

// login runs a complete login with provider.
func (e *oauthTestEnv) login(t *testing.T, p identityv1.OAuthProvider) *httptest.ResponseRecorder {
	state, c := e.loginRedirect(t, p)
	return e.callback(t, p, state, c)
}

// linkOAuth starts linking a provider to the caller's account.
func (e *oauthTestEnv) linkOAuth(t *testing.T, body string) *httptest.ResponseRecorder {
	ctx := reqctx.WithUser(t.Context(), e.caller)
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/auth/oauth/link", strings.NewReader(body))
	w := httptest.NewRecorder()
	e.h.LinkOAuth(w, r)
	return w
}

// startLink links provider to the caller's account and returns the state
// sent to the provider and the state cookie.
func (e *oauthTestEnv) startLink(t *testing.T, p identityv1.OAuthProvider) (string, *http.Cookie) {
	w := e.linkOAuth(t, `{"provider":"`+string(p)+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("LinkOAuth status = %d; body %s", w.Code, w.Body)
	}
	var resp dto.LinkOAuthAccountResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return authorizeState(t, resp.RedirectURL), stateCookie(t, w)
}

// callback runs the provider callback with the given state and cookies.
func (e *oauthTestEnv) callback(t *testing.T, p identityv1.OAuthProvider, state string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return e.callbackQuery(t, p, url.Values{"code": {"c"}, "state": {state}}, cookies...)
}

// callbackQuery runs the provider callback with the query the provider sends.
func (e *oauthTestEnv) callbackQuery(t *testing.T, p identityv1.OAuthProvider, q url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	ctx := context.WithValue(t.Context(), oauth2.HTTPClient, &http.Client{Transport: e.fake})
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/auth/oauth/"+string(p)+"/callback?"+q.Encode(), http.NoBody)
	r.SetPathValue("provider", string(p))
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	e.h.Callback(w, r)
	return w
}

// get returns the stored copy of u.
func (e *oauthTestEnv) get(t *testing.T, u *identityv1.User) *identityv1.User {
	got, err := e.svc.User.Get(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// providerIDs returns the identities linked to u as "provider:id".
func (e *oauthTestEnv) providerIDs(t *testing.T, u *identityv1.User) []string {
	got := e.get(t, u)
	ids := make([]string, 0, len(got.OAuthIdentities))
	for _, ident := range got.OAuthIdentities {
		ids = append(ids, string(ident.Provider)+":"+ident.ProviderID)
	}
	return ids
}

// sessions returns the number of active sessions of u.
func (e *oauthTestEnv) sessions(u *identityv1.User) int {
	n := 0
	for range e.svc.Session.GetActiveByUserID(u.ID) {
		n++
	}
	return n
}

// linkIdentity attaches the fake provider identity to u.
func (e *oauthTestEnv) linkIdentity(t *testing.T, u *identityv1.User, p identityv1.OAuthProvider) {
	if _, err := e.svc.User.Modify(u.ID, func(u *identityv1.User) error {
		u.OAuthIdentities = append(u.OAuthIdentities, identityv1.OAuthIdentity{Provider: p, ProviderID: fakeIDs[p]})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// wantLoggedIn checks that the login succeeded and returns the account that
// holds the provider identity.
func (e *oauthTestEnv) wantLoggedIn(t *testing.T, p identityv1.OAuthProvider, w *httptest.ResponseRecorder) *identityv1.User {
	wantRedirect(t, w, "/")
	wantCleared(t, w)
	u, err := e.svc.User.GetByOAuth(p, fakeIDs[p])
	if err != nil {
		t.Fatal(err)
	}
	if n := e.sessions(u); n != 1 {
		t.Errorf("%s sessions = %d, want 1", u.Email, n)
	}
	tokenString := ""
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == SessionCookieName {
			if !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/" || cookie.Domain != "" {
				t.Fatal("unsafe session cookie attributes")
			}
			tokenString = cookie.Value
		}
	}
	if tokenString == "" {
		t.Fatal("no browser session cookie")
	}
	claims := &SessionClaims{}
	if _, err := jwt.ParseWithClaims(tokenString, claims, func(_ *jwt.Token) (any, error) { return e.h.cfg.Snapshot().JWTSecret, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuedAt()); err != nil {
		t.Fatal(err)
	}
	sid, err := ksid.Parse(claims.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	session, err := e.svc.Session.Get(sid)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != u.ID.String() || claims.IssuedAt == nil || claims.Email == "" || session.UserID != u.ID || session.TokenHash != utils.HashToken(tokenString) || session.ExpiresAt.AsTime().Unix() != claims.ExpiresAt.Unix() {
		t.Fatal("provider credential is not bound to its tracked session")
	}
	return u
}

// wantVictimUntouched checks that the victim holds no identity, no session,
// and an unverified email.
func (e *oauthTestEnv) wantVictimUntouched(t *testing.T) {
	v := e.get(t, e.victim)
	if len(v.OAuthIdentities) != 0 {
		t.Errorf("victim identities = %v, want none", e.providerIDs(t, v))
	}
	if v.EmailVerified {
		t.Error("victim email verified")
	}
	if n := e.sessions(v); n != 0 {
		t.Errorf("victim sessions = %d, want 0", n)
	}
}

// wantRefused checks that the login returned to the login page with code, and
// that it created no account and signed in nobody.
func (e *oauthTestEnv) wantRefused(t *testing.T, w *httptest.ResponseRecorder, code dto.ErrorCode) {
	wantRedirect(t, w, "/login?oauth_error="+string(code))
	wantCleared(t, w)
	e.wantVictimUntouched(t)
	if n := e.svc.User.Count(); n != 2 {
		t.Errorf("accounts = %d, want 2", n)
	}
	if n := e.sessions(e.caller); n != 0 {
		t.Errorf("caller sessions = %d, want 0", n)
	}
}

// wantRejected checks that the callback returned to the login page as expired
// before the token exchange and linked nothing to either account.
func (e *oauthTestEnv) wantRejected(t *testing.T, w *httptest.ResponseRecorder) {
	wantRedirect(t, w, "/login?oauth_error="+string(dto.ErrorCodeExpired))
	if n := e.fake.exchanges.Load(); n != 0 {
		t.Errorf("token exchanges = %d, want 0", n)
	}
	for _, u := range []*identityv1.User{e.caller, e.victim} {
		if ids := e.providerIDs(t, u); len(ids) != 0 {
			t.Errorf("%s identities = %v, want none", u.Email, ids)
		}
	}
	wantCleared(t, w)
}

// wantRedirect checks that the response is a See Other redirect to location.
func wantRedirect(t *testing.T, w *httptest.ResponseRecorder, location string) {
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != location {
		t.Fatalf("status = %d, location %q; want %d %q; body %s", w.Code, w.Header().Get("Location"), http.StatusSeeOther, location, w.Body)
	}
}

// wantCleared checks that the response deletes the state cookie.
func wantCleared(t *testing.T, w *httptest.ResponseRecorder) {
	c := stateCookie(t, w)
	if c.MaxAge >= 0 || c.Value != "" {
		t.Errorf("state cookie = %+v, want deletion", c)
	}
}

func authorizeState(t *testing.T, location string) string {
	u, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	state := u.Query().Get("state")
	if state == "" {
		t.Fatalf("no state in %q", location)
	}
	return state
}

func stateCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == oauthStateCookie {
			return c
		}
	}
	t.Fatalf("no %s cookie in %v", oauthStateCookie, w.Header()["Set-Cookie"])
	return nil
}

// redirectCase is a base URL and whether the state cookie must be Secure.
type redirectCase struct {
	baseURL string
	secure  bool
}

// linkErrorCase is a LinkOAuth request body and its expected status.
type linkErrorCase struct {
	name string
	body string
	want int
}

// providerAPIsCase overrides fake provider API responses.
type providerAPIsCase struct {
	p    identityv1.OAuthProvider
	apis map[string]string
}

// providerEditCase changes the fake provider before a login.
type providerEditCase struct {
	p    identityv1.OAuthProvider
	edit func(f *fakeProviders)
}

// unverifiedEmailCase is a login whose provider email is not verified.
type unverifiedEmailCase struct {
	name  string
	p     identityv1.OAuthProvider
	email string
	edit  func(f *fakeProviders)
}

// idClaimsCase changes the fake Microsoft ID token claims.
type idClaimsCase struct {
	name string
	edit func(c map[string]any)
}

func TestOAuthHandler(t *testing.T) {
	t.Run("LoginRedirect", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			for _, tc := range []redirectCase{
				{"http://localhost:8080", false},
				{"https://mddb.example.com", true},
			} {
				t.Run(tc.baseURL, func(t *testing.T) {
					e := newOAuthTestEnv(t, tc.baseURL)
					state, c := e.loginRedirect(t, identityv1.OAuthProviderGitHub)
					if got, ok := oauthserver.ValidateState(c.Value, e.h.stateKey); !ok || got != state {
						t.Errorf("cookie state = %q, %t; want %q", got, ok, state)
					}
					if strings.HasPrefix(state, linkingStatePrefix) {
						t.Errorf("login state %q is a linking state", state)
					}
					if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Secure != tc.secure || c.Path != oauthStateCookiePath || c.MaxAge != oauthStateMaxAge {
						t.Errorf("cookie = %+v", c)
					}
				})
			}
		})
		t.Run("destination", func(t *testing.T) {
			for _, p := range providers {
				for _, returnPath := range []string{"/device", "/accept-invitation/org?token=organization-secret", "/accept-invitation/ws?token=workspace-secret"} {
					t.Run(string(p)+returnPath, func(t *testing.T) {
						e := newOAuthTestEnv(t, "https://mddb.example.com")
						r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/auth/oauth/"+string(p)+"?next="+url.QueryEscape(returnPath), http.NoBody)
						r.SetPathValue("provider", string(p))
						w := httptest.NewRecorder()
						e.h.LoginRedirect(w, r)
						if w.Code != http.StatusTemporaryRedirect {
							t.Fatalf("start = %d: %s", w.Code, w.Body)
						}
						state, cookie := authorizeState(t, w.Header().Get("Location")), stateCookie(t, w)
						changed := strings.Replace(state, base64.RawURLEncoding.EncodeToString([]byte(returnPath)), base64.RawURLEncoding.EncodeToString([]byte("/other")), 1)
						wantRedirect(t, e.callback(t, p, changed, cookie), "/login?oauth_error=EXPIRED")
						wantRedirect(t, e.callback(t, p, state, cookie), returnPath)
					})
				}
			}
			e := newOAuthTestEnv(t, "https://mddb.example.com")
			for _, path := range []string{"https://evil.example/device", "//evil.example", "/\\evil", "/%5cevil", "/%2fevil", "/%252fevil", "/%00device", "/%0ddevice", "/%7fdevice", "/%", "device"} {
				t.Run(path, func(t *testing.T) {
					r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/auth/oauth/google?next="+url.QueryEscape(path), http.NoBody)
					r.SetPathValue("provider", "google")
					w := httptest.NewRecorder()
					e.h.LoginRedirect(w, r)
					wantRedirect(t, w, "/login?oauth_error=VALIDATION_FAILED")
					if len(w.Result().Cookies()) != 0 {
						t.Fatalf("unsafe destination cookies %v", w.Result().Cookies())
					}
					// Even signed state must pass validation before exchanging credentials.
					state := loginStatePrefix + base64.RawURLEncoding.EncodeToString([]byte(path)) + ":nonce"
					cookie := e.h.stateCookie(oauthserver.SignState(state, e.h.stateKey), oauthStateMaxAge)
					wantRedirect(t, e.callback(t, identityv1.OAuthProviderGoogle, state, cookie), "/login?oauth_error=VALIDATION_FAILED")
					if n := e.fake.exchanges.Load(); n != 0 {
						t.Fatalf("unsafe signed destination exchanges = %d", n)
					}
				})
			}
		})
	})
	t.Run("LinkOAuth", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			e := newOAuthTestEnv(t, "http://localhost:8080")
			state, c := e.startLink(t, identityv1.OAuthProviderGitHub)
			if got, ok := oauthserver.ValidateState(c.Value, e.h.stateKey); !ok || got != state {
				t.Errorf("cookie state = %q, %t; want %q", got, ok, state)
			}
			if id, p := parseLinkingState(state); id != e.caller.ID || p != identityv1.OAuthProviderGitHub {
				t.Errorf("linking state names %v %q, want %v github", id, p, e.caller.ID)
			}
		})
		t.Run("error", func(t *testing.T) {
			for _, tc := range []linkErrorCase{
				{"malformed body", `{`, http.StatusBadRequest},
				{"unknown field", `{"provider":"github","user_id":"x"}`, http.StatusBadRequest},
				{"missing provider", `{}`, http.StatusBadRequest},
				{"unconfigured provider", `{"provider":"google"}`, http.StatusNotFound},
			} {
				t.Run(tc.name, func(t *testing.T) {
					e := newOAuthTestEnv(t, "http://localhost:8080")
					delete(e.h.providers, identityv1.OAuthProviderGoogle)
					w := e.linkOAuth(t, tc.body)
					if w.Code != tc.want {
						t.Errorf("status = %d, want %d; body %s", w.Code, tc.want, w.Body)
					}
					if len(w.Result().Cookies()) != 0 {
						t.Errorf("cookies = %v, want none", w.Header()["Set-Cookie"])
					}
				})
			}
		})
	})
	t.Run("Callback", func(t *testing.T) {
		t.Run("returning identity refreshes exact primary proof", func(t *testing.T) {
			for _, p := range providers {
				t.Run(string(p), func(t *testing.T) {
					e := newOAuthTestEnv(t, "http://localhost:8080")
					e.linkIdentity(t, e.caller, p)
					e.fake.setEmail(e.caller.Email, true)
					e.wantLoggedIn(t, p, e.login(t, p))
					if !e.get(t, e.caller).EmailVerified {
						t.Fatal("fresh matching provider proof not published")
					}
					if e.svc.User.Count() != 2 {
						t.Fatal("returning login created another account")
					}
				})
			}
		})
		t.Run("returning identity ignores stale or unrelated proof", func(t *testing.T) {
			for _, address := range []string{"other@example.com", "Caller@example.com", "unverified"} {
				t.Run(address, func(t *testing.T) {
					e := newOAuthTestEnv(t, "http://localhost:8080")
					e.linkIdentity(t, e.caller, identityv1.OAuthProviderGitHub)
					if _, err := e.svc.User.Modify(e.caller.ID, func(u *identityv1.User) error { u.OAuthIdentities[0].Email = u.Email; return nil }); err != nil {
						t.Fatal(err)
					}
					if address == "unverified" {
						e.fake.setEmail(e.caller.Email, false)
					} else {
						e.fake.setEmail(address, true)
					}
					e.wantLoggedIn(t, identityv1.OAuthProviderGitHub, e.login(t, identityv1.OAuthProviderGitHub))
					if e.get(t, e.caller).EmailVerified {
						t.Fatal("cached, nonmatching or unverified email proved primary")
					}
				})
			}
		})

		t.Run("link verifies exact current primary address", func(t *testing.T) {
			for _, p := range providers {
				t.Run(string(p), func(t *testing.T) {
					e := newOAuthTestEnv(t, "http://localhost:8080")
					e.fake.setEmail(e.caller.Email, true)
					state, c := e.startLink(t, p)
					if w := e.callback(t, p, state, c); w.Code != http.StatusSeeOther {
						t.Fatalf("link: %d %s", w.Code, w.Body)
					}
					stored, err := identity.NewUserService(filepath.Join(e.dir, "db", "users.jsonl"))
					if err != nil {
						t.Fatal(err)
					}
					u, err := stored.Get(e.caller.ID)
					if err != nil || !u.EmailVerified || u.Email != e.caller.Email || len(u.OAuthIdentities) != 1 {
						t.Fatalf("primary proof not persisted: %+v (%v)", u, err)
					}
					if e.svc.User.Count() != 2 || e.sessions(e.caller) != 0 || e.get(t, e.victim).EmailVerified {
						t.Fatal("link changed account/session ownership")
					}
				})
			}
		})
		t.Run("link cannot prove a different current address", func(t *testing.T) {
			for _, address := range []string{"other@example.com", "Caller@example.com", "changed@example.com"} {
				t.Run(address, func(t *testing.T) {
					e := newOAuthTestEnv(t, "http://localhost:8080")
					e.fake.setEmail(address, true)
					state, c := e.startLink(t, identityv1.OAuthProviderGitHub)
					if address == "changed@example.com" {
						e.fake.setEmail(e.caller.Email, true)
						if _, err := e.svc.User.Modify(e.caller.ID, func(u *identityv1.User) error { u.Email = address; return nil }); err != nil {
							t.Fatal(err)
						}
					}
					if w := e.callback(t, identityv1.OAuthProviderGitHub, state, c); w.Code != http.StatusSeeOther {
						t.Fatalf("link: %d", w.Code)
					}
					if e.get(t, e.caller).EmailVerified {
						t.Fatal("unrelated provider address verified primary")
					}
				})
			}
		})
		t.Run("link returns to its signed invitation destination", func(t *testing.T) {
			for _, kind := range []string{"org", "ws"} {
				t.Run(kind, func(t *testing.T) {
					e := newOAuthTestEnv(t, "http://localhost:8080")
					next := "/accept-invitation/" + kind + "?token=secret"
					body, err := json.Marshal(dto.LinkOAuthAccountRequest{Provider: dto.OAuthProviderGitHub, Next: next})
					if err != nil {
						t.Fatal(err)
					}
					w := e.linkOAuth(t, string(body))
					if w.Code != http.StatusOK {
						t.Fatalf("start: %d %s", w.Code, w.Body)
					}
					var out dto.LinkOAuthAccountResponse
					if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
						t.Fatal(err)
					}
					callback := e.callback(t, identityv1.OAuthProviderGitHub, authorizeState(t, out.RedirectURL), stateCookie(t, w))
					if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != next {
						t.Fatalf("callback: %d %v", callback.Code, callback.Header())
					}
				})
			}
		})
		t.Run("link recovery retains invitation on cancellation or claimed identity", func(t *testing.T) {
			for _, failure := range []string{"cancelled", "identity_claimed"} {
				t.Run(failure, func(t *testing.T) {
					e := newOAuthTestEnv(t, "http://localhost:8080")
					next := "/accept-invitation/ws?token=secret"
					body, err := json.Marshal(dto.LinkOAuthAccountRequest{Provider: dto.OAuthProviderGitHub, Next: next})
					if err != nil {
						t.Fatal(err)
					}
					w := e.linkOAuth(t, string(body))
					if w.Code != http.StatusOK {
						t.Fatalf("start: %d %s", w.Code, w.Body)
					}
					var out dto.LinkOAuthAccountResponse
					if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
						t.Fatal(err)
					}
					state, cookie := authorizeState(t, out.RedirectURL), stateCookie(t, w)
					var callback *httptest.ResponseRecorder
					want := "/accept-invitation/ws?oauth_error=CONFLICT&token=secret"
					if failure == "cancelled" {
						q := url.Values{"state": {state}, "error": {"access_denied"}}
						r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/auth/oauth/github/callback?"+q.Encode(), http.NoBody)
						r.SetPathValue("provider", "github")
						r.AddCookie(cookie)
						callback = httptest.NewRecorder()
						e.h.Callback(callback, r)
						want = next
						if e.fake.exchanges.Load() != 0 {
							t.Fatal("cancellation exchanged provider credentials")
						}
					} else {
						e.linkIdentity(t, e.victim, identityv1.OAuthProviderGitHub)
						callback = e.callback(t, identityv1.OAuthProviderGitHub, state, cookie)
					}
					if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != want {
						t.Fatalf("recovery: %d %v", callback.Code, callback.Header())
					}
					if len(e.get(t, e.caller).OAuthIdentities) != 0 || e.get(t, e.caller).EmailVerified {
						t.Fatal("failed link changed account proof")
					}
					wantCleared(t, callback)
				})
			}
		})

		t.Run("link rejects unsupported return destinations", func(t *testing.T) {
			for _, next := range []string{"https://evil.test/accept-invitation/org?token=x", "//evil.test", "/", "/settings/user", "/accept-invitation/other?token=x", "/accept-invitation/org", "/accept-invitation/org?token=x&next=//evil.test", "/accept-invitation/org?token=x&token=y", "/accept-invitation/%6frg?token=x", "/accept-invitation/ws?token=x#fragment"} {
				t.Run(next, func(t *testing.T) {
					e := newOAuthTestEnv(t, "http://localhost:8080")
					body, err := json.Marshal(dto.LinkOAuthAccountRequest{Provider: dto.OAuthProviderGitHub, Next: next})
					if err != nil {
						t.Fatal(err)
					}
					if w := e.linkOAuth(t, string(body)); w.Code != http.StatusBadRequest || len(w.Result().Cookies()) != 0 {
						t.Fatalf("unsafe start: %d %s", w.Code, w.Body)
					}
				})
			}
		})

		t.Run("history failure preserves completed login", func(t *testing.T) {
			e := newOAuthTestEnv(t, "http://localhost:8080")
			if err := os.WriteFile(filepath.Join(e.dir, ".git", "index.lock"), []byte("test owns lock"), 0o600); err != nil {
				t.Fatal(err)
			}
			u := e.wantLoggedIn(t, identityv1.OAuthProviderGoogle, e.login(t, identityv1.OAuthProviderGoogle))
			if e.svc.User.Count() != 3 || e.sessions(u) != 1 {
				t.Fatal("login did not persist exactly one account and session")
			}
		})
		t.Run("history failure preserves completed link", func(t *testing.T) {
			e := newOAuthTestEnv(t, "http://localhost:8080")
			state, c := e.startLink(t, identityv1.OAuthProviderGoogle)
			if err := os.WriteFile(filepath.Join(e.dir, ".git", "index.lock"), []byte("test owns lock"), 0o600); err != nil {
				t.Fatal(err)
			}
			wantRedirect(t, e.callback(t, identityv1.OAuthProviderGoogle, state, c), "/settings/user?oauth_linked=true")
			if ids := e.providerIDs(t, e.caller); len(ids) != 1 || ids[0] != "google:"+fakeIDs[identityv1.OAuthProviderGoogle] {
				t.Fatalf("persisted identities: %v", ids)
			}
		})
		t.Run("valid", func(t *testing.T) {
			t.Run("sign-in at the session quota succeeds", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				limit := e.h.cfg.Snapshot().Quotas.MaxSessionsPerUser
				var u *identityv1.User
				for i := range limit + 1 {
					w := e.login(t, identityv1.OAuthProviderGoogle)
					if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
						t.Fatalf("sign-in %d: %d %q", i+1, w.Code, w.Header().Get("Location"))
					}
					if u == nil {
						var err error
						if u, err = e.svc.User.GetByOAuth(identityv1.OAuthProviderGoogle, fakeIDs[identityv1.OAuthProviderGoogle]); err != nil {
							t.Fatal(err)
						}
					}
				}
				if n := e.sessions(u); n != limit {
					t.Fatalf("active sessions = %d, want %d", n, limit)
				}
			})
			t.Run("verified email creates an account", func(t *testing.T) {
				for _, p := range providers {
					t.Run(string(p), func(t *testing.T) {
						e := newOAuthTestEnv(t, "http://localhost:8080")
						// Graph attributes never become the account email.
						e.fake.apis[graphMeAPI] = `{"id":"m4242","mail":"victim@example.com","userPrincipalName":"victim@example.com"}`
						u := e.wantLoggedIn(t, p, e.login(t, p))
						if u.Email != "person@example.com" || !u.EmailVerified {
							t.Errorf("account %s, verified %t; want person@example.com, verified", u.Email, u.EmailVerified)
						}
						e.wantVictimUntouched(t)
					})
				}
			})
			t.Run("verified email attaches to the matching account", func(t *testing.T) {
				for _, tc := range []providerAPIsCase{
					{identityv1.OAuthProviderGitHub, map[string]string{
						githubEmailsAPI: `[{"email":"other@example.com","primary":true,"verified":false},{"email":"victim@example.com","primary":false,"verified":true}]`,
					}},
					{identityv1.OAuthProviderGoogle, map[string]string{
						googleUserAPI: `{"id":"g4242","email":"victim@example.com","verified_email":true}`,
					}},
					{identityv1.OAuthProviderMicrosoft, nil},
				} {
					t.Run(string(tc.p), func(t *testing.T) {
						e := newOAuthTestEnv(t, "http://localhost:8080")
						if _, err := e.svc.User.Modify(e.victim.ID, func(u *identityv1.User) error { u.EmailVerified = true; return nil }); err != nil {
							t.Fatal(err)
						}
						e.fake.idClaims["email"] = "victim@example.com"
						for k, v := range tc.apis {
							e.fake.apis[k] = v
						}
						u := e.wantLoggedIn(t, tc.p, e.login(t, tc.p))
						if u.ID != e.victim.ID || !u.EmailVerified {
							t.Errorf("identity on %s, verified %t; want victim@example.com, verified", u.Email, u.EmailVerified)
						}
					})
				}
			})
			t.Run("verified provider cannot adopt an unverified password account", func(t *testing.T) {
				for _, p := range providers {
					t.Run(string(p), func(t *testing.T) {
						e := newOAuthTestEnv(t, "http://localhost:8080")
						e.fake.setEmail(e.victim.Email, true)
						e.wantRefused(t, e.login(t, p), dto.ErrorCodeEmailInUse)
						if _, err := e.svc.User.Authenticate(e.victim.Email, "password123"); err != nil {
							t.Fatal("collision changed existing password", err)
						}
						if n := e.sessions(e.victim); n != 0 {
							t.Fatalf("victim sessions = %d, want zero", n)
						}
					})
				}
			})
			t.Run("GitHub ignores the unverified profile email", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				e.fake.apis[githubUserAPI] = `{"id":4242,"login":"octocat","email":"victim@example.com"}`
				e.fake.apis[githubEmailsAPI] = `[{"email":"victim@example.com","primary":true,"verified":false},{"email":"person@example.com","primary":false,"verified":true}]`
				u := e.wantLoggedIn(t, identityv1.OAuthProviderGitHub, e.login(t, identityv1.OAuthProviderGitHub))
				if u.Email != "person@example.com" {
					t.Errorf("account %s, want person@example.com", u.Email)
				}
			})
			t.Run("returning user signs in by provider ID", func(t *testing.T) {
				for _, p := range providers {
					t.Run(string(p), func(t *testing.T) {
						e := newOAuthTestEnv(t, "http://localhost:8080")
						e.linkIdentity(t, e.caller, p)
						e.fake.setEmail("victim@example.com", true)
						if u := e.wantLoggedIn(t, p, e.login(t, p)); u.ID != e.caller.ID {
							t.Errorf("signed in as %s, want caller@example.com", u.Email)
						}
						e.wantVictimUntouched(t)
					})
				}
			})
			t.Run("returning user signs in when the email source fails", func(t *testing.T) {
				for _, tc := range []providerEditCase{
					{identityv1.OAuthProviderGitHub, func(f *fakeProviders) { delete(f.apis, githubEmailsAPI) }},
					{identityv1.OAuthProviderMicrosoft, func(f *fakeProviders) { f.idClaims = nil }},
				} {
					t.Run(string(tc.p), func(t *testing.T) {
						e := newOAuthTestEnv(t, "http://localhost:8080")
						e.linkIdentity(t, e.caller, tc.p)
						tc.edit(e.fake)
						if u := e.wantLoggedIn(t, tc.p, e.login(t, tc.p)); u.ID != e.caller.ID {
							t.Errorf("signed in as %s, want caller@example.com", u.Email)
						}
					})
				}
			})
			t.Run("link when the email source fails", func(t *testing.T) {
				for _, tc := range []providerEditCase{
					{identityv1.OAuthProviderGitHub, func(f *fakeProviders) { delete(f.apis, githubEmailsAPI) }},
					{identityv1.OAuthProviderMicrosoft, func(f *fakeProviders) { f.idClaims = nil }},
				} {
					t.Run(string(tc.p), func(t *testing.T) {
						e := newOAuthTestEnv(t, "http://localhost:8080")
						tc.edit(e.fake)
						state, c := e.startLink(t, tc.p)
						wantRedirect(t, e.callback(t, tc.p, state, c), "/settings/user?oauth_linked=true")
						if ids := e.providerIDs(t, e.caller); len(ids) != 1 || ids[0] != string(tc.p)+":"+fakeIDs[tc.p] {
							t.Errorf("caller identities = %v", ids)
						}
						if got := e.get(t, e.caller).OAuthIdentities[0].Email; got != "" {
							t.Errorf("identity email = %q, want none", got)
						}
						e.wantVictimUntouched(t)
					})
				}
			})
			t.Run("link with an unverified email", func(t *testing.T) {
				for _, p := range providers {
					t.Run(string(p), func(t *testing.T) {
						e := newOAuthTestEnv(t, "http://localhost:8080")
						e.fake.setEmail("victim@example.com", false)
						e.fake.apis[graphMeAPI] = `{"id":"m4242","mail":"victim@example.com"}`
						state, c := e.startLink(t, p)
						w := e.callback(t, p, state, c)
						wantRedirect(t, w, "/settings/user?oauth_linked=true")
						wantCleared(t, w)
						if ids := e.providerIDs(t, e.caller); len(ids) != 1 || ids[0] != string(p)+":"+fakeIDs[p] {
							t.Errorf("caller identities = %v", ids)
						}
						if got := e.get(t, e.caller).OAuthIdentities[0].Email; got != "" {
							t.Errorf("identity email = %q, want none", got)
						}
						e.wantVictimUntouched(t)
					})
				}
			})
		})
		t.Run("error", func(t *testing.T) {
			t.Run("unverified email", func(t *testing.T) {
				for _, tc := range []unverifiedEmailCase{
					{"GitHub", identityv1.OAuthProviderGitHub, "victim@example.com", func(f *fakeProviders) {
						f.apis[githubUserAPI] = `{"id":4242,"login":"octocat","email":"victim@example.com"}`
					}},
					{"GitHub without email", identityv1.OAuthProviderGitHub, "victim@example.com", func(f *fakeProviders) {
						f.apis[githubEmailsAPI] = `[]`
					}},
					{"Google", identityv1.OAuthProviderGoogle, "victim@example.com", nil},
					{"Google new address", identityv1.OAuthProviderGoogle, "new@example.com", nil},
					{"Microsoft", identityv1.OAuthProviderMicrosoft, "victim@example.com", nil},
					{"Microsoft new address", identityv1.OAuthProviderMicrosoft, "new@example.com", nil},
					{"Microsoft without xms_edov", identityv1.OAuthProviderMicrosoft, "new@example.com", func(f *fakeProviders) {
						delete(f.idClaims, "xms_edov")
					}},
					{"Microsoft xms_edov not a boolean", identityv1.OAuthProviderMicrosoft, "new@example.com", func(f *fakeProviders) {
						f.idClaims["xms_edov"] = "true"
					}},
					{"Microsoft Graph mail", identityv1.OAuthProviderMicrosoft, "", func(f *fakeProviders) {
						delete(f.idClaims, "email")
						delete(f.idClaims, "xms_edov")
						f.apis[graphMeAPI] = `{"id":"m4242","mail":"victim@example.com"}`
					}},
					{"Microsoft Graph userPrincipalName", identityv1.OAuthProviderMicrosoft, "", func(f *fakeProviders) {
						delete(f.idClaims, "email")
						delete(f.idClaims, "xms_edov")
						f.apis[graphMeAPI] = `{"id":"m4242","userPrincipalName":"victim@example.com"}`
					}},
				} {
					t.Run(tc.name, func(t *testing.T) {
						e := newOAuthTestEnv(t, "http://localhost:8080")
						e.fake.setEmail(tc.email, false)
						if tc.edit != nil {
							tc.edit(e.fake)
						}
						e.wantRefused(t, e.login(t, tc.p), dto.ErrorCodeEmailNotVerified)
					})
				}
			})
			t.Run("invalid Microsoft ID token", func(t *testing.T) {
				for _, tc := range []idClaimsCase{
					{"missing", nil},
					{"other audience", func(c map[string]any) { c["aud"] = "other-client" }},
					{"expired", func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() }},
					{"no expiry", func(c map[string]any) { delete(c, "exp") }},
					{"issuer of another tenant", func(c map[string]any) {
						c["iss"] = "https://login.microsoftonline.com/99999999-2222-3333-4444-555555555555/v2.0"
					}},
					{"v1 issuer", func(c map[string]any) { c["iss"] = "https://sts.windows.net/" + fakeTenant + "/" }},
					{"no tenant", func(c map[string]any) {
						delete(c, "tid")
						c["iss"] = "https://login.microsoftonline.com//v2.0"
					}},
				} {
					t.Run(tc.name, func(t *testing.T) {
						e := newOAuthTestEnv(t, "http://localhost:8080")
						if tc.edit == nil {
							e.fake.idClaims = nil
						} else {
							tc.edit(e.fake.idClaims)
						}
						e.wantRefused(t, e.login(t, identityv1.OAuthProviderMicrosoft), dto.ErrorCodeOAuthError)
					})
				}
			})
			t.Run("GitHub emails unavailable", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				delete(e.fake.apis, githubEmailsAPI)
				e.wantRefused(t, e.login(t, identityv1.OAuthProviderGitHub), dto.ErrorCodeOAuthError)
			})
			t.Run("provider response", func(t *testing.T) {
				for _, tc := range []struct {
					name  string
					query url.Values
					want  string
				}{
					{"declined", url.Values{"error": {"access_denied"}}, "/login"},
					{"refused", url.Values{"error": {"server_error"}}, "/login?oauth_error=OAUTH_ERROR"},
					{"missing code", url.Values{}, "/login?oauth_error=OAUTH_ERROR"},
				} {
					t.Run(tc.name, func(t *testing.T) {
						e := newOAuthTestEnv(t, "http://localhost:8080")
						state, c := e.loginRedirect(t, identityv1.OAuthProviderGoogle)
						tc.query.Set("state", state)
						w := e.callbackQuery(t, identityv1.OAuthProviderGoogle, tc.query, c)
						wantRedirect(t, w, tc.want)
						wantCleared(t, w)
						if n := e.fake.exchanges.Load(); n != 0 {
							t.Errorf("token exchanges = %d, want 0", n)
						}
					})
				}
			})
			t.Run("failure returns to the starting page", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				e.fake.setEmail("new@example.com", false)
				r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/auth/oauth/google?next=%2Fdevice%3Fuser_code%3DABCD", http.NoBody)
				r.SetPathValue("provider", "google")
				w := httptest.NewRecorder()
				e.h.LoginRedirect(w, r)
				state, c := authorizeState(t, w.Header().Get("Location")), stateCookie(t, w)
				wantRedirect(t, e.callback(t, identityv1.OAuthProviderGoogle, state, c), "/device?oauth_error=EMAIL_NOT_VERIFIED&user_code=ABCD")
			})
			t.Run("link of an identity another account holds", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				e.linkIdentity(t, e.victim, identityv1.OAuthProviderGitHub)
				state, c := e.startLink(t, identityv1.OAuthProviderGitHub)
				wantRedirect(t, e.callback(t, identityv1.OAuthProviderGitHub, state, c), "/settings/user?oauth_error=CONFLICT")
				if ids := e.providerIDs(t, e.caller); len(ids) != 0 {
					t.Errorf("caller identities = %v, want none", ids)
				}
			})
			t.Run("unknown provider", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				state, c := e.loginRedirect(t, identityv1.OAuthProviderGoogle)
				wantRedirect(t, e.callback(t, "other", state, c), "/login?oauth_error=INVALID_PROVIDER")
			})
			t.Run("missing cookie", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				state, _ := e.loginRedirect(t, identityv1.OAuthProviderGitHub)
				e.wantRejected(t, e.callback(t, identityv1.OAuthProviderGitHub, state))
			})
			t.Run("mismatched state", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				_, c := e.loginRedirect(t, identityv1.OAuthProviderGitHub)
				other, _ := e.loginRedirect(t, identityv1.OAuthProviderGitHub)
				e.wantRejected(t, e.callback(t, identityv1.OAuthProviderGitHub, other, c))
			})
			t.Run("linking state naming another user", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				state, c := e.startLink(t, identityv1.OAuthProviderGitHub)
				forged := strings.Replace(state, e.caller.ID.String(), e.victim.ID.String(), 1)
				e.wantRejected(t, e.callback(t, identityv1.OAuthProviderGitHub, forged, c))
			})
			t.Run("forged linking state without cookie", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				e.wantRejected(t, e.callback(t, identityv1.OAuthProviderGitHub, linkingStatePrefix+e.victim.ID.String()+":github:x"))
			})
			t.Run("linking state signed with the JWT secret", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				forged := linkingStatePrefix + e.victim.ID.String() + ":github:x"
				c := e.h.stateCookie(oauthserver.SignState(forged, e.h.cfg.Snapshot().JWTSecret), oauthStateMaxAge)
				e.wantRejected(t, e.callback(t, identityv1.OAuthProviderGitHub, forged, c))
			})
		})
	})
}
