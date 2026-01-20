package handlers

import (
	"context"
	"testing"

	"github.com/maruel/mddb/backend/internal/server/dto"
)

func TestNewHealthHandler(t *testing.T) {
	handler := NewHealthHandler("1.0.0")
	if handler == nil {
		t.Fatal("NewHealthHandler returned nil")
	}
	if handler.version != "1.0.0" {
		t.Errorf("version = %q, want %q", handler.version, "1.0.0")
	}
}

func TestHealthHandler_Health(t *testing.T) {
	tests := []struct {
		name           string
		version        string
		expectedStatus string
	}{
		{
			name:           "basic health check",
			version:        "1.0.0",
			expectedStatus: "ok",
		},
		{
			name:           "dev version",
			version:        "dev",
			expectedStatus: "ok",
		},
		{
			name:           "empty version",
			version:        "",
			expectedStatus: "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHealthHandler(tt.version)
			resp, err := handler.Health(context.Background(), dto.HealthRequest{})

			if err != nil {
				t.Fatalf("Health() error = %v", err)
			}
			if resp == nil {
				t.Fatal("Health() returned nil response")
			}
			if resp.Status != tt.expectedStatus {
				t.Errorf("Status = %q, want %q", resp.Status, tt.expectedStatus)
			}
			if resp.Version != tt.version {
				t.Errorf("Version = %q, want %q", resp.Version, tt.version)
			}
		})
	}
}
