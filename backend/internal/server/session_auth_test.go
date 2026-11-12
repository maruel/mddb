// Tests mandatory JWT claims, persistent session binding, revocation, and authentication entrypoints.

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/utils"
	"golang.org/x/crypto/bcrypt"
)

func TestValidateJWTAndSession(t *testing.T) {
	dir := t.TempDir()
	users, err := identity.NewUserService(filepath.Join(dir, "users.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	users.SetBcryptCostForTesting(bcrypt.MinCost)
	user, err := users.Create("session@example.com", "password123", "User")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sessions.jsonl")
	sessions, err := identity.NewSessionService(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &handlers.Config{ServerConfig: storage.ServerConfig{JWTSecret: testJWTSecret}}
	request := func(token string) *http.Request {
		r := httptest.NewRequestWithContext(t.Context(), "GET", "/", http.NoBody)
		r.Header.Set("Authorization", "Bearer "+token)
		return r
	}
	t.Run("valid", func(t *testing.T) {
		first, _, err := cfg.IssueToken(sessions, user, "192.0.2.1", "Browser", "CA")
		if err != nil {
			t.Fatal(err)
		}
		second, _, err := cfg.IssueToken(sessions, user, "192.0.2.2", "Display", "CA")
		if err != nil {
			t.Fatal(err)
		}
		u, sid, tok, err := validateJWTAndSession(request(first), users, sessions, testJWTSecret)
		if err != nil || u.ID != user.ID || tok != first || sid.IsZero() {
			t.Fatalf("valid credential: %v", err)
		}
		if err := sessions.Revoke(sid); err != nil {
			t.Fatal(err)
		}
		restarted, err := identity.NewSessionService(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := validateJWTAndSession(request(first), users, restarted, testJWTSecret); err == nil {
			t.Fatal("revoked session survived restart")
		}
		if _, _, _, err := validateJWTAndSession(request(second), users, restarted, testJWTSecret); err != nil {
			t.Fatal("independent session rejected", err)
		}
	})
	t.Run("valid/authenticated request records use", func(t *testing.T) {
		token, sid, err := cfg.IssueToken(sessions, user, "192.0.2.1", "Browser", "CA")
		if err != nil {
			t.Fatal(err)
		}
		// Backdate the persisted use beyond the recording resolution.
		stale := storage.ToTime(time.Now().Add(-2 * identity.LastUsedResolution))
		data, err := os.ReadFile(path) //nolint:gosec // G304: fixture path comes from t.TempDir
		if err != nil {
			t.Fatal(err)
		}
		lines := bytes.Split(data, []byte("\n"))
		for i, line := range lines {
			var s identity.Session
			if json.Unmarshal(line, &s) != nil || s.ID != sid {
				continue
			}
			s.LastUsed = stale
			if lines[i], err = json.Marshal(&s); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(path, bytes.Join(lines, []byte("\n")), 0o600); err != nil {
			t.Fatal(err)
		}
		restarted, err := identity.NewSessionService(path)
		if err != nil {
			t.Fatal(err)
		}
		if s, err := restarted.Get(sid); err != nil || s.LastUsed != stale {
			t.Fatalf("backdated session: %v", err)
		}
		svc := &handlers.Services{User: users, Session: restarted}
		if _, _, err := validateAuthWithContext(t.Context(), request(token), svc, cfg); err != nil {
			t.Fatal(err)
		}
		if s, err := restarted.Get(sid); err != nil || !s.LastUsed.After(stale) {
			t.Fatalf("authenticated request did not record use: %v", err)
		}
	})
	t.Run("error", func(t *testing.T) {
		for _, name := range []string{"missing sub", "missing sid", "missing iat", "missing exp", "missing email", "invalid sub", "invalid sid", "future iat", "invalid lifetime", "expired JWT", "HS384", "unsigned", "missing session", "wrong user", "unknown user", "wrong hash", "wrong expiry", "revoked", "expired session", "deleted session", "missing session service", "weak key"} {
			t.Run(name, func(t *testing.T) {
				now := time.Now().Truncate(time.Second)
				exp := now.Add(time.Hour)
				sid := ksid.NewID()
				uid := user.ID
				hash := ""
				storedExp := exp
				claims := jwt.MapClaims{"sub": uid.String(), "email": user.Email, "sid": sid.String(), "iat": now.Unix(), "exp": exp.Unix()}
				method := jwt.SigningMethod(jwt.SigningMethodHS256)
				key := any(testJWTSecret)
				switch name {
				case "missing sub":
					delete(claims, "sub")
				case "missing sid":
					delete(claims, "sid")
				case "missing iat":
					delete(claims, "iat")
				case "missing exp":
					delete(claims, "exp")
				case "missing email":
					delete(claims, "email")
				case "invalid sub":
					claims["sub"] = "bad"
				case "invalid sid":
					claims["sid"] = "bad"
				case "future iat":
					claims["iat"] = exp.Add(time.Hour).Unix()
				case "invalid lifetime":
					claims["iat"] = now.Add(-time.Hour).Unix()
					claims["exp"] = now.Add(-2 * time.Hour).Unix()
				case "expired JWT":
					claims["exp"] = now.Add(-time.Minute).Unix()
				case "HS384":
					method = jwt.SigningMethodHS384
				case "unsigned":
					method = jwt.SigningMethodNone
					key = jwt.UnsafeAllowNoneSignatureType
				case "wrong user":
					uid = ksid.NewID()
				case "unknown user":
					uid = ksid.NewID()
					claims["sub"] = uid.String()
				case "wrong hash":
					hash = utils.HashToken("different credential")
				case "wrong expiry":
					storedExp = exp.Add(time.Hour)
				case "expired session", "deleted session":
					storedExp = now.Add(-time.Hour)
				}
				token, err := jwt.NewWithClaims(method, claims).SignedString(key)
				if err != nil {
					t.Fatal(err)
				}
				if hash == "" {
					hash = utils.HashToken(token)
				}
				if name != "missing session" {
					if _, err := sessions.CreateWithID(sid, uid, hash, "Browser", "", "", storage.ToTime(storedExp), 0); err != nil {
						t.Fatal(err)
					}
				}
				if name == "revoked" {
					if err := sessions.Revoke(sid); err != nil {
						t.Fatal(err)
					}
				}
				if name == "deleted session" {
					if _, err := sessions.CleanupExpired(0); err != nil {
						t.Fatal(err)
					}
				}
				svc := sessions
				secret := testJWTSecret
				if name == "missing session service" {
					svc = nil
				}
				if name == "weak key" {
					secret = []byte("weak")
				}
				if _, _, _, err := validateJWTAndSession(request(token), users, svc, secret); err == nil {
					t.Fatal("unsupported credential authenticated")
				}
			})
		}
	})
}

func TestTrackedLoginEntrypoints(t *testing.T) {
	e := setupTestEnv(t)
	var registration, login dto.AuthResponse
	if got := e.doJSON(t, "POST", "/api/v1/auth/register", dto.RegisterRequest{Email: "tracked@example.com", Password: "password123", Name: "User"}, &registration, ""); got != http.StatusOK {
		t.Fatalf("register: %d", got)
	}
	if got := e.doJSON(t, "POST", "/api/v1/auth/login", dto.LoginRequest{Email: "tracked@example.com", Password: "password123"}, &login, ""); got != http.StatusOK {
		t.Fatalf("login: %d", got)
	}
	var listed dto.ListSessionsResponse
	if got := e.doJSON(t, "GET", "/api/v1/auth/sessions", nil, &listed, registration.Token); got != http.StatusOK || len(listed.Sessions) != 2 {
		t.Fatalf("sessions: %d %+v", got, listed)
	}
	if got := e.doJSON(t, "POST", "/api/v1/auth/logout", nil, nil, login.Token); got != http.StatusOK {
		t.Fatalf("logout: %d", got)
	}
	if got := e.doJSON(t, "GET", "/api/v1/auth/me", nil, nil, login.Token); got != http.StatusUnauthorized {
		t.Fatalf("logged-out credential accepted: %d", got)
	}
	if got := e.doJSON(t, "GET", "/api/v1/auth/me", nil, nil, registration.Token); got != http.StatusOK {
		t.Fatalf("other session revoked: %d", got)
	}
}

// TestAuthorizationErrors checks that every authorization wrapper answers with
// the JSON error body the SDK client decodes.
func TestAuthorizationErrors(t *testing.T) {
	e := setupTestEnv(t)
	// The first account becomes the global admin; reg is an ordinary user.
	var admin, reg dto.AuthResponse
	for _, a := range []struct {
		email string
		resp  *dto.AuthResponse
	}{{"admin@example.com", &admin}, {"authz@example.com", &reg}} {
		if got := e.doJSON(t, "POST", "/api/v1/auth/register", dto.RegisterRequest{Email: a.email, Password: "password123", Name: "User"}, a.resp, ""); got != http.StatusOK {
			t.Fatalf("register %s: %d", a.email, got)
		}
	}
	unknown := ksid.NewID().String()
	for _, tc := range []struct {
		name, method, path, token string
		status                    int
		code                      dto.ErrorCode
	}{
		{"WrapAuth without credential", "GET", "/api/v1/auth/me", "", http.StatusUnauthorized, dto.ErrorCodeUnauthorized},
		{"raw without credential", "POST", "/api/v1/auth/oauth/link", "", http.StatusUnauthorized, dto.ErrorCodeUnauthorized},
		{"org without credential", "GET", "/api/v1/organizations/" + unknown, "", http.StatusUnauthorized, dto.ErrorCodeUnauthorized},
		{"org invalid ID", "GET", "/api/v1/organizations/bad!", reg.Token, http.StatusBadRequest, dto.ErrorCodeInvalidFormat},
		{"org non-member", "GET", "/api/v1/organizations/" + unknown, reg.Token, http.StatusForbidden, dto.ErrorCodeForbidden},
		{"workspace without credential", "GET", "/api/v1/workspaces/" + unknown, "", http.StatusUnauthorized, dto.ErrorCodeUnauthorized},
		{"workspace invalid ID", "GET", "/api/v1/workspaces/bad!", reg.Token, http.StatusBadRequest, dto.ErrorCodeInvalidFormat},
		{"workspace unknown", "GET", "/api/v1/workspaces/" + unknown, reg.Token, http.StatusNotFound, dto.ErrorCodeNotFound},
		{"global admin without credential", "GET", "/api/v1/server/config", "", http.StatusUnauthorized, dto.ErrorCodeUnauthorized},
		{"global admin non-admin", "GET", "/api/v1/server/config", reg.Token, http.StatusForbidden, dto.ErrorCodeForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var resp dto.ErrorResponse
			if got := e.doJSON(t, tc.method, tc.path, nil, &resp, tc.token); got != tc.status || resp.Error.Code != tc.code {
				t.Fatalf("got %d %q, want %d %q", got, resp.Error.Code, tc.status, tc.code)
			}
		})
	}
}

func TestGetSession(t *testing.T) {
	e := setupTestEnv(t)
	var reg dto.AuthResponse
	if got := e.doJSON(t, "POST", "/api/v1/auth/register", dto.RegisterRequest{Email: "session-probe@example.com", Password: "password123", Name: "User"}, &reg, ""); got != http.StatusOK {
		t.Fatalf("register: %d", got)
	}
	t.Run("valid", func(t *testing.T) {
		var resp dto.SessionUserResponse
		if got := e.doJSON(t, "GET", "/api/v1/auth/session", nil, &resp, reg.Token); got != http.StatusOK || resp.User == nil || resp.User.ID != reg.User.ID || resp.User.SessionID.IsZero() {
			t.Fatalf("signed in: %d %+v", got, resp.User)
		}
	})
	t.Run("signed out", func(t *testing.T) {
		for name, token := range map[string]string{"no credential": "", "invalid credential": "not-a-jwt"} {
			t.Run(name, func(t *testing.T) {
				var resp dto.SessionUserResponse
				if got := e.doJSON(t, "GET", "/api/v1/auth/session", nil, &resp, token); got != http.StatusOK || resp.User != nil {
					t.Fatalf("got %d %+v, want 200 and no user", got, resp.User)
				}
			})
		}
	})
}
