package storage

import (
	"strings"
	"time"
)

// SearchService provides full-text search across pages and databases.
type SearchService struct {
	pageService     *PageService
	databaseService *DatabaseService
}

// NewSearchService creates a new search service.
func NewSearchService(fileStore *FileStore) *SearchService {
	return &SearchService{
		pageService:     NewPageService(fileStore, nil),
		databaseService: NewDatabaseService(fileStore, nil),
	}
}

// SearchResult represents a single search result.
type SearchResult struct {
	Type      string    `json:"type"`      // "page" or "database" or "record"
	ID        string    `json:"id"`        // Page ID or Database ID
	RecordID  *string   `json:"record_id"` // Only for records
	Title     string    `json:"title"`
	Content   string    `json:"content"`     // Preview or snippet
	Matches   int       `json:"matches"`     // Number of matches found
	Score     float64   `json:"score"`       // Relevance score (0-1)
	Created   time.Time `json:"created"`
	Modified  time.Time `json:"modified"`
}

// SearchOptions controls search behavior.
type SearchOptions struct {
	Query       string // Search query (case-insensitive)
	Limit       int    // Max results (0 = no limit)
	MatchTitle  bool   // Search in titles (default: true)
	MatchBody   bool   // Search in body/content (default: true)
	MatchFields bool   // Search in record fields (default: true)
}

// Search performs a full-text search across pages and databases.
// Returns results sorted by relevance score (highest first).
func (s *SearchService) Search(opts SearchOptions) ([]SearchResult, error) {
	if opts.Query == "" {
		return []SearchResult{}, nil
	}

	// Default options
	if opts.MatchTitle == false && opts.MatchBody == false && opts.MatchFields == false {
		opts.MatchTitle = true
		opts.MatchBody = true
		opts.MatchFields = true
	}

	query := strings.ToLower(opts.Query)
	var results []SearchResult

	// Search pages
	if opts.MatchTitle || opts.MatchBody {
		pageResults := s.searchPages(query, opts)
		results = append(results, pageResults...)
	}

	// Search databases
	if opts.MatchTitle || opts.MatchFields {
		dbResults := s.searchDatabases(query, opts)
		results = append(results, dbResults...)
	}

	// Sort by score (descending)
	sortResultsByScore(results)

	// Apply limit
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results, nil
}

// searchPages searches through all pages.
func (s *SearchService) searchPages(query string, opts SearchOptions) []SearchResult {
	pages, err := s.pageService.ListPages()
	if err != nil {
		return []SearchResult{}
	}

	var results []SearchResult
	for _, page := range pages {
		matches := 0
		score := 0.0

		// Search title
		if opts.MatchTitle {
			titleMatches := countMatches(strings.ToLower(page.Title), query)
			if titleMatches > 0 {
				matches += titleMatches
				score += 0.5 * float64(titleMatches) // Title matches weighted higher
			}
		}

		// Search content
		if opts.MatchBody {
			contentMatches := countMatches(strings.ToLower(page.Content), query)
			if contentMatches > 0 {
				matches += contentMatches
				score += 0.1 * float64(contentMatches)
			}
		}

		if matches > 0 {
			// Normalize score to 0-1
			score = min(score, 1.0)

			// Create preview snippet from content
			preview := createPreview(page.Content, query, 100)

			results = append(results, SearchResult{
				Type:     "page",
				ID:       page.ID,
				Title:    page.Title,
				Content:  preview,
				Matches:  matches,
				Score:    score,
				Created:  page.Created,
				Modified: page.Modified,
			})
		}
	}

	return results
}

// searchDatabases searches through all databases and their records.
func (s *SearchService) searchDatabases(query string, opts SearchOptions) []SearchResult {
	databases, err := s.databaseService.ListDatabases()
	if err != nil {
		return []SearchResult{}
	}

	var results []SearchResult

	for _, db := range databases {
		dbMatches := 0

		// Search database title
		if opts.MatchTitle {
			titleMatches := countMatches(strings.ToLower(db.Title), query)
			if titleMatches > 0 {
				dbMatches += titleMatches
				// Add database itself as result
				results = append(results, SearchResult{
					Type:     "database",
					ID:       db.ID,
					Title:    db.Title,
					Content:  "Database schema",
					Matches:  titleMatches,
					Score:    0.5 * float64(titleMatches),
					Created:  db.Created,
					Modified: db.Modified,
				})
			}
		}

		// Search records in database
		if opts.MatchFields {
			records, err := s.databaseService.GetRecords(db.ID)
			if err != nil {
				continue
			}

			for _, record := range records {
				recordMatches := 0
				score := 0.0

				// Search all fields in the record
				for _, fieldValue := range record.Data {
					valueStr := valueToString(fieldValue)
					fieldMatches := countMatches(strings.ToLower(valueStr), query)
					if fieldMatches > 0 {
						recordMatches += fieldMatches
						score += 0.2 * float64(fieldMatches)
					}
				}

				if recordMatches > 0 {
					score = min(score, 1.0)

					// Create preview from record data
					preview := createRecordPreview(record.Data, query)

					results = append(results, SearchResult{
						Type:     "record",
						ID:       db.ID,
						RecordID: &record.ID,
						Title:    db.Title,
						Content:  preview,
						Matches:  recordMatches,
						Score:    score,
						Created:  record.Created,
						Modified: record.Modified,
					})
				}
			}
		}
	}

	return results
}

// countMatches counts how many times query appears in text.
func countMatches(text, query string) int {
	count := 0
	for {
		index := strings.Index(text, query)
		if index == -1 {
			break
		}
		count++
		text = text[index+len(query):]
	}
	return count
}

// createPreview creates a snippet around the first match.
func createPreview(text, query string, maxLen int) string {
	text = strings.ToLower(text)
	index := strings.Index(text, query)
	if index == -1 {
		// No match found, return first part of text
		if len(text) > maxLen {
			return text[:maxLen] + "..."
		}
		return text
	}

	// Create snippet with context
	start := index - 20
	if start < 0 {
		start = 0
	}

	end := index + len(query) + 30
	if end > len(text) {
		end = len(text)
	}

	snippet := text[start:end]

	// Add ellipsis if truncated
	prefix := ""
	suffix := ""
	if start > 0 {
		prefix = "..."
	}
	if end < len(text) {
		suffix = "..."
	}

	return prefix + snippet + suffix
}

// createRecordPreview creates a preview of record fields.
func createRecordPreview(data map[string]any, query string) string {
	var preview strings.Builder
	count := 0

	for fieldName, fieldValue := range data {
		if count >= 2 {
			break
		}
		valueStr := valueToString(fieldValue)
		if strings.Contains(strings.ToLower(valueStr), query) {
			if preview.Len() > 0 {
				preview.WriteString(", ")
			}
			preview.WriteString(fieldName + ": " + truncate(valueStr, 30))
			count++
		}
	}

	if preview.Len() == 0 {
		// Just show first field
		for fieldName, fieldValue := range data {
			valueStr := valueToString(fieldValue)
			return fieldName + ": " + truncate(valueStr, 50)
		}
	}

	return preview.String()
}

// valueToString converts any value to a string for searching.
func valueToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return formatFloat(val)
	case bool:
		return formatBool(val)
	case nil:
		return ""
	default:
		return ""
	}
}

// formatFloat formats a float for display.
func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return formatInt(int64(f))
	}
	return string(rune(int(f)))
}

// formatInt formats an int as a string.
func formatInt(i int64) string {
	if i == 0 {
		return "0"
	}
	var result strings.Builder
	negative := i < 0
	if negative {
		i = -i
	}
	for i > 0 {
		result.WriteRune(rune('0' + i%10))
		i /= 10
	}
	if negative {
		result.WriteRune('-')
	}
	// Reverse
	s := result.String()
	runes := make([]rune, len(s))
	for i, r := range s {
		runes[len(s)-1-i] = r
	}
	return string(runes)
}

// formatBool formats a boolean as a string.
func formatBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// truncate limits string length with ellipsis.
func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// sortResultsByScore sorts results by score in descending order.
func sortResultsByScore(results []SearchResult) {
	// Simple bubble sort (could use sort.Slice for production)
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// min returns the minimum of two values.
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
