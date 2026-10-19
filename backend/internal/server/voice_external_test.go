// Tests external gateway discovery, authentication and independently verified narrow tokens.

package server

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maruel/gomode"
	"github.com/maruel/gomode/oauth/oauthverify"
	voicev1 "github.com/maruel/gomode/voicegateway/api/v1"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
)

func TestExternalVoiceGateway(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"oauth", "scoped"} {
		t.Run(mode, func(t *testing.T) {
			env := setupTestEnv(t)
			var auth dto.AuthResponse
			if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{Email: "voice-external@example.com", Password: "Pass1234", Name: "Voice"}, &auth, ""); status != http.StatusOK {
				t.Fatalf("register: %d", status)
			}
			var h http.Handler
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) }))
			t.Cleanup(ts.Close)
			var key any
			var public ed25519.PublicKey
			if mode == "scoped" {
				pub, k, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				key = k
				public = pub
			} else {
				k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				key = k
			}
			der, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			gateway, err := NewExternalVoiceGateway("https://voice.example.com", ts.URL, "mddb-test", VoiceTokenMode(mode), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
			if err != nil {
				t.Fatal(err)
			}
			cfg := &storage.ServerConfig{JWTSecret: testJWTSecret, Quotas: storage.DefaultServerQuotas(false), RateLimits: storage.DefaultRateLimits()}
			h = NewRouter(env.services, &Config{ServerConfig: cfg, Version: "test", BaseURL: ts.URL, ExternalVoice: gateway, VoiceBridge: &testVoiceBridge{}})
			request := func(method, path, token string) *http.Response {
				r, err := http.NewRequestWithContext(t.Context(), method, ts.URL+path, http.NoBody)
				if err != nil {
					t.Fatal(err)
				}
				if token != "" {
					r.Header.Set("Authorization", "Bearer "+token)
				}
				res, err := http.DefaultClient.Do(r)
				if err != nil {
					t.Fatal(err)
				}

				return res
			}
			discovery := request("GET", "/.well-known/gomode.json", "")
			defer func() {
				if err := discovery.Body.Close(); err != nil {
					t.Error(err)
				}
			}()
			var manifest gomode.Settings
			if err := json.NewDecoder(discovery.Body).Decode(&manifest); err != nil {
				t.Fatal(err)
			}
			if v := manifest.WebShell.VoiceGateway; v.URL != "https://voice.example.com" || v.TokenEndpoint != "/api/v1/gomode/voice/token" || !v.AuthRequired {
				t.Fatalf("discovery: %+v", v)
			}
			anonymous := request("POST", voiceTokenEndpoint, "")
			defer func() {
				if err := anonymous.Body.Close(); err != nil {
					t.Error(err)
				}
			}()
			if anonymous.StatusCode != http.StatusUnauthorized {
				t.Fatalf("anonymous: %d", anonymous.StatusCode)
			}
			res := request("POST", voiceTokenEndpoint, auth.Token)
			defer func() {
				if err := res.Body.Close(); err != nil {
					t.Error(err)
				}
			}()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("token status: %d", res.StatusCode)
			}
			if res.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("token response is cacheable")
			}
			var svc voicev1.ServiceAuthorization
			if err := json.NewDecoder(res.Body).Decode(&svc); err != nil {
				t.Fatal(err)
			}
			if svc.Kind != "mddb" || svc.InstanceID != "mddb-test" || svc.BaseURL != ts.URL || svc.Token == auth.Token {
				t.Fatalf("service authorization: %+v", svc)
			}
			user, err := env.services.User.GetByEmail("voice-external@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if mode == "scoped" {
				claims, err := gomode.VerifyServiceScopedToken(svc.Token, public, gomode.ScopedTokenAudience)
				if err != nil {
					t.Fatal(err)
				}
				if claims.Subject != user.ID.String() || claims.BackendOrigin != ts.URL || claims.ServiceKind != "mddb" || claims.ServiceInstanceID != "mddb-test" || len(claims.Capabilities) != 1 || claims.Capabilities[0] != "voice.session" || time.Until(claims.Expiry) > 5*time.Minute {
					t.Fatalf("claims: %+v", claims)
				}
				if _, err := gomode.VerifyServiceScopedToken(svc.Token, public, "mddb"); err == nil {
					t.Fatal("voice token accepted for other audience")
				}
			} else {
				verifier := oauthverify.New(nil)
				claims, err := verifier.Verify(t.Context(), svc.Token, ts.URL, "voice-gateway", "voice.session")
				if err != nil {
					t.Fatal(err)
				}
				if claims.Subject != user.ID.String() || len(claims.Scopes) != 1 || time.Until(claims.Expiry) > 5*time.Minute {
					t.Fatalf("claims: %+v", claims)
				}
				if _, err := verifier.Verify(t.Context(), svc.Token, ts.URL, "mddb", "voice.session"); err == nil {
					t.Fatal("voice token accepted for other audience")
				}
				if _, err := verifier.Verify(t.Context(), svc.Token, ts.URL, "voice-gateway", "mcp"); err == nil {
					t.Fatal("voice token accepted for other scope")
				}
			}
			embedded := request("GET", "/api/voicegateway/v1/voice/choices", auth.Token)
			defer func() {
				if err := embedded.Body.Close(); err != nil {
					t.Error(err)
				}
			}()
			if strings.HasPrefix(embedded.Header.Get("Content-Type"), "application/json") {
				t.Fatal("external gateway did not override embedded gateway")
			}
		})
	}
}
