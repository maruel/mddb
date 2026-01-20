package storage

import (
	"context"
	"os"
	"testing"

	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/jsonldb"
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

	org, err := orgService.CreateOrganization(context.Background(), "Test Org")
	if err != nil {
		t.Fatal(err)
	}

	user := &entity.User{ID: testID(1000)}
	ctx := context.WithValue(context.Background(), entity.UserKey, user)
	ctx = context.WithValue(ctx, entity.OrgKey, org.ID)
	return ctx, orgService
}

func TestNewPageService(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-page-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	fileStore, err := NewFileStore(tempDir)
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
	tempDir, err := os.MkdirTemp("", "mddb-page-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	ctx, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewPageService(fileStore, nil, cache, orgService)

	// Test creating a page
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

	// Test creating page with empty title
	_, err = service.CreatePage(ctx, "", "content")
	if err == nil {
		t.Error("Expected error when creating page with empty title")
	}
}

func TestPageService_GetPage(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-page-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	ctx, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewPageService(fileStore, nil, cache, orgService)

	// Create a page first
	created, err := service.CreatePage(ctx, "Get Test Page", "Test content")
	if err != nil {
		t.Fatal(err)
	}

	// Test getting the page
	retrieved, err := service.GetPage(ctx, created.ID.String())
	if err != nil {
		t.Fatalf("GetPage failed: %v", err)
	}
	if retrieved.Title != "Get Test Page" {
		t.Errorf("Title = %q, want %q", retrieved.Title, "Get Test Page")
	}

	// Test getting with empty ID
	_, err = service.GetPage(ctx, "")
	if err == nil {
		t.Error("Expected error when getting page with empty ID")
	}

	// Test getting with invalid ID (contains invalid character @)
	_, err = service.GetPage(ctx, "invalid@id")
	if err == nil {
		t.Error("Expected error when getting page with invalid ID")
	}
}

func TestPageService_UpdatePage(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-page-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	ctx, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewPageService(fileStore, nil, cache, orgService)

	// Create a page first
	created, err := service.CreatePage(ctx, "Original Title", "Original content")
	if err != nil {
		t.Fatal(err)
	}

	// Update the page
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

	// Test updating with empty ID
	_, err = service.UpdatePage(ctx, "", "Title", "Content")
	if err == nil {
		t.Error("Expected error when updating page with empty ID")
	}

	// Test updating with empty title
	_, err = service.UpdatePage(ctx, created.ID.String(), "", "Content")
	if err == nil {
		t.Error("Expected error when updating page with empty title")
	}

	// Test updating with invalid ID (contains invalid character @)
	_, err = service.UpdatePage(ctx, "invalid@id", "Title", "Content")
	if err == nil {
		t.Error("Expected error when updating page with invalid ID")
	}
}

func TestPageService_DeletePage(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-page-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	ctx, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewPageService(fileStore, nil, cache, orgService)

	// Create a page first
	created, err := service.CreatePage(ctx, "Delete Test Page", "Content to delete")
	if err != nil {
		t.Fatal(err)
	}

	// Delete the page
	err = service.DeletePage(ctx, created.ID.String())
	if err != nil {
		t.Fatalf("DeletePage failed: %v", err)
	}

	// Verify page is deleted
	_, err = service.GetPage(ctx, created.ID.String())
	if err == nil {
		t.Error("Expected error when getting deleted page")
	}

	// Test deleting with empty ID
	err = service.DeletePage(ctx, "")
	if err == nil {
		t.Error("Expected error when deleting page with empty ID")
	}

	// Test deleting with invalid ID (contains invalid character @)
	err = service.DeletePage(ctx, "invalid@id")
	if err == nil {
		t.Error("Expected error when deleting page with invalid ID")
	}
}

func TestPageService_ListPages(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-page-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	ctx, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewPageService(fileStore, nil, cache, orgService)

	// List pages - should have the welcome page from org creation
	pages, err := service.ListPages(ctx)
	if err != nil {
		t.Fatalf("ListPages failed: %v", err)
	}
	initialCount := len(pages)

	// Create some pages
	_, _ = service.CreatePage(ctx, "Page 1", "Content 1")
	_, _ = service.CreatePage(ctx, "Page 2", "Content 2")
	_, _ = service.CreatePage(ctx, "Page 3", "Content 3")

	pages, err = service.ListPages(ctx)
	if err != nil {
		t.Fatalf("ListPages failed: %v", err)
	}
	if len(pages) != initialCount+3 {
		t.Errorf("Expected %d pages, got %d", initialCount+3, len(pages))
	}
}

func TestPageService_SearchPages(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-page-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	ctx, orgService := newTestContextWithOrg(t, tempDir)
	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewPageService(fileStore, nil, cache, orgService)

	// Create some pages
	_, _ = service.CreatePage(ctx, "Apple Recipes", "How to cook with apples")
	_, _ = service.CreatePage(ctx, "Orange Juice", "Making fresh juice")
	_, _ = service.CreatePage(ctx, "Banana Bread", "Contains apple cider vinegar")

	// Search by title
	results, err := service.SearchPages(ctx, "Apple")
	if err != nil {
		t.Fatalf("SearchPages failed: %v", err)
	}
	// Should match "Apple Recipes" (title) and "Banana Bread" (content has "apple")
	if len(results) != 2 {
		t.Errorf("Expected 2 search results for 'Apple', got %d", len(results))
	}

	// Search by content
	results, _ = service.SearchPages(ctx, "juice")
	if len(results) != 1 {
		t.Errorf("Expected 1 search result for 'juice', got %d", len(results))
	}

	// Search with no match
	results, _ = service.SearchPages(ctx, "xyz123uniquestring")
	if len(results) != 0 {
		t.Errorf("Expected 0 search results for 'xyz123uniquestring', got %d", len(results))
	}

	// Search with empty query
	results, _ = service.SearchPages(ctx, "")
	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty query, got %d", len(results))
	}
}

func TestPageService_GetPageHistory_NoGit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-page-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewPageService(fileStore, nil, cache, nil) // No git service

	ctx := newTestContext("")

	history, err := service.GetPageHistory(ctx, jsonldb.NewID().String())
	if err != nil {
		t.Fatalf("GetPageHistory failed: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("Expected empty history when git service is nil, got %d", len(history))
	}
}

func TestPageService_GetPageVersion_NoGit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mddb-page-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	fileStore, _ := NewFileStore(tempDir)
	cache := NewCache()
	service := NewPageService(fileStore, nil, cache, nil) // No git service

	ctx := newTestContext("")

	_, err = service.GetPageVersion(ctx, jsonldb.NewID().String(), "abc123")
	if err == nil {
		t.Error("Expected error when getting page version without git service")
	}
}
