package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/maruel/mddb/backend/internal/entity"
	"github.com/maruel/mddb/backend/internal/jsonldb"
)

// PageService handles page business logic.
type PageService struct {
	fileStore  *FileStore
	gitService *GitService
	cache      *Cache
	orgService *OrganizationService
}

// NewPageService creates a new page service.
func NewPageService(fileStore *FileStore, gitService *GitService, cache *Cache, orgService *OrganizationService) *PageService {
	return &PageService{
		fileStore:  fileStore,
		gitService: gitService,
		cache:      cache,
		orgService: orgService,
	}
}

// GetPage retrieves a page by ID.
func (s *PageService) GetPage(ctx context.Context, idStr string) (*entity.Page, error) {
	if idStr == "" {
		return nil, fmt.Errorf("page id cannot be empty")
	}

	id, err := jsonldb.DecodeID(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid page id: %w", err)
	}

	orgID := entity.GetOrgID(ctx)
	if page, ok := s.cache.GetPage(id); ok {
		return page, nil
	}

	page, err := s.fileStore.ReadPage(orgID, id)
	if err != nil {
		return nil, err
	}

	s.cache.SetPage(page)
	return page, nil
}

// CreatePage creates a new page with a generated numeric ID.
func (s *PageService) CreatePage(ctx context.Context, title, content string) (*entity.Page, error) {
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}

	orgID := entity.GetOrgID(ctx)

	// Check Quota
	if s.orgService != nil {
		org, err := s.orgService.GetOrganization(orgID)
		if err == nil && org.Quotas.MaxPages > 0 {
			count, _, err := s.fileStore.GetOrganizationUsage(orgID)
			if err == nil && count >= org.Quotas.MaxPages {
				return nil, fmt.Errorf("page quota exceeded (%d/%d)", count, org.Quotas.MaxPages)
			}
		}
	}

	id := jsonldb.NewID()

	page, err := s.fileStore.WritePage(orgID, id, title, content)
	if err != nil {
		return nil, err
	}

	// Invalidate node tree cache
	s.cache.InvalidateNodeTree()
	s.cache.SetPage(page)

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, "create", "page", id.String(), title); err != nil {
			// Log error but don't fail the operation
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return page, nil
}

// UpdatePage updates an existing page.
func (s *PageService) UpdatePage(ctx context.Context, idStr, title, content string) (*entity.Page, error) {
	if idStr == "" {
		return nil, fmt.Errorf("page id cannot be empty")
	}
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}

	id, err := jsonldb.DecodeID(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid page id: %w", err)
	}

	orgID := entity.GetOrgID(ctx)
	page, err := s.fileStore.UpdatePage(orgID, id, title, content)
	if err != nil {
		return nil, err
	}

	// Update cache
	s.cache.SetPage(page)
	s.cache.InvalidateNodeTree() // Title might have changed

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, "update", "page", idStr, "Updated content"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return page, nil
}

// DeletePage deletes a page.
func (s *PageService) DeletePage(ctx context.Context, idStr string) error {
	if idStr == "" {
		return fmt.Errorf("page id cannot be empty")
	}
	id, err := jsonldb.DecodeID(idStr)
	if err != nil {
		return fmt.Errorf("invalid page id: %w", err)
	}
	orgID := entity.GetOrgID(ctx)
	if err := s.fileStore.DeletePage(orgID, id); err != nil {
		return err
	}

	// Invalidate cache
	s.cache.InvalidatePage(id)
	s.cache.InvalidateNodeTree()

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, "delete", "page", idStr, "Deleted page"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}

// ListPages returns all pages.
func (s *PageService) ListPages(ctx context.Context) ([]*entity.Page, error) {
	orgID := entity.GetOrgID(ctx)
	return s.fileStore.ListPages(orgID)
}

// SearchPages performs a simple text search across pages.
func (s *PageService) SearchPages(ctx context.Context, query string) ([]*entity.Page, error) {
	if query == "" {
		return []*entity.Page{}, nil
	}

	orgID := entity.GetOrgID(ctx)
	pages, err := s.fileStore.ListPages(orgID)
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(query)
	var results []*entity.Page

	for _, page := range pages {
		if strings.Contains(strings.ToLower(page.Title), queryLower) ||
			strings.Contains(strings.ToLower(page.Content), queryLower) {
			results = append(results, page)
		}
	}

	return results, nil
}

// GetPageHistory returns the commit history for a page.
func (s *PageService) GetPageHistory(ctx context.Context, id string) ([]*entity.Commit, error) {
	if s.gitService == nil {
		return []*entity.Commit{}, nil
	}
	return s.gitService.GetHistory(ctx, "page", id)
}

// GetPageVersion returns the content of a page at a specific commit.
func (s *PageService) GetPageVersion(ctx context.Context, id, commitHash string) (string, error) {
	if s.gitService == nil {
		return "", fmt.Errorf("git service not available")
	}

	orgID := entity.GetOrgID(ctx)
	// New storage path: {orgID}/pages/{id}/index.md
	path := fmt.Sprintf("%s/pages/%s/index.md", orgID.String(), id)
	if orgID.IsZero() {
		path = fmt.Sprintf("pages/%s/index.md", id)
	}

	contentBytes, err := s.gitService.GetFileAtCommit(ctx, commitHash, path)
	if err != nil {
		return "", err
	}

	return string(contentBytes), nil
}
