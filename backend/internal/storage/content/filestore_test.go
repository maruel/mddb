package content

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// testFileStore creates a FileStore for testing with unlimited quota.
// It also creates an org in the service for quota testing.
func testFileStore(t *testing.T) (*FileStore, jsonldb.ID) {
	t.Helper()
	tmpDir := t.TempDir()

	gitClient, err := git.New(context.Background(), tmpDir, "test", "test@test.com")
	if err != nil {
		t.Fatalf("failed to create git client: %v", err)
	}

	orgService, err := identity.NewOrganizationService(filepath.Join(tmpDir, "organizations.jsonl"))
	if err != nil {
		t.Fatalf("failed to create OrganizationService: %v", err)
	}

	// Create a test organization with very high quotas (practically unlimited)
	org, err := orgService.Create(context.Background(), "Test Org")
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

	fs, err := NewFileStore(tmpDir, gitClient, orgService)
	if err != nil {
		t.Fatalf("failed to create FileStore: %v", err)
	}

	return fs, org.ID
}

// testFileStoreWithQuota creates a FileStore with a real OrganizationService for quota testing.
func testFileStoreWithQuota(t *testing.T) *FileStore {
	t.Helper()
	tmpDir := t.TempDir()

	gitClient, err := git.New(context.Background(), tmpDir, "test", "test@test.com")
	if err != nil {
		t.Fatalf("failed to create git client: %v", err)
	}

	orgService, err := identity.NewOrganizationService(filepath.Join(tmpDir, "organizations.jsonl"))
	if err != nil {
		t.Fatalf("failed to create OrganizationService: %v", err)
	}

	fs, err := NewFileStore(tmpDir, gitClient, orgService)
	if err != nil {
		t.Fatalf("failed to create FileStore: %v", err)
	}

	return fs
}

func TestFileStore(t *testing.T) {
	t.Run("PageOperations", func(t *testing.T) {
		fs, orgID := testFileStore(t)
		ctx := context.Background()
		author := Author{Name: "Test", Email: "test@test.com"}

		// Create org directory and initialize git repo
		if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
			t.Fatalf("failed to create org dir: %v", err)
		}
		if err := fs.Git.Init(ctx, orgID.String()); err != nil {
			t.Fatalf("failed to init org git repo: %v", err)
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
		ctx := context.Background()
		author := Author{Name: "Test", Email: "test@test.com"}

		// Create org directory and initialize git repo
		if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
			t.Fatalf("failed to create org dir: %v", err)
		}
		if err := fs.Git.Init(ctx, orgID.String()); err != nil {
			t.Fatalf("failed to init org git repo: %v", err)
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
}

func TestAsset(t *testing.T) {
	t.Run("Quota", func(t *testing.T) {
		fs := testFileStoreWithQuota(t)
		ctx := context.Background()
		author := Author{Name: "Test", Email: "test@test.com"}

		org, err := fs.orgSvc.Create(ctx, "Test Org")
		if err != nil {
			t.Fatalf("Failed to create org: %v", err)
		}
		orgID := org.ID

		pageID := jsonldb.ID(1)

		// Create org directory and initialize git repo
		if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
			t.Fatalf("failed to create org dir: %v", err)
		}
		if err := fs.Git.Init(ctx, orgID.String()); err != nil {
			t.Fatalf("failed to init org git repo: %v", err)
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
		ctx := context.Background()
		author := Author{Name: "Test", Email: "test@test.com"}

		// Create org directory and initialize git repo
		if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
			t.Fatalf("failed to create org dir: %v", err)
		}
		if err := fs.Git.Init(ctx, orgID.String()); err != nil {
			t.Fatalf("failed to init org git repo: %v", err)
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
