package storage

import (
	"fmt"
	"testing"

	"github.com/maruel/mddb/internal/models"
)

func TestSearchService_SearchPages(t *testing.T) {
	tmpDir := t.TempDir()
	fileStore, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}
	searchService := NewSearchService(fileStore)

	// Create test pages
	cache := NewCache()
	pageService := NewPageService(fileStore, nil, cache)
	_, _ = pageService.CreatePage("", "Getting Started", "This is a guide to get started with mddb project")
	_, _ = pageService.CreatePage("", "Advanced Topics", "Learn about advanced mddb configuration and optimization")
	_, _ = pageService.CreatePage("", "API Reference", "Complete mddb API documentation for developers")

	tests := []struct {
		name          string
		query         string
		expectResults int
		expectFirst   string
	}{
		{
			name:          "search in title",
			query:         "advanced",
			expectResults: 1,
			expectFirst:   "Advanced Topics",
		},
		{
			name:          "search in content",
			query:         "guide",
			expectResults: 1,
			expectFirst:   "Getting Started",
		},
		{
			name:          "search multiple pages",
			query:         "mddb",
			expectResults: 3,
		},
		{
			name:          "case insensitive search",
			query:         "API",
			expectResults: 1,
			expectFirst:   "API Reference",
		},
		{
			name:          "no results",
			query:         "nonexistent",
			expectResults: 0,
		},
		{
			name:          "empty query",
			query:         "",
			expectResults: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := searchService.Search("", SearchOptions{
				Query:      tt.query,
				MatchTitle: true,
				MatchBody:  true,
			})

			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			if len(results) != tt.expectResults {
				t.Errorf("Expected %d results, got %d", tt.expectResults, len(results))
			}

			if tt.expectResults > 0 && tt.expectFirst != "" {
				if results[0].Title != tt.expectFirst {
					t.Errorf("Expected first result '%s', got '%s'", tt.expectFirst, results[0].Title)
				}
			}

			// Verify all results are marked as pages
			for _, result := range results {
				if result.Type != "page" {
					t.Errorf("Expected type 'page', got '%s'", result.Type)
				}
			}
		})
	}
}

func TestSearchService_SearchRecords(t *testing.T) {
	tmpDir := t.TempDir()
	fileStore, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}
	searchService := NewSearchService(fileStore)

	// Create test database with records
	cache := NewCache()
	dbService := NewDatabaseService(fileStore, nil, cache)
	columns := []models.Column{
		{Name: "title", Type: "text", Required: true},
		{Name: "status", Type: "select", Options: []string{"todo", "done"}},
		{Name: "description", Type: "text"},
	}

	db, _ := dbService.CreateDatabase("", "Tasks", columns)

	// Create records
	_, _ = dbService.CreateRecord("", db.ID, map[string]any{"title": "Buy groceries", "status": "todo", "description": "Fresh vegetables"})
	_, _ = dbService.CreateRecord("", db.ID, map[string]any{"title": "Finish report", "status": "done", "description": "Quarterly performance"})
	_, _ = dbService.CreateRecord("", db.ID, map[string]any{"title": "Review code", "status": "todo", "description": "Pull request on main repo"})

	tests := []struct {
		name          string
		query         string
		expectResults int
	}{
		{
			name:          "search record title",
			query:         "groceries",
			expectResults: 1,
		},
		{
			name:          "search record status",
			query:         "todo",
			expectResults: 2,
		},
		{
			name:          "search record description",
			query:         "quarterly",
			expectResults: 1,
		},
		{
			name:          "case insensitive search",
			query:         "PULL",
			expectResults: 1,
		},
		{
			name:          "no matching records",
			query:         "nonexistent",
			expectResults: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := searchService.Search("", SearchOptions{
				Query:       tt.query,
				MatchFields: true,
			})

			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			if len(results) != tt.expectResults {
				t.Errorf("Expected %d results, got %d", tt.expectResults, len(results))
			}

			// Verify all results are marked as records
			for _, result := range results {
				if result.Type != "record" {
					t.Errorf("Expected type 'record', got '%s'", result.Type)
				}
				if result.RecordID == "" {
					t.Error("Expected RecordID to be set for record results")
				}
			}
		})
	}
}

func TestSearchService_Scoring(t *testing.T) {
	tmpDir := t.TempDir()
	fileStore, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}
	searchService := NewSearchService(fileStore)

	// Create pages where title match should score higher
	cache := NewCache()
	pageService := NewPageService(fileStore, nil, cache)
	_, _ = pageService.CreatePage("", "Python Programming", "This is about Java not Python")
	_, _ = pageService.CreatePage("", "Java Basics", "Learn Python programming fundamentals")

	results, err := searchService.Search("", SearchOptions{
		Query:      "python",
		MatchTitle: true,
		MatchBody:  true,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	// Title match should score higher than body match
	if results[0].Title != "Python Programming" {
		t.Errorf("Expected 'Python Programming' first (title match), got '%s'", results[0].Title)
	}

	if results[0].Score <= results[1].Score {
		t.Errorf("Title match should score higher than body match")
	}
}

func TestSearchService_Limit(t *testing.T) {
	tmpDir := t.TempDir()
	fileStore, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}
	searchService := NewSearchService(fileStore)

	// Create multiple pages
	cache := NewCache()
	pageService := NewPageService(fileStore, nil, cache)
	for i := range 10 {
		_, _ = pageService.CreatePage("", fmt.Sprintf("Test Page %d", i), "This is test content")
	}

	results, err := searchService.Search("", SearchOptions{
		Query:      "test",
		Limit:      2,
		MatchTitle: true,
		MatchBody:  true,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results (limited), got %d", len(results))
	}
}

func TestCountMatches(t *testing.T) {
	tests := []struct {
		text     string
		query    string
		expected int
	}{
		{"hello world hello", "hello", 2},
		{"the quick brown fox", "the", 1},
		{"aaa", "a", 3},
		{"no match here", "xyz", 0},
		{"", "test", 0},
	}

	for _, tt := range tests {
		got := countMatches(tt.text, tt.query)
		if got != tt.expected {
			t.Errorf("countMatches(%q, %q) = %d, want %d", tt.text, tt.query, got, tt.expected)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		text     string
		maxLen   int
		expected string
	}{
		{"hello world", 5, "hello..."},
		{"hello", 10, "hello"},
		{"test", 4, "test"},
		{"testing long string", 7, "testing..."},
	}

	for _, tt := range tests {
		got := truncate(tt.text, tt.maxLen)
		if got != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.text, tt.maxLen, got, tt.expected)
		}
	}
}

func TestValueToString(t *testing.T) {
	tests := []struct {
		value    any
		expected string
	}{
		{"hello", "hello"},
		{true, "true"},
		{false, "false"},
		{nil, ""},
	}

	for _, tt := range tests {
		got := valueToString(tt.value)
		if got != tt.expected {
			t.Errorf("valueToString(%v) = %q, want %q", tt.value, got, tt.expected)
		}
	}
}

func TestSearchService_Integration(t *testing.T) {
	tmpDir := t.TempDir()
	fileStore, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}
	searchService := NewSearchService(fileStore)

	// Create mixed content
	cache := NewCache()
	pageService := NewPageService(fileStore, nil, cache)
	_, _ = pageService.CreatePage("", "Blog Post", "Article about searchable content and web development")

	dbService := NewDatabaseService(fileStore, nil, cache)
	columns := []models.Column{
		{Name: "title", Type: "text", Required: true},
		{Name: "content", Type: "text"},
	}
	db, _ := dbService.CreateDatabase("", "Articles", columns)
	_, _ = dbService.CreateRecord("", db.ID, map[string]any{"title": "Getting Started with Go", "content": "Introduction to searchable content"})

	// Search should find both page and record
	results, err := searchService.Search("", SearchOptions{
		Query:       "searchable",
		MatchTitle:  true,
		MatchBody:   true,
		MatchFields: true,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) < 2 {
		t.Errorf("Expected at least 2 results, got %d", len(results))
		for i, r := range results {
			t.Logf("Result %d: type=%s, title=%s", i, r.Type, r.Title)
		}
	}

	// Check that we have both types
	hasPage := false
	hasRecord := false
	for _, result := range results {
		if result.Type == "page" {
			hasPage = true
		}
		if result.Type == "record" {
			hasRecord = true
		}
	}

	if !hasPage {
		t.Error("Expected at least one page result")
	}
	if !hasRecord {
		t.Error("Expected at least one record result")
	}
}