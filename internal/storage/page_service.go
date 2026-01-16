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
}

// NewPageService creates a new page service.
func NewPageService(fileStore *FileStore, gitService *GitService) *PageService {
	return &PageService{
		fileStore:  fileStore,
		gitService: gitService,
	}
}

// GetPage retrieves a page by ID.
func (s *PageService) GetPage(id string) (*models.Page, error) {
	if id == "" {
		return nil, fmt.Errorf("page id cannot be empty")
	}
	return s.fileStore.ReadPage(id)
}

// CreatePage creates a new page with a generated numeric ID.
func (s *PageService) CreatePage(title, content string) (*models.Page, error) {
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}

	// Generate numeric ID (monotonically increasing)
	id := s.fileStore.NextID()

	page, err := s.fileStore.WritePage(id, title, content)
	if err != nil {
		return nil, err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange("create", "page", id, title); err != nil {
			// Log error but don't fail the operation
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return page, nil
}

// UpdatePage updates an existing page.
func (s *PageService) UpdatePage(id, title, content string) (*models.Page, error) {
	if id == "" {
		return nil, fmt.Errorf("page id cannot be empty")
	}
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}

	page, err := s.fileStore.UpdatePage(id, title, content)
	if err != nil {
		return nil, err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange("update", "page", id, "Updated content"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return page, nil
}

// DeletePage deletes a page.
func (s *PageService) DeletePage(id string) error {
	if id == "" {
		return fmt.Errorf("page id cannot be empty")
	}
	if err := s.fileStore.DeletePage(id); err != nil {
		return err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange("delete", "page", id, "Deleted page"); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}

// ListPages returns all pages.
func (s *PageService) ListPages() ([]*models.Page, error) {
	return s.fileStore.ListPages()
}

// SearchPages performs a simple text search across pages.
func (s *PageService) SearchPages(query string) ([]*models.Page, error) {
	if query == "" {
		return []*models.Page{}, nil
	}

	pages, err := s.fileStore.ListPages()
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
