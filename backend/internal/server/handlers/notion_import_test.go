// Tests Notion import retirement and generation-safe replacement of cached document snapshots.

package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/notion"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
)

func TestNotionImportHandler(t *testing.T) {
	t.Run("cached page replacement", func(t *testing.T) {
		svc, wsID := testServices(t)
		if err := svc.FileStore.InitWorkspace(t.Context(), wsID); err != nil {
			t.Fatal(err)
		}
		store, err := svc.FileStore.GetWorkspaceStore(t.Context(), wsID)
		if err != nil {
			t.Fatal(err)
		}
		node, err := store.CreatePageUnderParent(t.Context(), 0, "Original", "old body", git.Author{})
		if err != nil {
			t.Fatal(err)
		}
		before, err := store.ReadPageSnapshot(t.Context(), node.ID, 0, 4096)
		if err != nil {
			t.Fatal(err)
		}
		writer := notion.NewWriter(svc.FileStore.RootDir(), wsID.String(), store.ImportTx)
		node.Title = "Imported"
		if err := writer.WriteNode(t.Context(), node, "new body"); err != nil {
			t.Fatal(err)
		}
		after, err := store.ReadPageSnapshot(t.Context(), node.ID, 0, 4096)
		if err != nil || after.Revision == before.Revision || after.Node.Title != `"Imported"` || after.Node.Content != "new body" {
			t.Fatalf("cached import snapshot: %+v, %v", after, err)
		}
		// The markdown replacement succeeds before a malformed metadata destination
		// fails. Both the imported bytes and invalidation survive this partial error.
		metadata := filepath.Join(svc.FileStore.RootDir(), wsID.String(), node.ID.String(), "metadata.json")
		if err := os.Mkdir(metadata, 0o700); err != nil {
			t.Fatal(err)
		}
		node.Type, node.Title = content.NodeTypeHybrid, "Partial"
		node.Views = []content.View{{Name: "View"}}
		if err := writer.WriteNode(t.Context(), node, "partial body"); err == nil {
			t.Fatal("expected metadata destination failure")
		}
		if err := os.Remove(metadata); err != nil {
			t.Fatal(err)
		}
		partial, err := store.ReadPageSnapshot(t.Context(), node.ID, 0, 4096)
		if err != nil || partial.Revision == after.Revision || partial.Node.Title != `"Partial"` || partial.Node.Content != "partial body" {
			t.Fatalf("partly failed import retained stale revision: %+v, %v", partial, err)
		}
	})
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
		writer := notion.NewWriter(svc.FileStore.RootDir(), wsID.String(), nil)
		if err := writer.WriteNode(t.Context(), &content.Node{ID: id, Title: "Imported", Type: content.NodeTypeDocument}, "body"); err != nil {
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
