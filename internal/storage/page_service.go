package storage

import (
	"fmt"
	"strings"

	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/utils"
)

// PageService handles page business logic.
type PageService struct {
	fileStore *FileStore
}

// NewPageService creates a new page service.
func NewPageService(fileStore *FileStore) *PageService {
	return &PageService{fileStore: fileStore}
}

// GetPage retrieves a page by ID.
func (s *PageService) GetPage(id string) (*models.Page, error) {
	if id == "" {
		return nil, fmt.Errorf("page id cannot be empty")
	}
	return s.fileStore.ReadPage(id)
}

// CreatePage creates a new page with a generated ID.
func (s *PageService) CreatePage(title, content string) (*models.Page, error) {
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}

	// Generate ID using UUID
	id, err := utils.GenerateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate page id: %w", err)
	}

	return s.fileStore.WritePage(id, title, content)
}

// UpdatePage updates an existing page.
func (s *PageService) UpdatePage(id, title, content string) (*models.Page, error) {
	if id == "" {
		return nil, fmt.Errorf("page id cannot be empty")
	}
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}

	return s.fileStore.UpdatePage(id, title, content)
}

// DeletePage deletes a page.
func (s *PageService) DeletePage(id string) error {
	if id == "" {
		return fmt.Errorf("page id cannot be empty")
	}
	return s.fileStore.DeletePage(id)
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
