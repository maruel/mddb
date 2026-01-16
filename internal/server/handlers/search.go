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

// SearchResponse is the response to a search request
type SearchResponse struct {
	Results []*storage.SearchResult `json:"results"`
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
	matchTitle := req.MatchTitle || (!req.MatchTitle && !req.MatchBody && !req.MatchFields)
	matchBody := req.MatchBody || (!req.MatchBody && !req.MatchTitle && !req.MatchFields)
	matchFields := req.MatchFields || (!req.MatchFields && !req.MatchTitle && !req.MatchBody)

	results, err := h.searchService.Search(storage.SearchOptions{
		Query:       req.Query,
		MatchTitle:  matchTitle,
		MatchBody:   matchBody,
		MatchFields: matchFields,
	})
	if err != nil {
		return nil, errors.InternalWithError("Search failed", err)
	}

	response := &SearchResponse{
		Results: make([]*storage.SearchResult, len(results)),
	}

	for i := range results {
		response.Results[i] = &results[i]
	}

	return response, nil
}
