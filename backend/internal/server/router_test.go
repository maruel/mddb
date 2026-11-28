// Tests router request policies, live settings, metadata, bandwidth and worker ownership.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
	"github.com/maruel/mddb/backend/internal/testlog"

	"github.com/gomodedev/gomode/httpserver/geoip"
	"github.com/maruel/mddb/backend/internal/server/bandwidth"
	"github.com/maruel/mddb/backend/internal/server/dto/v1"
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
						ServerSettings: storage.NewServerSettings(&storagev1.ServerConfig{JWTSecret: testJWTSecret, Quotas: storage.DefaultServerQuotas(false), RateLimits: storage.DefaultRateLimits()}),
						BaseURL:        "https://configured.example.com",
						TrustedProxies: []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")},
					}
					if tc.geo {
						// A CIDR-only checker exercises enrichment without a country DB.
						checker, err := geoip.NewChecker(t.Context(), testlog.New(t), "0.0.0.0/0,::/0", "", "")
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = checker.Close() })
						cfg.IPGeo = checker
					}
					router := NewRouter(testlog.New(t), env.services, cfg)
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
				checker, err := geo.Open(t.Context(), testlog.New(t), "")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = checker.Close() })
				router := NewRouter(testlog.New(t), env.services, &Config{
					ServerSettings: storage.NewServerSettings(&storagev1.ServerConfig{JWTSecret: testJWTSecret, Quotas: storage.DefaultServerQuotas(false), RateLimits: storage.DefaultRateLimits()}),
					IPGeo:          checker,
					TrustedProxies: []netip.Prefix{netip.MustParsePrefix("192.0.2.1/32")},
				})
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
				ServerSettings: storage.NewServerSettings(&storagev1.ServerConfig{JWTSecret: testJWTSecret, Quotas: storage.DefaultServerQuotas(false), RateLimits: storage.DefaultRateLimits()}),
				BaseURL:        "https://configured.example.com",
			}
			router := NewRouter(testlog.New(t), env.services, cfg)
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
		t.Run("cancels bandwidth wait", func(t *testing.T) {
			env := setupTestEnv(t)
			settings := &storagev1.ServerConfig{
				JWTSecret:  testJWTSecret,
				Quotas:     storage.DefaultServerQuotas(false),
				RateLimits: storage.DefaultRateLimits(),
			}
			settings.Quotas.MaxEgressBandwidthBps = 1
			cfg := &Config{ServerSettings: storage.NewServerSettings(settings)}
			synctest.Test(t, func(t *testing.T) {
				router := NewRouter(testlog.New(t), env.services, cfg)
				ctx, cancel := context.WithCancel(t.Context())
				t.Cleanup(cancel)
				w := httptest.NewRecorder()
				done := make(chan struct{})
				t.Cleanup(func() { <-done })
				go func() {
					router.ServeHTTP(w, httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/health", nil))
					close(done)
				}()
				synctest.Wait()
				select {
				case <-done:
					t.Fatal("response did not wait for bandwidth")
				default:
				}
				cancel()
				synctest.Wait()
				select {
				case <-done:
					if w.Body.Len() != 0 {
						t.Fatalf("canceled response wrote %q", w.Body.String())
					}
				default:
					t.Error("request stayed blocked after cancellation")
				}
			})
		})
	})

	t.Run("published settings reach authenticated handlers", func(t *testing.T) {
		t.Parallel()
		env := setupTestEnv(t)
		var login dto.AuthResponse
		if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{Email: "admin@example.com", Password: "Pass1234", Name: "Admin"}, &login, ""); status != http.StatusOK {
			t.Fatalf("register admin: %d", status)
		}
		var current dto.ServerConfigResponse
		if status := env.doJSON(t, http.MethodGet, "/api/v1/server/config", nil, &current, login.Token); status != http.StatusOK {
			t.Fatalf("read settings: %d", status)
		}
		data, err := json.Marshal(current.Quotas)
		if err != nil {
			t.Fatal(err)
		}
		var quotas dto.QuotasConfigUpdate
		if err := json.Unmarshal(data, &quotas); err != nil {
			t.Fatal(err)
		}
		quotas.MaxUsers = 1
		if status := env.doJSON(t, http.MethodPost, "/api/v1/server/config", dto.UpdateServerConfigRequest{Quotas: &quotas}, nil, login.Token); status != http.StatusOK {
			t.Fatalf("update settings: %d", status)
		}
		if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{Email: "member@example.com", Password: "Pass1234", Name: "Member"}, nil, ""); status != http.StatusTooManyRequests {
			t.Fatalf("registration ignored the published user quota: %d", status)
		}
	})
	t.Run("auth tier limits login without owning a worker", func(t *testing.T) {
		env := setupTestEnv(t)
		cfg := &Config{ServerSettings: storage.NewServerSettings(&storagev1.ServerConfig{
			JWTSecret:  testJWTSecret,
			Quotas:     storage.DefaultServerQuotas(false),
			RateLimits: storage.DefaultRateLimits(),
		})}
		// The bubble cannot complete if the router leaves a goroutine running.
		synctest.Test(t, func(t *testing.T) {
			r := NewRouter(testlog.New(t), env.services, cfg)
			for i := range 5 {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("{}")))
				if w.Code != http.StatusBadRequest {
					t.Fatalf("login status = %d", w.Code)
				}
				if got, want := w.Header().Get("X-RateLimit-Remaining"), strconv.Itoa(4-i); got != want {
					t.Errorf("login %d X-RateLimit-Remaining = %q, want %q", i+1, got, want)
				}
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("{}")))
			if w.Code != http.StatusTooManyRequests {
				t.Fatalf("login status after the burst = %d", w.Code)
			}
			if got := w.Header().Get("Retry-After"); got != "12" {
				t.Errorf("Retry-After = %q, want the 12 s until the next of 5 tokens per minute", got)
			}
		})
	})
}

func TestBandwidthWriter(t *testing.T) {
	t.Run("Write", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			w := httptest.NewRecorder()
			rw := &bandwidthWriter{ResponseWriter: w, ctx: t.Context(), bandwidthLimiter: bandwidth.NewLimiter(8)}
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
		t.Run("canceled", func(t *testing.T) {
			for _, limit := range []int64{0, 8} {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				w := httptest.NewRecorder()
				rw := &bandwidthWriter{ResponseWriter: w, ctx: ctx, bandwidthLimiter: bandwidth.NewLimiter(limit)}
				n, err := rw.Write([]byte("abcd"))
				if n != 0 || !errors.Is(err, context.Canceled) || w.Body.Len() != 0 {
					t.Fatalf("limit %d: write = %d, %v; body = %q", limit, n, err, w.Body.String())
				}
			}
		})
		t.Run("deadline", func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
				t.Cleanup(cancel)
				w := httptest.NewRecorder()
				rw := &bandwidthWriter{ResponseWriter: w, ctx: ctx, bandwidthLimiter: bandwidth.NewLimiter(8)}
				start := time.Now()
				n, err := rw.Write([]byte("1234567890123456"))
				if n != 0 || !errors.Is(err, context.DeadlineExceeded) || w.Body.Len() != 0 {
					t.Fatalf("write = %d, %v; body = %q", n, err, w.Body.String())
				}
				if elapsed := time.Since(start); elapsed != 100*time.Millisecond {
					t.Fatalf("deadline returned after %s", elapsed)
				}
			})
		})
	})
}
