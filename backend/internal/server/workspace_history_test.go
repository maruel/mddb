// Tests saved workspace HTTP mutations, events, cache updates, and history health.

package server

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/sse"
)

func TestWorkspaceHistory(t *testing.T) {
	env := setupTestEnv(t)
	env.services.Broker = sse.NewBroker()
	var auth dto.AuthResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{Email: "workspace-history@example.com", Password: "password123", Name: "History"}, &auth, ""); status != http.StatusOK {
		t.Fatalf("register=%d", status)
	}
	var org dto.OrganizationResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/organizations", dto.CreateOrganizationRequest{Name: "Org"}, &org, auth.Token); status != http.StatusOK {
		t.Fatalf("org=%d", status)
	}
	var ws dto.WorkspaceResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/organizations/"+org.ID.String()+"/workspaces", dto.CreateWorkspaceRequest{Name: "Workspace"}, &ws, auth.Token); status != http.StatusOK {
		t.Fatalf("workspace=%d", status)
	}
	base := "/api/v1/workspaces/" + ws.ID.String() + "/nodes/"
	var target dto.CreatePageResponse
	if status := env.doJSON(t, http.MethodPost, base+"0/page/create", dto.CreatePageRequest{Title: "Target"}, &target, auth.Token); status != http.StatusOK {
		t.Fatalf("target=%d", status)
	}
	var source dto.CreatePageResponse
	if status := env.doJSON(t, http.MethodPost, base+"0/page/create", dto.CreatePageRequest{Title: "Source"}, &source, auth.Token); status != http.StatusOK {
		t.Fatalf("source=%d", status)
	}
	var table dto.CreateTableUnderParentResponse
	if status := env.doJSON(t, http.MethodPost, base+"0/table/create", dto.CreateTableRequest{Title: "Records"}, &table, auth.Token); status != http.StatusOK {
		t.Fatalf("table=%d", status)
	}
	store, err := env.fileStore.GetWorkspaceStore(t.Context(), ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetBacklinks(t.Context(), target.ID); err != nil {
		t.Fatal(err)
	}
	events, unsubscribe := env.services.Broker.SubscribeEvents(ws.ID)
	t.Cleanup(unsubscribe)
	// Submodule metadata is absorbed by the real production creation path.
	gitFile, err := os.ReadFile(filepath.Join(env.dataDir, ws.ID.String(), ".git"))
	if err != nil {
		t.Fatal(err)
	}
	gitDir := filepath.Clean(filepath.Join(env.dataDir, ws.ID.String(), strings.TrimSpace(strings.TrimPrefix(string(gitFile), "gitdir: "))))
	lock := filepath.Join(gitDir, "index.lock")
	if err := os.WriteFile(lock, []byte("test owns this lock"), 0o600); err != nil {
		t.Fatal(err)
	}
	content := "[Target](../" + target.ID.String() + "/index.md)"
	var updated dto.UpdatePageResponse
	if status := env.doJSON(t, http.MethodPost, base+source.ID.String()+"/page", dto.UpdatePageRequest{Title: "Saved", Content: content, ExpectedRevision: pageRevision(t, t.Context(), store, source.ID)}, &updated, auth.Token); status != http.StatusOK {
		t.Fatalf("saved page=%d", status)
	}
	select {
	case event := <-events:
		if event.Type != dto.EventNodeUpdated || event.NodeID != source.ID {
			t.Fatalf("page event=%+v", event)
		}
	default:
		t.Fatal("missing page event")
	}
	links, err := store.GetBacklinks(t.Context(), target.ID)
	if err != nil || len(links) != 1 || links[0].NodeID != source.ID {
		t.Fatalf("backlinks=%+v err=%v", links, err)
	}
	var record dto.CreateRecordResponse
	if status := env.doJSON(t, http.MethodPost, base+table.ID.String()+"/table/records/create", dto.CreateRecordRequest{Data: map[string]any{}}, &record, auth.Token); status != http.StatusOK {
		t.Fatalf("saved record=%d", status)
	}
	select {
	case event := <-events:
		if event.Type != dto.EventRecordsChanged || event.RecordID != record.ID {
			t.Fatalf("record event=%+v", event)
		}
	default:
		t.Fatal("missing record event")
	}
	var read dto.NodeResponse
	if status := env.doJSON(t, http.MethodGet, base+source.ID.String(), nil, &read, auth.Token); status != http.StatusOK || read.Content != content {
		t.Fatalf("saved read=%d %+v", status, read)
	}
	var failure dto.ErrorResponse
	if status := env.doJSON(t, http.MethodGet, "/api/v1/health", nil, &failure, ""); status != http.StatusServiceUnavailable || failure.Error.Code != dto.ErrorCodeStorageError {
		t.Fatalf("pending health=%d %+v", status, failure)
	}
	if failure.Error.Message != "Storage history is pending recovery" {
		t.Fatalf("unsanitized health=%+v", failure)
	}
	env.fileStore.InvalidateAllStores()
	if !env.fileStore.HasPendingHistory() {
		t.Fatal("cache invalidation lost pending history")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	// Push must reconcile saved history before it can report remote success.
	// A missing remote is expected here; it must still leave real local history.
	if err := store.Repo().Push(t.Context(), "missing", ""); err == nil {
		t.Fatal("missing remote unexpectedly succeeded")
	}
	if env.fileStore.HasPendingHistory() {
		t.Fatal("local history did not recover before push")
	}
	for _, path := range []string{source.ID.String() + "/index.md", table.ID.String() + "/data.jsonl"} {
		saved, err := os.ReadFile(filepath.Join(env.dataDir, ws.ID.String(), path)) //nolint:gosec // G304: saved file in the test-owned workspace.
		if err != nil {
			t.Fatal(err)
		}
		recorded, err := store.Repo().GetFileAtCommit(t.Context(), "HEAD", path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(saved, recorded) {
			t.Fatalf("history did not record current saved snapshot for %s", path)
		}
	}

	var health dto.HealthResponse
	if status := env.doJSON(t, http.MethodGet, "/api/v1/health", nil, &health, ""); status != http.StatusOK {
		t.Fatalf("recovered health=%d", status)
	}
	select {
	case event := <-events:
		t.Fatalf("recovery replayed mutation: %+v", event)
	default:
	}
	var records dto.ListRecordsResponse
	if status := env.doJSON(t, http.MethodGet, base+table.ID.String()+"/table/records", nil, &records, auth.Token); status != http.StatusOK || len(records.Records) != 1 || records.Records[0].ID != record.ID {
		t.Fatalf("saved records=%d %+v", status, records)
	}
}
