package storage

import (
	"context"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)

// newTestContextWithOrg creates a test context with a real organization.
// It creates an organization and returns the context with that org ID.
func newTestContextWithOrg(t *testing.T, tempDir string) (context.Context, *OrganizationService) {
	t.Helper()
	fileStore, err := NewFileStore(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	orgService, err := NewOrganizationService(tempDir, fileStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	org, err := orgService.CreateOrganization(t.Context(), "Test Org")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(t.Context(), entity.UserKey, &entity.User{ID: testID(1000)})
	return context.WithValue(ctx, entity.OrgKey, org.ID), orgService
}

func TestNewPageService(t *testing.T) {
	fileStore, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cache := NewCache()
	service := NewPageService(fileStore, nil, cache, nil)
	if service == nil {
		t.Fatal("NewPageService returned nil")
	}
	if service.fileStore != fileStore {
		t.Error("fileStore not properly assigned")
	}
	if service.cache != cache {
		t.Error("cache not properly assigned")
	}
}

func TestPageService_CreatePage(t *testing.T) {
	tempDir := t.TempDir()
	ctx, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	service := NewPageService(fileStore, nil, NewCache(), orgService)
	page, err := service.CreatePage(ctx, "Test Page", "# Hello World")
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
	if _, err = service.CreatePage(ctx, "", "content"); err == nil {
		t.Error("Expected error when creating page with empty title")
	}
}

func TestPageService_GetPage(t *testing.T) {
	tempDir := t.TempDir()
	ctx, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	service := NewPageService(fileStore, nil, NewCache(), orgService)
	created, err := service.CreatePage(ctx, "Get Test Page", "Test content")
	if err != nil {
		t.Fatal(err)
	}
	retrieved, err := service.GetPage(ctx, created.ID.String())
	if err != nil {
		t.Fatalf("GetPage failed: %v", err)
	}
	if retrieved.Title != "Get Test Page" {
		t.Errorf("Title = %q, want %q", retrieved.Title, "Get Test Page")
	}
	if _, err = service.GetPage(ctx, ""); err == nil {
		t.Error("Expected error when getting page with empty ID")
	}
	if _, err = service.GetPage(ctx, "invalid@id"); err == nil {
		t.Error("Expected error when getting page with invalid ID")
	}
}

func TestPageService_UpdatePage(t *testing.T) {
	tempDir := t.TempDir()
	ctx, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	service := NewPageService(fileStore, nil, NewCache(), orgService)
	created, err := service.CreatePage(ctx, "Original Title", "Original content")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.UpdatePage(ctx, created.ID.String(), "New Title", "New content")
	if err != nil {
		t.Fatalf("UpdatePage failed: %v", err)
	}
	if updated.Title != "New Title" {
		t.Errorf("Title = %q, want %q", updated.Title, "New Title")
	}
	if updated.Content != "New content" {
		t.Errorf("Content = %q, want %q", updated.Content, "New content")
	}
	if _, err = service.UpdatePage(ctx, "", "Title", "Content"); err == nil {
		t.Error("Expected error when updating page with empty ID")
	}
	if _, err = service.UpdatePage(ctx, created.ID.String(), "", "Content"); err == nil {
		t.Error("Expected error when updating page with empty title")
	}
	if _, err = service.UpdatePage(ctx, "invalid@id", "Title", "Content"); err == nil {
		t.Error("Expected error when updating page with invalid ID")
	}
}

func TestPageService_DeletePage(t *testing.T) {
	tempDir := t.TempDir()
	ctx, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	service := NewPageService(fileStore, nil, NewCache(), orgService)
	created, err := service.CreatePage(ctx, "Delete Test Page", "Content to delete")
	if err != nil {
		t.Fatal(err)
	}
	if err = service.DeletePage(ctx, created.ID.String()); err != nil {
		t.Fatalf("DeletePage failed: %v", err)
	}
	if _, err = service.GetPage(ctx, created.ID.String()); err == nil {
		t.Error("Expected error when getting deleted page")
	}
	if err = service.DeletePage(ctx, ""); err == nil {
		t.Error("Expected error when deleting page with empty ID")
	}
	if err = service.DeletePage(ctx, "invalid@id"); err == nil {
		t.Error("Expected error when deleting page with invalid ID")
	}
}

func TestPageService_ListPages(t *testing.T) {
	tempDir := t.TempDir()
	ctx, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	service := NewPageService(fileStore, nil, NewCache(), orgService)
	pages, err := service.ListPages(ctx)
	if err != nil {
		t.Fatalf("ListPages failed: %v", err)
	}
	initialCount := len(pages)
	_, _ = service.CreatePage(ctx, "Page 1", "Content 1")
	_, _ = service.CreatePage(ctx, "Page 2", "Content 2")
	_, _ = service.CreatePage(ctx, "Page 3", "Content 3")
	if pages, err = service.ListPages(ctx); err != nil {
		t.Fatalf("ListPages failed: %v", err)
	}
	if len(pages) != initialCount+3 {
		t.Errorf("Expected %d pages, got %d", initialCount+3, len(pages))
	}
}

func TestPageService_SearchPages(t *testing.T) {
	tempDir := t.TempDir()
	ctx, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	service := NewPageService(fileStore, nil, NewCache(), orgService)
	_, _ = service.CreatePage(ctx, "Apple Recipes", "How to cook with apples")
	_, _ = service.CreatePage(ctx, "Orange Juice", "Making fresh juice")
	_, _ = service.CreatePage(ctx, "Banana Bread", "Contains apple cider vinegar")
	results, err := service.SearchPages(ctx, "Apple")
	if err != nil {
		t.Fatalf("SearchPages failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 search results for 'Apple', got %d", len(results))
	}
	if results, _ = service.SearchPages(ctx, "juice"); len(results) != 1 {
		t.Errorf("Expected 1 search result for 'juice', got %d", len(results))
	}
	if results, _ = service.SearchPages(ctx, "xyz123uniquestring"); len(results) != 0 {
		t.Errorf("Expected 0 search results for 'xyz123uniquestring', got %d", len(results))
	}
	if results, _ = service.SearchPages(ctx, ""); len(results) != 0 {
		t.Errorf("Expected 0 results for empty query, got %d", len(results))
	}
}

func TestPageService_GetPageHistory_NoGit(t *testing.T) {
	fileStore, _ := NewFileStore(t.TempDir())
	service := NewPageService(fileStore, nil, NewCache(), nil)
	history, err := service.GetPageHistory(newTestContext(t, ""), jsonldb.NewID().String())
	if err != nil {
		t.Fatalf("GetPageHistory failed: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("Expected empty history when git service is nil, got %d", len(history))
	}
}

func TestPageService_GetPageVersion_NoGit(t *testing.T) {
	fileStore, _ := NewFileStore(t.TempDir())
	service := NewPageService(fileStore, nil, NewCache(), nil)
	if _, err := service.GetPageVersion(newTestContext(t, ""), jsonldb.NewID().String(), "abc123"); err == nil {
		t.Error("Expected error when getting page version without git service")
	}
}
