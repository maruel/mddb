package handlers

import (
	"context"

	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/storage"
)

// PageHandler handles page-related HTTP requests
type PageHandler struct {
	fileStore *storage.FileStore
}

// NewPageHandler creates a new page handler
func NewPageHandler(fileStore *storage.FileStore) *PageHandler {
	return &PageHandler{fileStore: fileStore}
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
	// TODO: Implement listing pages from filesystem
	return &ListPagesResponse{Pages: []any{}}, nil
}

// GetPage returns a specific page by ID
func (h *PageHandler) GetPage(ctx context.Context, req GetPageRequest) (*GetPageResponse, error) {
	// TODO: Implement getting a page (req.ID is populated from path parameter)
	return nil, errors.NewAPIError(404, "Page not found")
}

// CreatePage creates a new page
func (h *PageHandler) CreatePage(ctx context.Context, req CreatePageRequest) (*CreatePageResponse, error) {
	// TODO: Implement creating a page
	return &CreatePageResponse{ID: "placeholder"}, nil
}

// UpdatePage updates an existing page
func (h *PageHandler) UpdatePage(ctx context.Context, req UpdatePageRequest) (*UpdatePageResponse, error) {
	// TODO: Implement updating a page (req.ID is populated from path parameter)
	return nil, errors.NewAPIError(404, "Page not found")
}

// DeletePage deletes a page
func (h *PageHandler) DeletePage(ctx context.Context, req DeletePageRequest) (*DeletePageResponse, error) {
	// TODO: Implement deleting a page (req.ID is populated from path parameter)
	return &DeletePageResponse{}, nil
}
