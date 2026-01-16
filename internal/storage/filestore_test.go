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

	// Test WritePage (with numeric ID)
	page, err := fs.WritePage("", "1", "Test Title", "# Test Content")
	if err != nil {
		t.Fatalf("failed to write page: %v", err)
	}

	if page.ID != "1" {
		t.Errorf("expected ID '1', got %q", page.ID)
	}
	if page.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got %q", page.Title)
	}

	// Test PageExists
	if !fs.PageExists("", "1") {
		t.Error("page should exist after WritePage")
	}

	// Test ReadPage
	readPage, err := fs.ReadPage("", "1")
	if err != nil {
		t.Fatalf("failed to read page: %v", err)
	}

	if readPage.Title != "Test Title" {
		t.Errorf("expected title 'Test Title', got %q", readPage.Title)
	}
	if readPage.Content != "\n\n# Test Content" {
		t.Errorf("expected content '\n\n# Test Content', got %q", readPage.Content)
	}

	// Test UpdatePage
	updated, err := fs.UpdatePage("", "1", "Updated Title", "# Updated Content")
	if err != nil {
		t.Fatalf("failed to update page: %v", err)
	}

	if updated.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %q", updated.Title)
	}

	// Verify update persisted
	readUpdated, err := fs.ReadPage("", "1")
	if err != nil {
		t.Fatalf("failed to read updated page: %v", err)
	}

	if readUpdated.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %q", readUpdated.Title)
	}

	// Test DeletePage
	err = fs.DeletePage("", "1")
	if err != nil {
		t.Fatalf("failed to delete page: %v", err)
	}

	if fs.PageExists("", "1") {
		t.Error("page should not exist after DeletePage")
	}

	// Test error handling for non-existent page
	_, err = fs.ReadPage("", "999")
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

	// Create multiple pages with numeric IDs
	pages := []struct {
		id    string
		title string
	}{
		{"1", "First Page"},
		{"2", "Second Page"},
		{"3", "Third Page"},
	}

	for _, p := range pages {
		_, err := fs.WritePage("", p.id, p.title, "Content")
		if err != nil {
			t.Fatalf("failed to write page %s: %v", p.id, err)
		}
	}

	// List pages
	listed, err := fs.ListPages("")
	if err != nil {
		t.Fatalf("failed to list pages: %v", err)
	}

	if len(listed) != len(pages) {
		t.Errorf("expected %d pages, got %d", len(pages), len(listed))
	}

	// Verify directory structure
	expectedDir := filepath.Join(fs.pagesDir, "1")
	if _, err := os.Stat(expectedDir); err != nil {
		t.Errorf("expected page directory %s to exist: %v", expectedDir, err)
	}

	expectedFile := filepath.Join(expectedDir, "index.md")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Errorf("expected file %s to exist: %v", expectedFile, err)
	}
}

func TestMarkdownFormatting(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create FileStore: %v", err)
	}

	// Write page with specific content
	_, err = fs.WritePage("", "1", "Format Test", "# Content\n\nWith multiple lines")
	if err != nil {
		t.Fatalf("failed to write page: %v", err)
	}

	// Read the file directly to verify format
	filePath := filepath.Join(fs.pagesDir, "1", "index.md")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	content := string(data)

	// Verify front matter structure
	if !contains(content, "---") {
		t.Error("expected front matter delimiters")
	}
	if !contains(content, "id: 1") {
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
			if j+len(sep) <= len(s) && s[j:j+len(sep)] == sep {
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