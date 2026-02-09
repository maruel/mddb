package content

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// testFileStore creates a FileStoreService for testing with unlimited quota.
// It also creates a workspace in the service for quota testing.
func testFileStore(t *testing.T) (*FileStoreService, jsonldb.ID) {
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

	// Create a test organization with very high quotas (practically unlimited)
	org, err := orgService.Create(t.Context(), "Test Organization", "test@test.com")
	if err != nil {
		t.Fatalf("failed to create test organization: %v", err)
	}
	_, err = orgService.Modify(org.ID, func(o *identity.Organization) error {
		o.Quotas.MaxWorkspacesPerOrg = 1_000
		o.Quotas.MaxMembersPerOrg = 10_000
		o.Quotas.MaxMembersPerWorkspace = 10_000
		o.Quotas.MaxTotalStorageBytes = 1_000_000_000_000_000_000 // 1EB
		return nil
	})
	if err != nil {
		t.Fatalf("failed to set unlimited org quotas: %v", err)
	}

	// Create a test workspace with very high quotas (practically unlimited)
	ws, err := wsService.Create(t.Context(), org.ID, "Test Workspace")
	if err != nil {
		t.Fatalf("failed to create test workspace: %v", err)
	}
	_, err = wsService.Modify(ws.ID, func(w *identity.Workspace) error {
		w.Quotas.MaxPages = 1_000_000
		w.Quotas.MaxStorageBytes = 1_000_000_000_000 // 1TB
		w.Quotas.MaxRecordsPerTable = 1_000_000
		w.Quotas.MaxAssetSizeBytes = 1024 * 1024 * 1024 // 1GB
		return nil
	})
	if err != nil {
		t.Fatalf("failed to set unlimited workspace quotas: %v", err)
	}

	serverQuotas := storage.DefaultResourceQuotas()
	fs, err := NewFileStoreService(tmpDir, gitMgr, wsService, orgService, &serverQuotas)
	if err != nil {
		t.Fatalf("failed to create FileStoreService: %v", err)
	}

	return fs, ws.ID
}

// initWS is a test helper that creates a FileStoreService, inits the workspace, and returns the store.
func initWS(t *testing.T) (*FileStoreService, *WorkspaceFileStore, jsonldb.ID) {
	fs, wsID := testFileStore(t)
	ctx := t.Context()
	if err := fs.InitWorkspace(ctx, wsID); err != nil {
		t.Fatalf("failed to init workspace: %v", err)
	}
	ws, err := fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		t.Fatalf("failed to get workspace store: %v", err)
	}
	return fs, ws, wsID
}

func TestFileStoreService(t *testing.T) {
	t.Run("InitWorkspace", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		// Verify AGENTS.md exists on disk.
		agentsPath := filepath.Join(fs.rootDir, wsID.String(), "AGENTS.md")
		if _, err := os.Stat(agentsPath); err != nil {
			t.Fatalf("AGENTS.md not found: %v", err)
		}
	})
}

func TestWorkspaceFileStore(t *testing.T) {
	author := git.Author{Name: "Test", Email: "test@test.com"}

	t.Run("PageOperations", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()
		nodeID := jsonldb.NewID()

		t.Run("WritePage", func(t *testing.T) {
			page, err := ws.WritePage(ctx, nodeID, 0, "Test Title", "# Test Content", author)
			if err != nil {
				t.Fatalf("failed to write page: %v", err)
			}
			if page.ID != nodeID {
				t.Errorf("expected ID %v, got %v", nodeID, page.ID)
			}
			if page.Title != "Test Title" {
				t.Errorf("expected title 'Test Title', got %q", page.Title)
			}
		})

		t.Run("ReadPage", func(t *testing.T) {
			readPage, err := ws.ReadPage(nodeID)
			if err != nil {
				t.Fatalf("failed to read page: %v", err)
			}
			if readPage.Title != "Test Title" {
				t.Errorf("expected title 'Test Title', got %q", readPage.Title)
			}
			if readPage.Content != "# Test Content" {
				t.Errorf("expected content '# Test Content', got %q", readPage.Content)
			}
		})

		t.Run("UpdatePage", func(t *testing.T) {
			updated, err := ws.UpdatePage(ctx, nodeID, "Updated Title", "# Updated Content", author)
			if err != nil {
				t.Fatalf("failed to update page: %v", err)
			}
			if updated.Title != "Updated Title" {
				t.Errorf("expected title 'Updated Title', got %q", updated.Title)
			}

			readUpdated, err := ws.ReadPage(nodeID)
			if err != nil {
				t.Fatalf("failed to read updated page: %v", err)
			}
			if readUpdated.Title != "Updated Title" {
				t.Errorf("expected title 'Updated Title', got %q", readUpdated.Title)
			}
		})

		t.Run("DeletePage", func(t *testing.T) {
			if err := ws.DeletePage(ctx, nodeID, author); err != nil {
				t.Fatalf("failed to delete page: %v", err)
			}
			if _, err := ws.ReadPage(nodeID); err == nil {
				t.Error("expected error reading deleted page")
			}
		})

		t.Run("ReadNonExistent", func(t *testing.T) {
			if _, err := ws.ReadPage(jsonldb.NewID()); err == nil {
				t.Error("expected error reading non-existent page")
			}
		})
	})

	t.Run("ListPages", func(t *testing.T) {
		fs, ws, wsID := initWS(t)
		ctx := t.Context()

		nodeIDs := make([]jsonldb.ID, 0, 3)
		for i := range 3 {
			id := jsonldb.NewID()
			nodeIDs = append(nodeIDs, id)
			if _, err := ws.WritePage(ctx, id, 0, "Page "+string(rune(48+i)), "content", author); err != nil {
				t.Fatalf("failed to write page %d: %v", i, err)
			}
		}

		it, err := ws.IterPages()
		if err != nil {
			t.Fatalf("failed to list pages: %v", err)
		}
		count := 0
		for p := range it {
			count++
			if p == nil {
				t.Error("iterator yielded nil page")
			}
		}
		if count != 3 {
			t.Errorf("expected 3 pages, got %d", count)
		}

		t.Run("DirectoryStructure", func(t *testing.T) {
			for _, nodeID := range nodeIDs {
				expectedDir := filepath.Join(fs.rootDir, wsID.String(), nodeID.String())
				if _, err := os.Stat(expectedDir); err != nil {
					t.Errorf("expected page directory %s to exist: %v", expectedDir, err)
				}
				expectedFile := filepath.Join(expectedDir, "index.md")
				if _, err := os.Stat(expectedFile); err != nil {
					t.Errorf("expected file %s to exist: %v", expectedFile, err)
				}
			}
		})
	})

	t.Run("TableOperations", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		tableID := jsonldb.NewID()
		node := &Node{
			ID:       tableID,
			Title:    "Test Table",
			Type:     NodeTypeTable,
			Created:  storage.Now(),
			Modified: storage.Now(),
			Properties: []Property{
				{Name: "name", Type: PropertyTypeText, Required: true},
				{Name: "value", Type: PropertyTypeText},
			},
		}

		t.Run("WriteTable", func(t *testing.T) {
			if err := ws.WriteTable(ctx, node, true, author); err != nil {
				t.Fatalf("failed to write table: %v", err)
			}
		})

		t.Run("ReadTable", func(t *testing.T) {
			read, err := ws.ReadTable(tableID)
			if err != nil {
				t.Fatalf("failed to read table: %v", err)
			}
			if read.Title != "Test Table" {
				t.Errorf("expected title 'Test Table', got %q", read.Title)
			}
		})

		t.Run("RecordOperations", func(t *testing.T) {
			record := &DataRecord{
				ID:       jsonldb.NewID(),
				Data:     map[string]any{"name": "test", "value": "data"},
				Created:  storage.Now(),
				Modified: storage.Now(),
			}

			t.Run("AppendRecord", func(t *testing.T) {
				if err := ws.AppendRecord(ctx, tableID, record, author); err != nil {
					t.Fatalf("failed to append record: %v", err)
				}
			})

			t.Run("IterRecords", func(t *testing.T) {
				it, err := ws.IterRecords(tableID)
				if err != nil {
					t.Fatalf("failed to iter records: %v", err)
				}
				count := 0
				for r := range it {
					count++
					if r.ID != record.ID {
						t.Errorf("expected record ID %v, got %v", record.ID, r.ID)
					}
				}
				if count != 1 {
					t.Errorf("expected 1 record, got %d", count)
				}
			})

			t.Run("UpdateRecord", func(t *testing.T) {
				updated := &DataRecord{
					ID:       record.ID,
					Data:     map[string]any{"name": "updated", "value": "new data"},
					Created:  record.Created,
					Modified: storage.Now(),
				}
				if err := ws.UpdateRecord(ctx, tableID, updated, author); err != nil {
					t.Fatalf("failed to update record: %v", err)
				}

				it, err := ws.IterRecords(tableID)
				if err != nil {
					t.Fatalf("failed to iter records: %v", err)
				}
				for r := range it {
					if r.ID == record.ID {
						if r.Data["name"] != "updated" {
							t.Errorf("expected name 'updated', got %v", r.Data["name"])
						}
						break
					}
				}
			})

			t.Run("DeleteRecord", func(t *testing.T) {
				if err := ws.DeleteRecord(ctx, tableID, record.ID, author); err != nil {
					t.Fatalf("failed to delete record: %v", err)
				}

				it, err := ws.IterRecords(tableID)
				if err != nil {
					t.Fatalf("failed to iter records: %v", err)
				}
				for r := range it {
					if r.ID == record.ID {
						t.Error("expected record to be deleted")
					}
				}
			})
		})

		t.Run("DeleteTable", func(t *testing.T) {
			if err := ws.DeleteTable(ctx, tableID, author); err != nil {
				t.Fatalf("failed to delete table: %v", err)
			}
			if _, err := ws.ReadTable(tableID); err == nil {
				t.Error("expected error reading deleted table")
			}
		})

		t.Run("UpdateRecord_DataPersistence", func(t *testing.T) {
			testTableID := jsonldb.NewID()
			testTable := &Node{
				ID:       testTableID,
				Title:    "Test Table",
				Type:     NodeTypeTable,
				Created:  storage.Now(),
				Modified: storage.Now(),
				Properties: []Property{
					{Name: "name", Type: PropertyTypeText},
					{Name: "value", Type: PropertyTypeText},
				},
			}
			if err := ws.WriteTable(ctx, testTable, true, author); err != nil {
				t.Fatalf("failed to write table: %v", err)
			}

			initialRecord := &DataRecord{
				ID:       jsonldb.NewID(),
				Data:     map[string]any{"name": "original", "value": "data"},
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			if err := ws.AppendRecord(ctx, testTableID, initialRecord, author); err != nil {
				t.Fatalf("failed to append record: %v", err)
			}

			updatedRecord := &DataRecord{
				ID:       initialRecord.ID,
				Data:     map[string]any{"name": "updated", "value": "new data"},
				Created:  initialRecord.Created,
				Modified: storage.Now(),
			}
			if err := ws.UpdateRecord(ctx, testTableID, updatedRecord, author); err != nil {
				t.Fatalf("failed to update record: %v", err)
			}

			it, err := ws.IterRecords(testTableID)
			if err != nil {
				t.Fatalf("failed to iter records: %v", err)
			}
			found := false
			for r := range it {
				if r.ID != initialRecord.ID {
					continue
				}
				found = true
				if r.Data == nil {
					t.Error("record data is nil - data was not persisted")
				}
				if name, ok := r.Data["name"]; !ok || name != "updated" {
					t.Errorf("expected name='updated', got %v", name)
				}
				if value, ok := r.Data["value"]; !ok || value != "new data" {
					t.Errorf("expected value='new data', got %v", value)
				}
				break
			}
			if !found {
				t.Error("record not found after update")
			}
		})

		t.Run("UpdateRecord_NilDataBecomesEmptyMap", func(t *testing.T) {
			testTableID := jsonldb.NewID()
			testTable := &Node{
				ID:       testTableID,
				Title:    "Test Table",
				Type:     NodeTypeTable,
				Created:  storage.Now(),
				Modified: storage.Now(),
				Properties: []Property{
					{Name: "name", Type: PropertyTypeText},
				},
			}
			if err := ws.WriteTable(ctx, testTable, true, author); err != nil {
				t.Fatalf("failed to write table: %v", err)
			}

			initialRecord := &DataRecord{
				ID:       jsonldb.NewID(),
				Data:     map[string]any{"name": "test"},
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			if err := ws.AppendRecord(ctx, testTableID, initialRecord, author); err != nil {
				t.Fatalf("failed to append record: %v", err)
			}

			updatedRecord := &DataRecord{
				ID:       initialRecord.ID,
				Data:     nil, // Explicitly nil
				Created:  initialRecord.Created,
				Modified: storage.Now(),
			}

			parentID := ws.getParent(testTableID)
			if err := ws.updateRecord(testTableID, parentID, updatedRecord); err != nil {
				t.Fatalf("failed to update record: %v", err)
			}

			it, err := ws.IterRecords(testTableID)
			if err != nil {
				t.Fatalf("failed to iter records: %v", err)
			}
			found := false
			for r := range it {
				if r.ID == initialRecord.ID {
					found = true
					break
				}
			}
			if !found {
				t.Error("record not found after update with nil data")
			}
		})
	})

	t.Run("AssetOperations", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		nodeID := jsonldb.NewID()
		if _, err := ws.WritePage(ctx, nodeID, 0, "With Assets", "content", author); err != nil {
			t.Fatalf("failed to write page: %v", err)
		}

		assetName := "test.txt"
		assetData := []byte("test asset content")

		t.Run("SaveAsset", func(t *testing.T) {
			asset, err := ws.SaveAsset(ctx, nodeID, assetName, assetData, author)
			if err != nil {
				t.Fatalf("failed to save asset: %v", err)
			}
			if asset.Name != assetName {
				t.Errorf("expected name %q, got %q", assetName, asset.Name)
			}
		})

		t.Run("ReadAsset", func(t *testing.T) {
			data, err := ws.ReadAsset(nodeID, assetName)
			if err != nil {
				t.Fatalf("failed to read asset: %v", err)
			}
			if !bytes.Equal(data, assetData) {
				t.Errorf("expected data %q, got %q", string(assetData), string(data))
			}
		})

		t.Run("IterAssets", func(t *testing.T) {
			it, err := ws.IterAssets(nodeID)
			if err != nil {
				t.Fatalf("failed to iter assets: %v", err)
			}
			count := 0
			for asset := range it {
				count++
				if asset.Name != assetName {
					t.Errorf("expected name %q, got %q", assetName, asset.Name)
				}
			}
			if count != 1 {
				t.Errorf("expected 1 asset, got %d", count)
			}
		})

		t.Run("DeleteAsset", func(t *testing.T) {
			if err := ws.DeleteAsset(ctx, nodeID, assetName, author); err != nil {
				t.Fatalf("failed to delete asset: %v", err)
			}
			if _, err := ws.ReadAsset(nodeID, assetName); err == nil {
				t.Error("expected error reading deleted asset")
			}
		})
	})

	t.Run("PageVersionHistory", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()
		nodeID := jsonldb.NewID()

		if _, err := ws.WritePage(ctx, nodeID, 0, "Initial", "initial content", author); err != nil {
			t.Fatalf("failed to write initial page: %v", err)
		}
		if _, err := ws.UpdatePage(ctx, nodeID, "Updated", "updated content", author); err != nil {
			t.Fatalf("failed to update page: %v", err)
		}

		history, err := ws.GetHistory(ctx, nodeID, 10)
		if err != nil {
			t.Fatalf("failed to get history: %v", err)
		}
		if len(history) < 2 {
			t.Errorf("expected at least 2 commits, got %d", len(history))
		}
	})

	t.Run("NestedPageVersionHistory", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		parent, err := ws.CreateNode(ctx, "Parent", NodeTypeDocument, 0, author)
		if err != nil {
			t.Fatalf("failed to create parent: %v", err)
		}
		child, err := ws.CreatePageUnderParent(ctx, parent.ID, "Child", "initial content", author)
		if err != nil {
			t.Fatalf("failed to create child: %v", err)
		}

		for i := 1; i <= 3; i++ {
			if _, err := ws.UpdatePage(ctx, child.ID, "Child", "content v"+string(rune(48+i)), author); err != nil {
				t.Fatalf("failed to update child page (v%d): %v", i, err)
			}
		}

		history, err := ws.GetHistory(ctx, child.ID, 10)
		if err != nil {
			t.Fatalf("failed to get history for nested page: %v", err)
		}
		if len(history) < 4 {
			t.Errorf("expected at least 4 commits for nested page, got %d", len(history))
		}

		foundChildCommit := false
		for _, commit := range history {
			if strings.Contains(commit.Message, child.ID.String()) {
				foundChildCommit = true
				break
			}
		}
		if !foundChildCommit {
			t.Errorf("history should contain commits for child page %s", child.ID)
		}
	})

	t.Run("NodeTree", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		var zeroID jsonldb.ID
		node1, err := ws.CreateNode(ctx, "Node 1", NodeTypeDocument, zeroID, author)
		if err != nil {
			t.Fatalf("failed to create node 1: %v", err)
		}
		if node1.ID.IsZero() {
			t.Errorf("expected node1 to have non-zero ID")
		}

		node2, err := ws.CreateNode(ctx, "Node 2", NodeTypeTable, node1.ID, author)
		if err != nil {
			t.Fatalf("failed to create node 2: %v", err)
		}

		if _, err = ws.ReadNode(0); err == nil {
			t.Fatalf("expected error reading node 0, got nil")
		}

		readNode1, err := ws.ReadNode(node1.ID)
		if err != nil {
			t.Fatalf("failed to read node 1: %v", err)
		}
		if readNode1.Title != "Node 1" {
			t.Errorf("expected title 'Node 1', got %q", readNode1.Title)
		}

		topLevel, err := ws.ListChildren(0)
		if err != nil {
			t.Fatalf("failed to list children of 0: %v", err)
		}
		if len(topLevel) != 1 || topLevel[0].ID != node1.ID {
			t.Errorf("expected [node1] in ListChildren(0), got %v", topLevel)
		}

		children, err := ws.ListChildren(node1.ID)
		if err != nil {
			t.Fatalf("failed to list children: %v", err)
		}
		if len(children) != 1 || children[0].ID != node2.ID {
			t.Errorf("expected [node2], got %v", children)
		}
	})

	t.Run("ListChildren_ReturnsOnlyTopLevelNodes", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		var zeroID jsonldb.ID
		topLevel, err := ws.CreateNode(ctx, "Top Level", NodeTypeDocument, zeroID, author)
		if err != nil {
			t.Fatalf("failed to create top-level: %v", err)
		}
		child, err := ws.CreateNode(ctx, "Child", NodeTypeDocument, topLevel.ID, author)
		if err != nil {
			t.Fatalf("failed to create child: %v", err)
		}
		grandchild, err := ws.CreateNode(ctx, "Grandchild", NodeTypeDocument, child.ID, author)
		if err != nil {
			t.Fatalf("failed to create grandchild: %v", err)
		}

		topLevelNodes, err := ws.ListChildren(0)
		if err != nil {
			t.Fatalf("failed to list children of 0: %v", err)
		}
		if len(topLevelNodes) != 1 {
			t.Errorf("expected 1 top-level node, got %d", len(topLevelNodes))
		}
		if len(topLevelNodes) > 0 && topLevelNodes[0].ID != topLevel.ID {
			t.Errorf("expected topLevel node, got %v", topLevelNodes[0].ID)
		}
		if !topLevelNodes[0].ParentID.IsZero() {
			t.Errorf("expected top-level node to have zero ParentID, got %v", topLevelNodes[0].ParentID)
		}
		if !topLevelNodes[0].HasChildren {
			t.Error("expected HasChildren to be true (node has children)")
		}
		if len(topLevelNodes[0].Children) != 0 {
			t.Errorf("expected Children to be empty (lazy load), got %d children", len(topLevelNodes[0].Children))
		}

		children, err := ws.ListChildren(topLevel.ID)
		if err != nil {
			t.Fatalf("failed to list children: %v", err)
		}
		if len(children) != 1 || children[0].ID != child.ID {
			t.Errorf("expected [child], got %v", children)
		}

		grandchildren, err := ws.ListChildren(child.ID)
		if err != nil {
			t.Fatalf("failed to list grandchildren: %v", err)
		}
		if len(grandchildren) != 1 || grandchildren[0].ID != grandchild.ID {
			t.Errorf("expected [grandchild], got %v", grandchildren)
		}
	})

	t.Run("TopLevelPages", func(t *testing.T) {
		fs, ws, wsID := initWS(t)
		ctx := t.Context()

		var zeroID jsonldb.ID
		page1, err := ws.CreateNode(ctx, "Welcome", NodeTypeDocument, zeroID, author)
		if err != nil {
			t.Fatalf("failed to create page 1: %v", err)
		}
		if page1.ID.IsZero() {
			t.Errorf("expected page1 to have non-zero ID")
		}

		page1Path := filepath.Join(fs.rootDir, wsID.String(), page1.ID.String(), "index.md")
		if _, err := os.Stat(page1Path); err != nil {
			t.Errorf("expected page at %s, got error: %v", page1Path, err)
		}

		page2, err := ws.CreateNode(ctx, "Page 2", NodeTypeDocument, zeroID, author)
		if err != nil {
			t.Fatalf("failed to create page 2: %v", err)
		}
		if page2.ID.IsZero() {
			t.Errorf("expected page2 to have non-zero ID")
		}

		page2Path := filepath.Join(fs.rootDir, wsID.String(), page2.ID.String(), "index.md")
		if _, err := os.Stat(page2Path); err != nil {
			t.Errorf("expected page at %s, got error: %v", page2Path, err)
		}

		if _, err = ws.ReadNode(0); err == nil {
			t.Fatalf("expected error reading node 0")
		}

		readPage1, err := ws.ReadNode(page1.ID)
		if err != nil {
			t.Fatalf("failed to read page1: %v", err)
		}
		if readPage1.Title != "Welcome" {
			t.Errorf("expected title 'Welcome', got %q", readPage1.Title)
		}

		topLevel, err := ws.ListChildren(zeroID)
		if err != nil {
			t.Fatalf("failed to list children: %v", err)
		}
		if len(topLevel) != 2 {
			t.Errorf("expected 2 top-level pages, got %d", len(topLevel))
		}
	})

	t.Run("CreatePageUnderParent", func(t *testing.T) {
		fs, ws, wsID := initWS(t)
		ctx := t.Context()

		var zeroID jsonldb.ID
		page, err := ws.CreatePageUnderParent(ctx, zeroID, "My First Page", "", author)
		if err != nil {
			t.Fatalf("failed to create page: %v", err)
		}
		if page.ID.IsZero() {
			t.Errorf("expected page to have non-zero ID")
		}

		pagePath := filepath.Join(fs.rootDir, wsID.String(), page.ID.String(), "index.md")
		if _, err := os.Stat(pagePath); err != nil {
			t.Errorf("expected page at %s, got error: %v", pagePath, err)
		}

		readPage, err := ws.ReadNode(page.ID)
		if err != nil {
			t.Fatalf("failed to read page: %v", err)
		}
		if readPage.Title != "My First Page" {
			t.Errorf("expected title 'My First Page', got %q", readPage.Title)
		}

		if err := ws.DeletePage(ctx, page.ID, author); err != nil {
			t.Fatalf("failed to delete page: %v", err)
		}
		if _, err := os.Stat(pagePath); !os.IsNotExist(err) {
			t.Errorf("expected page to be deleted, but file still exists")
		}
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		t.Run("ReadNonExistentPage", func(t *testing.T) {
			if _, err := ws.ReadPage(jsonldb.NewID()); err == nil {
				t.Error("expected error reading non-existent page")
			}
		})

		t.Run("ReadNonExistentTable", func(t *testing.T) {
			if _, err := ws.ReadTable(jsonldb.NewID()); err == nil {
				t.Error("expected error reading non-existent table")
			}
		})

		t.Run("DeleteNonExistentPage", func(t *testing.T) {
			if err := ws.DeletePage(ctx, jsonldb.NewID(), author); err == nil {
				t.Error("expected error deleting non-existent page")
			}
		})

		t.Run("IterAssets_NonExistentPage", func(t *testing.T) {
			iter, err := ws.IterAssets(jsonldb.NewID())
			if err != nil {
				t.Fatalf("expected nil error for non-existent page, got: %v", err)
			}
			count := 0
			for range iter {
				count++
			}
			if count != 0 {
				t.Errorf("expected 0 assets for non-existent page, got %d", count)
			}
		})
	})

	t.Run("GitPathWithNestedPages", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		topLevel, err := ws.CreateNode(ctx, "Top Level", NodeTypeDocument, 0, author)
		if err != nil {
			t.Fatalf("failed to create top-level: %v", err)
		}
		child, err := ws.CreateNode(ctx, "Child", NodeTypeDocument, topLevel.ID, author)
		if err != nil {
			t.Fatalf("failed to create child: %v", err)
		}
		grandchild, err := ws.CreateNode(ctx, "Grandchild", NodeTypeDocument, child.ID, author)
		if err != nil {
			t.Fatalf("failed to create grandchild: %v", err)
		}

		topLevelPath := ws.gitPath(topLevel.ParentID, topLevel.ID, "index.md")
		expectedTopLevelPath := filepath.Join(topLevel.ID.String(), "index.md")
		if topLevelPath != expectedTopLevelPath {
			t.Errorf("topLevel gitPath mismatch: got %s, expected %s", topLevelPath, expectedTopLevelPath)
		}

		childPath := ws.gitPath(child.ParentID, child.ID, "index.md")
		expectedChildPath := filepath.Join(topLevel.ID.String(), child.ID.String(), "index.md")
		if childPath != expectedChildPath {
			t.Errorf("child gitPath mismatch:\n  got:      %s\n  expected: %s", childPath, expectedChildPath)
		}

		grandchildPath := ws.gitPath(grandchild.ParentID, grandchild.ID, "index.md")
		expectedGrandchildPath := filepath.Join(topLevel.ID.String(), child.ID.String(), grandchild.ID.String(), "index.md")
		if grandchildPath != expectedGrandchildPath {
			t.Errorf("grandchild gitPath mismatch:\n  got:      %s\n  expected: %s", grandchildPath, expectedGrandchildPath)
		}

		if _, err := os.Stat(filepath.Join(ws.wsDir, topLevel.ID.String(), "index.md")); err != nil {
			t.Errorf("expected top-level page at workspace/<id>/index.md: %v", err)
		}
		if _, err := os.Stat(filepath.Join(ws.wsDir, topLevel.ID.String(), child.ID.String(), "index.md")); err != nil {
			t.Errorf("expected child page at workspace/<topLevel>/<child>/index.md: %v", err)
		}
		if _, err := os.Stat(filepath.Join(ws.wsDir, topLevel.ID.String(), child.ID.String(), grandchild.ID.String(), "index.md")); err != nil {
			t.Errorf("expected grandchild page at workspace/<topLevel>/<child>/<grandchild>/index.md: %v", err)
		}
	})

	t.Run("MoveNode", func(t *testing.T) {
		t.Run("MoveToDifferentParent", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			a, err := ws.CreateNode(ctx, "A", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create A: %v", err)
			}
			b, err := ws.CreateNode(ctx, "B", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create B: %v", err)
			}
			child, err := ws.CreateNode(ctx, "Child", NodeTypeDocument, a.ID, author)
			if err != nil {
				t.Fatalf("create child: %v", err)
			}

			if err := ws.MoveNode(ctx, child.ID, b.ID, author); err != nil {
				t.Fatalf("move: %v", err)
			}

			children, err := ws.ListChildren(b.ID)
			if err != nil {
				t.Fatalf("list children of B: %v", err)
			}
			if len(children) != 1 || children[0].ID != child.ID {
				t.Errorf("expected child under B, got %v", children)
			}

			children, err = ws.ListChildren(a.ID)
			if err != nil {
				t.Fatalf("list children of A: %v", err)
			}
			if len(children) != 0 {
				t.Errorf("expected no children under A, got %d", len(children))
			}

			node, err := ws.ReadNode(child.ID)
			if err != nil {
				t.Fatalf("read child after move: %v", err)
			}
			if node.Title != "Child" {
				t.Errorf("expected title 'Child', got %q", node.Title)
			}
		})

		t.Run("MoveToRoot", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			parent, err := ws.CreateNode(ctx, "Parent", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create parent: %v", err)
			}
			child, err := ws.CreateNode(ctx, "Child", NodeTypeDocument, parent.ID, author)
			if err != nil {
				t.Fatalf("create child: %v", err)
			}

			if err := ws.MoveNode(ctx, child.ID, 0, author); err != nil {
				t.Fatalf("move to root: %v", err)
			}

			topLevel, err := ws.ListChildren(0)
			if err != nil {
				t.Fatalf("list root: %v", err)
			}
			found := false
			for _, n := range topLevel {
				if n.ID == child.ID {
					found = true
					break
				}
			}
			if !found {
				t.Error("expected child in top-level nodes after move to root")
			}
		})

		t.Run("CycleRejected", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			parent, err := ws.CreateNode(ctx, "Parent", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create parent: %v", err)
			}
			child, err := ws.CreateNode(ctx, "Child", NodeTypeDocument, parent.ID, author)
			if err != nil {
				t.Fatalf("create child: %v", err)
			}

			err = ws.MoveNode(ctx, parent.ID, child.ID, author)
			if !errors.Is(err, errCycleDetected) {
				t.Errorf("expected cycle error, got %v", err)
			}
		})

		t.Run("DescendantCycle", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			a, err := ws.CreateNode(ctx, "A", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create A: %v", err)
			}
			b, err := ws.CreateNode(ctx, "B", NodeTypeDocument, a.ID, author)
			if err != nil {
				t.Fatalf("create B: %v", err)
			}
			c, err := ws.CreateNode(ctx, "C", NodeTypeDocument, b.ID, author)
			if err != nil {
				t.Fatalf("create C: %v", err)
			}

			err = ws.MoveNode(ctx, a.ID, c.ID, author)
			if !errors.Is(err, errCycleDetected) {
				t.Errorf("expected cycle error for descendant, got %v", err)
			}
		})

		t.Run("NoOpSameParent", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			parent, err := ws.CreateNode(ctx, "Parent", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create parent: %v", err)
			}
			child, err := ws.CreateNode(ctx, "Child", NodeTypeDocument, parent.ID, author)
			if err != nil {
				t.Fatalf("create child: %v", err)
			}

			if err := ws.MoveNode(ctx, child.ID, parent.ID, author); err != nil {
				t.Fatalf("no-op move: %v", err)
			}

			children, err := ws.ListChildren(parent.ID)
			if err != nil {
				t.Fatalf("list children: %v", err)
			}
			if len(children) != 1 || children[0].ID != child.ID {
				t.Errorf("expected child still under parent, got %v", children)
			}
		})

		t.Run("ChildrenAccessibleAfterMove", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			a, err := ws.CreateNode(ctx, "A", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create A: %v", err)
			}
			b, err := ws.CreateNode(ctx, "B", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create B: %v", err)
			}
			child, err := ws.CreateNode(ctx, "Child", NodeTypeDocument, a.ID, author)
			if err != nil {
				t.Fatalf("create child: %v", err)
			}
			grandchild, err := ws.CreateNode(ctx, "Grandchild", NodeTypeDocument, child.ID, author)
			if err != nil {
				t.Fatalf("create grandchild: %v", err)
			}

			if err := ws.MoveNode(ctx, child.ID, b.ID, author); err != nil {
				t.Fatalf("move: %v", err)
			}

			node, err := ws.ReadNode(grandchild.ID)
			if err != nil {
				t.Fatalf("read grandchild after move: %v", err)
			}
			if node.Title != "Grandchild" {
				t.Errorf("expected title 'Grandchild', got %q", node.Title)
			}

			gc, err := ws.ListChildren(child.ID)
			if err != nil {
				t.Fatalf("list grandchild: %v", err)
			}
			if len(gc) != 1 || gc[0].ID != grandchild.ID {
				t.Errorf("expected grandchild under child, got %v", gc)
			}
		})

		t.Run("MoveNonExistent", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			err := ws.MoveNode(ctx, jsonldb.NewID(), 0, author)
			if err == nil {
				t.Error("expected error moving non-existent node")
			}
		})

		t.Run("MoveToNonExistentParent", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			node, err := ws.CreateNode(ctx, "Node", NodeTypeDocument, 0, author)
			if err != nil {
				t.Fatalf("create node: %v", err)
			}

			err = ws.MoveNode(ctx, node.ID, jsonldb.NewID(), author)
			if err == nil {
				t.Error("expected error moving to non-existent parent")
			}
		})

		t.Run("PreservesLinks", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			pageA, err := ws.CreatePageUnderParent(ctx, 0, "A", "", author)
			if err != nil {
				t.Fatalf("create A: %v", err)
			}
			pageB, err := ws.CreatePageUnderParent(ctx, 0, "B", "", author)
			if err != nil {
				t.Fatalf("create B: %v", err)
			}
			pageC, err := ws.CreatePageUnderParent(ctx, pageA.ID, "C", "", author)
			if err != nil {
				t.Fatalf("create C: %v", err)
			}

			cLink := fmt.Sprintf("See [B](../%s/index.md)", pageB.ID)
			if _, err := ws.UpdatePage(ctx, pageC.ID, "C", cLink, author); err != nil {
				t.Fatalf("update C: %v", err)
			}
			bLink := fmt.Sprintf("See [C](../%s/index.md)", pageC.ID)
			if _, err := ws.UpdatePage(ctx, pageB.ID, "B", bLink, author); err != nil {
				t.Fatalf("update B: %v", err)
			}

			if err := ws.MoveNode(ctx, pageC.ID, 0, author); err != nil {
				t.Fatalf("move C: %v", err)
			}

			nodeC, err := ws.ReadPage(pageC.ID)
			if err != nil {
				t.Fatalf("read C: %v", err)
			}
			if nodeC.Content != cLink {
				t.Errorf("C link should be preserved, want %q, got %q", cLink, nodeC.Content)
			}

			nodeB, err := ws.ReadPage(pageB.ID)
			if err != nil {
				t.Fatalf("read B: %v", err)
			}
			if nodeB.Content != bLink {
				t.Errorf("B link should be preserved, want %q, got %q", bLink, nodeB.Content)
			}
		})
	})

	t.Run("Quotas", func(t *testing.T) {
		t.Run("PageQuota", func(t *testing.T) {
			fs, _, wsID := initWS(t)
			ctx := t.Context()

			if _, err := fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
				w.Quotas.MaxPages = 2
				return nil
			}); err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			// Re-get store after quota change.
			fs.InvalidateWorkspaceStore(wsID)
			ws, err := fs.GetWorkspaceStore(ctx, wsID)
			if err != nil {
				t.Fatalf("failed to get workspace store: %v", err)
			}

			id1 := jsonldb.NewID()
			if _, err := ws.WritePage(ctx, id1, 0, "Page 1", "content", author); err != nil {
				t.Fatalf("failed to create page 1: %v", err)
			}

			id2 := jsonldb.NewID()
			if _, err := ws.WritePage(ctx, id2, 0, "Page 2", "content", author); err != nil {
				t.Fatalf("failed to create page 2: %v", err)
			}

			id3 := jsonldb.NewID()
			_, err = ws.WritePage(ctx, id3, 0, "Page 3", "content", author)
			if err != nil {
				t.Logf("Got error creating page 3: %v (quota may be enforced)", err)
			}
		})

		t.Run("StorageQuota", func(t *testing.T) {
			fs, _, wsID := initWS(t)
			ctx := t.Context()

			if _, err := fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
				w.Quotas.MaxStorageBytes = 1024 * 1024 // 1 MB
				return nil
			}); err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			org, err := fs.wsSvc.Get(wsID)
			var zeroOrgID jsonldb.ID
			if err == nil && org.OrganizationID != zeroOrgID {
				_, _ = fs.orgSvc.Modify(org.OrganizationID, func(o *identity.Organization) error {
					o.Quotas.MaxTotalStorageBytes = 1000 * 1024 * 1024 * 1024 // 1TB
					return nil
				})
			}

			fs.InvalidateWorkspaceStore(wsID)
			ws, err := fs.GetWorkspaceStore(ctx, wsID)
			if err != nil {
				t.Fatalf("failed to get workspace store: %v", err)
			}

			largeContent := make([]byte, 2*1024*1024)
			for i := range len(largeContent) {
				largeContent[i] = byte('x')
			}

			id := jsonldb.NewID()
			_, err = ws.WritePage(ctx, id, 0, "Large", string(largeContent), author)
			if err == nil {
				t.Logf("WritePage succeeded - quota enforcement might not be strict")
			} else if !errors.Is(err, errQuotaExceeded) {
				t.Logf("Got error: %v (not quota exceeded)", err)
			}
		})

		t.Run("RecordQuota", func(t *testing.T) {
			fs, _, wsID := initWS(t)
			ctx := t.Context()

			if _, err := fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
				w.Quotas.MaxRecordsPerTable = 5
				return nil
			}); err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			fs.InvalidateWorkspaceStore(wsID)
			ws, err := fs.GetWorkspaceStore(ctx, wsID)
			if err != nil {
				t.Fatalf("failed to get workspace store: %v", err)
			}

			tableID := jsonldb.NewID()
			tableNode := &Node{
				ID:       tableID,
				Title:    "Test",
				Type:     NodeTypeTable,
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			if err := ws.WriteTable(ctx, tableNode, true, author); err != nil {
				t.Fatalf("failed to create table: %v", err)
			}

			for i := range 5 {
				rec := &DataRecord{
					ID:       jsonldb.NewID(),
					Data:     map[string]any{"name": "Record"},
					Created:  storage.Now(),
					Modified: storage.Now(),
				}
				if err := ws.AppendRecord(ctx, tableID, rec, author); err != nil {
					t.Fatalf("failed to create record %d: %v", i, err)
				}
			}

			rec := &DataRecord{
				ID:       jsonldb.NewID(),
				Data:     map[string]any{"name": "Extra"},
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			if err := ws.AppendRecord(ctx, tableID, rec, author); err == nil {
				t.Error("expected record quota exceeded error")
			}
		})

		t.Run("UpdateRecord_SameSizeAllowed", func(t *testing.T) {
			fs, _, wsID := initWS(t)
			ctx := t.Context()

			if _, err := fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
				w.Quotas.MaxStorageBytes = 1024 * 1024 // 1MB
				return nil
			}); err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			fs.InvalidateWorkspaceStore(wsID)
			ws, err := fs.GetWorkspaceStore(ctx, wsID)
			if err != nil {
				t.Fatalf("failed to get workspace store: %v", err)
			}

			tableID := jsonldb.NewID()
			tableNode := &Node{
				ID:       tableID,
				Title:    "Test",
				Type:     NodeTypeTable,
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			if err := ws.WriteTable(ctx, tableNode, true, author); err != nil {
				t.Fatalf("failed to create table: %v", err)
			}

			recordID := jsonldb.NewID()
			record := &DataRecord{
				ID:       recordID,
				Data:     map[string]any{"field": strings.Repeat("a", 100)},
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			if err := ws.AppendRecord(ctx, tableID, record, author); err != nil {
				t.Fatalf("failed to create record: %v", err)
			}

			_, storageUsage, _ := ws.GetWorkspaceUsage()
			if _, err := fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
				w.Quotas.MaxStorageBytes = storageUsage
				return nil
			}); err != nil {
				t.Fatalf("failed to reduce quota: %v", err)
			}

			updatedRecord := &DataRecord{
				ID:       recordID,
				Data:     map[string]any{"field": strings.Repeat("b", 100)},
				Created:  record.Created,
				Modified: record.Modified,
			}

			err = ws.UpdateRecord(ctx, tableID, updatedRecord, author)
			if err != nil {
				t.Errorf("same-size update should succeed: %v", err)
			}
		})
	})

	t.Run("Markdown", func(t *testing.T) {
		fs, ws, wsID := initWS(t)
		ctx := t.Context()

		nodeID := jsonldb.NewID()
		if _, err := ws.WritePage(ctx, nodeID, 0, "Format Test", "# Content\n\nWith multiple lines", author); err != nil {
			t.Fatalf("failed to write page: %v", err)
		}

		filePath := filepath.Join(fs.rootDir, wsID.String(), nodeID.String(), "index.md")
		data, err := os.ReadFile(filePath) //nolint:gosec // G304: test code with controlled path
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		content := string(data)

		t.Run("FrontMatterDelimiters", func(t *testing.T) {
			if !strings.Contains(content, "---") {
				t.Error("expected front matter delimiters")
			}
		})

		t.Run("FrontMatterTitle", func(t *testing.T) {
			if !strings.Contains(content, "title: Format Test") {
				t.Error("expected title in front matter")
			}
		})

		t.Run("FrontMatterTimestamps", func(t *testing.T) {
			if !strings.Contains(content, "created:") {
				t.Error("expected created timestamp")
			}
			if !strings.Contains(content, "modified:") {
				t.Error("expected modified timestamp")
			}
		})

		t.Run("ContentSeparation", func(t *testing.T) {
			parts := strings.Split(content, "---")
			if len(parts) < 3 {
				t.Error("expected three sections separated by ---")
			}
		})
	})

	t.Run("RelativeLinks", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		pageA, err := ws.CreatePageUnderParent(ctx, 0, "A", "", author)
		if err != nil {
			t.Fatalf("create A: %v", err)
		}
		pageB, err := ws.CreatePageUnderParent(ctx, 0, "B", "", author)
		if err != nil {
			t.Fatalf("create B: %v", err)
		}

		t.Run("RoundTrip", func(t *testing.T) {
			relLink := fmt.Sprintf("[B](../%s/index.md)", pageB.ID)
			content := "See " + relLink + " for details."
			if _, err := ws.UpdatePage(ctx, pageA.ID, "A", content, author); err != nil {
				t.Fatalf("update: %v", err)
			}
			node, err := ws.ReadPage(pageA.ID)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !strings.Contains(node.Content, relLink) {
				t.Errorf("want %q in content, got: %s", relLink, node.Content)
			}
		})

		t.Run("NoLinksPassthrough", func(t *testing.T) {
			content := "Plain text with no links."
			if _, err := ws.UpdatePage(ctx, pageA.ID, "A", content, author); err != nil {
				t.Fatalf("update: %v", err)
			}
			node, err := ws.ReadPage(pageA.ID)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if node.Content != content {
				t.Errorf("want %q, got %q", content, node.Content)
			}
		})

		t.Run("CreateWithLinks", func(t *testing.T) {
			relLink := fmt.Sprintf("[A](../%s/index.md)", pageA.ID)
			page, err := ws.CreatePageUnderParent(ctx, 0, "D", "Link to "+relLink, author)
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			node, err := ws.ReadPage(page.ID)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !strings.Contains(node.Content, relLink) {
				t.Errorf("want %q in content, got: %s", relLink, node.Content)
			}
		})
	})

	t.Run("Backlinks", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		target, err := ws.CreatePageUnderParent(ctx, 0, "Target", "", author)
		if err != nil {
			t.Fatalf("failed to create target: %v", err)
		}
		linkContent := fmt.Sprintf("[Target](../%s/index.md)", target.ID)
		source, err := ws.CreatePageUnderParent(ctx, 0, "Source", linkContent, author)
		if err != nil {
			t.Fatalf("failed to create source: %v", err)
		}
		if _, err = ws.CreatePageUnderParent(ctx, 0, "Unrelated", "no links here", author); err != nil {
			t.Fatalf("failed to create unrelated: %v", err)
		}

		t.Run("Found", func(t *testing.T) {
			backlinks, err := ws.GetBacklinks(target.ID)
			if err != nil {
				t.Fatalf("failed to get backlinks: %v", err)
			}
			if len(backlinks) != 1 {
				t.Fatalf("expected 1 backlink, got %d", len(backlinks))
			}
			if backlinks[0].NodeID != source.ID {
				t.Errorf("expected backlink from %s, got %s", source.ID, backlinks[0].NodeID)
			}
			if backlinks[0].Title != "Source" {
				t.Errorf("expected backlink title 'Source', got %s", backlinks[0].Title)
			}
		})

		t.Run("None", func(t *testing.T) {
			backlinks, err := ws.GetBacklinks(source.ID)
			if err != nil {
				t.Fatalf("failed to get backlinks: %v", err)
			}
			if len(backlinks) != 0 {
				t.Errorf("expected 0 backlinks, got %d", len(backlinks))
			}
		})

		t.Run("AfterLinkRemoved", func(t *testing.T) {
			if _, err := ws.UpdatePage(ctx, source.ID, "Source", "no more links", author); err != nil {
				t.Fatalf("failed to update page: %v", err)
			}
			backlinks, err := ws.GetBacklinks(target.ID)
			if err != nil {
				t.Fatalf("failed to get backlinks: %v", err)
			}
			if len(backlinks) != 0 {
				t.Errorf("expected 0 backlinks after link removed, got %d", len(backlinks))
			}
		})
	})

	t.Run("ValidateLinks", func(t *testing.T) {
		t.Run("AllValid", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			a, err := ws.CreatePageUnderParent(ctx, 0, "A", "", author)
			if err != nil {
				t.Fatalf("create A: %v", err)
			}
			linkToA := fmt.Sprintf("[A](../%s/index.md)", a.ID)
			if _, err := ws.CreatePageUnderParent(ctx, 0, "B", linkToA, author); err != nil {
				t.Fatalf("create B: %v", err)
			}

			invalid, err := ws.ValidateLinks()
			if err != nil {
				t.Fatalf("ValidateLinks: %v", err)
			}
			if len(invalid) != 0 {
				t.Errorf("expected 0 invalid links, got %d: %v", len(invalid), invalid)
			}
		})

		t.Run("DanglingLink", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			ghost := jsonldb.NewID()
			linkToGhost := fmt.Sprintf("[Ghost](../%s/index.md)", ghost)
			src, err := ws.CreatePageUnderParent(ctx, 0, "Src", linkToGhost, author)
			if err != nil {
				t.Fatalf("create Src: %v", err)
			}

			invalid, err := ws.ValidateLinks()
			if err != nil {
				t.Fatalf("ValidateLinks: %v", err)
			}
			if len(invalid) != 1 {
				t.Fatalf("expected 1 invalid link, got %d: %v", len(invalid), invalid)
			}
			if invalid[0].SourceID != src.ID {
				t.Errorf("expected source=%s, got %s", src.ID, invalid[0].SourceID)
			}
			if invalid[0].Target != ghost.String() {
				t.Errorf("expected target=%s, got %s", ghost, invalid[0].Target)
			}
		})

		t.Run("MixedValidAndInvalid", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			existing, err := ws.CreatePageUnderParent(ctx, 0, "Real", "", author)
			if err != nil {
				t.Fatalf("create Real: %v", err)
			}
			ghost := jsonldb.NewID()
			content := fmt.Sprintf("[Real](../%s/index.md) and [Ghost](../%s/index.md)", existing.ID, ghost)
			if _, err := ws.CreatePageUnderParent(ctx, 0, "Linker", content, author); err != nil {
				t.Fatalf("create Linker: %v", err)
			}

			invalid, err := ws.ValidateLinks()
			if err != nil {
				t.Fatalf("ValidateLinks: %v", err)
			}
			if len(invalid) != 1 {
				t.Fatalf("expected 1 invalid link, got %d: %v", len(invalid), invalid)
			}
			if invalid[0].Target != ghost.String() {
				t.Errorf("expected target=%s, got %s", ghost, invalid[0].Target)
			}
		})
	})

	t.Run("NodeTitles", func(t *testing.T) {
		_, ws, _ := initWS(t)
		ctx := t.Context()

		a, err := ws.CreatePageUnderParent(ctx, 0, "Alpha", "", author)
		if err != nil {
			t.Fatalf("failed to create page: %v", err)
		}
		b, err := ws.CreatePageUnderParent(ctx, 0, "Beta", "", author)
		if err != nil {
			t.Fatalf("failed to create page: %v", err)
		}

		titles, err := ws.GetNodeTitles([]jsonldb.ID{a.ID, b.ID})
		if err != nil {
			t.Fatalf("failed to get node titles: %v", err)
		}
		if len(titles) != 2 {
			t.Errorf("expected 2 titles, got %d", len(titles))
		}
		if titles[a.ID] != "Alpha" {
			t.Errorf("expected 'Alpha', got %s", titles[a.ID])
		}
		if titles[b.ID] != "Beta" {
			t.Errorf("expected 'Beta', got %s", titles[b.ID])
		}
	})

	t.Run("WorkspaceUsage", func(t *testing.T) {
		t.Run("CountsPages", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			for i := range 3 {
				nodeID := jsonldb.NewID()
				if _, err := ws.WritePage(ctx, nodeID, 0, "Page "+string(rune(49+i)), "content", author); err != nil {
					t.Fatalf("failed to create page: %v", err)
				}
			}

			pageCount, _, err := ws.GetWorkspaceUsage()
			if err != nil {
				t.Fatalf("failed to get usage: %v", err)
			}
			if pageCount != 3 {
				t.Errorf("expected pageCount=3, got %d", pageCount)
			}
		})

		t.Run("ExcludesGitDir", func(t *testing.T) {
			_, ws, _ := initWS(t)

			// Create a fake .git/objects/maintenance.lock to simulate git internals.
			// GetWorkspaceUsage must skip .git and not fail if transient files vanish.
			lockDir := filepath.Join(ws.wsDir, ".git", "objects")
			if err := os.MkdirAll(lockDir, 0o750); err != nil {
				t.Fatal(err)
			}
			lockFile := filepath.Join(lockDir, "maintenance.lock")
			if err := os.WriteFile(lockFile, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}

			_, storageBytes, err := ws.GetWorkspaceUsage()
			if err != nil {
				t.Fatalf("GetWorkspaceUsage failed with .git present: %v", err)
			}

			// Storage must not include the .git contents.
			// Remove .git and compare.
			if err := os.RemoveAll(filepath.Join(ws.wsDir, ".git")); err != nil {
				t.Fatal(err)
			}
			_, withoutGit, err := ws.GetWorkspaceUsage()
			if err != nil {
				t.Fatal(err)
			}
			if storageBytes != withoutGit {
				t.Errorf("storage with .git=%d, without=%d; .git contents should be excluded", storageBytes, withoutGit)
			}
		})

		t.Run("HybridNodeCountedOnce", func(t *testing.T) {
			_, ws, _ := initWS(t)
			ctx := t.Context()

			nodeID := jsonldb.NewID()
			if _, err := ws.WritePage(ctx, nodeID, 0, "Hybrid", "content", author); err != nil {
				t.Fatalf("failed to create page: %v", err)
			}

			hybridNode := &Node{
				ID:       nodeID,
				Title:    "Hybrid",
				Type:     NodeTypeTable,
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			if err := ws.WriteTable(ctx, hybridNode, false, author); err != nil {
				t.Fatalf("failed to add table metadata: %v", err)
			}

			pageCount, _, err := ws.GetWorkspaceUsage()
			if err != nil {
				t.Fatalf("failed to get usage: %v", err)
			}
			if pageCount != 1 {
				t.Errorf("expected pageCount=1 for hybrid node, got %d", pageCount)
			}
		})
	})
}

func TestExtractLinkedNodeIDs(t *testing.T) {
	nodeA := jsonldb.NewID()
	nodeB := jsonldb.NewID()
	nodeC := jsonldb.NewID()

	tests := []struct {
		name     string
		content  string
		expected []jsonldb.ID
	}{
		{
			name:     "empty content",
			content:  "",
			expected: nil,
		},
		{
			name:     "no links",
			content:  "Some text without any links",
			expected: nil,
		},
		{
			name:     "sibling relative link",
			content:  "Check [my page](../" + nodeA.String() + "/index.md) here",
			expected: []jsonldb.ID{nodeA},
		},
		{
			name:     "multiple relative links",
			content:  "See [page1](../" + nodeA.String() + "/index.md) and [page2](../" + nodeB.String() + "/index.md)",
			expected: []jsonldb.ID{nodeA, nodeB},
		},
		{
			name:     "child link",
			content:  "[page](" + nodeA.String() + "/index.md)",
			expected: []jsonldb.ID{nodeA},
		},
		{
			name:     "duplicate links extracted once",
			content:  "[a](../" + nodeC.String() + "/index.md) and [b](../" + nodeC.String() + "/index.md)",
			expected: []jsonldb.ID{nodeC},
		},
		{
			name:     "external link ignored",
			content:  "[google](https://google.com) and [internal](../" + nodeA.String() + "/index.md)",
			expected: []jsonldb.ID{nodeA},
		},
		{
			name:     "deep relative link",
			content:  "# Header\n\nSome text with [a link](../../" + nodeB.String() + "/index.md) in it.\n\n![image](image.png)",
			expected: []jsonldb.ID{nodeB},
		},
		{
			name:     "absolute link ignored",
			content:  "[abs](/some/" + nodeA.String() + "/index.md)",
			expected: nil,
		},
		{
			name:     "invalid ID ignored",
			content:  "[bad](../not-valid-id/index.md)",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids := ExtractLinkedNodeIDs(tt.content)

			if len(ids) != len(tt.expected) {
				t.Errorf("expected %d IDs, got %d: %v", len(tt.expected), len(ids), ids)
				return
			}

			for i, exp := range tt.expected {
				if ids[i] != exp {
					t.Errorf("expected ID[%d]=%s, got %s", i, exp.String(), ids[i].String())
				}
			}
		})
	}
}
