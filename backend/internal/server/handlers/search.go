package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/models"
	"github.com/maruel/mddb/backend/internal/storage"
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

// Search performs a full-text search across all nodes.
func (h *SearchHandler) Search(ctx context.Context, req models.SearchRequest) (*models.SearchResponse, error) {
	results, err := h.searchService.Search(ctx, models.SearchOptions{
		Query:       req.Query,
		Limit:       req.Limit,
		MatchTitle:  req.MatchTitle,
		MatchBody:   req.MatchBody,
		MatchFields: req.MatchFields,
	})
	if err != nil {
		return nil, models.InternalWithError("Failed to perform search", err)
	}

	return &models.SearchResponse{Results: results}, nil
}
