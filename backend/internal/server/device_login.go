// Mounts reusable device-login handlers behind mddb request limits.

package server

import (
	"log/slog"
	"net/http"

	"github.com/gomodedev/gomode/devicelogin"
	"github.com/gomodedev/gomode/httpserver"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/ratetier"
)

const deviceLoginPrefix = "/api/v1/auth/device/"

func mountDeviceLogin(log *slog.Logger, mux *http.ServeMux, svc *handlers.Services, cfg *Config, hcfg *handlers.Config, authh *handlers.AuthHandler, limiters *ratetier.Limiters) {
	core := devicelogin.New(log, &deviceLoginHost{services: svc, config: cfg, auth: authh})
	for _, route := range []struct {
		path    string
		handler http.HandlerFunc
		tier    *ratetier.Tier
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
			if !checkRateLimit(w, route.tier, httpserver.ClientIP(ctx)) {
				return
			}
			r = r.WithContext(ctx)
			if limit := hcfg.Snapshot().Quotas.MaxRequestBodyBytes; limit > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			route.handler(w, r)
		})
	}
}
