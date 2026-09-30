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

	"github.com/maruel/gomode/oauth/oauthserver"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
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
var fakeIDs = map[identity.OAuthProvider]string{
	identity.OAuthProviderGitHub:    "4242",
	identity.OAuthProviderGoogle:    "g4242",
	identity.OAuthProviderMicrosoft: "m4242",
}

var providers = []identity.OAuthProvider{
	identity.OAuthProviderGitHub,
	identity.OAuthProviderGoogle,
	identity.OAuthProviderMicrosoft,
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
	h    *OAuthHandler
	svc  *Services
	fake *fakeProviders
	// caller holds the session that makes the requests.
	caller *identity.User
	// victim is another account the caller must not reach. It holds
	// victim@example.com, which it never verified.
	victim *identity.User
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
	sessions, err := identity.NewSessionService(filepath.Join(db, "sessions.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := git.NewRootRepo(t.Context(), dir, "test", "test@test.com")
	if err != nil {
		t.Fatal(err)
	}
	serverCfg, err := storage.LoadServerConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc := &Services{User: users, Session: sessions, RootRepo: root}
	h := NewOAuthHandler(svc, &Config{ServerConfig: *serverCfg, BaseURL: baseURL})
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
	return &oauthTestEnv{h: h, svc: svc, fake: newFakeProviders(), caller: caller, victim: victim}
}

// loginRedirect starts a login with provider and returns the state sent to
// the provider and the state cookie.
func (e *oauthTestEnv) loginRedirect(t *testing.T, p identity.OAuthProvider) (string, *http.Cookie) {
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
func (e *oauthTestEnv) login(t *testing.T, p identity.OAuthProvider) *httptest.ResponseRecorder {
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
func (e *oauthTestEnv) startLink(t *testing.T, p identity.OAuthProvider) (string, *http.Cookie) {
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
func (e *oauthTestEnv) callback(t *testing.T, p identity.OAuthProvider, state string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	ctx := context.WithValue(t.Context(), oauth2.HTTPClient, &http.Client{Transport: e.fake})
	q := url.Values{"code": {"c"}, "state": {state}}
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
func (e *oauthTestEnv) get(t *testing.T, u *identity.User) *identity.User {
	got, err := e.svc.User.Get(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// providerIDs returns the identities linked to u as "provider:id".
func (e *oauthTestEnv) providerIDs(t *testing.T, u *identity.User) []string {
	got := e.get(t, u)
	ids := make([]string, 0, len(got.OAuthIdentities))
	for _, ident := range got.OAuthIdentities {
		ids = append(ids, string(ident.Provider)+":"+ident.ProviderID)
	}
	return ids
}

// sessions returns the number of active sessions of u.
func (e *oauthTestEnv) sessions(u *identity.User) int {
	n := 0
	for range e.svc.Session.GetActiveByUserID(u.ID) {
		n++
	}
	return n
}

// linkIdentity attaches the fake provider identity to u.
func (e *oauthTestEnv) linkIdentity(t *testing.T, u *identity.User, p identity.OAuthProvider) {
	if _, err := e.svc.User.Modify(u.ID, func(u *identity.User) error {
		u.OAuthIdentities = append(u.OAuthIdentities, identity.OAuthIdentity{Provider: p, ProviderID: fakeIDs[p]})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// wantLoggedIn checks that the login succeeded and returns the account that
// holds the provider identity.
func (e *oauthTestEnv) wantLoggedIn(t *testing.T, p identity.OAuthProvider, w *httptest.ResponseRecorder) *identity.User {
	if w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), "/?token=") {
		t.Fatalf("status = %d, location %q; body %s", w.Code, w.Header().Get("Location"), w.Body)
	}
	wantCleared(t, w)
	u, err := e.svc.User.GetByOAuth(p, fakeIDs[p])
	if err != nil {
		t.Fatal(err)
	}
	if n := e.sessions(u); n != 1 {
		t.Errorf("%s sessions = %d, want 1", u.Email, n)
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

// wantRefused checks that the login failed with status and code, and that it
// created no account and signed in nobody.
func (e *oauthTestEnv) wantRefused(t *testing.T, w *httptest.ResponseRecorder, status int, code dto.ErrorCode) {
	var resp dto.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || w.Code != status || resp.Error.Code != code {
		t.Errorf("status = %d, body %s; want %d %s", w.Code, w.Body, status, code)
	}
	wantCleared(t, w)
	e.wantVictimUntouched(t)
	if n := e.svc.User.Count(); n != 2 {
		t.Errorf("accounts = %d, want 2", n)
	}
	if n := e.sessions(e.caller); n != 0 {
		t.Errorf("caller sessions = %d, want 0", n)
	}
}

// wantRejected checks that the callback failed before the token exchange and
// linked nothing to either account.
func (e *oauthTestEnv) wantRejected(t *testing.T, w *httptest.ResponseRecorder) {
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body %s", w.Code, http.StatusBadRequest, w.Body)
	}
	if n := e.fake.exchanges.Load(); n != 0 {
		t.Errorf("token exchanges = %d, want 0", n)
	}
	for _, u := range []*identity.User{e.caller, e.victim} {
		if ids := e.providerIDs(t, u); len(ids) != 0 {
			t.Errorf("%s identities = %v, want none", u.Email, ids)
		}
	}
	wantCleared(t, w)
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
	p    identity.OAuthProvider
	apis map[string]string
}

// providerEditCase changes the fake provider before a login.
type providerEditCase struct {
	p    identity.OAuthProvider
	edit func(f *fakeProviders)
}

// unverifiedEmailCase is a login whose provider email is not verified.
type unverifiedEmailCase struct {
	name  string
	p     identity.OAuthProvider
	email string
	edit  func(f *fakeProviders)
}

// idClaimsCase changes the fake Microsoft ID token claims.
type idClaimsCase struct {
	name string
	edit func(c map[string]any)
}

func TestNewOAuthHandler(t *testing.T) {
	t.Run("LoginRedirect", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			for _, tc := range []redirectCase{
				{"http://localhost:8080", false},
				{"https://mddb.example.com", true},
			} {
				t.Run(tc.baseURL, func(t *testing.T) {
					e := newOAuthTestEnv(t, tc.baseURL)
					state, c := e.loginRedirect(t, identity.OAuthProviderGitHub)
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
	})
	t.Run("LinkOAuth", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			e := newOAuthTestEnv(t, "http://localhost:8080")
			state, c := e.startLink(t, identity.OAuthProviderGitHub)
			if got, ok := oauthserver.ValidateState(c.Value, e.h.stateKey); !ok || got != state {
				t.Errorf("cookie state = %q, %t; want %q", got, ok, state)
			}
			if id, p := parseLinkingState(state); id != e.caller.ID || p != identity.OAuthProviderGitHub {
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
					delete(e.h.providers, identity.OAuthProviderGoogle)
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
		t.Run("valid", func(t *testing.T) {
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
					{identity.OAuthProviderGitHub, map[string]string{
						githubEmailsAPI: `[{"email":"other@example.com","primary":true,"verified":false},{"email":"victim@example.com","primary":false,"verified":true}]`,
					}},
					{identity.OAuthProviderGoogle, map[string]string{
						googleUserAPI: `{"id":"g4242","email":"victim@example.com","verified_email":true}`,
					}},
					{identity.OAuthProviderMicrosoft, nil},
				} {
					t.Run(string(tc.p), func(t *testing.T) {
						e := newOAuthTestEnv(t, "http://localhost:8080")
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
			t.Run("GitHub ignores the unverified profile email", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				e.fake.apis[githubUserAPI] = `{"id":4242,"login":"octocat","email":"victim@example.com"}`
				e.fake.apis[githubEmailsAPI] = `[{"email":"victim@example.com","primary":true,"verified":false},{"email":"person@example.com","primary":false,"verified":true}]`
				u := e.wantLoggedIn(t, identity.OAuthProviderGitHub, e.login(t, identity.OAuthProviderGitHub))
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
					{identity.OAuthProviderGitHub, func(f *fakeProviders) { delete(f.apis, githubEmailsAPI) }},
					{identity.OAuthProviderMicrosoft, func(f *fakeProviders) { f.idClaims = nil }},
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
					{identity.OAuthProviderGitHub, func(f *fakeProviders) { delete(f.apis, githubEmailsAPI) }},
					{identity.OAuthProviderMicrosoft, func(f *fakeProviders) { f.idClaims = nil }},
				} {
					t.Run(string(tc.p), func(t *testing.T) {
						e := newOAuthTestEnv(t, "http://localhost:8080")
						tc.edit(e.fake)
						state, c := e.startLink(t, tc.p)
						w := e.callback(t, tc.p, state, c)
						if w.Code != http.StatusFound || w.Header().Get("Location") != "/?oauth_linked=true" {
							t.Fatalf("status = %d, location %q; body %s", w.Code, w.Header().Get("Location"), w.Body)
						}
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
						if w.Code != http.StatusFound || w.Header().Get("Location") != "/?oauth_linked=true" {
							t.Fatalf("status = %d, location %q; body %s", w.Code, w.Header().Get("Location"), w.Body)
						}
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
					{"GitHub", identity.OAuthProviderGitHub, "victim@example.com", func(f *fakeProviders) {
						f.apis[githubUserAPI] = `{"id":4242,"login":"octocat","email":"victim@example.com"}`
					}},
					{"GitHub without email", identity.OAuthProviderGitHub, "victim@example.com", func(f *fakeProviders) {
						f.apis[githubEmailsAPI] = `[]`
					}},
					{"Google", identity.OAuthProviderGoogle, "victim@example.com", nil},
					{"Google new address", identity.OAuthProviderGoogle, "new@example.com", nil},
					{"Microsoft", identity.OAuthProviderMicrosoft, "victim@example.com", nil},
					{"Microsoft new address", identity.OAuthProviderMicrosoft, "new@example.com", nil},
					{"Microsoft without xms_edov", identity.OAuthProviderMicrosoft, "new@example.com", func(f *fakeProviders) {
						delete(f.idClaims, "xms_edov")
					}},
					{"Microsoft xms_edov not a boolean", identity.OAuthProviderMicrosoft, "new@example.com", func(f *fakeProviders) {
						f.idClaims["xms_edov"] = "true"
					}},
					{"Microsoft Graph mail", identity.OAuthProviderMicrosoft, "", func(f *fakeProviders) {
						delete(f.idClaims, "email")
						delete(f.idClaims, "xms_edov")
						f.apis[graphMeAPI] = `{"id":"m4242","mail":"victim@example.com"}`
					}},
					{"Microsoft Graph userPrincipalName", identity.OAuthProviderMicrosoft, "", func(f *fakeProviders) {
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
						e.wantRefused(t, e.login(t, tc.p), http.StatusForbidden, dto.ErrorCodeEmailNotVerified)
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
						e.wantRefused(t, e.login(t, identity.OAuthProviderMicrosoft), http.StatusBadRequest, dto.ErrorCodeValidationFailed)
					})
				}
			})
			t.Run("GitHub emails unavailable", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				delete(e.fake.apis, githubEmailsAPI)
				e.wantRefused(t, e.login(t, identity.OAuthProviderGitHub), http.StatusInternalServerError, dto.ErrorCodeOAuthError)
			})
			t.Run("missing cookie", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				state, _ := e.loginRedirect(t, identity.OAuthProviderGitHub)
				e.wantRejected(t, e.callback(t, identity.OAuthProviderGitHub, state))
			})
			t.Run("mismatched state", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				_, c := e.loginRedirect(t, identity.OAuthProviderGitHub)
				other, _ := e.loginRedirect(t, identity.OAuthProviderGitHub)
				e.wantRejected(t, e.callback(t, identity.OAuthProviderGitHub, other, c))
			})
			t.Run("linking state naming another user", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				state, c := e.startLink(t, identity.OAuthProviderGitHub)
				forged := strings.Replace(state, e.caller.ID.String(), e.victim.ID.String(), 1)
				e.wantRejected(t, e.callback(t, identity.OAuthProviderGitHub, forged, c))
			})
			t.Run("forged linking state without cookie", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				e.wantRejected(t, e.callback(t, identity.OAuthProviderGitHub, linkingStatePrefix+e.victim.ID.String()+":github:x"))
			})
			t.Run("linking state signed with the JWT secret", func(t *testing.T) {
				e := newOAuthTestEnv(t, "http://localhost:8080")
				forged := linkingStatePrefix + e.victim.ID.String() + ":github:x"
				c := e.h.stateCookie(oauthserver.SignState(forged, e.h.cfg.JWTSecret), oauthStateMaxAge)
				e.wantRejected(t, e.callback(t, identity.OAuthProviderGitHub, forged, c))
			})
		})
	})
}
