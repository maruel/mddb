package githubapp

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestGenerateJWT(t *testing.T) {
	key := generateTestKey(t)
	c := NewClient(12345, key)

	tokenStr, err := c.GenerateJWT()
	if err != nil {
		t.Fatal(err)
	}

	// Parse and verify the JWT.
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			t.Fatalf("unexpected signing method: %v", token.Header["alg"])
		}
		return &key.PublicKey, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		t.Fatal("invalid token claims")
	}

	iss, _ := claims.GetIssuer()
	if iss != "12345" {
		t.Fatalf("unexpected issuer: %s", iss)
	}

	exp, _ := claims.GetExpirationTime()
	if exp == nil || time.Until(exp.Time) < 9*time.Minute {
		t.Fatal("JWT expiry too short")
	}
}

func TestGetInstallationToken(t *testing.T) {
	key := generateTestKey(t)

	expiry := time.Now().Add(1 * time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/installations/42/access_tokens" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		auth := r.Header.Get("Authorization")
		if auth == "" || len(auth) < 8 {
			t.Error("missing Authorization header")
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":      "ghs_test_token_123",
			"expires_at": expiry.Format(time.RFC3339),
		})
	}))
	defer server.Close()

	c := NewClient(12345, key)
	c.httpClient = server.Client()

	// Patch the URL by replacing the http client's transport.
	c.httpClient.Transport = &rewriteTransport{base: server.URL}

	token, tokenExpiry, err := c.GetInstallationToken(t.Context(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if token != "ghs_test_token_123" { //nolint:gosec // test value
		t.Fatalf("unexpected token: %s", token)
	}
	if tokenExpiry.Before(time.Now()) {
		t.Fatal("token already expired")
	}

	// Second call should use cache.
	token2, _, err := c.GetInstallationToken(t.Context(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if token2 != token {
		t.Fatal("expected cached token")
	}
}

func TestListInstallationRepos(t *testing.T) {
	key := generateTestKey(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app/installations/42/access_tokens":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":      "ghs_test",
				"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			})
		case "/installation/repositories":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repositories": []map[string]any{
					{
						"full_name": "org/repo",
						"owner":     map[string]string{"login": "org"},
						"name":      "repo",
						"private":   true,
						"html_url":  "https://github.com/org/repo",
						"clone_url": "https://github.com/org/repo.git",
					},
				},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	c := NewClient(12345, key)
	c.httpClient = server.Client()
	c.httpClient.Transport = &rewriteTransport{base: server.URL}

	repos, err := c.ListInstallationRepos(t.Context(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].FullName != "org/repo" {
		t.Fatalf("unexpected repo: %s", repos[0].FullName)
	}
}

// rewriteTransport rewrites requests to point at a test server.
type rewriteTransport struct {
	base string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.base[len("http://"):]
	return http.DefaultTransport.RoundTrip(req)
}
