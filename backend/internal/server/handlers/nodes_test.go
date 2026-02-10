package handlers

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/maruel/mddb/backend/internal/rid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// testServices creates a Services struct with a FileStore for testing.
func testServices(t *testing.T) (*Services, rid.ID) {
	t.Helper()
	tmpDir := t.TempDir()

	gitMgr := git.NewManager(tmpDir, "test", "test@test.com")

	orgService, err := identity.NewOrganizationService(filepath.Join(tmpDir, "organizations.jsonl"))
	if err != nil {
		t.Fatalf("failed to create OrganizationService: %v", err)
	}

	wsService, err := identity.NewWorkspaceService(filepath.Join(tmpDir, "workspaces.jsonl"))
	if err != nil {
		t.Fatalf("failed to create WorkspaceService: %v", err)
	}

	// Create test org and workspace with high quotas
	org, err := orgService.Create(t.Context(), "Test Organization", "test@test.com")
	if err != nil {
		t.Fatalf("failed to create test organization: %v", err)
	}
	// Use Modify to set quotas, ignoring error as we control inputs
	_, _ = orgService.Modify(org.ID, func(o *identity.Organization) error {
		o.Quotas.MaxTotalStorageBytes = 1e18
		return nil
	})

	ws, err := wsService.Create(t.Context(), org.ID, "Test Workspace")
	if err != nil {
		t.Fatalf("failed to create test workspace: %v", err)
	}
	_, _ = wsService.Modify(ws.ID, func(w *identity.Workspace) error {
		w.Quotas.MaxStorageBytes = 1e12
		w.Quotas.MaxPages = 1000
		return nil
	})

	serverQuotas := storage.DefaultResourceQuotas()
	fs, err := content.NewFileStoreService(tmpDir, gitMgr, wsService, orgService, &serverQuotas)
	if err != nil {
		t.Fatalf("failed to create FileStoreService: %v", err)
	}

	return &Services{FileStore: fs}, ws.ID
}

func TestNodeHandler(t *testing.T) {
	t.Run("GetNodeVersion", func(t *testing.T) {
		t.Run("NestedNode", func(t *testing.T) {
			svc, wsID := testServices(t)
			ctx := t.Context()
			author := git.Author{Name: "Test", Email: "test@test.com"}

			if err := svc.FileStore.InitWorkspace(ctx, wsID); err != nil {
				t.Fatalf("failed to init workspace: %v", err)
			}

			wsStore, err := svc.FileStore.GetWorkspaceStore(ctx, wsID)
			if err != nil {
				t.Fatalf("failed to get workspace store: %v", err)
			}

			// Create parent
			// Note: 0 is used as rid.ID zero value for root parent
			var rootID rid.ID
			parent, err := wsStore.CreateNode(ctx, "Parent", content.NodeTypeDocument, rootID, author)
			if err != nil {
				t.Fatalf("failed to create parent: %v", err)
			}

			// Create child
			child, err := wsStore.CreatePageUnderParent(ctx, parent.ID, "Child", "child content", author)
			if err != nil {
				t.Fatalf("failed to create child: %v", err)
			}

			// Update child to generate more history (optional, but good)
			_, err = wsStore.UpdatePage(ctx, child.ID, "Child Updated", "child content v2", author)
			if err != nil {
				t.Fatalf("failed to update child: %v", err)
			}

			// Get history to find the latest commit hash
			history, err := wsStore.GetHistory(ctx, child.ID, 1)
			if err != nil {
				t.Fatalf("failed to get history: %v", err)
			}
			if len(history) == 0 {
				t.Fatal("expected history to not be empty")
			}
			latestHash := history[0].Hash

			// Setup Handler
			h := &NodeHandler{
				Svc: svc,
				Cfg: &Config{},
			}

			// Test GetNodeVersion with nested node
			req := &dto.GetNodeVersionRequest{
				WsID: wsID,
				ID:   child.ID,
				Hash: latestHash,
			}

			resp, err := h.GetNodeVersion(ctx, wsID, nil, req)
			if err != nil {
				t.Fatalf("GetNodeVersion failed: %v", err)
			}

			// Ensure the content matches the body only (front matter should be stripped)
			if resp.Content != "child content v2" {
				t.Errorf("expected content 'child content v2', got %q", resp.Content)
			}
			// Ensure it does NOT contain front matter delimiters
			if strings.Contains(resp.Content, "---") {
				t.Errorf("content should not contain front matter, got %q", resp.Content)
			}
		})
	})
}
