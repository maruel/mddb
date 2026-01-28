package handlers

import (
	"testing"

	"github.com/maruel/mddb/backend/internal/server/dto"
)

func TestHealthHandler(t *testing.T) {
	t.Run("GetHealth", func(t *testing.T) {
		cfg := &Config{
			Version:   "1.0.0",
			GoVersion: "go1.24.0",
			Revision:  "abc1234",
			Dirty:     true,
		}
		handler := &HealthHandler{Cfg: cfg}
		resp, err := handler.GetHealth(t.Context(), &dto.HealthRequest{})
		if err != nil {
			t.Fatalf("GetHealth() error = %v", err)
		}
		if resp.Status != "ok" {
			t.Errorf("Status = %q, want %q", resp.Status, "ok")
		}
		if resp.Version != "1.0.0" {
			t.Errorf("Version = %q, want %q", resp.Version, "1.0.0")
		}
		if resp.GoVersion != "go1.24.0" {
			t.Errorf("GoVersion = %q, want %q", resp.GoVersion, "go1.24.0")
		}
		if resp.Revision != "abc1234" {
			t.Errorf("Revision = %q, want %q", resp.Revision, "abc1234")
		}
		if !resp.Dirty {
			t.Error("Dirty = false, want true")
		}
	})
}
