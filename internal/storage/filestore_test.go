package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStorePageOperations(t *testing.T) {
	// Create temporary directory for testing
	tmpDir := t.TempDir()

	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create FileStore: %v", err)
	}

	// Test WritePage
	page, err := fs.WritePage("test-page", "Test Title", "# Test Content")
	if err != nil {
		t.Fatalf("failed to write page: %v", err)
	}

	if page.ID != "test-page" {
		t.Errorf("expected ID 'test-page', got %q", page.ID)
	}
	if page.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got %q", page.Title)
	}

	// Test PageExists
	if !fs.PageExists("test-page") {
		t.Error("page should exist after WritePage")
	}

	// Test ReadPage
	readPage, err := fs.ReadPage("test-page")
	if err != nil {
		t.Fatalf("failed to read page: %v", err)
	}

	if readPage.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got %q", readPage.Title)
	}
	if readPage.Content != "\n# Test Content" {
		t.Errorf("expected content '\\n# Test Content', got %q", readPage.Content)
	}

	// Test UpdatePage
	updated, err := fs.UpdatePage("test-page", "Updated Title", "# Updated Content")
	if err != nil {
		t.Fatalf("failed to update page: %v", err)
	}

	if updated.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %q", updated.Title)
	}

	// Verify update persisted
	readUpdated, err := fs.ReadPage("test-page")
	if err != nil {
		t.Fatalf("failed to read updated page: %v", err)
	}

	if readUpdated.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %q", readUpdated.Title)
	}

	// Test DeletePage
	err = fs.DeletePage("test-page")
	if err != nil {
		t.Fatalf("failed to delete page: %v", err)
	}

	if fs.PageExists("test-page") {
		t.Error("page should not exist after DeletePage")
	}

	// Test error handling for non-existent page
	_, err = fs.ReadPage("non-existent")
	if err == nil {
		t.Error("expected error reading non-existent page")
	}
}

func TestFileStoreListPages(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create FileStore: %v", err)
	}

	// Create multiple pages
	pages := []struct {
		id    string
		title string
	}{
		{"page-one", "First Page"},
		{"page-two", "Second Page"},
		{"nested/page", "Nested Page"},
	}

	for _, p := range pages {
		_, err := fs.WritePage(p.id, p.title, "Content")
		if err != nil {
			t.Fatalf("failed to write page %s: %v", p.id, err)
		}
	}

	// List pages
	listed, err := fs.ListPages()
	if err != nil {
		t.Fatalf("failed to list pages: %v", err)
	}

	if len(listed) != len(pages) {
		t.Errorf("expected %d pages, got %d", len(pages), len(listed))
	}

	// Verify file structure
	expectedFile := filepath.Join(fs.pagesDir, "page-one.md")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Errorf("expected file %s to exist: %v", expectedFile, err)
	}

	nestedFile := filepath.Join(fs.pagesDir, "nested", "page.md")
	if _, err := os.Stat(nestedFile); err != nil {
		t.Errorf("expected nested file %s to exist: %v", nestedFile, err)
	}
}

func TestMarkdownFormatting(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create FileStore: %v", err)
	}

	// Write page with specific content
	_, err = fs.WritePage("format-test", "Format Test", "# Content\n\nWith multiple lines")
	if err != nil {
		t.Fatalf("failed to write page: %v", err)
	}

	// Read the file directly to verify format
	filePath := filepath.Join(fs.pagesDir, "format-test.md")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	content := string(data)

	// Verify front matter structure
	if !contains(content, "---") {
		t.Error("expected front matter delimiters")
	}
	if !contains(content, "id: format-test") {
		t.Error("expected id in front matter")
	}
	if !contains(content, "title: Format Test") {
		t.Error("expected title in front matter")
	}
	if !contains(content, "created:") {
		t.Error("expected created timestamp")
	}
	if !contains(content, "modified:") {
		t.Error("expected modified timestamp")
	}

	// Verify content separation
	parts := splitN(content, "---", 3)
	if len(parts) < 3 {
		t.Error("expected three sections separated by ---")
	}
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
	for i := 0; i < n && s != ""; i++ {
		idx := -1
		for j := range len(s) - len(sep) + 1 {
			if s[j:j+len(sep)] == sep {
				idx = j
				break
			}
		}
		if idx == -1 {
			result = append(result, s)
			break
		}
		result = append(result, s[:idx])
		s = s[idx+len(sep):]
	}
	return result
}
