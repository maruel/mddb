package content

import (
	"context"
	"errors"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// SearchService handles full-text search across nodes.
// TODO: This needs refactoring to work with workspace stores (iterate workspaces in org).
type SearchService struct {
	fileStore *FileStoreService
}

// NewSearchService creates a new search service.
func NewSearchService(fileStore *FileStoreService) *SearchService {
	return &SearchService{
		fileStore: fileStore,
	}
}

// Search performs a full-text search across all nodes.
func (s *SearchService) Search(ctx context.Context, orgID jsonldb.ID, opts SearchOptions) ([]SearchResult, error) {
	// TODO: Implement full-text search working with workspace stores
	_ = ctx
	_ = orgID
	_ = opts
	return nil, errors.New("search not implemented")
}
