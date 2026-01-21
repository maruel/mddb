package content

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// mockQuotaGetterPageService implements the QuotaGetter interface for testing.
type mockQuotaGetterPageService struct {
	quotas map[jsonldb.ID]entity.Quota
}

func (m *mockQuotaGetterPageService) GetQuota(ctx context.Context, orgID jsonldb.ID) (entity.Quota, error) {
	if quota, exists := m.quotas[orgID]; exists {
		return quota, nil
	}
	// Return default quota if not found
	return entity.Quota{MaxPages: 100, MaxStorage: 1000000, MaxUsers: 10}, nil
}

// newTestContextWithOrg creates a test context with a real organization.
// It creates an organization and returns the context with that org ID, the org ID itself, and the git service.
func newTestContextWithOrg(t *testing.T, tempDir string) (context.Context, jsonldb.ID, *identity.OrganizationService, *git.Client) {
	t.Helper()
	gitService, err := git.New(t.Context(), tempDir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	orgService, err := identity.NewOrganizationService(filepath.Join(tempDir, "organizations.jsonl"), tempDir, gitService)
	if err != nil {
		t.Fatal(err)
	}
	org, err := orgService.Create(t.Context(), "Test Org")
	if err != nil {
		t.Fatal(err)
	}
	return t.Context(), org.ID, orgService, gitService
}

func TestNewPageService(t *testing.T) {
	tempDir := t.TempDir()
	fileStore, err := NewFileStore(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	gitService, err := git.New(t.Context(), tempDir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	mockQuotaGetter := &mockQuotaGetterPageService{
		quotas: map[jsonldb.ID]entity.Quota{
			jsonldb.ID(100): {MaxPages: 100, MaxStorage: 1000000, MaxUsers: 10},
		},
	}
	service := NewPageService(fileStore, gitService, mockQuotaGetter)
	if service == nil {
		t.Fatal("NewPageService returned nil")
	}
	if service.FileStore != fileStore {
		t.Error("fileStore not properly assigned")
	}
}

func TestPageService_CreatePage(t *testing.T) {
	tempDir := t.TempDir()
	ctx, orgID, _, gitService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	mockQuotaGetter := &mockQuotaGetterPageService{
		quotas: map[jsonldb.ID]entity.Quota{
			orgID: {MaxPages: 100, MaxStorage: 1000000, MaxUsers: 10},
		},
	}
	service := NewPageService(fileStore, gitService, mockQuotaGetter)
	page, err := service.Create(ctx, orgID, "Test Page", "# Hello World", "", "")
	if err != nil {
		t.Fatalf("CreatePage failed: %v", err)
	}
	if page.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", page.Title, "Test Page")
	}
	if page.Content != "# Hello World" {
		t.Errorf("Content = %q, want %q", page.Content, "# Hello World")
	}
	if page.ID.IsZero() {
		t.Error("Expected non-zero page ID")
	}
	if _, err = service.Create(ctx, orgID, "", "content", "", ""); err == nil {
		t.Error("Expected error when creating page with empty title")
	}
}

func TestPageService_GetPage(t *testing.T) {
	tempDir := t.TempDir()
	ctx, orgID, _, gitService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	mockQuotaGetter := &mockQuotaGetterPageService{
		quotas: map[jsonldb.ID]entity.Quota{
			orgID: {MaxPages: 100, MaxStorage: 1000000, MaxUsers: 10},
		},
	}
	service := NewPageService(fileStore, gitService, mockQuotaGetter)
	created, err := service.Create(ctx, orgID, "Get Test Page", "Test content", "", "")
	if err != nil {
		t.Fatal(err)
	}
	retrieved, err := service.Get(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("GetPage failed: %v", err)
	}
	if retrieved.Title != "Get Test Page" {
		t.Errorf("Title = %q, want %q", retrieved.Title, "Get Test Page")
	}
	var emptyID jsonldb.ID
	if _, err = service.Get(ctx, orgID, emptyID); err == nil {
		t.Error("Expected error when getting page with empty ID")
	}
	if _, err = service.Get(ctx, orgID, jsonldb.ID(0)); err == nil {
		t.Error("Expected error when getting page with invalid ID")
	}
}

func TestPageService_UpdatePage(t *testing.T) {
	tempDir := t.TempDir()
	ctx, orgID, _, gitService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	mockQuotaGetter := &mockQuotaGetterPageService{
		quotas: map[jsonldb.ID]entity.Quota{
			orgID: {MaxPages: 100, MaxStorage: 1000000, MaxUsers: 10},
		},
	}
	service := NewPageService(fileStore, gitService, mockQuotaGetter)
	created, err := service.Create(ctx, orgID, "Original Title", "Original content", "", "")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.Update(ctx, orgID, created.ID, "New Title", "New content", "", "")
	if err != nil {
		t.Fatalf("UpdatePage failed: %v", err)
	}
	if updated.Title != "New Title" {
		t.Errorf("Title = %q, want %q", updated.Title, "New Title")
	}
	if updated.Content != "New content" {
		t.Errorf("Content = %q, want %q", updated.Content, "New content")
	}
	var emptyID2 jsonldb.ID
	if _, err = service.Update(ctx, orgID, emptyID2, "Title", "Content", "", ""); err == nil {
		t.Error("Expected error when updating page with empty ID")
	}
	if _, err = service.Update(ctx, orgID, created.ID, "", "Content", "", ""); err == nil {
		t.Error("Expected error when updating page with empty title")
	}
	if _, err = service.Update(ctx, orgID, jsonldb.ID(0), "Title", "Content", "", ""); err == nil {
		t.Error("Expected error when updating page with invalid ID")
	}
}

func TestPageService_DeletePage(t *testing.T) {
	tempDir := t.TempDir()
	ctx, orgID, _, gitService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	mockQuotaGetter := &mockQuotaGetterPageService{
		quotas: map[jsonldb.ID]entity.Quota{
			orgID: {MaxPages: 100, MaxStorage: 1000000, MaxUsers: 10},
		},
	}
	service := NewPageService(fileStore, gitService, mockQuotaGetter)
	created, err := service.Create(ctx, orgID, "Delete Test Page", "Content to delete", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Delete(ctx, orgID, created.ID, "", ""); err != nil {
		t.Fatalf("DeletePage failed: %v", err)
	}
	if _, err = service.Get(ctx, orgID, created.ID); err == nil {
		t.Error("Expected error when getting deleted page")
	}
	var emptyID3 jsonldb.ID
	if err = service.Delete(ctx, orgID, emptyID3, "", ""); err == nil {
		t.Error("Expected error when deleting page with empty ID")
	}
	if err = service.Delete(ctx, orgID, jsonldb.ID(0), "", ""); err == nil {
		t.Error("Expected error when deleting page with invalid ID")
	}
}

func TestPageService_ListPages(t *testing.T) {
	tempDir := t.TempDir()
	ctx, orgID, _, gitService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	mockQuotaGetter := &mockQuotaGetterPageService{
		quotas: map[jsonldb.ID]entity.Quota{
			orgID: {MaxPages: 100, MaxStorage: 1000000, MaxUsers: 10},
		},
	}
	service := NewPageService(fileStore, gitService, mockQuotaGetter)
	pages, err := service.List(ctx, orgID)
	if err != nil {
		t.Fatalf("ListPages failed: %v", err)
	}
	initialCount := len(pages)
	if _, err := service.Create(ctx, orgID, "Page 1", "Content 1", "", ""); err != nil {
		t.Fatalf("Create Page 1 failed: %v", err)
	}
	if _, err := service.Create(ctx, orgID, "Page 2", "Content 2", "", ""); err != nil {
		t.Fatalf("Create Page 2 failed: %v", err)
	}
	if _, err := service.Create(ctx, orgID, "Page 3", "Content 3", "", ""); err != nil {
		t.Fatalf("Create Page 3 failed: %v", err)
	}
	if pages, err = service.List(ctx, orgID); err != nil {
		t.Fatalf("ListPages failed: %v", err)
	}
	if len(pages) != initialCount+3 {
		t.Errorf("Expected %d pages, got %d", initialCount+3, len(pages))
	}
}

func TestPageService_SearchPages(t *testing.T) {
	tempDir := t.TempDir()
	ctx, orgID, _, gitService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	mockQuotaGetter := &mockQuotaGetterPageService{
		quotas: map[jsonldb.ID]entity.Quota{
			orgID: {MaxPages: 100, MaxStorage: 1000000, MaxUsers: 10},
		},
	}
	service := NewPageService(fileStore, gitService, mockQuotaGetter)
	if _, err := service.Create(ctx, orgID, "Apple Recipes", "How to cook with apples", "", ""); err != nil {
		t.Fatalf("Create Apple Recipes failed: %v", err)
	}
	if _, err := service.Create(ctx, orgID, "Orange Juice", "Making fresh juice", "", ""); err != nil {
		t.Fatalf("Create Orange Juice failed: %v", err)
	}
	if _, err := service.Create(ctx, orgID, "Banana Bread", "Contains apple cider vinegar", "", ""); err != nil {
		t.Fatalf("Create Banana Bread failed: %v", err)
	}
	results, err := service.Search(ctx, orgID, "Apple")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 search results for 'Apple', got %d", len(results))
	}
	if results, err = service.Search(ctx, orgID, "juice"); err != nil {
		t.Fatalf("Search failed: %v", err)
	} else if len(results) != 1 {
		t.Errorf("Expected 1 search result for 'juice', got %d", len(results))
	}
	if results, err = service.Search(ctx, orgID, "xyz123uniquestring"); err != nil {
		t.Fatalf("Search failed: %v", err)
	} else if len(results) != 0 {
		t.Errorf("Expected 0 search results for 'xyz123uniquestring', got %d", len(results))
	}
	if results, err = service.Search(ctx, orgID, ""); err != nil {
		t.Fatalf("Search failed: %v", err)
	} else if len(results) != 0 {
		t.Errorf("Expected 0 results for empty query, got %d", len(results))
	}
}
