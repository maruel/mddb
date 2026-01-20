package content

import (
	"fmt"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

func TestSearchService_SearchPages(t *testing.T) {
	tmpDir := t.TempDir()
	fileStore, err := infra.NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}
	searchService := NewSearchService(fileStore)
	orgID := jsonldb.ID(100)

	// Create test pages
	cache := infra.NewCache()
	pageService := NewPageService(fileStore, nil, cache, nil)
	ctx := newTestContext(t, orgID.String())
	_, _ = pageService.CreatePage(ctx, orgID, "Getting Started", "This is a guide to get started with mddb project")
	_, _ = pageService.CreatePage(ctx, orgID, "Advanced Topics", "Learn about advanced mddb configuration and optimization")
	_, _ = pageService.CreatePage(ctx, orgID, "API Reference", "Complete mddb API documentation for developers")

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
			results, err := searchService.Search(ctx, orgID, entity.SearchOptions{
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
	fileStore, err := infra.NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}
	searchService := NewSearchService(fileStore)
	orgID := jsonldb.ID(100)
	ctx := newTestContext(t, orgID.String())

	// Create test database with records
	cache := infra.NewCache()
	dbService := NewDatabaseService(fileStore, nil, cache, nil)
	columns := []entity.Property{
		{Name: "title", Type: "text", Required: true},
		{Name: "status", Type: entity.PropertyTypeText},
		{Name: "description", Type: "text"},
	}

	db, _ := dbService.CreateDatabase(ctx, orgID, "Tasks", columns)

	// Create records
	_, _ = dbService.CreateRecord(ctx, orgID, db.ID, map[string]any{"title": "Buy groceries", "status": "todo", "description": "Fresh vegetables"})
	_, _ = dbService.CreateRecord(ctx, orgID, db.ID, map[string]any{"title": "Finish report", "status": "done", "description": "Quarterly performance"})
	_, _ = dbService.CreateRecord(ctx, orgID, db.ID, map[string]any{"title": "Review code", "status": "todo", "description": "Pull request on main repo"})

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
			results, err := searchService.Search(ctx, orgID, entity.SearchOptions{
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
				if result.Type == "record" {
					if result.RecordID.IsZero() {
						t.Error("Expected RecordID to be set for record results")
					}
				}
			}
		})
	}
}

func TestSearchService_Scoring(t *testing.T) {
	tmpDir := t.TempDir()
	fileStore, err := infra.NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}
	searchService := NewSearchService(fileStore)
	orgID := jsonldb.ID(100)
	ctx := newTestContext(t, orgID.String())

	// Create pages where title match should score higher
	cache := infra.NewCache()
	pageService := NewPageService(fileStore, nil, cache, nil)
	_, _ = pageService.CreatePage(ctx, orgID, "Python Programming", "This is about Java not Python")
	_, _ = pageService.CreatePage(ctx, orgID, "Java Basics", "Learn Python programming fundamentals")

	results, err := searchService.Search(ctx, orgID, entity.SearchOptions{
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
	fileStore, err := infra.NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}
	searchService := NewSearchService(fileStore)
	orgID := jsonldb.ID(100)
	ctx := newTestContext(t, orgID.String())

	// Create multiple pages
	cache := infra.NewCache()
	pageService := NewPageService(fileStore, nil, cache, nil)
	for i := range 10 {
		_, _ = pageService.CreatePage(ctx, orgID, fmt.Sprintf("Test Page %d", i), "This is test content")
	}

	results, err := searchService.Search(ctx, orgID, entity.SearchOptions{
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

func TestSearchService_Integration(t *testing.T) {
	tmpDir := t.TempDir()
	fileStore, err := infra.NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}
	searchService := NewSearchService(fileStore)
	orgID := jsonldb.ID(100)
	ctx := newTestContext(t, orgID.String())

	// Create mixed content
	cache := infra.NewCache()
	pageService := NewPageService(fileStore, nil, cache, nil)
	_, _ = pageService.CreatePage(ctx, orgID, "Blog Post", "Article about searchable content and web development")

	dbService := NewDatabaseService(fileStore, nil, cache, nil)
	columns := []entity.Property{
		{Name: "title", Type: "text", Required: true},
		{Name: "content", Type: "text"},
	}
	db, _ := dbService.CreateDatabase(ctx, orgID, "Articles", columns)
	_, _ = dbService.CreateRecord(ctx, orgID, db.ID, map[string]any{"title": "Getting Started with Go", "content": "Introduction to searchable content"})

	// Search should find both page and record
	results, err := searchService.Search(ctx, orgID, entity.SearchOptions{
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
