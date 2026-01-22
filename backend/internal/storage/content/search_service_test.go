package content

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func TestSearchService(t *testing.T) {
	t.Run("SearchPages", func(t *testing.T) {
		fs, orgID := testFileStore(t)
		ctx := context.Background()
		author := Author{Name: "Test", Email: "test@test.com"}
		searchService := NewSearchService(fs)

		// Create org directory and initialize git repo
		if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
			t.Fatalf("failed to create org dir: %v", err)
		}
		if err := fs.Git.Init(ctx, orgID.String()); err != nil {
			t.Fatalf("failed to init org git repo: %v", err)
		}

		// Create test pages
		if _, err := fs.WritePage(ctx, orgID, jsonldb.NewID(), "Getting Started", "This is a guide to get started with mddb project", author); err != nil {
			t.Fatalf("WritePage Getting Started failed: %v", err)
		}
		if _, err := fs.WritePage(ctx, orgID, jsonldb.NewID(), "Advanced Topics", "Learn about advanced mddb configuration and optimization", author); err != nil {
			t.Fatalf("WritePage Advanced Topics failed: %v", err)
		}
		if _, err := fs.WritePage(ctx, orgID, jsonldb.NewID(), "API Reference", "Complete mddb API documentation for developers", author); err != nil {
			t.Fatalf("WritePage API Reference failed: %v", err)
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
	})

	t.Run("SearchRecords", func(t *testing.T) {
		fs, orgID := testFileStore(t)
		ctx := context.Background()
		author := Author{Name: "Test", Email: "test@test.com"}
		searchService := NewSearchService(fs)

		// Create org directory and initialize git repo
		if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
			t.Fatalf("failed to create org dir: %v", err)
		}
		if err := fs.Git.Init(ctx, orgID.String()); err != nil {
			t.Fatalf("failed to init org git repo: %v", err)
		}

		// Create test table with records
		dbID := jsonldb.NewID()
		node := &Node{
			ID:    dbID,
			Title: "Tasks",
			Type:  NodeTypeTable,
			Properties: []Property{
				{Name: "title", Type: "text", Required: true},
				{Name: "status", Type: PropertyTypeText},
				{Name: "description", Type: "text"},
			},
			Created:  time.Now(),
			Modified: time.Now(),
		}
		if err := fs.WriteTable(ctx, orgID, node, true, author); err != nil {
			t.Fatalf("WriteTable failed: %v", err)
		}

		// Create records
		records := []map[string]any{
			{"title": "Buy groceries", "status": "todo", "description": "Fresh vegetables"},
			{"title": "Finish report", "status": "done", "description": "Quarterly performance"},
			{"title": "Review code", "status": "todo", "description": "Pull request on main repo"},
		}
		for _, data := range records {
			rec := &DataRecord{
				ID:       jsonldb.NewID(),
				Data:     data,
				Created:  time.Now(),
				Modified: time.Now(),
			}
			if err := fs.AppendRecord(ctx, orgID, dbID, rec, author); err != nil {
				t.Fatalf("AppendRecord failed: %v", err)
			}
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
	})

	t.Run("Scoring", func(t *testing.T) {
		fs, orgID := testFileStore(t)
		ctx := context.Background()
		author := Author{Name: "Test", Email: "test@test.com"}
		searchService := NewSearchService(fs)

		// Create org directory and initialize git repo
		if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
			t.Fatalf("failed to create org dir: %v", err)
		}
		if err := fs.Git.Init(ctx, orgID.String()); err != nil {
			t.Fatalf("failed to init org git repo: %v", err)
		}

		// Create pages where title match should score higher
		if _, err := fs.WritePage(ctx, orgID, jsonldb.NewID(), "Python Programming", "This is about Java not Python", author); err != nil {
			t.Fatalf("WritePage Python Programming failed: %v", err)
		}
		if _, err := fs.WritePage(ctx, orgID, jsonldb.NewID(), "Java Basics", "Learn Python programming fundamentals", author); err != nil {
			t.Fatalf("WritePage Java Basics failed: %v", err)
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
	})

	t.Run("Limit", func(t *testing.T) {
		fs, orgID := testFileStore(t)
		ctx := context.Background()
		author := Author{Name: "Test", Email: "test@test.com"}
		searchService := NewSearchService(fs)

		// Create org directory and initialize git repo
		if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
			t.Fatalf("failed to create org dir: %v", err)
		}
		if err := fs.Git.Init(ctx, orgID.String()); err != nil {
			t.Fatalf("failed to init org git repo: %v", err)
		}

		// Create multiple pages
		for i := range 10 {
			if _, err := fs.WritePage(ctx, orgID, jsonldb.NewID(), fmt.Sprintf("Test Page %d", i), "This is test content", author); err != nil {
				t.Fatalf("WritePage Test Page %d failed: %v", i, err)
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
	})

	t.Run("Integration", func(t *testing.T) {
		fs, orgID := testFileStore(t)
		ctx := context.Background()
		author := Author{Name: "Test", Email: "test@test.com"}
		searchService := NewSearchService(fs)

		// Create org directory and initialize git repo
		if err := os.MkdirAll(filepath.Join(fs.rootDir, orgID.String()), 0o750); err != nil {
			t.Fatalf("failed to create org dir: %v", err)
		}
		if err := fs.Git.Init(ctx, orgID.String()); err != nil {
			t.Fatalf("failed to init org git repo: %v", err)
		}

		// Create mixed content - page
		if _, err := fs.WritePage(ctx, orgID, jsonldb.NewID(), "Blog Post", "Article about searchable content and web development", author); err != nil {
			t.Fatalf("WritePage Blog Post failed: %v", err)
		}

		// Create table with record
		dbID := jsonldb.NewID()
		node := &Node{
			ID:    dbID,
			Title: "Articles",
			Type:  NodeTypeTable,
			Properties: []Property{
				{Name: "title", Type: "text", Required: true},
				{Name: "content", Type: "text"},
			},
			Created:  time.Now(),
			Modified: time.Now(),
		}
		if err := fs.WriteTable(ctx, orgID, node, true, author); err != nil {
			t.Fatalf("WriteTable failed: %v", err)
		}
		rec := &DataRecord{
			ID:       jsonldb.NewID(),
			Data:     map[string]any{"title": "Getting Started with Go", "content": "Introduction to searchable content"},
			Created:  time.Now(),
			Modified: time.Now(),
		}
		if err := fs.AppendRecord(ctx, orgID, dbID, rec, author); err != nil {
			t.Fatalf("AppendRecord failed: %v", err)
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
	})

	t.Run("CreateSnippet", func(t *testing.T) {
		fs, _ := testFileStore(t)
		searchSvc := NewSearchService(fs)

		t.Run("ValidUTF8WithMultiByteChars", func(t *testing.T) {
			content := strings.Repeat("日本語", 20) + "query" + strings.Repeat("中文字", 20)
			snippet := searchSvc.createSnippet(content, "query")

			if !utf8.ValidString(snippet) {
				t.Errorf("snippet contains invalid UTF-8: %q", snippet)
			}
		})

		t.Run("QueryAtBeginning", func(t *testing.T) {
			content := "query" + strings.Repeat("日本語テスト", 30)
			snippet := searchSvc.createSnippet(content, "query")

			if !utf8.ValidString(snippet) {
				t.Errorf("snippet contains invalid UTF-8: %q", snippet)
			}
		})

		t.Run("QueryAtEnd", func(t *testing.T) {
			content := strings.Repeat("日本語テスト", 30) + "query"
			snippet := searchSvc.createSnippet(content, "query")

			if !utf8.ValidString(snippet) {
				t.Errorf("snippet contains invalid UTF-8: %q", snippet)
			}
		})
	})
}
