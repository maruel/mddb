// Tests router ownership of background rate-limiter workers.

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/maruel/mddb/backend/internal/storage"
)

func TestRouter(t *testing.T) {
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
		if !env.services.StartBackground(t.Context(), time.Second, func(context.Context) {}) {
			t.Fatal("closing one router stopped shared services")
		}
	})
}
