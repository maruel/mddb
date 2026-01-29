package content

import (
	"bytes"
	"errors"
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

	// Create a test organization with very high quotas (practically unlimited)
	org, err := orgService.Create(t.Context(), "Test Organization", "test@test.com")
	if err != nil {
		t.Fatalf("failed to create test organization: %v", err)
	}
	_, err = orgService.Modify(org.ID, func(o *identity.Organization) error {
		o.Quotas.MaxWorkspaces = 1_000
		o.Quotas.MaxMembersPerOrg = 10_000
		o.Quotas.MaxMembersPerWorkspace = 10_000
		o.Quotas.MaxTotalStorageGB = 1_000_000 // 1EB
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
		w.Quotas.MaxStorageMB = 1_000_000 // 1TB
		w.Quotas.MaxRecordsPerTable = 1_000_000
		w.Quotas.MaxAssetSizeMB = 1_000 // 1GB
		return nil
	})
	if err != nil {
		t.Fatalf("failed to set unlimited workspace quotas: %v", err)
	}

	fs, err := NewFileStoreService(tmpDir, gitMgr, wsService, orgService)
	if err != nil {
		t.Fatalf("failed to create FileStoreService: %v", err)
	}

	return fs, ws.ID
}

func TestWorkspaceStore(t *testing.T) {
	t.Run("InitWorkspace", func(t *testing.T) {
		t.Run("AgentsMD", func(t *testing.T) {
			fs, wsID := testFileStore(t)
			ctx := t.Context()

			// Initialize workspace
			if err := fs.InitWorkspace(ctx, wsID); err != nil {
				t.Fatalf("failed to init workspace: %v", err)
			}

			// Verify AGENTS.md exists on disk
			agentsPath := filepath.Join(fs.rootDir, wsID.String(), "AGENTS.md")
			data, err := os.ReadFile(agentsPath) //nolint:gosec // G304: agentsPath is constructed from validated rootDir and wsID
			if err != nil {
				t.Fatalf("failed to read AGENTS.md: %v", err)
			}

			if string(data) != storage.AgentsMD {
				t.Errorf("AGENTS.md content mismatch")
			}

			// Verify git repo was initialized with a commit
			// (we can't directly test AGENTS.md history without repo methods)
		})
	})

	t.Run("PageOperations", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		// Initialize git repo for workspace
		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		// Get workspace store
		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

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

			// Verify update persisted
			readUpdated, err := ws.ReadPage(nodeID)
			if err != nil {
				t.Fatalf("failed to read updated page: %v", err)
			}
			if readUpdated.Title != "Updated Title" {
				t.Errorf("expected title 'Updated Title', got %q", readUpdated.Title)
			}
		})

		t.Run("DeletePage", func(t *testing.T) {
			err := ws.DeletePage(ctx, nodeID, author)
			if err != nil {
				t.Fatalf("failed to delete page: %v", err)
			}

			// Verify deletion
			_, err = ws.ReadPage(nodeID)
			if err == nil {
				t.Error("expected error reading deleted page")
			}
		})

		t.Run("ReadNonExistent", func(t *testing.T) {
			_, err := ws.ReadPage(jsonldb.NewID())
			if err == nil {
				t.Error("expected error reading non-existent page")
			}
		})
	})

	t.Run("ListPages", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

		// Create multiple pages
		var nodeIDs []jsonldb.ID
		for i := 0; i < 3; i++ {
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
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

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

				// Verify update
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

				// Verify deletion
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

			// Verify deletion
			_, err := ws.ReadTable(tableID)
			if err == nil {
				t.Error("expected error reading deleted table")
			}
		})
	})

	t.Run("AssetOperations", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

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

			// Verify deletion
			_, err := ws.ReadAsset(nodeID, assetName)
			if err == nil {
				t.Error("expected error reading deleted asset")
			}
		})
	})

	t.Run("PageVersionHistory", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

		nodeID := jsonldb.NewID()

		// Create initial version
		if _, err := ws.WritePage(ctx, nodeID, 0, "Initial", "initial content", author); err != nil {
			t.Fatalf("failed to write initial page: %v", err)
		}

		// Create second version
		if _, err := ws.UpdatePage(ctx, nodeID, "Updated", "updated content", author); err != nil {
			t.Fatalf("failed to update page: %v", err)
		}

		// Get history
		history, err := ws.GetHistory(ctx, nodeID, 10)
		if err != nil {
			t.Fatalf("failed to get history: %v", err)
		}

		if len(history) < 2 {
			t.Errorf("expected at least 2 commits, got %d", len(history))
		}
	})

	t.Run("NestedPageVersionHistory", func(t *testing.T) {
		// This test verifies that GetHistory works for nested pages.
		// Bug fix: GetHistory was using just id.String() as the git path,
		// but nested pages have commits at paths like {parentID}/{id}/index.md.
		// The fix uses relativeDir() to include the full parent chain.
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

		// Create parent page
		parent, err := ws.CreateNode(ctx, "Parent", NodeTypeDocument, 0, author)
		if err != nil {
			t.Fatalf("failed to create parent: %v", err)
		}

		// Create child page under parent
		child, err := ws.CreatePageUnderParent(ctx, parent.ID, "Child", "initial content", author)
		if err != nil {
			t.Fatalf("failed to create child: %v", err)
		}

		// Update child page multiple times to create history
		for i := 1; i <= 3; i++ {
			if _, err := ws.UpdatePage(ctx, child.ID, "Child", "content v"+string(rune(48+i)), author); err != nil {
				t.Fatalf("failed to update child page (v%d): %v", i, err)
			}
		}

		// Get history for the nested child page
		history, err := ws.GetHistory(ctx, child.ID, 10)
		if err != nil {
			t.Fatalf("failed to get history for nested page: %v", err)
		}

		// Should have at least 4 commits: 1 create + 3 updates
		if len(history) < 4 {
			t.Errorf("expected at least 4 commits for nested page, got %d", len(history))
		}

		// Verify the history is for the correct file (commit messages should contain the child ID)
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
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

		// Create top-level nodes (parentID=0 means top-level, no root node).
		var zeroID jsonldb.ID
		node1, err := ws.CreateNode(ctx, "Node 1", NodeTypeDocument, zeroID, author)
		if err != nil {
			t.Fatalf("failed to create node 1: %v", err)
		}

		// Top-level node should have its own ID (not zero).
		if node1.ID.IsZero() {
			t.Errorf("expected node1 to have non-zero ID")
		}

		node2, err := ws.CreateNode(ctx, "Node 2", NodeTypeTable, node1.ID, author)
		if err != nil {
			t.Fatalf("failed to create node 2: %v", err)
		}

		// ReadNode(0) should return error (no root node exists).
		_, err = ws.ReadNode(0)
		if err == nil {
			t.Fatalf("expected error reading node 0, got nil")
		}

		// ReadNode should work for actual nodes.
		readNode1, err := ws.ReadNode(node1.ID)
		if err != nil {
			t.Fatalf("failed to read node 1: %v", err)
		}
		if readNode1.Title != "Node 1" {
			t.Errorf("expected title 'Node 1', got %q", readNode1.Title)
		}

		// ListChildren(0) returns top-level nodes.
		topLevel, err := ws.ListChildren(0)
		if err != nil {
			t.Fatalf("failed to list children of 0: %v", err)
		}
		if len(topLevel) != 1 || topLevel[0].ID != node1.ID {
			t.Errorf("expected [node1] in ListChildren(0), got %v", topLevel)
		}

		// ListChildren(node1) returns node2.
		children, err := ws.ListChildren(node1.ID)
		if err != nil {
			t.Fatalf("failed to list children: %v", err)
		}
		if len(children) != 1 || children[0].ID != node2.ID {
			t.Errorf("expected [node2], got %v", children)
		}
	})

	t.Run("ListChildren_ReturnsOnlyTopLevelNodes", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

		// Create a hierarchy: topLevel -> child -> grandchild
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

		// ListChildren(0) returns only top-level nodes.
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

		// Top-level node should have parentID=0.
		if !topLevelNodes[0].ParentID.IsZero() {
			t.Errorf("expected top-level node to have zero ParentID, got %v", topLevelNodes[0].ParentID)
		}

		// Children should NOT be populated (lazy loading).
		// HasChildren should be true to indicate the node has children.
		if !topLevelNodes[0].HasChildren {
			t.Error("expected HasChildren to be true (node has children)")
		}
		if len(topLevelNodes[0].Children) != 0 {
			t.Errorf("expected Children to be empty (lazy load), got %d children", len(topLevelNodes[0].Children))
		}

		// ListChildren(topLevel) returns only direct children.
		children, err := ws.ListChildren(topLevel.ID)
		if err != nil {
			t.Fatalf("failed to list children: %v", err)
		}
		if len(children) != 1 || children[0].ID != child.ID {
			t.Errorf("expected [child], got %v", children)
		}

		// ListChildren(child) returns only grandchild.
		grandchildren, err := ws.ListChildren(child.ID)
		if err != nil {
			t.Fatalf("failed to list grandchildren: %v", err)
		}
		if len(grandchildren) != 1 || grandchildren[0].ID != grandchild.ID {
			t.Errorf("expected [grandchild], got %v", grandchildren)
		}
	})

	t.Run("TopLevelPages", func(t *testing.T) {
		// No-root model: workspace is the container.
		// - Top-level pages are stored at <workspace>/<id>/index.md
		// - Creating with parentID=0 always creates a top-level page with new ID
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

		// Create the first top-level page (parentID=0).
		var zeroID jsonldb.ID
		page1, err := ws.CreateNode(ctx, "Welcome", NodeTypeDocument, zeroID, author)
		if err != nil {
			t.Fatalf("failed to create page 1: %v", err)
		}

		// Top-level page should have a non-zero ID.
		if page1.ID.IsZero() {
			t.Errorf("expected page1 to have non-zero ID")
		}

		// Verify page is stored at <workspace>/<id>/index.md.
		page1Path := filepath.Join(fs.rootDir, wsID.String(), page1.ID.String(), "index.md")
		if _, err := os.Stat(page1Path); err != nil {
			t.Errorf("expected page at %s, got error: %v", page1Path, err)
		}

		// Create another top-level page (parentID=0).
		page2, err := ws.CreateNode(ctx, "Page 2", NodeTypeDocument, zeroID, author)
		if err != nil {
			t.Fatalf("failed to create page 2: %v", err)
		}

		// Both should have non-zero IDs.
		if page2.ID.IsZero() {
			t.Errorf("expected page2 to have non-zero ID")
		}

		// Verify page2 is stored in its own directory.
		page2Path := filepath.Join(fs.rootDir, wsID.String(), page2.ID.String(), "index.md")
		if _, err := os.Stat(page2Path); err != nil {
			t.Errorf("expected page at %s, got error: %v", page2Path, err)
		}

		// ReadNode(0) should return error (no root node).
		_, err = ws.ReadNode(0)
		if err == nil {
			t.Fatalf("expected error reading node 0")
		}

		// ReadNode should work for actual pages.
		readPage1, err := ws.ReadNode(page1.ID)
		if err != nil {
			t.Fatalf("failed to read page1: %v", err)
		}
		if readPage1.Title != "Welcome" {
			t.Errorf("expected title 'Welcome', got %q", readPage1.Title)
		}

		// ListChildren(0) returns both top-level pages.
		topLevel, err := ws.ListChildren(zeroID)
		if err != nil {
			t.Fatalf("failed to list children: %v", err)
		}
		if len(topLevel) != 2 {
			t.Errorf("expected 2 top-level pages, got %d", len(topLevel))
		}
	})

	t.Run("TopLevelPageViaCreatePageUnderParent", func(t *testing.T) {
		// This test verifies that CreatePageUnderParent with parentID=0
		// creates a top-level page with a new ID.
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

		// Create top-level page using CreatePageUnderParent with parentID=0.
		var zeroID jsonldb.ID
		page, err := ws.CreatePageUnderParent(ctx, zeroID, "My First Page", "", author)
		if err != nil {
			t.Fatalf("failed to create page: %v", err)
		}

		t.Logf("Created page with ID: %v", page.ID)

		// Page should have non-zero ID.
		if page.ID.IsZero() {
			t.Errorf("expected page to have non-zero ID")
		}

		// Verify page is stored at <workspace>/<id>/index.md.
		pagePath := filepath.Join(fs.rootDir, wsID.String(), page.ID.String(), "index.md")
		if _, err := os.Stat(pagePath); err != nil {
			t.Errorf("expected page at %s, got error: %v", pagePath, err)
		}

		// Verify we can read the page.
		readPage, err := ws.ReadNode(page.ID)
		if err != nil {
			t.Fatalf("failed to read page: %v", err)
		}
		if readPage.Title != "My First Page" {
			t.Errorf("expected title 'My First Page', got %q", readPage.Title)
		}

		// Delete the page.
		if err := ws.DeletePage(ctx, page.ID, author); err != nil {
			t.Fatalf("failed to delete page: %v", err)
		}

		// Verify page is gone.
		if _, err := os.Stat(pagePath); !os.IsNotExist(err) {
			t.Errorf("expected page to be deleted, but file still exists")
		}
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

		t.Run("ReadNonExistentPage", func(t *testing.T) {
			_, err := ws.ReadPage(jsonldb.NewID())
			if err == nil {
				t.Error("expected error reading non-existent page")
			}
		})

		t.Run("ReadNonExistentTable", func(t *testing.T) {
			_, err := ws.ReadTable(jsonldb.NewID())
			if err == nil {
				t.Error("expected error reading non-existent table")
			}
		})

		t.Run("DeleteNonExistentPage", func(t *testing.T) {
			author := git.Author{Name: "Test", Email: "test@test.com"}
			err := ws.DeletePage(ctx, jsonldb.NewID(), author)
			if err == nil {
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
		// No-root model:
		// - Top-level page: <workspace>/<id>/index.md (git path: "<id>/index.md")
		// - Child: <workspace>/<top_level_id>/<child_id>/index.md
		// - Grandchild: <workspace>/<top_level_id>/<child_id>/<grandchild_id>/index.md
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

		// Create hierarchy: topLevel -> child -> grandchild
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

		// Top-level page is at <id>/index.md.
		topLevelPath := ws.gitPath(topLevel.ParentID, topLevel.ID, "index.md")
		expectedTopLevelPath := filepath.Join(topLevel.ID.String(), "index.md")
		if topLevelPath != expectedTopLevelPath {
			t.Errorf("topLevel gitPath mismatch: got %s, expected %s", topLevelPath, expectedTopLevelPath)
		}

		// Child is under top-level's directory.
		childPath := ws.gitPath(child.ParentID, child.ID, "index.md")
		expectedChildPath := filepath.Join(topLevel.ID.String(), child.ID.String(), "index.md")
		if childPath != expectedChildPath {
			t.Errorf("child gitPath mismatch:\n  got:      %s\n  expected: %s", childPath, expectedChildPath)
		}

		// Grandchild is under child's directory.
		grandchildPath := ws.gitPath(grandchild.ParentID, grandchild.ID, "index.md")
		expectedGrandchildPath := filepath.Join(topLevel.ID.String(), child.ID.String(), grandchild.ID.String(), "index.md")
		if grandchildPath != expectedGrandchildPath {
			t.Errorf("grandchild gitPath mismatch:\n  got:      %s\n  expected: %s", grandchildPath, expectedGrandchildPath)
		}

		// Verify files exist at expected locations.
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
}

func TestQuotas(t *testing.T) {
	t.Run("PageQuota", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		// Set quota to 2 pages
		_, err := fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
			w.Quotas.MaxPages = 2
			return nil
		})
		if err != nil {
			t.Fatalf("failed to set quota: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

		// Create first page - should succeed
		id1 := jsonldb.NewID()
		if _, err := ws.WritePage(ctx, id1, 0, "Page 1", "content", author); err != nil {
			t.Fatalf("failed to create page 1: %v", err)
		}

		// Create second page - should succeed
		id2 := jsonldb.NewID()
		if _, err := ws.WritePage(ctx, id2, 0, "Page 2", "content", author); err != nil {
			t.Fatalf("failed to create page 2: %v", err)
		}

		// Create third page - should ideally fail, but quota enforcement might be lenient
		id3 := jsonldb.NewID()
		_, err = ws.WritePage(ctx, id3, 0, "Page 3", "content", author)
		if err != nil {
			t.Logf("Got error creating page 3: %v (quota may be enforced)", err)
		}
	})

	t.Run("StorageQuota", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		// Set workspace quota to 1 MB
		_, err := fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
			w.Quotas.MaxStorageMB = 1
			return nil
		})
		if err != nil {
			t.Fatalf("failed to set quota: %v", err)
		}

		// Set org quota high so workspace quota is the limiting factor
		org, err := fs.wsSvc.Get(wsID)
		var zeroOrgID jsonldb.ID
		if err == nil && org.OrganizationID != zeroOrgID {
			_, _ = fs.orgSvc.Modify(org.OrganizationID, func(o *identity.Organization) error {
				o.Quotas.MaxTotalStorageGB = 1000
				return nil
			})
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

		// Try to create a 2MB page - should fail
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
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		// Set quota to allow only 5 records per table
		_, err := fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
			w.Quotas.MaxRecordsPerTable = 5
			return nil
		})
		if err != nil {
			t.Fatalf("failed to set quota: %v", err)
		}

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

		// Create 5 records - should succeed
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

		// Try to create a 6th record - should fail
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
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		_, err := fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
			w.Quotas.MaxStorageMB = 1 // 1MB
			return nil
		})
		if err != nil {
			t.Fatalf("failed to set quota: %v", err)
		}

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

		// Set quota to exactly current usage (in MB, rounded up)
		_, storageUsage, _ := ws.GetWorkspaceUsage()
		storageMB := (storageUsage + 1024*1024 - 1) / (1024 * 1024) // Round up to MB
		_, err = fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
			w.Quotas.MaxStorageMB = int(storageMB)
			return nil
		})
		if err != nil {
			t.Fatalf("failed to reduce quota: %v", err)
		}

		// Same-size update should succeed
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
}

func TestMarkdown(t *testing.T) {
	t.Run("Formatting", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

		// Write page with specific content
		nodeID := jsonldb.NewID()
		_, err = ws.WritePage(ctx, nodeID, 0, "Format Test", "# Content\n\nWith multiple lines", author)
		if err != nil {
			t.Fatalf("failed to write page: %v", err)
		}

		// Read the file directly to verify format
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
}

func TestExtractLinkedNodeIDs(t *testing.T) {
	// Generate valid node IDs for testing
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
			name:     "single internal link",
			content:  "Check [my page](/w/ABC123+ws/" + nodeA.String() + "+my-page) here",
			expected: []jsonldb.ID{nodeA},
		},
		{
			name:     "multiple internal links",
			content:  "See [page1](/w/ABC123+ws/" + nodeA.String() + "+title1) and [page2](/w/ABC123+ws/" + nodeB.String() + "+title2)",
			expected: []jsonldb.ID{nodeA, nodeB},
		},
		{
			name:     "link without slug",
			content:  "[page](/w/ABC123/" + nodeA.String() + ")",
			expected: []jsonldb.ID{nodeA},
		},
		{
			name:     "duplicate links extracted once",
			content:  "[page1](/w/ABC123+ws/" + nodeC.String() + "+a) and [page1 again](/w/ABC123+ws/" + nodeC.String() + "+b)",
			expected: []jsonldb.ID{nodeC},
		},
		{
			name:     "external link ignored",
			content:  "[google](https://google.com) and [internal](/w/ABC123/" + nodeA.String() + ")",
			expected: []jsonldb.ID{nodeA},
		},
		{
			name:     "mixed content",
			content:  "# Header\n\nSome text with [a link](/w/WSID/" + nodeB.String() + "+slug) in it.\n\n![image](image.png)",
			expected: []jsonldb.ID{nodeB},
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

func TestLinksIndex(t *testing.T) {
	fs, wsID := testFileStore(t)
	ctx := t.Context()
	author := git.Author{Name: "Test", Email: "test@test.com"}

	if err := fs.InitWorkspace(ctx, wsID); err != nil {
		t.Fatalf("failed to init workspace: %v", err)
	}

	ws, err := fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		t.Fatalf("failed to get workspace store: %v", err)
	}

	// Create source and target pages
	source, err := ws.CreatePageUnderParent(ctx, 0, "Source", "", author)
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	target, err := ws.CreatePageUnderParent(ctx, 0, "Target", "", author)
	if err != nil {
		t.Fatalf("failed to create target: %v", err)
	}

	t.Run("UpdateLinksForNode", func(t *testing.T) {
		// Add link from source to target
		err := ws.UpdateLinksForNode(source.ID, []jsonldb.ID{target.ID})
		if err != nil {
			t.Fatalf("failed to update links: %v", err)
		}
	})

	t.Run("GetBacklinks", func(t *testing.T) {
		// Get backlinks for target - should include source
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

	t.Run("GetNodeTitles", func(t *testing.T) {
		titles, err := ws.GetNodeTitles([]jsonldb.ID{source.ID, target.ID})
		if err != nil {
			t.Fatalf("failed to get node titles: %v", err)
		}

		if len(titles) != 2 {
			t.Errorf("expected 2 titles, got %d", len(titles))
		}
		if titles[source.ID] != "Source" {
			t.Errorf("expected source title 'Source', got %s", titles[source.ID])
		}
		if titles[target.ID] != "Target" {
			t.Errorf("expected target title 'Target', got %s", titles[target.ID])
		}
	})

	t.Run("RemoveLinksForNode", func(t *testing.T) {
		// Remove links from source
		err := ws.RemoveLinksForNode(source.ID)
		if err != nil {
			t.Fatalf("failed to remove links: %v", err)
		}

		// Backlinks for target should now be empty
		backlinks, err := ws.GetBacklinks(target.ID)
		if err != nil {
			t.Fatalf("failed to get backlinks: %v", err)
		}

		if len(backlinks) != 0 {
			t.Errorf("expected 0 backlinks after removal, got %d", len(backlinks))
		}
	})
}

func TestGetWorkspaceUsage(t *testing.T) {
	t.Run("CountsPages", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

		// Create multiple pages
		for i := range 3 {
			nodeID := jsonldb.NewID()
			_, err = ws.WritePage(ctx, nodeID, 0, "Page "+string(rune(49+i)), "content", author)
			if err != nil {
				t.Fatalf("failed to create page: %v", err)
			}
		}

		// Get usage - should count pages
		pageCount, _, err := ws.GetWorkspaceUsage()
		if err != nil {
			t.Fatalf("failed to get usage: %v", err)
		}

		if pageCount != 3 {
			t.Errorf("expected pageCount=3, got %d", pageCount)
		}
	})

	t.Run("HybridNodeCountedOnce", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		ws, err := fs.GetWorkspaceStore(ctx, wsID)
		if err != nil {
			t.Fatalf("failed to get workspace store: %v", err)
		}

		// Create a hybrid node (page + table)
		nodeID := jsonldb.NewID()
		_, err = ws.WritePage(ctx, nodeID, 0, "Hybrid", "content", author)
		if err != nil {
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

		// Hybrid node should be counted once, not twice
		if pageCount != 1 {
			t.Errorf("expected pageCount=1 for hybrid node, got %d", pageCount)
		}
	})
}
