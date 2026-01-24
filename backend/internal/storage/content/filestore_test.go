package content

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// testFileStore creates a FileStore for testing with unlimited quota.
// It also creates an org in the service for quota testing.
func testFileStore(t *testing.T) (*FileStore, jsonldb.ID) {
	t.Helper()
	tmpDir := t.TempDir()

	gitMgr := git.NewManager(tmpDir, "test", "test@test.com")

	orgService, err := identity.NewOrganizationService(filepath.Join(tmpDir, "organizations.jsonl"))
	if err != nil {
		t.Fatalf("failed to create OrganizationService: %v", err)
	}

	// Create a test organization with very high quotas (practically unlimited)
	org, err := orgService.Create(t.Context(), "Test Org")
	if err != nil {
		t.Fatalf("failed to create test org: %v", err)
	}
	_, err = orgService.Modify(org.ID, func(o *identity.Organization) error {
		o.Quotas.MaxPages = 1_000_000
		o.Quotas.MaxStorage = 1_000_000_000_000 // 1TB
		o.Quotas.MaxRecordsPerTable = 1_000_000
		o.Quotas.MaxAssetSize = 1_000_000_000 // 1GB
		return nil
	})
	if err != nil {
		t.Fatalf("failed to set unlimited quotas: %v", err)
	}

	fs, err := NewFileStore(tmpDir, gitMgr, orgService)
	if err != nil {
		t.Fatalf("failed to create FileStore: %v", err)
	}

	return fs, org.ID
}

// testFileStoreWithQuota creates a FileStore with a real OrganizationService for quota testing.
func testFileStoreWithQuota(t *testing.T) *FileStore {
	t.Helper()
	tmpDir := t.TempDir()

	gitMgr := git.NewManager(tmpDir, "test", "test@test.com")

	orgService, err := identity.NewOrganizationService(filepath.Join(tmpDir, "organizations.jsonl"))
	if err != nil {
		t.Fatalf("failed to create OrganizationService: %v", err)
	}

	fs, err := NewFileStore(tmpDir, gitMgr, orgService)
	if err != nil {
		t.Fatalf("failed to create FileStore: %v", err)
	}

	return fs
}

func TestFileStore(t *testing.T) {
	t.Run("PageOperations", func(t *testing.T) {
		fs, orgID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		// Initialize git repo for org
		if err := fs.InitOrg(ctx, orgID); err != nil {
			t.Fatalf("failed to init org: %v", err)
		}

		pageID := jsonldb.ID(1)

		t.Run("WritePage", func(t *testing.T) {
			page, err := fs.WritePage(ctx, orgID, pageID, "Test Title", "# Test Content", author)
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
			if !fs.PageExists(orgID, pageID) {
				t.Error("page should exist after WritePage")
			}
		})

		t.Run("ReadPage", func(t *testing.T) {
			readPage, err := fs.ReadPage(orgID, pageID)
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
			updated, err := fs.UpdatePage(ctx, orgID, pageID, "Updated Title", "# Updated Content", author)
			if err != nil {
				t.Fatalf("failed to update page: %v", err)
			}
			if updated.Title != "Updated Title" {
				t.Errorf("expected title 'Updated Title', got %q", updated.Title)
			}

			// Verify update persisted
			readUpdated, err := fs.ReadPage(orgID, pageID)
			if err != nil {
				t.Fatalf("failed to read updated page: %v", err)
			}
			if readUpdated.Title != "Updated Title" {
				t.Errorf("expected title 'Updated Title', got %q", readUpdated.Title)
			}
		})

		t.Run("DeletePage", func(t *testing.T) {
			err := fs.DeletePage(ctx, orgID, pageID, author)
			if err != nil {
				t.Fatalf("failed to delete page: %v", err)
			}
			if fs.PageExists(orgID, pageID) {
				t.Error("page should not exist after DeletePage")
			}
		})

		t.Run("ReadNonExistent", func(t *testing.T) {
			_, err := fs.ReadPage(orgID, jsonldb.ID(999))
			if err == nil {
				t.Error("expected error reading non-existent page")
			}
		})
	})

	t.Run("ListPages", func(t *testing.T) {
		fs, orgID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		// Initialize git repo for org
		if err := fs.InitOrg(ctx, orgID); err != nil {
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
			_, err := fs.WritePage(ctx, orgID, p.id, p.title, "Content", author)
			if err != nil {
				t.Fatalf("failed to write page %v: %v", p.id, err)
			}
		}

		t.Run("IterPages", func(t *testing.T) {
			it, err := fs.IterPages(orgID)
			if err != nil {
				t.Fatalf("failed to list pages: %v", err)
			}
			listed := slices.Collect(it)
			if len(listed) != len(pages) {
				t.Errorf("expected %d pages, got %d", len(pages), len(listed))
			}
		})

		t.Run("DirectoryStructure", func(t *testing.T) {
			expectedDir := filepath.Join(fs.rootDir, orgID.String(), "pages", jsonldb.ID(1).String())
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
			fs, orgID := testFileStore(t)
			if err := fs.InitOrg(t.Context(), orgID); err != nil {
				t.Fatalf("failed to init org: %v", err)
			}
			author := git.Author{Name: "Test", Email: "test@test.com"}

			nonExistentID := jsonldb.ID(99999)
			err := fs.DeletePage(t.Context(), orgID, nonExistentID, author)

			if err == nil {
				t.Error("expected error when deleting non-existent page, got nil")
			}
		})

		t.Run("UpdateRecord_SameSizeData", func(t *testing.T) {
			fs, orgID := testFileStore(t)
			ctx := t.Context()
			author := git.Author{Name: "Test", Email: "test@test.com"}

			if err := fs.InitOrg(ctx, orgID); err != nil {
				t.Fatalf("failed to init org: %v", err)
			}

			// Set quota to allow operations
			_, err := fs.orgSvc.Modify(orgID, func(o *identity.Organization) error {
				o.Quotas.MaxStorage = 1000
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
				Created:  time.Now(),
				Modified: time.Now(),
			}
			if err := fs.WriteTable(ctx, orgID, tableNode, true, author); err != nil {
				t.Fatalf("failed to create table: %v", err)
			}

			recordID := jsonldb.NewID()
			record := &DataRecord{
				ID:       recordID,
				Data:     map[string]any{"field": strings.Repeat("a", 200)},
				Created:  time.Now(),
				Modified: time.Now(),
			}
			if err := fs.AppendRecord(ctx, orgID, tableID, record, author); err != nil {
				t.Fatalf("failed to append record: %v", err)
			}

			updatedRecord := &DataRecord{
				ID:       recordID,
				Data:     map[string]any{"field": strings.Repeat("b", 200)},
				Created:  record.Created,
				Modified: time.Now(),
			}

			err = fs.UpdateRecord(ctx, orgID, tableID, updatedRecord, author)
			if err != nil {
				t.Errorf("update with same-size data should succeed, but got: %v", err)
			}
		})

		t.Run("IterAssets", func(t *testing.T) {
			fs, orgID := testFileStore(t)
			ctx := t.Context()
			author := git.Author{Name: "Test", Email: "test@test.com"}

			if err := fs.InitOrg(ctx, orgID); err != nil {
				t.Fatalf("failed to init org: %v", err)
			}

			pageID := jsonldb.NewID()
			_, err := fs.WritePage(ctx, orgID, pageID, "Test Page", "content", author)
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
				_, err := fs.SaveAsset(ctx, orgID, pageID, a.name, a.data, author)
				if err != nil {
					t.Fatalf("failed to save asset %s: %v", a.name, err)
				}
			}

			iter, err := fs.IterAssets(orgID, pageID)
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
			fs, orgID := testFileStore(t)
			if err := fs.InitOrg(t.Context(), orgID); err != nil {
				t.Fatalf("failed to init org: %v", err)
			}

			iter, err := fs.IterAssets(orgID, jsonldb.ID(99999))
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
			fs, orgID := testFileStore(t)
			ctx := t.Context()
			author := git.Author{Name: "Test", Email: "test@test.com"}

			if err := fs.InitOrg(ctx, orgID); err != nil {
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
					node, err := fs.CreateNode(ctx, orgID, "Test "+tc.name, tc.nodeType, author)
					if err != nil {
						t.Fatalf("CreateNode failed: %v", err)
					}

					if node.Type != tc.nodeType {
						t.Errorf("expected type %v, got %v", tc.nodeType, node.Type)
					}

					pageDir := filepath.Join(fs.rootDir, orgID.String(), "pages", node.ID.String())
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
		t.Run("UpdatePage_StorageQuota", func(t *testing.T) {
			fs, orgID := testFileStore(t)
			ctx := t.Context()
			author := git.Author{Name: "Test", Email: "test@test.com"}

			if err := fs.InitOrg(ctx, orgID); err != nil {
				t.Fatalf("failed to init org: %v", err)
			}

			_, err := fs.orgSvc.Modify(orgID, func(o *identity.Organization) error {
				o.Quotas.MaxStorage = 500
				return nil
			})
			if err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			pageID := jsonldb.NewID()
			_, err = fs.WritePage(ctx, orgID, pageID, "Small", "x", author)
			if err != nil {
				t.Fatalf("failed to create small page: %v", err)
			}

			// Update to exceed storage quota
			largeContent := make([]byte, 1000)
			for i := range largeContent {
				largeContent[i] = 'x'
			}

			_, err = fs.UpdatePage(ctx, orgID, pageID, "Large", string(largeContent), author)
			if err == nil {
				t.Error("expected quota exceeded error when updating page to exceed storage")
			}
		})

		t.Run("CreateNode_StorageQuota", func(t *testing.T) {
			fs, orgID := testFileStore(t)
			ctx := t.Context()
			author := git.Author{Name: "Test", Email: "test@test.com"}

			if err := fs.InitOrg(ctx, orgID); err != nil {
				t.Fatalf("failed to init org: %v", err)
			}

			// Set very low storage quota (50 bytes - less than a single node's files)
			_, err := fs.orgSvc.Modify(orgID, func(o *identity.Organization) error {
				o.Quotas.MaxStorage = 50
				o.Quotas.MaxPages = 100
				return nil
			})
			if err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			_, err = fs.CreateNode(ctx, orgID, "Test Node", NodeTypeDocument, author)
			if err == nil {
				t.Error("expected storage quota exceeded error")
			}
		})

		t.Run("WriteTable_UpdateStorageQuota", func(t *testing.T) {
			fs, orgID := testFileStore(t)
			ctx := t.Context()
			author := git.Author{Name: "Test", Email: "test@test.com"}

			if err := fs.InitOrg(ctx, orgID); err != nil {
				t.Fatalf("failed to init org: %v", err)
			}

			_, err := fs.orgSvc.Modify(orgID, func(o *identity.Organization) error {
				o.Quotas.MaxStorage = 1000
				return nil
			})
			if err != nil {
				t.Fatalf("failed to set quota: %v", err)
			}

			tableID := jsonldb.NewID()
			tableNode := &Node{
				ID:       tableID,
				Title:    "Small",
				Type:     NodeTypeTable,
				Created:  time.Now(),
				Modified: time.Now(),
			}
			if err := fs.WriteTable(ctx, orgID, tableNode, true, author); err != nil {
				t.Fatalf("failed to create table: %v", err)
			}

			// Reduce storage quota
			_, err = fs.orgSvc.Modify(orgID, func(o *identity.Organization) error {
				o.Quotas.MaxStorage = 100
				return nil
			})
			if err != nil {
				t.Fatalf("failed to reduce quota: %v", err)
			}

			// Update with many properties to exceed quota
			tableNode.Properties = make([]Property, 50)
			for i := range tableNode.Properties {
				tableNode.Properties[i] = Property{
					Name: strings.Repeat("x", 50),
					Type: PropertyTypeText,
				}
			}
			tableNode.Modified = time.Now()

			err = fs.WriteTable(ctx, orgID, tableNode, false, author)
			if err == nil {
				t.Error("expected storage quota exceeded error on update")
			}
		})

		t.Run("UpdateRecord_SameSizeAllowed", func(t *testing.T) {
			fs, orgID := testFileStore(t)
			ctx := t.Context()
			author := git.Author{Name: "Test", Email: "test@test.com"}

			if err := fs.InitOrg(ctx, orgID); err != nil {
				t.Fatalf("failed to init org: %v", err)
			}

			_, err := fs.orgSvc.Modify(orgID, func(o *identity.Organization) error {
				o.Quotas.MaxStorage = 500
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
				Created:  time.Now(),
				Modified: time.Now(),
			}
			if err := fs.WriteTable(ctx, orgID, tableNode, true, author); err != nil {
				t.Fatalf("failed to create table: %v", err)
			}

			recordID := jsonldb.NewID()
			record := &DataRecord{
				ID:       recordID,
				Data:     map[string]any{"field": strings.Repeat("a", 100)},
				Created:  time.Now(),
				Modified: time.Now(),
			}
			if err := fs.AppendRecord(ctx, orgID, tableID, record, author); err != nil {
				t.Fatalf("failed to create record: %v", err)
			}

			// Set quota to exactly current usage
			_, storageUsage, _ := fs.GetOrganizationUsage(orgID)
			_, err = fs.orgSvc.Modify(orgID, func(o *identity.Organization) error {
				o.Quotas.MaxStorage = storageUsage
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
				Modified: time.Now(),
			}

			err = fs.UpdateRecord(ctx, orgID, tableID, updatedRecord, author)
			if err != nil {
				t.Errorf("same-size update should succeed: %v", err)
			}
		})
	})
}

func TestAsset(t *testing.T) {
	t.Run("Quota", func(t *testing.T) {
		fs := testFileStoreWithQuota(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		org, err := fs.orgSvc.Create(ctx, "Test Org")
		if err != nil {
			t.Fatalf("Failed to create org: %v", err)
		}
		orgID := org.ID

		pageID := jsonldb.ID(1)

		// Initialize git repo for org
		if err := fs.InitOrg(ctx, orgID); err != nil {
			t.Fatalf("failed to init org: %v", err)
		}

		t.Run("MaxAssetSize", func(t *testing.T) {
			// Set small asset size quota
			_, err = fs.orgSvc.Modify(orgID, func(o *identity.Organization) error {
				o.Quotas.MaxAssetSize = 10
				return nil
			})
			if err != nil {
				t.Fatalf("Failed to modify org quota: %v", err)
			}

			// Try to save asset larger than quota
			_, err = fs.SaveAsset(ctx, orgID, pageID, "test.txt", []byte("this is more than 10 bytes"), author)
			if err == nil {
				t.Error("Expected error when exceeding asset size quota")
			}

			// Save asset within quota
			_, err = fs.SaveAsset(ctx, orgID, pageID, "small.txt", []byte("small"), author)
			if err != nil {
				t.Errorf("Unexpected error saving small asset: %v", err)
			}
		})

		t.Run("MaxStorage", func(t *testing.T) {
			// Set small total storage quota
			_, err = fs.orgSvc.Modify(orgID, func(o *identity.Organization) error {
				o.Quotas.MaxStorage = 100
				o.Quotas.MaxAssetSize = 100 // Ensure single asset fits
				return nil
			})
			if err != nil {
				t.Fatalf("Failed to modify org quota: %v", err)
			}

			// Save first asset
			_, err = fs.SaveAsset(ctx, orgID, pageID, "1.txt", []byte("0123456789"), author) // 10 bytes
			if err != nil {
				t.Fatalf("Failed to save first asset: %v", err)
			}

			// Save second asset
			_, err = fs.SaveAsset(ctx, orgID, pageID, "2.txt", []byte("0123456789012345678901234567890123456789"), author) // 40 bytes
			if err != nil {
				t.Fatalf("Failed to save second asset: %v", err)
			}

			// Total usage is now ~50 bytes + overhead.
			// Try to save something that definitely exceeds 100.
			largeData := make([]byte, 100)
			_, err = fs.SaveAsset(ctx, orgID, pageID, "large.txt", largeData, author)
			if err == nil {
				t.Error("Expected error when exceeding total storage quota")
			}
		})
	})
}

func TestMarkdown(t *testing.T) {
	t.Run("Formatting", func(t *testing.T) {
		fs, orgID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		// Initialize git repo for org
		if err := fs.InitOrg(ctx, orgID); err != nil {
			t.Fatalf("failed to init org: %v", err)
		}

		// Write page with specific content
		pageID := jsonldb.ID(1)
		_, err := fs.WritePage(ctx, orgID, pageID, "Format Test", "# Content\n\nWith multiple lines", author)
		if err != nil {
			t.Fatalf("failed to write page: %v", err)
		}

		// Read the file directly to verify format
		filePath := filepath.Join(fs.rootDir, orgID.String(), "pages", pageID.String(), "index.md")
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

func TestGetOrganizationUsage(t *testing.T) {
	t.Run("CountsTablesAndPages", func(t *testing.T) {
		fs, orgID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitOrg(ctx, orgID); err != nil {
			t.Fatalf("failed to init org: %v", err)
		}

		// Set quota to allow 2 items
		_, err := fs.orgSvc.Modify(orgID, func(o *identity.Organization) error {
			o.Quotas.MaxPages = 2
			return nil
		})
		if err != nil {
			t.Fatalf("failed to set quota: %v", err)
		}

		// Create one page
		pageID := jsonldb.NewID()
		_, err = fs.WritePage(ctx, orgID, pageID, "Page 1", "content", author)
		if err != nil {
			t.Fatalf("failed to create page: %v", err)
		}

		// Create one table
		tableID := jsonldb.NewID()
		tableNode := &Node{
			ID:       tableID,
			Title:    "Table 1",
			Type:     NodeTypeTable,
			Created:  time.Now(),
			Modified: time.Now(),
		}
		if err := fs.WriteTable(ctx, orgID, tableNode, true, author); err != nil {
			t.Fatalf("failed to create table: %v", err)
		}

		// Get usage - should count both page and table
		pageCount, _, err := fs.GetOrganizationUsage(orgID)
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
			Created:  time.Now(),
			Modified: time.Now(),
		}
		err = fs.WriteTable(ctx, orgID, tableNode2, true, author)
		if err == nil {
			t.Error("expected quota exceeded error when creating third item")
		}
	})

	t.Run("HybridNodeCountedOnce", func(t *testing.T) {
		fs, orgID := testFileStore(t)
		ctx := t.Context()
		author := git.Author{Name: "Test", Email: "test@test.com"}

		if err := fs.InitOrg(ctx, orgID); err != nil {
			t.Fatalf("failed to init org: %v", err)
		}

		// Create a hybrid node (page + table)
		hybridID := jsonldb.NewID()
		_, err := fs.WritePage(ctx, orgID, hybridID, "Hybrid", "content", author)
		if err != nil {
			t.Fatalf("failed to create page: %v", err)
		}

		hybridNode := &Node{
			ID:       hybridID,
			Title:    "Hybrid",
			Type:     NodeTypeTable,
			Created:  time.Now(),
			Modified: time.Now(),
		}
		if err := fs.WriteTable(ctx, orgID, hybridNode, false, author); err != nil {
			t.Fatalf("failed to add table metadata: %v", err)
		}

		pageCount, _, err := fs.GetOrganizationUsage(orgID)
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
