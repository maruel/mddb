// Package handlers provides HTTP request handlers for the API.
package handlers

import (
	"context"

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

// ListPages returns a list of all pages
func (h *PageHandler) ListPages(ctx context.Context, req models.ListPagesRequest) (*models.ListPagesResponse, error) {
	pages, err := h.pageService.ListPages(ctx)
	if err != nil {
		return nil, models.InternalWithError("Failed to list pages", err)
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

	return &models.ListPagesResponse{Pages: pageList}, nil
}

// GetPage returns a specific page by ID
func (h *PageHandler) GetPage(ctx context.Context, req models.GetPageRequest) (*models.GetPageResponse, error) {
	page, err := h.pageService.GetPage(ctx, req.ID)
	if err != nil {
		return nil, models.NotFound("page")
	}

	return &models.GetPageResponse{
		ID:      page.ID,
		Title:   page.Title,
		Content: page.Content,
	}, nil
}

// CreatePage creates a new page
func (h *PageHandler) CreatePage(ctx context.Context, req models.CreatePageRequest) (*models.CreatePageResponse, error) {
	if req.Title == "" {
		return nil, models.MissingField("title")
	}

	page, err := h.pageService.CreatePage(ctx, req.Title, req.Content)
	if err != nil {
		return nil, models.InternalWithError("Failed to create page", err)
	}

	return &models.CreatePageResponse{ID: page.ID}, nil
}

// UpdatePage updates an existing page
func (h *PageHandler) UpdatePage(ctx context.Context, req models.UpdatePageRequest) (*models.UpdatePageResponse, error) {
	page, err := h.pageService.UpdatePage(ctx, req.ID, req.Title, req.Content)
	if err != nil {
		return nil, models.NotFound("page")
	}

	return &models.UpdatePageResponse{ID: page.ID}, nil
}

// DeletePage deletes a page
func (h *PageHandler) DeletePage(ctx context.Context, req models.DeletePageRequest) (*models.DeletePageResponse, error) {
	err := h.pageService.DeletePage(ctx, req.ID)
	if err != nil {
		return nil, models.NotFound("page")
	}

	return &models.DeletePageResponse{}, nil
}

// GetPageHistory returns the history of a page
func (h *PageHandler) GetPageHistory(ctx context.Context, req models.GetPageHistoryRequest) (*models.GetPageHistoryResponse, error) {
	history, err := h.pageService.GetPageHistory(ctx, req.ID)
	if err != nil {
		return nil, models.InternalWithError("Failed to get page history", err)
	}

	return &models.GetPageHistoryResponse{History: history}, nil
}

// GetPageVersion returns a specific version of a page
func (h *PageHandler) GetPageVersion(ctx context.Context, req models.GetPageVersionRequest) (*models.GetPageVersionResponse, error) {
	content, err := h.pageService.GetPageVersion(ctx, req.ID, req.Hash)
	if err != nil {
		return nil, models.InternalWithError("Failed to get page version", err)
	}

	return &models.GetPageVersionResponse{Content: content}, nil
}
