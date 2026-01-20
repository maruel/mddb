package content

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

// SearchService handles full-text search across nodes.
type SearchService struct {
	fileStore *infra.FileStore
}

// NewSearchService creates a new search service.
func NewSearchService(fileStore *infra.FileStore) *SearchService {
	return &SearchService{
		fileStore: fileStore,
	}
}

// Search performs a full-text search across all nodes.
func (s *SearchService) Search(ctx context.Context, orgID jsonldb.ID, opts entity.SearchOptions) ([]entity.SearchResult, error) {
	if opts.Query == "" {
		return nil, nil
	}

	if !opts.MatchTitle && !opts.MatchBody && !opts.MatchFields {
		opts.MatchTitle = true
		opts.MatchBody = true
		opts.MatchFields = true
	}

	query := strings.ToLower(opts.Query)
	var results []entity.SearchResult

	// Search pages
	if opts.MatchTitle || opts.MatchBody {
		pageResults := s.searchPages(orgID, query, opts)
		results = append(results, pageResults...)
	}

	// Search databases
	if opts.MatchFields {
		dbResults := s.searchDatabases(orgID, query, opts)
		results = append(results, dbResults...)
	}

	// Sort by score
	sortResultsByScore(results)

	// Apply limit
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results, nil
}

func (s *SearchService) searchPages(orgID jsonldb.ID, query string, opts entity.SearchOptions) []entity.SearchResult {
	nodes, _ := s.fileStore.ReadNodeTree(orgID)
	var results []entity.SearchResult

	var processNodes func([]*entity.Node)
	processNodes = func(list []*entity.Node) {
		for _, node := range list {
			if node.Type != entity.NodeTypeDatabase {
				score := 0.0
				matches := make(map[string]string)
				snippet := ""

				if opts.MatchTitle && strings.Contains(strings.ToLower(node.Title), query) {
					score += 10.0
					matches["title"] = node.Title
				}

				if opts.MatchBody && strings.Contains(strings.ToLower(node.Content), query) {
					score += 5.0
					matches["content"] = query
					snippet = s.createSnippet(node.Content, query)
				}

				if score > 0 {
					results = append(results, entity.SearchResult{
						Type:     "page",
						NodeID:   node.ID,
						Title:    node.Title,
						Snippet:  snippet,
						Score:    min(score, 100.0),
						Matches:  matches,
						Modified: node.Modified,
					})
				}
			}
			if len(node.Children) > 0 {
				processNodes(node.Children)
			}
		}
	}
	processNodes(nodes)
	return results
}

func (s *SearchService) searchDatabases(orgID jsonldb.ID, query string, opts entity.SearchOptions) []entity.SearchResult { //nolint:unparam // opts might be used for future database-specific filtering
	nodes, _ := s.fileStore.ReadNodeTree(orgID)
	var results []entity.SearchResult

	var processNodes func([]*entity.Node)
	processNodes = func(list []*entity.Node) {
		for _, node := range list {
			if node.Type != entity.NodeTypeDocument {
				records, _ := s.fileStore.ReadRecords(orgID, node.ID)
				for _, record := range records {
					score := 0.0
					matches := make(map[string]string)
					matchedField := ""

					for key, val := range record.Data {
						strVal := valueToString(val)
						if strings.Contains(strings.ToLower(strVal), query) {
							score += 2.0
							matches[key] = strVal
							matchedField = key
						}
					}

					if score > 0 {
						recordID := record.ID
						results = append(results, entity.SearchResult{
							Type:     "record",
							NodeID:   node.ID,
							RecordID: recordID,
							Title:    node.Title,
							Snippet:  fmt.Sprintf("%s: %s", matchedField, matches[matchedField]),
							Score:    min(score, 100.0),
							Matches:  matches,
							Modified: record.Modified,
						})
					}
				}
			}
			if len(node.Children) > 0 {
				processNodes(node.Children)
			}
		}
	}
	processNodes(nodes)
	return results
}

func (s *SearchService) createSnippet(content, query string) string {
	idx := strings.Index(strings.ToLower(content), query)
	if idx == -1 {
		return truncate(content, 100)
	}

	start := idx - 50
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + 50
	if end > len(content) {
		end = len(content)
	}

	snippet := content[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(content) {
		snippet += "..."
	}
	return snippet
}

func valueToString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case int, int64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%.2f", val)
	case bool:
		return formatBool(val)
	case []any:
		var parts []string
		for _, item := range val {
			parts = append(parts, valueToString(item))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprintf("%v", v)
	}
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
func sortResultsByScore(results []entity.SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
}
