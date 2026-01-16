package handlers

import (
	"context"

	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/storage"
)

// SearchHandler handles search-related HTTP requests
type SearchHandler struct {
	searchService *storage.SearchService
}

// NewSearchHandler creates a new search handler
func NewSearchHandler(fileStore *storage.FileStore) *SearchHandler {
	return &SearchHandler{
		searchService: storage.NewSearchService(fileStore),
	}
}

// SearchRequest is a request to search pages and databases
type SearchRequest struct {
	Query       string `json:"query"`
	Limit       int    `json:"limit,omitempty"`
	MatchTitle  bool   `json:"match_title,omitempty"`
	MatchBody   bool   `json:"match_body,omitempty"`
	MatchFields bool   `json:"match_fields,omitempty"`
}

// SearchResponse is a response containing search results
type SearchResponse struct {
	Results []SearchResultDTO `json:"results"`
	Total   int               `json:"total"`
	Query   string            `json:"query"`
}

// SearchResultDTO is the DTO version of SearchResult for API responses
type SearchResultDTO struct {
	Type     string  `json:"type"`
	ID       string  `json:"id"`
	RecordID *string `json:"record_id,omitempty"`
	Title    string  `json:"title"`
	Content  string  `json:"content"`
	Matches  int     `json:"matches"`
	Score    float64 `json:"score"`
	Created  string  `json:"created"`
	Modified string  `json:"modified"`
}

// Search performs a full-text search
func (h *SearchHandler) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	if req.Query == "" {
		return nil, errors.MissingField("query")
	}

	// Set defaults if not specified
	matchTitle := req.MatchTitle || (req.MatchTitle == false && req.MatchBody == false && req.MatchFields == false)
	matchBody := req.MatchBody || (req.MatchBody == false && req.MatchTitle == false && req.MatchFields == false)
	matchFields := req.MatchFields || (req.MatchFields == false && req.MatchTitle == false && req.MatchBody == false)

	results, err := h.searchService.Search(storage.SearchOptions{
		Query:       req.Query,
		Limit:       req.Limit,
		MatchTitle:  matchTitle,
		MatchBody:   matchBody,
		MatchFields: matchFields,
	})

	if err != nil {
		return nil, errors.InternalWithError("Search failed", err)
	}

	// Convert results to DTOs
	resultDTOs := make([]SearchResultDTO, len(results))
	for i, result := range results {
		resultDTOs[i] = SearchResultDTO{
			Type:     result.Type,
			ID:       result.ID,
			RecordID: result.RecordID,
			Title:    result.Title,
			Content:  result.Content,
			Matches:  result.Matches,
			Score:    result.Score,
			Created:  result.Created.Format("2006-01-02T15:04:05Z07:00"),
			Modified: result.Modified.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	return &SearchResponse{
		Results: resultDTOs,
		Total:   len(resultDTOs),
		Query:   req.Query,
	}, nil
}
