package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
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

// GetPage retrieves a page by ID and returns it as a Node.
func (s *PageService) GetPage(ctx context.Context, orgID, id jsonldb.ID) (*entity.Node, error) {
	if id.IsZero() {
		return nil, fmt.Errorf("page id cannot be empty")
	}

	return s.fileStore.ReadPage(orgID, id)
}

// CreatePage creates a new page with a generated numeric ID and returns it as a Node.
func (s *PageService) CreatePage(ctx context.Context, orgID jsonldb.ID, title, content string) (*entity.Node, error) {
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}

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

	node, err := s.fileStore.WritePage(orgID, id, title, content)
	if err != nil {
		return nil, err
	}

	// Invalidate node tree cache
	s.cache.InvalidateNodeTree()

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "create", "page", id.String(), title); err != nil {
			// Log error but don't fail the operation
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return node, nil
}

// UpdatePage updates an existing page and returns it as a Node.
func (s *PageService) UpdatePage(ctx context.Context, orgID, id jsonldb.ID, title, content string) (*entity.Node, error) {
	if id.IsZero() {
		return nil, fmt.Errorf("page id cannot be empty")
	}
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}

	node, err := s.fileStore.UpdatePage(orgID, id, title, content)
	if err != nil {
		return nil, err
	}

	// Invalidate cache (title might have changed)
	s.cache.InvalidateNodeTree()

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "update", "page", id.String(), "Updated content"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return node, nil
}

// DeletePage deletes a page.
func (s *PageService) DeletePage(ctx context.Context, orgID, id jsonldb.ID) error {
	if id.IsZero() {
		return fmt.Errorf("page id cannot be empty")
	}
	if err := s.fileStore.DeletePage(orgID, id); err != nil {
		return err
	}

	// Invalidate cache
	s.cache.InvalidateNodeTree()

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "delete", "page", id.String(), "Deleted page"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}

// ListPages returns all pages as Nodes.
func (s *PageService) ListPages(ctx context.Context, orgID jsonldb.ID) ([]*entity.Node, error) {
	return s.fileStore.ListPages(orgID)
}

// SearchPages performs a simple text search across pages.
func (s *PageService) SearchPages(ctx context.Context, orgID jsonldb.ID, query string) ([]*entity.Node, error) {
	if query == "" {
		return []*entity.Node{}, nil
	}

	nodes, err := s.fileStore.ListPages(orgID)
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(query)
	var results []*entity.Node

	for _, node := range nodes {
		if strings.Contains(strings.ToLower(node.Title), queryLower) ||
			strings.Contains(strings.ToLower(node.Content), queryLower) {
			results = append(results, node)
		}
	}

	return results, nil
}

// GetPageHistory returns the commit history for a page.
func (s *PageService) GetPageHistory(ctx context.Context, orgID, id jsonldb.ID) ([]*entity.Commit, error) {
	if s.gitService == nil {
		return []*entity.Commit{}, nil
	}
	return s.gitService.GetHistory(ctx, orgID, "page", id.String())
}

// GetPageVersion returns the content of a page at a specific commit.
func (s *PageService) GetPageVersion(ctx context.Context, orgID, id jsonldb.ID, commitHash string) (string, error) {
	if s.gitService == nil {
		return "", fmt.Errorf("git service not available")
	}

	// New storage path: {orgID}/pages/{id}/index.md
	path := fmt.Sprintf("%s/pages/%s/index.md", orgID.String(), id.String())
	if orgID.IsZero() {
		path = fmt.Sprintf("pages/%s/index.md", id.String())
	}

	contentBytes, err := s.gitService.GetFileAtCommit(ctx, orgID, commitHash, path)
	if err != nil {
		return "", err
	}

	return string(contentBytes), nil
}
