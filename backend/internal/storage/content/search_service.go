package content

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// SearchService handles full-text search across nodes.
type SearchService struct {
	fileStore *FileStore
}

// NewSearchService creates a new search service.
func NewSearchService(fileStore *FileStore) *SearchService {
	return &SearchService{
		fileStore: fileStore,
	}
}

// Search performs a full-text search across all nodes.
func (s *SearchService) Search(ctx context.Context, orgID jsonldb.ID, opts SearchOptions) ([]SearchResult, error) {
	if opts.Query == "" {
		return nil, nil
	}

	if !opts.MatchTitle && !opts.MatchBody && !opts.MatchFields {
		opts.MatchTitle = true
		opts.MatchBody = true
		opts.MatchFields = true
	}

	query := strings.ToLower(opts.Query)
	var results []SearchResult

	// Search pages
	if opts.MatchTitle || opts.MatchBody {
		pageResults := s.searchPages(orgID, query, opts)
		results = append(results, pageResults...)
	}

	// Search tables
	if opts.MatchFields {
		tableResults := s.searchTables(orgID, query, opts)
		results = append(results, tableResults...)
	}

	// Sort by score
	sortResultsByScore(results)

	// Apply limit
	if opts.Limit > 0 && len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results, nil
}

func (s *SearchService) searchPages(orgID jsonldb.ID, query string, opts SearchOptions) []SearchResult {
	nodes, err := s.fileStore.ReadNodeTree(orgID)
	if err != nil {
		slog.Warn("Failed to read node tree for page search", "orgID", orgID, "error", err)
		return nil
	}
	var results []SearchResult

	var processNodes func([]*Node)
	processNodes = func(list []*Node) {
		for _, node := range list {
			if node.Type != NodeTypeTable {
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
					results = append(results, SearchResult{
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

func (s *SearchService) searchTables(orgID jsonldb.ID, query string, opts SearchOptions) []SearchResult { //nolint:unparam // opts might be used for future table-specific filtering
	nodes, err := s.fileStore.ReadNodeTree(orgID)
	if err != nil {
		slog.Warn("Failed to read node tree for table search", "orgID", orgID, "error", err)
		return nil
	}
	var results []SearchResult

	var processNodes func([]*Node)
	processNodes = func(list []*Node) {
		for _, node := range list {
			if node.Type != NodeTypeDocument {
				it, err := s.fileStore.IterRecords(orgID, node.ID)
				if err != nil {
					slog.Warn("Failed to iterate records for table search", "orgID", orgID, "nodeID", node.ID, "error", err)
					continue
				}
				for record := range it {
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
						results = append(results, SearchResult{
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
	lowerContent := strings.ToLower(content)
	lowerQuery := strings.ToLower(query)

	// Find byte index first
	byteIdx := strings.Index(lowerContent, lowerQuery)
	if byteIdx == -1 {
		return truncate(content, 100)
	}

	// Convert to rune slice to avoid splitting multi-byte characters
	runes := []rune(content)

	// Calculate rune index by counting runes up to byteIdx
	runeIdx := len([]rune(content[:byteIdx]))
	queryRuneLen := len([]rune(query))

	start := max(runeIdx-50, 0)
	end := min(runeIdx+queryRuneLen+50, len(runes))

	snippet := string(runes[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(runes) {
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

// truncate limits string length with ellipsis using rune count.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return s
}

// sortResultsByScore sorts results by score in descending order.
func sortResultsByScore(results []SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
}
