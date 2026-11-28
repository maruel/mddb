// Tests complete settings updates, rejection isolation, and live runtime controls.

package handlers

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"

	"github.com/maruel/mddb/backend/internal/server/bandwidth"
	"github.com/maruel/mddb/backend/internal/server/dto/v1"
	"github.com/maruel/mddb/backend/internal/server/ratelimit"
	"github.com/maruel/mddb/backend/internal/storage"
)

func TestServerHandler(t *testing.T) {
	t.Parallel()
	setup := func(t *testing.T) (*ServerHandler, *bandwidth.Limiter, *ratelimit.Limiters) {
		dir := t.TempDir()
		cfg, err := storage.LoadServerConfig(dir, false)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Quotas.NotificationRetentionDays = 7
		cfg.Quotas.MaxNotificationsPerUser = 23
		if err := storage.SaveServerConfig(cfg, dir); err != nil {
			t.Fatal(err)
		}
		bw := bandwidth.NewLimiter(0)
		rl := ratelimit.NewLimiters(storage.DefaultRateLimits(), false)
		return &ServerHandler{Settings: storage.NewServerSettings(cfg), ConfigDir: dir, BandwidthLimiter: bw, RateLimiters: rl}, bw, rl
	}
	t.Run("UpdateConfig/valid", func(t *testing.T) {
		t.Parallel()
		h, bw, rl := setup(t)
		before := h.Settings.Snapshot()
		response, err := h.GetConfig(t.Context(), nil, &dto.ServerConfigRequest{})
		if err != nil {
			t.Fatal(err)
		}
		// The real client sends the exposed quota fields back. Fields omitted
		// from the endpoint, such as notification retention, must survive.
		data, err := json.Marshal(response.Quotas)
		if err != nil {
			t.Fatal(err)
		}
		var quotas dto.QuotasConfigUpdate
		if err := json.Unmarshal(data, &quotas); err != nil {
			t.Fatal(err)
		}
		quotas.MaxPages = 4
		quotas.MaxEgressBandwidthBps = 16
		req := &dto.UpdateServerConfigRequest{Quotas: &quotas, RateLimits: &dto.RateLimitsConfigUpdate{WriteRatePerMin: 1}}
		out, err := h.UpdateConfig(t.Context(), nil, req)
		if err != nil {
			t.Fatal(err)
		}
		if !out.Ok {
			t.Fatal("update did not succeed")
		}
		want := *before
		want.Quotas.MaxPages = 4
		want.Quotas.MaxEgressBandwidthBps = 16
		want.RateLimits = storagev1.RateLimits{WriteRatePerMin: 1}
		if !reflect.DeepEqual(h.Settings.Snapshot(), &want) {
			t.Fatalf("active settings changed unrelated fields: %+v", h.Settings.Snapshot())
		}
		restarted, err := storage.LoadServerConfig(h.ConfigDir, false)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(restarted, &want) {
			t.Fatal("restart lost settings")
		}
		if bw.Allow(32) <= 0 {
			t.Fatal("bandwidth control did not receive the new limit")
		}
		if !rl.Write.Limiter.Allow("user").Allowed || rl.Write.Limiter.Allow("user").Allowed {
			t.Fatal("write rate did not receive the new limit")
		}
		if !rl.Auth.Limiter.Allow("ip").Allowed || rl.Auth.Limiter.Allow("ip").Limit != 0 {
			t.Fatal("zero auth rate did not become unlimited")
		}
	})
	t.Run("UpdateConfig/error", func(t *testing.T) {
		for _, failure := range []string{"invalid quotas", "invalid mixed fields", "save"} {
			t.Run(failure, func(t *testing.T) {
				t.Parallel()
				h, bw, rl := setup(t)
				previous := h.Settings.Snapshot()
				path := filepath.Join(h.ConfigDir, "settings.json")
				data, err := os.ReadFile(path) //nolint:gosec // G304: isolated test configuration file.
				if err != nil {
					t.Fatal(err)
				}
				req := &dto.UpdateServerConfigRequest{RateLimits: &dto.RateLimitsConfigUpdate{WriteRatePerMin: 19}}
				switch failure {
				case "invalid quotas":
					req.Quotas = &dto.QuotasConfigUpdate{}
				case "invalid mixed fields":
					req.Quotas = &dto.QuotasConfigUpdate{ResourceQuotas: dto.ResourceQuotas{MaxPages: 1, MaxStorageBytes: 1, MaxRecordsPerTable: 1, MaxAssetSizeBytes: 1, MaxTablesPerWorkspace: 1, MaxColumnsPerTable: 1}}
					req.RateLimits = &dto.RateLimitsConfigUpdate{WriteRatePerMin: -1}
				case "save":
					h.ConfigDir = path
				}
				if _, err := h.UpdateConfig(t.Context(), nil, req); err == nil {
					t.Fatal("invalid update succeeded")
				}
				if h.Settings.Snapshot() != previous {
					t.Fatal("failed update published a candidate")
				}
				after, err := os.ReadFile(path) //nolint:gosec // G304: isolated test configuration file.
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(data, after) {
					t.Fatal("failed update changed persisted settings")
				}
				if bw.Allow(1<<20) != 0 || rl.Write.Limiter.Allow("user").Limit != 60 {
					t.Fatal("failed update changed runtime controls")
				}
			})
		}
	})
}
