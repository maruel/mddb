// Handles full-text search endpoints.

package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// SearchHandler handles search-related HTTP requests.
type SearchHandler struct {
	Svc *Services
}

// Search performs a full-text search across all nodes.
func (h *SearchHandler) Search(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.SearchRequest) (*dto.SearchResponse, error) {
	results, err := h.Svc.Search.Search(ctx, wsID, content.SearchOptions{
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
