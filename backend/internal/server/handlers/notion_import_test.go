// Tests Notion import lifecycle status.

package handlers

import (
	"context"
	"testing"
	"time"
)

func TestNotionImportHandler(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		svc, wsID := testServices(t)
		h := NewNotionImportHandler(svc, &Config{})
		state := &importState{status: "running"}
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		t.Cleanup(cancel)
		h.runImport(ctx, wsID, "unused", state)
		if state.status != "failed" || state.message != "Import exceeded the 24-hour time limit" {
			t.Fatalf("deadline: got status %q, message %q", state.status, state.message)
		}
	})
}
