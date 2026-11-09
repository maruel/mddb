// Tests router request policies, session metadata, bandwidth and worker ownership.

package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maruel/gomode/httpserver/geoip"
	"github.com/maruel/mddb/backend/internal/server/bandwidth"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
)

func TestRouter(t *testing.T) {
	t.Run("ServeHTTP", func(t *testing.T) {
		t.Run("session metadata", func(t *testing.T) {
			for _, tc := range []struct {
				name, peer, forwarded, ip, country string
				geo                                bool
			}{
				{name: "trusted proxy", peer: "192.0.2.1:1234", forwarded: "100.101.102.103", ip: "100.101.102.103", country: "tailscale", geo: true},
				{name: "untrusted peer", peer: "127.0.0.1:1234", forwarded: "100.101.102.103", ip: "127.0.0.1", country: "local", geo: true},
				{name: "unknown country allowed", peer: "192.0.2.1:1234", forwarded: "8.8.8.8", ip: "8.8.8.8", geo: true},
				{name: "optional geo disabled", peer: "192.0.2.1:1234", forwarded: "100.101.102.103", ip: "100.101.102.103"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					env := setupTestEnv(t)
					cfg := &Config{
						ServerConfig:   &storage.ServerConfig{JWTSecret: testJWTSecret, Quotas: storage.DefaultServerQuotas(false), RateLimits: storage.DefaultRateLimits()},
						BaseURL:        "https://configured.example.com",
						TrustedProxies: []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")},
					}
					if tc.geo {
						// A CIDR-only checker exercises enrichment without a country DB.
						checker, err := geoip.NewChecker(t.Context(), slog.New(slog.DiscardHandler), "0.0.0.0/0,::/0", "", "")
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = checker.Close() })
						cfg.IPGeo = checker
					}
					router := NewRouter(env.services, cfg)
					t.Cleanup(router.Close)
					req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "https://other.example.com/api/v1/auth/register", strings.NewReader(`{"email":"metadata@example.com","password":"password123","name":"Metadata"}`))
					req.RemoteAddr = tc.peer
					req.Header.Set("X-Forwarded-For", tc.forwarded)
					req.Header.Set("User-Agent", "test-browser")
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)
					if w.Code != http.StatusOK {
						t.Fatalf("register: %d %s", w.Code, w.Body.String())
					}
					var auth dto.AuthResponse
					if err := json.Unmarshal(w.Body.Bytes(), &auth); err != nil {
						t.Fatal(err)
					}
					session, err := env.services.Session.Get(auth.User.SessionID)
					if err != nil {
						t.Fatal(err)
					}
					if session.IPAddress != tc.ip || session.CountryCode != tc.country || session.DeviceInfo != "test-browser" {
						t.Fatalf("session metadata: ip=%q country=%q device=%q", session.IPAddress, session.CountryCode, session.DeviceInfo)
					}
				})
			}
		})
		t.Run("configured origin filtering", func(t *testing.T) {
			request := func(t *testing.T, peer, forwarded string) (*httptest.ResponseRecorder, *testEnv) {
				env := setupTestEnv(t)
				geo := geoip.Config{AllowOrigins: []string{"local", "203.0.113.0/24"}}
				checker, err := geo.Open(t.Context(), slog.New(slog.DiscardHandler), "")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = checker.Close() })
				router := NewRouter(env.services, &Config{
					ServerConfig:   &storage.ServerConfig{JWTSecret: testJWTSecret, Quotas: storage.DefaultServerQuotas(false), RateLimits: storage.DefaultRateLimits()},
					IPGeo:          checker,
					TrustedProxies: []netip.Prefix{netip.MustParsePrefix("192.0.2.1/32")},
				})
				t.Cleanup(router.Close)
				req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"filter@example.com","password":"password123","name":"Filter"}`))
				req.RemoteAddr = peer
				req.Header.Set("X-Forwarded-For", forwarded)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				return w, env
			}
			t.Run("valid", func(t *testing.T) {
				for _, forwarded := range []string{"127.0.0.1", "203.0.113.5"} {
					t.Run(forwarded, func(t *testing.T) {
						w, env := request(t, "192.0.2.1:1234", forwarded)
						if w.Code != http.StatusOK || env.services.User.Count() != 1 {
							t.Fatalf("registration: status=%d users=%d body=%s", w.Code, env.services.User.Count(), w.Body.String())
						}
					})
				}
			})
			t.Run("error", func(t *testing.T) {
				for _, tc := range []struct{ name, peer, forwarded string }{
					{"unknown forwarded client", "192.0.2.1:1234", "8.8.8.8"},
					{"untrusted spoof", "198.51.100.1:1234", "127.0.0.1"},
				} {
					t.Run(tc.name, func(t *testing.T) {
						w, env := request(t, tc.peer, tc.forwarded)
						if w.Code != http.StatusForbidden || env.services.User.Count() != 0 {
							t.Fatalf("blocked registration: status=%d users=%d", w.Code, env.services.User.Count())
						}
					})
				}
			})
		})
		t.Run("host and cross-origin policies", func(t *testing.T) {
			env := setupTestEnv(t)
			cfg := &Config{
				ServerConfig: &storage.ServerConfig{JWTSecret: testJWTSecret, Quotas: storage.DefaultServerQuotas(false), RateLimits: storage.DefaultRateLimits()},
				BaseURL:      "https://configured.example.com",
			}
			router := NewRouter(env.services, cfg)
			t.Cleanup(router.Close)
			for _, host := range []string{"first.example.com", "second.example.com"} {
				r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+host+"/api/v1/health", http.NoBody)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Code != http.StatusOK {
					t.Fatalf("host %s: %d", host, w.Code)
				}
			}
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "https://first.example.com/api/v1/auth/register", strings.NewReader(`{"email":"cross@example.com","password":"password123","name":"Cross"}`))
			r.Header.Set("Origin", "https://attacker.example.com")
			r.Header.Set("Sec-Fetch-Site", "cross-site")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != http.StatusForbidden || env.services.User.Count() != 0 {
				t.Fatalf("cross-site registration: %d, users=%d", w.Code, env.services.User.Count())
			}
		})
	})

	t.Run("Close", func(t *testing.T) {
		env := setupTestEnv(t)
		cfg := &Config{ServerConfig: &storage.ServerConfig{
			JWTSecret:  testJWTSecret,
			Quotas:     storage.DefaultServerQuotas(false),
			RateLimits: storage.DefaultRateLimits(),
		}}
		synctest.Test(t, func(t *testing.T) {
			r := NewRouter(env.services, cfg)
			var wg sync.WaitGroup
			wg.Go(r.Close)
			t.Cleanup(wg.Wait)
			// Request processing and rate-limit enforcement survive cleanup shutdown.
			for range 5 {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("{}")))
				if w.Code != http.StatusBadRequest {
					t.Fatalf("login status = %d", w.Code)
				}
			}
			wg.Wait()
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("{}")))
			if w.Code != http.StatusTooManyRequests {
				t.Fatalf("login limit after Close = %d", w.Code)
			}
			// The bubble cannot complete if any owned cleanup worker remains alive.
		})
	})
}

func TestBandwidthWriter(t *testing.T) {
	t.Run("Write", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			w := httptest.NewRecorder()
			rw := &bandwidthWriter{ResponseWriter: w, bandwidthLimiter: bandwidth.NewLimiter(8)}
			start := time.Now()
			for _, b := range []string{"12345678", "abcd"} {
				if _, err := rw.Write([]byte(b)); err != nil {
					t.Fatal(err)
				}
			}
			if got := time.Since(start); got != 500*time.Millisecond {
				t.Fatalf("bandwidth wait = %s, want 500ms", got)
			}
			if got := w.Body.String(); got != "12345678abcd" {
				t.Fatalf("body = %q", got)
			}
		})
	})
}
