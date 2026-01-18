// Package handlers provides HTTP request handlers for the API.
package handlers

import (
	"context"

	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/storage"
)

// PageHandler handles page-related HTTP requests
type PageHandler struct {
	pageService *storage.PageService
}

// NewPageHandler creates a new page handler
func NewPageHandler(fileStore *storage.FileStore, gitService *storage.GitService, cache *storage.Cache, orgService *storage.OrganizationService) *PageHandler {
	return &PageHandler{
		pageService: storage.NewPageService(fileStore, gitService, cache, orgService),
	}
}

// ListPagesRequest is a request to list all pages.
type ListPagesRequest struct {
	OrgID string `path:"orgID"`
}

// ListPagesResponse is a response containing a list of pages.
type ListPagesResponse struct {
	Pages []any `json:"pages"`
}

// GetPageRequest is a request to get a page.
type GetPageRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// GetPageResponse is a response containing a page.
type GetPageResponse struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// CreatePageRequest is a request to create a page.
type CreatePageRequest struct {
	OrgID   string `path:"orgID"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// CreatePageResponse is a response from creating a page.
type CreatePageResponse struct {
	ID string `json:"id"`
}

// UpdatePageRequest is a request to update a page.
type UpdatePageRequest struct {
	OrgID   string `path:"orgID"`
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
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// DeletePageResponse is a response from deleting a page.
type DeletePageResponse struct{}

// GetPageHistoryRequest is a request to get page history.
type GetPageHistoryRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
}

// GetPageHistoryResponse is a response containing page history.
type GetPageHistoryResponse struct {
	History []*models.Commit `json:"history"`
}

// GetPageVersionRequest is a request to get a specific page version.
type GetPageVersionRequest struct {
	OrgID string `path:"orgID"`
	ID    string `path:"id"`
	Hash  string `path:"hash"`
}

// GetPageVersionResponse is a response containing page content at a version.
type GetPageVersionResponse struct {
	Content string `json:"content"`
}

// ListPages returns a list of all pages
func (h *PageHandler) ListPages(ctx context.Context, req ListPagesRequest) (*ListPagesResponse, error) {
	pages, err := h.pageService.ListPages(ctx)
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
	page, err := h.pageService.GetPage(ctx, req.ID)
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

	page, err := h.pageService.CreatePage(ctx, req.Title, req.Content)
	if err != nil {
		return nil, errors.InternalWithError("Failed to create page", err)
	}

	return &CreatePageResponse{ID: page.ID}, nil
}

// UpdatePage updates an existing page
func (h *PageHandler) UpdatePage(ctx context.Context, req UpdatePageRequest) (*UpdatePageResponse, error) {
	page, err := h.pageService.UpdatePage(ctx, req.ID, req.Title, req.Content)
	if err != nil {
		return nil, errors.NotFound("page")
	}

	return &UpdatePageResponse{ID: page.ID}, nil
}

// DeletePage deletes a page
func (h *PageHandler) DeletePage(ctx context.Context, req DeletePageRequest) (*DeletePageResponse, error) {
	err := h.pageService.DeletePage(ctx, req.ID)
	if err != nil {
		return nil, errors.NotFound("page")
	}

	return &DeletePageResponse{}, nil
}

// GetPageHistory returns the history of a page
func (h *PageHandler) GetPageHistory(ctx context.Context, req GetPageHistoryRequest) (*GetPageHistoryResponse, error) {
	history, err := h.pageService.GetPageHistory(ctx, req.ID)
	if err != nil {
		return nil, errors.InternalWithError("Failed to get page history", err)
	}

	return &GetPageHistoryResponse{History: history}, nil
}

// GetPageVersion returns a specific version of a page
func (h *PageHandler) GetPageVersion(ctx context.Context, req GetPageVersionRequest) (*GetPageVersionResponse, error) {
	content, err := h.pageService.GetPageVersion(ctx, req.ID, req.Hash)
	if err != nil {
		return nil, errors.InternalWithError("Failed to get page version", err)
	}

	return &GetPageVersionResponse{Content: content}, nil
}