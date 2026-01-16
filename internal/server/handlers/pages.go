package handlers

import (
	"context"

	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/storage"
)

// PageHandler handles page-related HTTP requests
type PageHandler struct {
	pageService *storage.PageService
}

// NewPageHandler creates a new page handler
func NewPageHandler(fileStore *storage.FileStore) *PageHandler {
	return &PageHandler{
		pageService: storage.NewPageService(fileStore),
	}
}

// Request/Response types for pages
type ListPagesRequest struct{}

type ListPagesResponse struct {
	Pages []any `json:"pages"`
}

type GetPageRequest struct {
	ID string `path:"id"`
}

type GetPageResponse struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type CreatePageRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type CreatePageResponse struct {
	ID string `json:"id"`
}

type UpdatePageRequest struct {
	ID      string `path:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type UpdatePageResponse struct {
	ID string `json:"id"`
}

type DeletePageRequest struct {
	ID string `path:"id"`
}

type DeletePageResponse struct{}

// ListPages returns a list of all pages
func (h *PageHandler) ListPages(ctx context.Context, req ListPagesRequest) (*ListPagesResponse, error) {
	pages, err := h.pageService.ListPages()
	if err != nil {
		return nil, errors.NewAPIError(500, "Failed to list pages")
	}

	pageList := make([]any, len(pages))
	for i, p := range pages {
		pageList[i] = map[string]any{
			"id":       p.ID,
			"title":    p.Title,
			"created":  p.Created,
			"modified": p.Modified,
		}
	}

	return &ListPagesResponse{Pages: pageList}, nil
}

// GetPage returns a specific page by ID
func (h *PageHandler) GetPage(ctx context.Context, req GetPageRequest) (*GetPageResponse, error) {
	page, err := h.pageService.GetPage(req.ID)
	if err != nil {
		return nil, errors.NewAPIError(404, "Page not found")
	}

	return &GetPageResponse{
		ID:      page.ID,
		Title:   page.Title,
		Content: page.Content,
	}, nil
}

// CreatePage creates a new page
func (h *PageHandler) CreatePage(ctx context.Context, req CreatePageRequest) (*CreatePageResponse, error) {
	page, err := h.pageService.CreatePage(req.Title, req.Content)
	if err != nil {
		return nil, errors.NewAPIError(400, "Failed to create page")
	}

	return &CreatePageResponse{ID: page.ID}, nil
}

// UpdatePage updates an existing page
func (h *PageHandler) UpdatePage(ctx context.Context, req UpdatePageRequest) (*UpdatePageResponse, error) {
	page, err := h.pageService.UpdatePage(req.ID, req.Title, req.Content)
	if err != nil {
		return nil, errors.NewAPIError(404, "Page not found")
	}

	return &UpdatePageResponse{ID: page.ID}, nil
}

// DeletePage deletes a page
func (h *PageHandler) DeletePage(ctx context.Context, req DeletePageRequest) (*DeletePageResponse, error) {
	err := h.pageService.DeletePage(req.ID)
	if err != nil {
		return nil, errors.NewAPIError(404, "Page not found")
	}

	return &DeletePageResponse{}, nil
}
