package storage

import (
	"fmt"
	"strings"

	"github.com/maruel/mddb/internal/models"
)

// PageService handles page business logic.
type PageService struct {
	fileStore  *FileStore
	gitService *GitService
	cache      *Cache
}

// NewPageService creates a new page service.
func NewPageService(fileStore *FileStore, gitService *GitService, cache *Cache) *PageService {
	return &PageService{
		fileStore:  fileStore,
		gitService: gitService,
		cache:      cache,
	}
}

// GetPage retrieves a page by ID.
func (s *PageService) GetPage(orgID, id string) (*models.Page, error) {
	if id == "" {
		return nil, fmt.Errorf("page id cannot be empty")
	}

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
func (s *PageService) CreatePage(orgID, title, content string) (*models.Page, error) {
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}

	// Generate numeric ID (monotonically increasing)
	id := s.fileStore.NextID(orgID)

	page, err := s.fileStore.WritePage(orgID, id, title, content)
	if err != nil {
		return nil, err
	}

	// Invalidate node tree cache
	s.cache.InvalidateNodeTree()
	s.cache.SetPage(page)

	if s.gitService != nil {
		if err := s.gitService.CommitChange("create", "page", id, title); err != nil {
			// Log error but don't fail the operation
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return page, nil
}

// UpdatePage updates an existing page.
func (s *PageService) UpdatePage(orgID, id, title, content string) (*models.Page, error) {
	if id == "" {
		return nil, fmt.Errorf("page id cannot be empty")
	}
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}

	page, err := s.fileStore.UpdatePage(orgID, id, title, content)
	if err != nil {
		return nil, err
	}

	// Update cache
	s.cache.SetPage(page)
	s.cache.InvalidateNodeTree() // Title might have changed

	if s.gitService != nil {
		if err := s.gitService.CommitChange("update", "page", id, "Updated content"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return page, nil
}

// DeletePage deletes a page.
func (s *PageService) DeletePage(orgID, id string) error {
	if id == "" {
		return fmt.Errorf("page id cannot be empty")
	}
	if err := s.fileStore.DeletePage(orgID, id); err != nil {
		return err
	}

	// Invalidate cache
	s.cache.InvalidatePage(id)
	s.cache.InvalidateNodeTree()

	if s.gitService != nil {
		if err := s.gitService.CommitChange("delete", "page", id, "Deleted page"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}

// ListPages returns all pages.
func (s *PageService) ListPages(orgID string) ([]*models.Page, error) {
	return s.fileStore.ListPages(orgID)
}

// SearchPages performs a simple text search across pages.
func (s *PageService) SearchPages(orgID, query string) ([]*models.Page, error) {
	if query == "" {
		return []*models.Page{}, nil
	}

	pages, err := s.fileStore.ListPages(orgID)
	if err != nil {
		return nil, err
	}

	queryLower := strings.ToLower(query)
	var results []*models.Page

	for _, page := range pages {
		if strings.Contains(strings.ToLower(page.Title), queryLower) ||
			strings.Contains(strings.ToLower(page.Content), queryLower) {
			results = append(results, page)
		}
	}

	return results, nil
}

// GetPageHistory returns the commit history for a page.
func (s *PageService) GetPageHistory(orgID, id string) ([]*Commit, error) {
	if s.gitService == nil {
		return []*Commit{}, nil
	}
	return s.gitService.GetHistory("page", id)
}

// GetPageVersion returns the content of a page at a specific commit.
func (s *PageService) GetPageVersion(orgID, id, commitHash string) (string, error) {
	if s.gitService == nil {
		return "", fmt.Errorf("git service not available")
	}

	// Path might need adjustment for org-based storage if git tracks orgs/ too
	path := fmt.Sprintf("orgs/%s/pages/%s/index.md", orgID, id)
	if orgID == "" {
		path = fmt.Sprintf("pages/%s/index.md", id)
	}

	contentBytes, err := s.gitService.GetFileAtCommit(commitHash, path)
	if err != nil {
		return "", err
	}

	return string(contentBytes), nil
}
