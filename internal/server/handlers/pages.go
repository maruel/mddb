// Package handlers provides HTTP request handlers for the API.
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
func NewPageHandler(fileStore *storage.FileStore, gitService *storage.GitService) *PageHandler {
	return &PageHandler{
		pageService: storage.NewPageService(fileStore, gitService),
	}
}

// ListPagesRequest is a request to list all pages.
type ListPagesRequest struct{}

// ListPagesResponse is a response containing a list of pages.
type ListPagesResponse struct {
	Pages []any `json:"pages"`
}

// GetPageRequest is a request to get a page.
type GetPageRequest struct {
	ID string `path:"id"`
}

// GetPageResponse is a response containing a page.
type GetPageResponse struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// CreatePageRequest is a request to create a page.
type CreatePageRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// CreatePageResponse is a response from creating a page.
type CreatePageResponse struct {
	ID string `json:"id"`
}

// UpdatePageRequest is a request to update a page.
type UpdatePageRequest struct {
	ID      string `path:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// UpdatePageResponse is a response from updating a page.
type UpdatePageResponse struct {
	ID string `json:"id"`
}

// DeletePageRequest is a request to delete a page.
type DeletePageRequest struct {
	ID string `path:"id"`
}

// DeletePageResponse is a response from deleting a page.
type DeletePageResponse struct{}

// ListPages returns a list of all pages
func (h *PageHandler) ListPages(ctx context.Context, req ListPagesRequest) (*ListPagesResponse, error) {
	pages, err := h.pageService.ListPages()
	if err != nil {
		return nil, errors.InternalWithError("Failed to list pages", err)
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
		return nil, errors.NotFound("page")
	}

	return &GetPageResponse{
		ID:      page.ID,
		Title:   page.Title,
		Content: page.Content,
	}, nil
}

// CreatePage creates a new page
func (h *PageHandler) CreatePage(ctx context.Context, req CreatePageRequest) (*CreatePageResponse, error) {
	if req.Title == "" {
		return nil, errors.MissingField("title")
	}

	page, err := h.pageService.CreatePage(req.Title, req.Content)
	if err != nil {
		return nil, errors.InternalWithError("Failed to create page", err)
	}

	return &CreatePageResponse{ID: page.ID}, nil
}

// UpdatePage updates an existing page
func (h *PageHandler) UpdatePage(ctx context.Context, req UpdatePageRequest) (*UpdatePageResponse, error) {
	page, err := h.pageService.UpdatePage(req.ID, req.Title, req.Content)
	if err != nil {
		return nil, errors.NotFound("page")
	}

	return &UpdatePageResponse{ID: page.ID}, nil
}

// DeletePage deletes a page
func (h *PageHandler) DeletePage(ctx context.Context, req DeletePageRequest) (*DeletePageResponse, error) {
	err := h.pageService.DeletePage(req.ID)
	if err != nil {
		return nil, errors.NotFound("page")
	}

	return &DeletePageResponse{}, nil
}
