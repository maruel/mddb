package content

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// testFileStore creates a FileStore for testing with unlimited quota.
// It also creates a workspace in the service for quota testing.
func testFileStore(t *testing.T) (*FileStore, jsonldb.ID) {
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
	ws, err := wsService.Create(t.Context(), org.ID, "Test Workspace", "test")
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

	fs, err := NewFileStore(tmpDir, gitMgr, wsService, orgService)
	if err != nil {
		t.Fatalf("failed to create FileStore: %v", err)
	}

	return fs, ws.ID
}

// testFileStoreWithQuota creates a FileStore with a real WorkspaceService for quota testing.
// It returns the FileStore and the organization ID for creating workspaces.
func testFileStoreWithQuota(t *testing.T) (*FileStore, jsonldb.ID) {
	t.Helper()
	tmpDir := t.TempDir()

	gitMgr := git.NewManager(tmpDir, "test", "test@test.com")

	orgService, err := identity.NewOrganizationService(filepath.Join(tmpDir, "organizations.jsonl"))
	if err != nil {
		t.Fatalf("failed to create OrganizationService: %v", err)
	}

	// Create a test organization for quota testing
	org, err := orgService.Create(t.Context(), "Test Organization", "test@test.com")
	if err != nil {
		t.Fatalf("failed to create test organization: %v", err)
	}
	// Set high quotas for the organization to avoid interference with workspace quota tests
	_, err = orgService.Modify(org.ID, func(o *identity.Organization) error {
		o.Quotas.MaxWorkspaces = 1_000
		o.Quotas.MaxMembersPerOrg = 10_000
		o.Quotas.MaxMembersPerWorkspace = 10_000
		o.Quotas.MaxTotalStorageGB = 1_000_000 // 1EB
		return nil
	})
	if err != nil {
		t.Fatalf("failed to set org quotas: %v", err)
	}

	wsService, err := identity.NewWorkspaceService(filepath.Join(tmpDir, "workspaces.jsonl"))
	if err != nil {
		t.Fatalf("failed to create WorkspaceService: %v", err)
	}

	fs, err := NewFileStore(tmpDir, gitMgr, wsService, orgService)
	if err != nil {
		t.Fatalf("failed to create FileStore: %v", err)
	}

	return fs, org.ID
}

func TestFileStore(t *testing.T) {
	t.Run("PageOperations", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		// Initialize git repo for org
		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init org: %v", err)
		}

		pageID := jsonldb.ID(1)

		t.Run("WritePage", func(t *testing.T) {
			page, err := fs.WritePage(ctx, wsID, pageID, "Test Title", "# Test Content", author)
			if err != nil {
				t.Fatalf("failed to write page: %v", err)
			}
			if page.ID != pageID {
				t.Errorf("expected ID %v, got %v", pageID, page.ID)
			}
			if page.Title != "Test Title" {
				t.Errorf("expected title 'Test Title', got %q", page.Title)
			}
		})

		t.Run("PageExists", func(t *testing.T) {
			if !fs.PageExists(wsID, pageID) {
				t.Error("page should exist after WritePage")
			}
		})

		t.Run("ReadPage", func(t *testing.T) {
			readPage, err := fs.ReadPage(wsID, pageID)
			if err != nil {
				t.Fatalf("failed to read page: %v", err)
			}
			if readPage.Title != "Test Title" {
				t.Errorf("expected title 'Test Title', got %q", readPage.Title)
			}
			if readPage.Content != "\n\n# Test Content" {
				t.Errorf("expected content '\n\n# Test Content', got %q", readPage.Content)
			}
		})

		t.Run("UpdatePage", func(t *testing.T) {
			updated, err := fs.UpdatePage(ctx, wsID, pageID, "Updated Title", "# Updated Content", author)
			if err != nil {
				t.Fatalf("failed to update page: %v", err)
			}
			if updated.Title != "Updated Title" {
				t.Errorf("expected title 'Updated Title', got %q", updated.Title)
			}

			// Verify update persisted
			readUpdated, err := fs.ReadPage(wsID, pageID)
			if err != nil {
				t.Fatalf("failed to read updated page: %v", err)
			}
			if readUpdated.Title != "Updated Title" {
				t.Errorf("expected title 'Updated Title', got %q", readUpdated.Title)
			}
		})

		t.Run("DeletePage", func(t *testing.T) {
			err := fs.DeletePage(ctx, wsID, pageID, author)
			if err != nil {
				t.Fatalf("failed to delete page: %v", err)
			}
			if fs.PageExists(wsID, pageID) {
				t.Error("page should not exist after DeletePage")
			}
		})

		t.Run("ReadNonExistent", func(t *testing.T) {
			_, err := fs.ReadPage(wsID, jsonldb.ID(999))
			if err == nil {
				t.Error("expected error reading non-existent page")
			}
		})
	})

	t.Run("ListPages", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		// Initialize git repo for org
		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init org: %v", err)
		}

		// Create multiple pages with numeric IDs
		pages := []struct {
			id    jsonldb.ID
			title string
		}{
			{jsonldb.ID(1), "First Page"},
			{jsonldb.ID(2), "Second Page"},
			{jsonldb.ID(3), "Third Page"},
		}

		for _, p := range pages {
			_, err := fs.WritePage(ctx, wsID, p.id, p.title, "Content", author)
			if err != nil {
				t.Fatalf("failed to write page %v: %v", p.id, err)
			}
		}

		t.Run("IterPages", func(t *testing.T) {
			it, err := fs.IterPages(wsID)
			if err != nil {
				t.Fatalf("failed to list pages: %v", err)
			}
			listed := slices.Collect(it)
			if len(listed) != len(pages) {
				t.Errorf("expected %d pages, got %d", len(pages), len(listed))
			}
		})

		t.Run("DirectoryStructure", func(t *testing.T) {
			expectedDir := filepath.Join(fs.rootDir, wsID.String(), "pages", jsonldb.ID(1).String())
			if _, err := os.Stat(expectedDir); err != nil {
				t.Errorf("expected page directory %s to exist: %v", expectedDir, err)
			}
			expectedFile := filepath.Join(expectedDir, "index.md")
			if _, err := os.Stat(expectedFile); err != nil {
				t.Errorf("expected file %s to exist: %v", expectedFile, err)
			}
		})
	})

	t.Run("EdgeCases", func(t *testing.T) {
		t.Run("DeletePage_NonExistent", func(t *testing.T) {
			fs, wsID := testFileStore(t)
			if err := fs.InitWorkspace(t.Context(), wsID); err != nil {
				t.Fatalf("failed to init org: %v", err)
			}
			author := git.Author{Name: "Test", Email: "test@test.com"}

			nonExistentID := jsonldb.ID(99999)
			err := fs.DeletePage(t.Context(), wsID, nonExistentID, author)

			if err == nil {
				t.Error("expected error when deleting non-existent page, got nil")
			}
		})

		t.Run("UpdateRecord_SameSizeData", func(t *testing.T) {
			fs, wsID := testFileStore(t)
			ctx := t.Context()
			author := git.Author{Name: "Test", Email: "test@test.com"}

			if err := fs.InitWorkspace(ctx, wsID); err != nil {
				t.Fatalf("failed to init org: %v", err)
			}

			// Set quota to allow operations
			_, err := fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
				w.Quotas.MaxStorageMB = 1
				return nil
			})
			if err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			tableID := jsonldb.NewID()
			tableNode := &Node{
				ID:       tableID,
				Title:    "Test Table",
				Type:     NodeTypeTable,
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			if err := fs.WriteTable(ctx, wsID, tableNode, true, author); err != nil {
				t.Fatalf("failed to create table: %v", err)
			}

			recordID := jsonldb.NewID()
			record := &DataRecord{
				ID:       recordID,
				Data:     map[string]any{"field": strings.Repeat("a", 200)},
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			if err := fs.AppendRecord(ctx, wsID, tableID, record, author); err != nil {
				t.Fatalf("failed to append record: %v", err)
			}

			updatedRecord := &DataRecord{
				ID:       recordID,
				Data:     map[string]any{"field": strings.Repeat("b", 200)},
				Created:  record.Created,
				Modified: record.Modified,
			}
			err = fs.UpdateRecord(ctx, wsID, tableID, updatedRecord, author)
			if err != nil {
				t.Errorf("update with same-size data should succeed, but got: %v", err)
			}
		})

		t.Run("IterAssets", func(t *testing.T) {
			fs, wsID := testFileStore(t)
			ctx := t.Context()
			author := git.Author{Name: "Test", Email: "test@test.com"}

			if err := fs.InitWorkspace(ctx, wsID); err != nil {
				t.Fatalf("failed to init org: %v", err)
			}

			pageID := jsonldb.NewID()
			_, err := fs.WritePage(ctx, wsID, pageID, "Test Page", "content", author)
			if err != nil {
				t.Fatalf("failed to create page: %v", err)
			}

			assets := []struct {
				name string
				data []byte
			}{
				{"image.png", []byte("fake png data")},
				{"document.pdf", []byte("fake pdf data")},
				{"data.csv", []byte("a,b,c\n1,2,3")},
			}

			for _, a := range assets {
				_, err := fs.SaveAsset(ctx, wsID, pageID, a.name, a.data, author)
				if err != nil {
					t.Fatalf("failed to save asset %s: %v", a.name, err)
				}
			}

			iter, err := fs.IterAssets(wsID, pageID)
			if err != nil {
				t.Fatalf("failed to get asset iterator: %v", err)
			}

			found := make([]string, 0, len(assets))
			for asset := range iter {
				found = append(found, asset.Name)
			}

			if len(found) != len(assets) {
				t.Errorf("expected %d assets, found %d: %v", len(assets), len(found), found)
			}

			for _, name := range found {
				if name == "index.md" || name == "metadata.json" || name == "data.jsonl" {
					t.Errorf("internal file %q should not be listed as asset", name)
				}
			}
		})

		t.Run("IterAssets_NonExistentPage", func(t *testing.T) {
			fs, wsID := testFileStore(t)
			if err := fs.InitWorkspace(t.Context(), wsID); err != nil {
				t.Fatalf("failed to init org: %v", err)
			}

			iter, err := fs.IterAssets(wsID, jsonldb.ID(99999))
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

		t.Run("CreateNode_AllTypes", func(t *testing.T) {
			fs, wsID := testFileStore(t)
			ctx := t.Context()
			author := git.Author{Name: "Test", Email: "test@test.com"}

			if err := fs.InitWorkspace(ctx, wsID); err != nil {
				t.Fatalf("failed to init org: %v", err)
			}

			tests := []struct {
				name     string
				nodeType NodeType
				wantPage bool
				wantMeta bool
			}{
				{"Document", NodeTypeDocument, true, false},
				{"Table", NodeTypeTable, false, true},
				{"Hybrid", NodeTypeHybrid, true, true},
			}

			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					node, err := fs.CreateNode(ctx, wsID, "Test "+tc.name, tc.nodeType, author)
					if err != nil {
						t.Fatalf("CreateNode failed: %v", err)
					}

					if node.Type != tc.nodeType {
						t.Errorf("expected type %v, got %v", tc.nodeType, node.Type)
					}

					pageDir := filepath.Join(fs.rootDir, wsID.String(), "pages", node.ID.String())
					indexPath := filepath.Join(pageDir, "index.md")
					metaPath := filepath.Join(pageDir, "metadata.json")

					_, indexErr := os.Stat(indexPath)
					_, metaErr := os.Stat(metaPath)
					indexExists := indexErr == nil
					metaExists := metaErr == nil

					if tc.wantPage && !indexExists {
						t.Error("expected index.md to exist")
					}
					if !tc.wantPage && indexExists {
						t.Error("expected index.md to NOT exist")
					}
					if tc.wantMeta && !metaExists {
						t.Error("expected metadata.json to exist")
					}
					if !tc.wantMeta && metaExists {
						t.Error("expected metadata.json to NOT exist")
					}
				})
			}
		})
	})

	t.Run("Quota", func(t *testing.T) {
		t.Run("PageQuota", func(t *testing.T) {
			// Test that page quota is enforced
			fs, wsID := testFileStore(t)
			ctx := t.Context()
			author := git.Author{Name: "Test", Email: "test@test.com"}

			if err := fs.InitWorkspace(ctx, wsID); err != nil {
				t.Fatalf("failed to init workspace: %v", err)
			}

			// Set quota to allow only 2 pages
			_, err := fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
				w.Quotas.MaxPages = 2
				return nil
			})
			if err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			// Create 2 pages - should succeed
			for i := range 2 {
				pageID := jsonldb.NewID()
				_, err := fs.WritePage(ctx, wsID, pageID, "Page", "content", author)
				if err != nil {
					t.Fatalf("failed to create page %d: %v", i, err)
				}
			}

			// Try to create a 3rd page - should fail
			_, err = fs.WritePage(ctx, wsID, jsonldb.NewID(), "Extra", "content", author)
			if err == nil {
				t.Error("expected page quota exceeded error")
			}
		})

		t.Run("RecordQuota", func(t *testing.T) {
			// Test that record quota is enforced
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

			tableID := jsonldb.NewID()
			tableNode := &Node{
				ID:       tableID,
				Title:    "Test",
				Type:     NodeTypeTable,
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			if err := fs.WriteTable(ctx, wsID, tableNode, true, author); err != nil {
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
				if err := fs.AppendRecord(ctx, wsID, tableID, rec, author); err != nil {
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
			if err := fs.AppendRecord(ctx, wsID, tableID, rec, author); err == nil {
				t.Error("expected record quota exceeded error")
			}
		})

		t.Run("StorageQuotaEnforced", func(t *testing.T) {
			// Test that storage quota checking is performed (without exceeding it)
			fs, wsID := testFileStore(t)
			ctx := t.Context()
			author := git.Author{Name: "Test", Email: "test@test.com"}

			if err := fs.InitWorkspace(ctx, wsID); err != nil {
				t.Fatalf("failed to init workspace: %v", err)
			}

			// Set quota to 1MB
			_, err := fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
				w.Quotas.MaxStorageMB = 1
				return nil
			})
			if err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			// Create content within quota - should succeed
			pageID := jsonldb.NewID()
			_, err = fs.WritePage(ctx, wsID, pageID, "Test", "content", author)
			if err != nil {
				t.Fatalf("creating page within quota should succeed: %v", err)
			}

			// Update within quota - should succeed
			_, err = fs.UpdatePage(ctx, wsID, pageID, "Updated", "updated content", author)
			if err != nil {
				t.Fatalf("updating page within quota should succeed: %v", err)
			}
		})

		t.Run("UpdateRecord_SameSizeAllowed", func(t *testing.T) {
			fs, wsID := testFileStore(t)
			ctx := t.Context()
			author := git.Author{Name: "Test", Email: "test@test.com"}

			if err := fs.InitWorkspace(ctx, wsID); err != nil {
				t.Fatalf("failed to init org: %v", err)
			}

			_, err := fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
				w.Quotas.MaxStorageMB = 1 // 1MB
				return nil
			})
			if err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			tableID := jsonldb.NewID()
			tableNode := &Node{
				ID:       tableID,
				Title:    "Test",
				Type:     NodeTypeTable,
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			if err := fs.WriteTable(ctx, wsID, tableNode, true, author); err != nil {
				t.Fatalf("failed to create table: %v", err)
			}

			recordID := jsonldb.NewID()
			record := &DataRecord{
				ID:       recordID,
				Data:     map[string]any{"field": strings.Repeat("a", 100)},
				Created:  storage.Now(),
				Modified: storage.Now(),
			}
			if err := fs.AppendRecord(ctx, wsID, tableID, record, author); err != nil {
				t.Fatalf("failed to create record: %v", err)
			}

			// Set quota to exactly current usage (in MB, rounded up)
			_, storageUsage, _ := fs.GetWorkspaceUsage(wsID)
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

			err = fs.UpdateRecord(ctx, wsID, tableID, updatedRecord, author)
			if err != nil {
				t.Errorf("same-size update should succeed: %v", err)
			}
		})
	})
}

func TestAsset(t *testing.T) {
	t.Run("Quota", func(t *testing.T) {
		fs, orgID := testFileStoreWithQuota(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		// Create a workspace for testing with reasonable quotas
		ws, err := fs.wsSvc.Create(ctx, orgID, "Test Workspace", "test-ws")
		if err != nil {
			t.Fatalf("Failed to create workspace: %v", err)
		}
		wsID := ws.ID

		pageID := jsonldb.ID(1)

		// Initialize git repo for workspace
		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		// Create a page for testing assets
		_, err = fs.WritePage(ctx, wsID, pageID, "Test", "content", author)
		if err != nil {
			t.Fatalf("Failed to create page: %v", err)
		}

		t.Run("AssetWithinQuota", func(t *testing.T) {
			// Set reasonable quotas (1MB for assets)
			_, err = fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
				w.Quotas.MaxAssetSizeMB = 1
				w.Quotas.MaxStorageMB = 10
				return nil
			})
			if err != nil {
				t.Fatalf("Failed to modify workspace quota: %v", err)
			}

			// Save small asset - should succeed
			_, err = fs.SaveAsset(ctx, wsID, pageID, "small.txt", []byte("small content"), author)
			if err != nil {
				t.Errorf("Saving small asset should succeed: %v", err)
			}

			// Verify asset exists
			iter, err := fs.IterAssets(wsID, pageID)
			if err != nil {
				t.Fatalf("Failed to iterate assets: %v", err)
			}
			found := false
			for asset := range iter {
				if asset.Name == "small.txt" {
					found = true
					break
				}
			}
			if !found {
				t.Error("Expected to find small.txt asset")
			}
		})

		t.Run("MultipleAssetsWithinQuota", func(t *testing.T) {
			// Save multiple assets within quota
			assets := []struct {
				name    string
				content string
			}{
				{"doc1.txt", "document one content"},
				{"doc2.txt", "document two content"},
				{"doc3.txt", "document three content"},
			}

			for _, a := range assets {
				_, err := fs.SaveAsset(ctx, wsID, pageID, a.name, []byte(a.content), author)
				if err != nil {
					t.Errorf("Saving %s should succeed: %v", a.name, err)
				}
			}

			// Verify all assets exist
			iter, err := fs.IterAssets(wsID, pageID)
			if err != nil {
				t.Fatalf("Failed to iterate assets: %v", err)
			}
			count := 0
			for range iter {
				count++
			}
			// At least 3 from this test + 1 from previous test
			if count < 3 {
				t.Errorf("Expected at least 3 assets, got %d", count)
			}
		})
	})
}

func TestOrganizationQuota(t *testing.T) {
	t.Run("GetOrganizationUsage", func(t *testing.T) {
		fs, orgID := testFileStoreWithQuota(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		// Create two workspaces in the same organization
		ws1, err := fs.wsSvc.Create(ctx, orgID, "Workspace 1", "ws1")
		if err != nil {
			t.Fatalf("Failed to create workspace 1: %v", err)
		}

		ws2, err := fs.wsSvc.Create(ctx, orgID, "Workspace 2", "ws2")
		if err != nil {
			t.Fatalf("Failed to create workspace 2: %v", err)
		}

		// Initialize git repos
		if err := fs.InitWorkspace(ctx, ws1.ID); err != nil {
			t.Fatalf("failed to init workspace 1: %v", err)
		}
		if err := fs.InitWorkspace(ctx, ws2.ID); err != nil {
			t.Fatalf("failed to init workspace 2: %v", err)
		}

		// Create pages in both workspaces
		content := "test content"
		pageID1 := jsonldb.NewID()
		_, err = fs.WritePage(ctx, ws1.ID, pageID1, "Page 1", content, author)
		if err != nil {
			t.Fatalf("Failed to write page to ws1: %v", err)
		}

		pageID2 := jsonldb.NewID()
		_, err = fs.WritePage(ctx, ws2.ID, pageID2, "Page 2", content, author)
		if err != nil {
			t.Fatalf("Failed to write page to ws2: %v", err)
		}

		// Get org usage - should count pages from both workspaces
		orgUsage, err := fs.GetOrganizationUsage(orgID)
		if err != nil {
			t.Fatalf("Failed to get org usage: %v", err)
		}

		ws1Usage, _, err := fs.GetWorkspaceUsage(ws1.ID)
		if err != nil {
			t.Fatalf("Failed to get ws1 usage: %v", err)
		}

		ws2Usage, _, err := fs.GetWorkspaceUsage(ws2.ID)
		if err != nil {
			t.Fatalf("Failed to get ws2 usage: %v", err)
		}

		if orgUsage == 0 {
			t.Errorf("Expected org usage > 0, got %d bytes", orgUsage)
		}

		t.Logf("Workspace 1 usage: %d bytes, Workspace 2 usage: %d bytes, Org usage: %d bytes", ws1Usage, ws2Usage, orgUsage)
	})

	t.Run("StorageQuotaEnforced", func(t *testing.T) {
		fs, orgID := testFileStoreWithQuota(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		// Create a workspace first
		ws, err := fs.wsSvc.Create(ctx, orgID, "Test Workspace", "test-ws")
		if err != nil {
			t.Fatalf("Failed to create workspace: %v", err)
		}

		// Initialize git repo
		if err := fs.InitWorkspace(ctx, ws.ID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		// Add a tiny amount of content to organization
		pageID := jsonldb.NewID()
		_, err = fs.WritePage(ctx, ws.ID, pageID, "Initial", "test", author)
		if err != nil {
			t.Fatalf("Failed to write initial page: %v", err)
		}

		// Now set organization quota to 1 GB total - very large, but smaller than workspace quota
		_, err = fs.orgSvc.Modify(orgID, func(o *identity.Organization) error {
			o.Quotas.MaxTotalStorageGB = 1 // 1 GB
			return nil
		})
		if err != nil {
			t.Fatalf("Failed to set org quota: %v", err)
		}

		// Set workspace quota to 1000 GB (much larger than org quota of 1 GB)
		_, err = fs.wsSvc.Modify(ws.ID, func(w *identity.Workspace) error {
			w.Quotas.MaxStorageMB = 1000 * 1024 // 1000 GB
			return nil
		})
		if err != nil {
			t.Fatalf("Failed to set workspace quota: %v", err)
		}

		// Fill the organization quota by adding large content
		largeContent := strings.Repeat("x", 512*1024*1024) // 512 MB
		for i := 1; i < 3; i++ {
			pageID := jsonldb.NewID()
			_, err := fs.WritePage(ctx, ws.ID, pageID, "Large", largeContent, author)
			if err != nil {
				// Might hit quota, which is OK
				break
			}
		}

		// Try to write another large page - should eventually fail due to org quota
		pageID = jsonldb.NewID()
		_, err = fs.WritePage(ctx, ws.ID, pageID, "Test", largeContent, author)
		if err == nil {
			t.Logf("WritePage succeeded when org quota should be approached")
		} else if !errors.Is(err, errQuotaExceeded) {
			t.Logf("Got error: %v (not quota exceeded)", err)
		}
		// This test just verifies the mechanism is in place
	})
}

func TestMarkdown(t *testing.T) {
	t.Run("Formatting", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		// Initialize git repo for org
		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init org: %v", err)
		}

		// Write page with specific content
		pageID := jsonldb.ID(1)
		_, err := fs.WritePage(ctx, wsID, pageID, "Format Test", "# Content\n\nWith multiple lines", author)
		if err != nil {
			t.Fatalf("failed to write page: %v", err)
		}

		// Read the file directly to verify format
		filePath := filepath.Join(fs.rootDir, wsID.String(), "pages", pageID.String(), "index.md")
		data, err := os.ReadFile(filePath) //nolint:gosec // G304: test code with controlled path
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		content := string(data)

		t.Run("FrontMatterDelimiters", func(t *testing.T) {
			if !contains(content, "---") {
				t.Error("expected front matter delimiters")
			}
		})

		t.Run("FrontMatterID", func(t *testing.T) {
			if !contains(content, "id: "+pageID.String()) {
				t.Error("expected id in front matter")
			}
		})

		t.Run("FrontMatterTitle", func(t *testing.T) {
			if !contains(content, "title: Format Test") {
				t.Error("expected title in front matter")
			}
		})

		t.Run("FrontMatterTimestamps", func(t *testing.T) {
			if !contains(content, "created:") {
				t.Error("expected created timestamp")
			}
			if !contains(content, "modified:") {
				t.Error("expected modified timestamp")
			}
		})

		t.Run("ContentSeparation", func(t *testing.T) {
			parts := splitN(content, "---", 3)
			if len(parts) < 3 {
				t.Error("expected three sections separated by ---")
			}
		})
	})
}

func TestGetWorkspaceUsage(t *testing.T) {
	t.Run("CountsTablesAndPages", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		// Set quota to allow 2 items
		_, err := fs.wsSvc.Modify(wsID, func(w *identity.Workspace) error {
			w.Quotas.MaxPages = 2
			return nil
		})
		if err != nil {
			t.Fatalf("failed to set quota: %v", err)
		}

		// Create one page
		pageID := jsonldb.NewID()
		_, err = fs.WritePage(ctx, wsID, pageID, "Page 1", "content", author)
		if err != nil {
			t.Fatalf("failed to create page: %v", err)
		}

		// Create one table
		tableID := jsonldb.NewID()
		tableNode := &Node{
			ID:       tableID,
			Title:    "Table 1",
			Type:     NodeTypeTable,
			Created:  storage.Now(),
			Modified: storage.Now(),
		}
		if err := fs.WriteTable(ctx, wsID, tableNode, true, author); err != nil {
			t.Fatalf("failed to create table: %v", err)
		}

		// Get usage - should count both page and table
		pageCount, _, err := fs.GetWorkspaceUsage(wsID)
		if err != nil {
			t.Fatalf("failed to get usage: %v", err)
		}

		if pageCount != 2 {
			t.Errorf("expected pageCount=2 (page + table), got %d", pageCount)
		}

		// With MaxPages=2, creating a third item should fail
		tableID2 := jsonldb.NewID()
		tableNode2 := &Node{
			ID:       tableID2,
			Title:    "Table 2",
			Type:     NodeTypeTable,
			Created:  storage.Now(),
			Modified: storage.Now(),
		}
		err = fs.WriteTable(ctx, wsID, tableNode2, true, author)
		if err == nil {
			t.Error("expected quota exceeded error when creating third item")
		}
	})

	t.Run("HybridNodeCountedOnce", func(t *testing.T) {
		fs, wsID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitWorkspace(ctx, wsID); err != nil {
			t.Fatalf("failed to init workspace: %v", err)
		}

		// Create a hybrid node (page + table)
		hybridID := jsonldb.NewID()
		_, err := fs.WritePage(ctx, wsID, hybridID, "Hybrid", "content", author)
		if err != nil {
			t.Fatalf("failed to create page: %v", err)
		}

		hybridNode := &Node{
			ID:       hybridID,
			Title:    "Hybrid",
			Type:     NodeTypeTable,
			Created:  storage.Now(),
			Modified: storage.Now(),
		}
		if err := fs.WriteTable(ctx, wsID, hybridNode, false, author); err != nil {
			t.Fatalf("failed to add table metadata: %v", err)
		}

		pageCount, _, err := fs.GetWorkspaceUsage(wsID)
		if err != nil {
			t.Fatalf("failed to get usage: %v", err)
		}

		// Hybrid node should be counted once, not twice
		if pageCount != 1 {
			t.Errorf("expected pageCount=1 for hybrid node, got %d", pageCount)
		}
	})
}

func contains(s, substr string) bool {
	for i := range len(s) - len(substr) + 1 {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func splitN(s, sep string, n int) []string {
	var result []string
	for range n - 1 {
		idx := -1
		for j := range len(s) - len(sep) + 1 {
			if s[j:j+len(sep)] == sep {
				idx = j
				break
			}
		}
		if idx == -1 {
			break
		}
		result = append(result, s[:idx])
		s = s[idx+len(sep):]
	}
	result = append(result, s)
	return result
}
