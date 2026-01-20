package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/entity"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

// SearchHandler handles search-related HTTP requests.
type SearchHandler struct {
	searchService *content.SearchService
}

// NewSearchHandler creates a new search handler.
func NewSearchHandler(fileStore *infra.FileStore) *SearchHandler {
	return &SearchHandler{
		searchService: content.NewSearchService(fileStore),
	}
}

// Search performs a full-text search across all nodes.
func (h *SearchHandler) Search(ctx context.Context, orgID jsonldb.ID, _ *entity.User, req dto.SearchRequest) (*dto.SearchResponse, error) {
	results, err := h.searchService.Search(ctx, orgID, entity.SearchOptions{
		Query:       req.Query,
		Limit:       req.Limit,
		MatchTitle:  req.MatchTitle,
		MatchBody:   req.MatchBody,
		MatchFields: req.MatchFields,
	})
	if err != nil {
		return nil, dto.InternalWithError("Failed to perform search", err)
	}
	return &dto.SearchResponse{Results: searchResultsToDTO(results)}, nil
}
