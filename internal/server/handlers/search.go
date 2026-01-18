package handlers

import (
	"context"

	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/models"
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
	OrgID       string `path:"orgID"`
	Query       string `json:"query"`
	Limit       int    `json:"limit,omitempty"`
	MatchTitle  bool   `json:"match_title,omitempty"`
	MatchBody   bool   `json:"match_body,omitempty"`
	MatchFields bool   `json:"match_fields,omitempty"`
}

// SearchResponse is the response to a search request
type SearchResponse struct {
	Results []models.SearchResult `json:"results"`
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

// Search performs a full-text search across all nodes.
func (h *SearchHandler) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	results, err := h.searchService.Search(ctx, models.SearchOptions{
		Query:       req.Query,
		Limit:       req.Limit,
		MatchTitle:  req.MatchTitle,
		MatchBody:   req.MatchBody,
		MatchFields: req.MatchFields,
	})
	if err != nil {
		return nil, errors.InternalWithError("Failed to perform search", err)
	}

	return &SearchResponse{Results: results}, nil
}