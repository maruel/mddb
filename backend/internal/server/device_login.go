// Mounts reusable device-login handlers behind mddb request limits.

package server

import (
	"net/http"

	"github.com/maruel/gomode/devicelogin"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/ratelimit"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
)

const deviceLoginPrefix = "/api/v1/auth/device/"

func mountDeviceLogin(mux *http.ServeMux, svc *handlers.Services, cfg *Config, hcfg *handlers.Config, authh *handlers.AuthHandler, limiters *ratelimit.Limiters) {
	core := devicelogin.New(&deviceLoginHost{services: svc, config: cfg, auth: authh})
	for _, route := range []struct {
		path    string
		handler http.HandlerFunc
		tier    *ratelimit.Tier
	}{
		{deviceLoginPrefix + "start", core.Start, &limiters.Auth},
		// Polling is a read despite its private JSON body. The auth tier permits only five requests/minute.
		{deviceLoginPrefix + "poll", core.Poll, &limiters.ReadUnauth},
		{browserDevicePoll, core.Poll, &limiters.ReadUnauth},
		{deviceLoginPrefix + "lookup", core.Lookup, &limiters.Auth},
		{deviceLoginPrefix + "approve", core.Approve, &limiters.Auth},
	} {
		mux.HandleFunc("POST "+route.path, func(w http.ResponseWriter, r *http.Request) {
			ctx := addRequestMetadataToContext(r.Context(), r)
			w, ok := checkRateLimit(w, route.tier, reqctx.ClientIP(ctx))
			if !ok {
				return
			}
			r = r.WithContext(ctx)
			if hcfg.Quotas.MaxRequestBodyBytes > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, hcfg.Quotas.MaxRequestBodyBytes)
			}
			route.handler(w, r)
		})
	}
}
