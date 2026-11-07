// Tests Notion import lifecycle status.

package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/notion"
	"github.com/maruel/mddb/backend/internal/storage/content"
)

func TestNotionImportHandler(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		svc, wsID := testServices(t)
		if err := svc.FileStore.InitWorkspace(t.Context(), wsID); err != nil {
			t.Fatal(err)
		}
		store, err := svc.FileStore.GetWorkspaceStore(t.Context(), wsID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ListNodeSummaries(t.Context(), nil, 0, 50); err != nil {
			t.Fatal(err)
		}
		id := ksid.NewID()
		writer := notion.NewWriter(svc.FileStore.RootDir(), wsID.String())
		if err := writer.WriteNode(&content.Node{ID: id, Title: "Imported", Type: content.NodeTypeDocument}, "body"); err != nil {
			t.Fatal(err)
		}
		h := NewNotionImportHandler(svc, &Config{})
		state := &importState{status: "running"}
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		t.Cleanup(cancel)
		h.runImport(ctx, wsID, "unused", state)
		fresh, err := svc.FileStore.GetWorkspaceStore(t.Context(), wsID)
		if err != nil {
			t.Fatal(err)
		}
		page, err := fresh.ListNodeSummaries(t.Context(), nil, 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, n := range page.Nodes {
			if n.ID == id {
				found = true
			}
		}
		if !found {
			t.Fatal("imported node missing after worker retirement")
		}
		if state.status != "failed" || state.message != "Import exceeded the 24-hour time limit" {
			t.Fatalf("deadline: got status %q, message %q", state.status, state.message)
		}
	})
}
