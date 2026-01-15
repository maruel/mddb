package content

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/git"
)

// mockQuotaGetterSearch implements the QuotaGetter interface for testing.
type mockQuotaGetterSearch struct {
	quotas map[jsonldb.ID]entity.Quota
}

func (m *mockQuotaGetterSearch) GetQuota(ctx context.Context, orgID jsonldb.ID) (entity.Quota, error) {
	if quota, exists := m.quotas[orgID]; exists {
		return quota, nil
	}
	// Return default quota if not found
	return entity.Quota{MaxPages: 100, MaxStorage: 1000000, MaxUsers: 10}, nil
}

func TestSearchService_SearchPages(t *testing.T) {
	tmpDir := t.TempDir()
	fileStore, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}
	gitService, err := git.New(t.Context(), tmpDir, "", "")
	if err != nil {
		t.Fatalf("Failed to create git service: %v", err)
	}
	searchService := NewSearchService(fileStore)
	orgID := jsonldb.ID(100)
	if err := os.MkdirAll(filepath.Join(tmpDir, orgID.String()), 0o750); err != nil {
		t.Fatalf("Failed to create org dir: %v", err)
	}
	if err := gitService.Init(t.Context(), orgID.String()); err != nil {
		t.Fatalf("Failed to init git for org: %v", err)
	}

	// Create test pages
	mockQuotaGetter := &mockQuotaGetterSearch{
		quotas: map[jsonldb.ID]entity.Quota{
			orgID: {MaxPages: 100, MaxStorage: 1000000, MaxUsers: 10},
		},
	}
	pageService := NewPageService(fileStore, gitService, mockQuotaGetter)
	ctx := t.Context()
	if _, err := pageService.Create(ctx, orgID, "Getting Started", "This is a guide to get started with mddb project", "", ""); err != nil {
		t.Fatalf("Create Getting Started failed: %v", err)
	}
	if _, err := pageService.Create(ctx, orgID, "Advanced Topics", "Learn about advanced mddb configuration and optimization", "", ""); err != nil {
		t.Fatalf("Create Advanced Topics failed: %v", err)
	}
	if _, err := pageService.Create(ctx, orgID, "API Reference", "Complete mddb API documentation for developers", "", ""); err != nil {
		t.Fatalf("Create API Reference failed: %v", err)
	}

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
			results, err := searchService.Search(ctx, orgID, SearchOptions{
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
	gitService, err := git.New(t.Context(), tmpDir, "", "")
	if err != nil {
		t.Fatalf("Failed to create git service: %v", err)
	}
	searchService := NewSearchService(fileStore)
	orgID := jsonldb.ID(100)
	if err := os.MkdirAll(filepath.Join(tmpDir, orgID.String()), 0o750); err != nil {
		t.Fatalf("Failed to create org dir: %v", err)
	}
	if err := gitService.Init(t.Context(), orgID.String()); err != nil {
		t.Fatalf("Failed to init git for org: %v", err)
	}
	ctx := t.Context()

	// Create test database with records
	mockQuotaGetterDB := &mockQuotaGetterSearch{
		quotas: map[jsonldb.ID]entity.Quota{
			orgID: {MaxPages: 100, MaxStorage: 1000000, MaxUsers: 10},
		},
	}
	dbService := NewDatabaseService(fileStore, gitService, mockQuotaGetterDB)
	columns := []Property{
		{Name: "title", Type: "text", Required: true},
		{Name: "status", Type: PropertyTypeText},
		{Name: "description", Type: "text"},
	}

	db, err := dbService.Create(ctx, orgID, "Tasks", columns)
	if err != nil {
		t.Fatalf("Create database failed: %v", err)
	}

	// Create records
	if _, err := dbService.CreateRecord(ctx, orgID, db.ID, map[string]any{"title": "Buy groceries", "status": "todo", "description": "Fresh vegetables"}); err != nil {
		t.Fatalf("Create record failed: %v", err)
	}
	if _, err := dbService.CreateRecord(ctx, orgID, db.ID, map[string]any{"title": "Finish report", "status": "done", "description": "Quarterly performance"}); err != nil {
		t.Fatalf("Create record failed: %v", err)
	}
	if _, err := dbService.CreateRecord(ctx, orgID, db.ID, map[string]any{"title": "Review code", "status": "todo", "description": "Pull request on main repo"}); err != nil {
		t.Fatalf("Create record failed: %v", err)
	}

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
			results, err := searchService.Search(ctx, orgID, SearchOptions{
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
	fileStore, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}
	gitService, err := git.New(t.Context(), tmpDir, "", "")
	if err != nil {
		t.Fatalf("Failed to create git service: %v", err)
	}
	searchService := NewSearchService(fileStore)
	orgID := jsonldb.ID(100)
	if err := os.MkdirAll(filepath.Join(tmpDir, orgID.String()), 0o750); err != nil {
		t.Fatalf("Failed to create org dir: %v", err)
	}
	if err := gitService.Init(t.Context(), orgID.String()); err != nil {
		t.Fatalf("Failed to init git for org: %v", err)
	}
	ctx := t.Context()

	// Create pages where title match should score higher
	mockQuotaGetterScoring := &mockQuotaGetterSearch{
		quotas: map[jsonldb.ID]entity.Quota{
			orgID: {MaxPages: 100, MaxStorage: 1000000, MaxUsers: 10},
		},
	}
	pageService := NewPageService(fileStore, gitService, mockQuotaGetterScoring)
	if _, err := pageService.Create(ctx, orgID, "Python Programming", "This is about Java not Python", "", ""); err != nil {
		t.Fatalf("Create Python Programming failed: %v", err)
	}
	if _, err := pageService.Create(ctx, orgID, "Java Basics", "Learn Python programming fundamentals", "", ""); err != nil {
		t.Fatalf("Create Java Basics failed: %v", err)
	}

	results, err := searchService.Search(ctx, orgID, SearchOptions{
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
	gitService, err := git.New(t.Context(), tmpDir, "", "")
	if err != nil {
		t.Fatalf("Failed to create git service: %v", err)
	}
	searchService := NewSearchService(fileStore)
	orgID := jsonldb.ID(100)
	if err := os.MkdirAll(filepath.Join(tmpDir, orgID.String()), 0o750); err != nil {
		t.Fatalf("Failed to create org dir: %v", err)
	}
	if err := gitService.Init(t.Context(), orgID.String()); err != nil {
		t.Fatalf("Failed to init git for org: %v", err)
	}
	ctx := t.Context()

	// Create multiple pages
	mockQuotaGetterLimit := &mockQuotaGetterSearch{
		quotas: map[jsonldb.ID]entity.Quota{
			orgID: {MaxPages: 100, MaxStorage: 1000000, MaxUsers: 10},
		},
	}
	pageService := NewPageService(fileStore, gitService, mockQuotaGetterLimit)
	for i := range 10 {
		if _, err := pageService.Create(ctx, orgID, fmt.Sprintf("Test Page %d", i), "This is test content", "", ""); err != nil {
			t.Fatalf("Create Test Page %d failed: %v", i, err)
		}
	}

	results, err := searchService.Search(ctx, orgID, SearchOptions{
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
	fileStore, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}
	gitService, err := git.New(t.Context(), tmpDir, "", "")
	if err != nil {
		t.Fatalf("Failed to create git service: %v", err)
	}
	searchService := NewSearchService(fileStore)
	orgID := jsonldb.ID(100)
	if err := os.MkdirAll(filepath.Join(tmpDir, orgID.String()), 0o750); err != nil {
		t.Fatalf("Failed to create org dir: %v", err)
	}
	if err := gitService.Init(t.Context(), orgID.String()); err != nil {
		t.Fatalf("Failed to init git for org: %v", err)
	}
	ctx := t.Context()

	// Create mixed content
	mockQuotaGetterIntegration := &mockQuotaGetterSearch{
		quotas: map[jsonldb.ID]entity.Quota{
			orgID: {MaxPages: 100, MaxStorage: 1000000, MaxUsers: 10},
		},
	}
	pageService := NewPageService(fileStore, gitService, mockQuotaGetterIntegration)
	if _, err := pageService.Create(ctx, orgID, "Blog Post", "Article about searchable content and web development", "", ""); err != nil {
		t.Fatalf("Create Blog Post failed: %v", err)
	}

	dbService := NewDatabaseService(fileStore, gitService, mockQuotaGetterIntegration)
	columns := []Property{
		{Name: "title", Type: "text", Required: true},
		{Name: "content", Type: "text"},
	}
	db, err := dbService.Create(ctx, orgID, "Articles", columns)
	if err != nil {
		t.Fatalf("Create database failed: %v", err)
	}
	if _, err := dbService.CreateRecord(ctx, orgID, db.ID, map[string]any{"title": "Getting Started with Go", "content": "Introduction to searchable content"}); err != nil {
		t.Fatalf("Create record failed: %v", err)
	}

	// Search should find both page and record
	results, err := searchService.Search(ctx, orgID, SearchOptions{
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
