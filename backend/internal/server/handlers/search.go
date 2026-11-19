// Handles full-text search endpoints.

package handlers

import (
	"context"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto/v1"
	"github.com/maruel/mddb/backend/internal/storage/content"
)

// SearchHandler handles search-related HTTP requests.
type SearchHandler struct {
	Svc *Services
}

// Search performs a full-text search across all nodes.
func (h *SearchHandler) Search(ctx context.Context, wsID ksid.ID, _ *identityv1.User, req *dto.SearchRequest) (*dto.SearchResponse, error) {
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
