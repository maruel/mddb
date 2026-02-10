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
	t.Helper()
	dir, err := os.MkdirTemp("", "mddb-views-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("Failed to remove temp dir: %v", err)
		}
	})

	// Initialize ID generation for testing
	if err := ksid.InitIDSlice(0, 1); err != nil {
		t.Fatal(err)
	}

	// Create identity services
	orgSvc, err := identity.NewOrganizationService(filepath.Join(dir, "orgs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	wsSvc, err := identity.NewWorkspaceService(filepath.Join(dir, "workspaces.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	// Create git manager
	gitMgr := git.NewManager(dir, "Test", "test@example.com")

	// Create FileStoreService
	serverQuotas := storage.DefaultResourceQuotas()
	fileStore, err := content.NewFileStoreService(dir, gitMgr, wsSvc, orgSvc, &serverQuotas)
	if err != nil {
		t.Fatal(err)
	}

	svc := &Services{
		FileStore:    fileStore,
		Organization: orgSvc,
		Workspace:    wsSvc,
	}

	ctx := t.Context()

	// Create a test organization
	org, err := orgSvc.Create(ctx, "Test Org", "billing@example.com")
	if err != nil {
		t.Fatal(err)
	}

	// Create a test workspace
	ws, err := wsSvc.Create(ctx, org.ID, "Test Workspace")
	if err != nil {
		t.Fatal(err)
	}

	// Initialize workspace files (creates .git, AGENTS.md, pages dir)
	if err := fileStore.InitWorkspace(ctx, ws.ID); err != nil {
		t.Fatal(err)
	}

	return svc, ws.ID
}

func createTestTable(t *testing.T, svc *Services, wsID ksid.ID) ksid.ID {
	t.Helper()
	ctx := t.Context()
	ws, err := svc.FileStore.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		t.Fatal(err)
	}

	author := git.Author{Name: "Test User", Email: "test@example.com"}
	props := []content.Property{
		{Name: "Name", Type: content.PropertyTypeText},
		{Name: "Age", Type: content.PropertyTypeNumber},
	}

	node, err := ws.CreateTableUnderParent(ctx, 0, "Test Table", props, author)
	if err != nil {
		t.Fatal(err)
	}
	return node.ID
}

func TestViewCRUD(t *testing.T) {
	svc, wsID := setupTestServices(t)

	ctx := t.Context()
	user := &identity.User{Name: "Test User", Email: "test@example.com"}
	nodeID := createTestTable(t, svc, wsID)

	vh := &ViewHandler{Svc: svc}

	// 1. Create View
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
	viewID := respCreate.ID

	// Verify view exists
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
	if node.Views[0].ID != viewID {
		t.Errorf("Expected view ID %v, got %v", viewID, node.Views[0].ID)
	}

	// 2. Update View
	reqUpdate := &dto.UpdateViewRequest{
		WsID:   wsID,
		NodeID: nodeID,
		ViewID: viewID,
		Name:   "Updated View",
		Filters: []dto.Filter{
			{Property: "Age", Operator: dto.FilterOpGreaterThan, Value: float64(18)},
		},
	}
	_, err = vh.UpdateView(ctx, wsID, user, reqUpdate)
	if err != nil {
		t.Fatalf("UpdateView failed: %v", err)
	}

	// Verify update
	node, err = ws.ReadTable(nodeID)
	if err != nil {
		t.Fatalf("ReadTable failed: %v", err)
	}
	if node.Views[0].Name != "Updated View" {
		t.Errorf("Expected name 'Updated View', got %q", node.Views[0].Name)
	}
	if len(node.Views[0].Filters) != 1 {
		t.Errorf("Expected 1 filter, got %d", len(node.Views[0].Filters))
	}

	// 3. ListRecords with View
	// First add data
	record1 := &content.DataRecord{ID: ksid.NewID(), Data: map[string]any{"Name": "Alice", "Age": float64(25)}}
	record2 := &content.DataRecord{ID: ksid.NewID(), Data: map[string]any{"Name": "Bob", "Age": float64(10)}}
	author := git.Author{Name: user.Name, Email: user.Email}
	if err := ws.AppendRecord(ctx, nodeID, record1, author); err != nil {
		t.Fatalf("AppendRecord failed: %v", err)
	}
	if err := ws.AppendRecord(ctx, nodeID, record2, author); err != nil {
		t.Fatalf("AppendRecord failed: %v", err)
	}

	nh := &NodeHandler{Svc: svc}

	// Test ListRecords with ViewID (should filter out Bob)
	listReq := &dto.ListRecordsRequest{
		WsID:   wsID,
		ID:     nodeID,
		ViewID: viewID,
		Limit:  100,
	}
	listResp, err := nh.ListRecords(ctx, wsID, user, listReq)
	if err != nil {
		t.Fatalf("ListRecords failed: %v", err)
	}
	if len(listResp.Records) != 1 {
		t.Errorf("Expected 1 record, got %d", len(listResp.Records))
	} else if listResp.Records[0].Data["Name"] != "Alice" {
		t.Errorf("Expected Alice, got %v", listResp.Records[0].Data["Name"])
	}

	// Test ListRecords with ad-hoc Filter (override view? no, we append? implementation says override if present)
	// Actually implementation:
	// if req.Filters != "" { filters = ... } (overwrites whatever was set from ViewID)
	// So ad-hoc replaces view filters.

	// Filter for Age < 15 (should match Bob)
	listReqAdHoc := &dto.ListRecordsRequest{
		WsID:    wsID,
		ID:      nodeID,
		Filters: `[{"property":"Age","operator":"lt","value":15}]`,
		Limit:   100,
	}
	listRespAdHoc, err := nh.ListRecords(ctx, wsID, user, listReqAdHoc)
	if err != nil {
		t.Fatalf("ListRecords (ad-hoc) failed: %v", err)
	}
	if len(listRespAdHoc.Records) != 1 {
		t.Errorf("Expected 1 record, got %d", len(listRespAdHoc.Records))
	} else if listRespAdHoc.Records[0].Data["Name"] != "Bob" {
		t.Errorf("Expected Bob, got %v", listRespAdHoc.Records[0].Data["Name"])
	}

	// 4. Delete View
	reqDelete := &dto.DeleteViewRequest{
		WsID:   wsID,
		NodeID: nodeID,
		ViewID: viewID,
	}
	_, err = vh.DeleteView(ctx, wsID, user, reqDelete)
	if err != nil {
		t.Fatalf("DeleteView failed: %v", err)
	}

	// Verify deletion
	node, err = ws.ReadTable(nodeID)
	if err != nil {
		t.Fatalf("ReadTable failed: %v", err)
	}
	if len(node.Views) != 0 {
		t.Errorf("Expected 0 views, got %d", len(node.Views))
	}
}

func TestListRecordsValidation(t *testing.T) {
	svc, wsID := setupTestServices(t)

	ctx := t.Context()
	user := &identity.User{Name: "Test User", Email: "test@example.com"}
	nodeID := createTestTable(t, svc, wsID)

	nh := &NodeHandler{Svc: svc}

	t.Run("InvalidViewID", func(t *testing.T) {
		req := &dto.ListRecordsRequest{
			WsID:   wsID,
			ID:     nodeID,
			ViewID: ksid.NewID(), // Non-existent view
			Limit:  100,
		}
		_, err := nh.ListRecords(ctx, wsID, user, req)
		if err == nil {
			t.Error("Expected error for non-existent view ID")
		}
	})

	t.Run("MalformedFiltersJSON", func(t *testing.T) {
		req := &dto.ListRecordsRequest{
			WsID:    wsID,
			ID:      nodeID,
			Filters: `{invalid json`,
			Limit:   100,
		}
		_, err := nh.ListRecords(ctx, wsID, user, req)
		if err == nil {
			t.Error("Expected error for malformed filters JSON")
		}
	})

	t.Run("MalformedSortsJSON", func(t *testing.T) {
		req := &dto.ListRecordsRequest{
			WsID:  wsID,
			ID:    nodeID,
			Sorts: `not valid json`,
			Limit: 100,
		}
		_, err := nh.ListRecords(ctx, wsID, user, req)
		if err == nil {
			t.Error("Expected error for malformed sorts JSON")
		}
	})

	t.Run("InvalidFilterOperator", func(t *testing.T) {
		req := &dto.ListRecordsRequest{
			WsID:    wsID,
			ID:      nodeID,
			Filters: `[{"property":"Name","operator":"invalid_op","value":"test"}]`,
			Limit:   100,
		}
		_, err := nh.ListRecords(ctx, wsID, user, req)
		if err == nil {
			t.Error("Expected error for invalid filter operator")
		}
	})

	t.Run("UnknownFilterProperty", func(t *testing.T) {
		req := &dto.ListRecordsRequest{
			WsID:    wsID,
			ID:      nodeID,
			Filters: `[{"property":"NonExistent","operator":"equals","value":"test"}]`,
			Limit:   100,
		}
		_, err := nh.ListRecords(ctx, wsID, user, req)
		if err == nil {
			t.Error("Expected error for unknown filter property")
		}
	})

	t.Run("UnknownSortProperty", func(t *testing.T) {
		req := &dto.ListRecordsRequest{
			WsID:  wsID,
			ID:    nodeID,
			Sorts: `[{"property":"NonExistent","direction":"asc"}]`,
			Limit: 100,
		}
		_, err := nh.ListRecords(ctx, wsID, user, req)
		if err == nil {
			t.Error("Expected error for unknown sort property")
		}
	})

	t.Run("InvalidSortDirection", func(t *testing.T) {
		req := &dto.ListRecordsRequest{
			WsID:  wsID,
			ID:    nodeID,
			Sorts: `[{"property":"Name","direction":"invalid"}]`,
			Limit: 100,
		}
		_, err := nh.ListRecords(ctx, wsID, user, req)
		if err == nil {
			t.Error("Expected error for invalid sort direction")
		}
	})

	t.Run("DuplicateFilterProperty", func(t *testing.T) {
		req := &dto.ListRecordsRequest{
			WsID:    wsID,
			ID:      nodeID,
			Filters: `[{"property":"Name","operator":"equals","value":"a"},{"property":"Name","operator":"contains","value":"b"}]`,
			Limit:   100,
		}
		_, err := nh.ListRecords(ctx, wsID, user, req)
		if err == nil {
			t.Error("Expected error for duplicate filter property")
		}
	})

	t.Run("DuplicateSortProperty", func(t *testing.T) {
		req := &dto.ListRecordsRequest{
			WsID:  wsID,
			ID:    nodeID,
			Sorts: `[{"property":"Name","direction":"asc"},{"property":"Name","direction":"desc"}]`,
			Limit: 100,
		}
		_, err := nh.ListRecords(ctx, wsID, user, req)
		if err == nil {
			t.Error("Expected error for duplicate sort property")
		}
	})
}

func TestCreateViewValidation(t *testing.T) {
	t.Run("InvalidViewType", func(t *testing.T) {
		req := &dto.CreateViewRequest{
			WsID:   ksid.NewID(),
			NodeID: ksid.NewID(),
			Name:   "Test View",
			Type:   "invalid_type",
		}
		err := req.Validate()
		if err == nil {
			t.Error("Expected validation error for invalid view type")
		}
	})

	t.Run("ValidViewType", func(t *testing.T) {
		req := &dto.CreateViewRequest{
			WsID:   ksid.NewID(),
			NodeID: ksid.NewID(),
			Name:   "Test View",
			Type:   dto.ViewTypeTable,
		}
		err := req.Validate()
		if err != nil {
			t.Errorf("Unexpected validation error: %v", err)
		}
	})
}
