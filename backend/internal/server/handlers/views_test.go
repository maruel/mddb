// Tests for table views API handlers.

package handlers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func setupTestServices(t *testing.T) (*Services, ksid.ID) {
	dir, err := os.MkdirTemp("", "mddb-views-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Failed to remove temp dir: %v", err)
		}
	})

	if err := ksid.InitIDSlice(0, 1); err != nil {
		t.Fatal(err)
	}

	orgSvc, err := identity.NewOrganizationService(filepath.Join(dir, "orgs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	wsSvc, err := identity.NewWorkspaceService(filepath.Join(dir, "workspaces.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	gitMgr := git.NewManager(dir, "test", "test@test.com")
	quotas := storage.DefaultResourceQuotas()
	fileStore, err := content.NewFileStoreService(dir, gitMgr, wsSvc, orgSvc, &quotas)
	if err != nil {
		t.Fatal(err)
	}

	svc := &Services{
		Organization: orgSvc,
		Workspace:    wsSvc,
		FileStore:    fileStore,
	}

	ctx := t.Context()

	org, err := orgSvc.Create(ctx, "Test Org", "billing@example.com")
	if err != nil {
		t.Fatal(err)
	}

	ws, err := wsSvc.Create(ctx, org.ID, "Test Workspace")
	if err != nil {
		t.Fatal(err)
	}

	if err := fileStore.InitWorkspace(ctx, ws.ID); err != nil {
		t.Fatal(err)
	}

	return svc, ws.ID
}

func TestViewHandler(t *testing.T) {
	t.Run("CreateView", func(t *testing.T) {
		t.Run("validation", func(t *testing.T) {
			t.Run("invalid view type", func(t *testing.T) {
				req := &dto.CreateViewRequest{
					WsID:   ksid.NewID(),
					NodeID: ksid.NewID(),
					Name:   "Test View",
					Type:   "invalid_type",
				}
				if err := req.Validate(); err == nil {
					t.Error("Expected validation error for invalid view type")
				}
			})

			t.Run("valid view type", func(t *testing.T) {
				req := &dto.CreateViewRequest{
					WsID:   ksid.NewID(),
					NodeID: ksid.NewID(),
					Name:   "Test View",
					Type:   dto.ViewTypeTable,
				}
				if err := req.Validate(); err != nil {
					t.Errorf("Unexpected validation error: %v", err)
				}
			})
		})

		t.Run("valid", func(t *testing.T) {
			svc, wsID := setupTestServices(t)
			ctx := t.Context()
			user := &identity.User{Name: "Test User", Email: "test@example.com"}
			nodeID := createTestTable(t, svc, wsID)
			vh := &ViewHandler{Svc: svc}

			reqCreate := &dto.CreateViewRequest{
				WsID:   wsID,
				NodeID: nodeID,
				Name:   "My View",
				Type:   dto.ViewTypeTable,
			}
			respCreate, err := vh.CreateView(ctx, wsID, user, reqCreate)
			if err != nil {
				t.Fatalf("CreateView failed: %v", err)
			}
			if respCreate.ID.IsZero() {
				t.Fatal("CreateView returned zero ID")
			}

			ws, err := svc.FileStore.GetWorkspaceStore(ctx, wsID)
			if err != nil {
				t.Fatalf("GetWorkspaceStore failed: %v", err)
			}
			node, err := ws.ReadTable(nodeID)
			if err != nil {
				t.Fatalf("ReadTable failed: %v", err)
			}
			if len(node.Views) != 1 {
				t.Errorf("Expected 1 view, got %d", len(node.Views))
			}
			if node.Views[0].ID != respCreate.ID {
				t.Errorf("Expected view ID %v, got %v", respCreate.ID, node.Views[0].ID)
			}
		})
	})

	t.Run("UpdateView", func(t *testing.T) {
		svc, wsID := setupTestServices(t)
		ctx := t.Context()
		user := &identity.User{Name: "Test User", Email: "test@example.com"}
		nodeID := createTestTable(t, svc, wsID)
		vh := &ViewHandler{Svc: svc}

		respCreate, err := vh.CreateView(ctx, wsID, user, &dto.CreateViewRequest{
			WsID:   wsID,
			NodeID: nodeID,
			Name:   "My View",
			Type:   dto.ViewTypeTable,
		})
		if err != nil {
			t.Fatalf("CreateView failed: %v", err)
		}

		reqUpdate := &dto.UpdateViewRequest{
			WsID:   wsID,
			NodeID: nodeID,
			ViewID: respCreate.ID,
			Name:   "Updated View",
			Filters: []dto.Filter{
				{Property: "Age", Operator: dto.FilterOpGreaterThan, Value: float64(18)},
			},
		}
		if _, err := vh.UpdateView(ctx, wsID, user, reqUpdate); err != nil {
			t.Fatalf("UpdateView failed: %v", err)
		}

		ws, err := svc.FileStore.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("GetWorkspaceStore failed: %v", err)
		}
		node, err := ws.ReadTable(nodeID)
		if err != nil {
			t.Fatalf("ReadTable failed: %v", err)
		}
		if node.Views[0].Name != "Updated View" {
			t.Errorf("Expected name 'Updated View', got %q", node.Views[0].Name)
		}
		if len(node.Views[0].Filters) != 1 {
			t.Errorf("Expected 1 filter, got %d", len(node.Views[0].Filters))
		}
	})

	t.Run("DeleteView", func(t *testing.T) {
		svc, wsID := setupTestServices(t)
		ctx := t.Context()
		user := &identity.User{Name: "Test User", Email: "test@example.com"}
		nodeID := createTestTable(t, svc, wsID)
		vh := &ViewHandler{Svc: svc}

		respCreate, err := vh.CreateView(ctx, wsID, user, &dto.CreateViewRequest{
			WsID:   wsID,
			NodeID: nodeID,
			Name:   "My View",
			Type:   dto.ViewTypeTable,
		})
		if err != nil {
			t.Fatalf("CreateView failed: %v", err)
		}

		reqDelete := &dto.DeleteViewRequest{
			WsID:   wsID,
			NodeID: nodeID,
			ViewID: respCreate.ID,
		}
		if _, err := vh.DeleteView(ctx, wsID, user, reqDelete); err != nil {
			t.Fatalf("DeleteView failed: %v", err)
		}

		ws, err := svc.FileStore.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("GetWorkspaceStore failed: %v", err)
		}
		node, err := ws.ReadTable(nodeID)
		if err != nil {
			t.Fatalf("ReadTable failed: %v", err)
		}
		if len(node.Views) != 0 {
			t.Errorf("Expected 0 views, got %d", len(node.Views))
		}
	})
}
