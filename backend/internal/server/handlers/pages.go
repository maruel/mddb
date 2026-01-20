// Package handlers provides HTTP request handlers for the REST API.
//
// Each handler type wraps storage services, validates inputs, and returns
// standardized responses. Handlers accept request models and delegate business
// logic to the storage package.
package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
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
func (h *PageHandler) ListPages(ctx context.Context, req dto.ListPagesRequest) (*dto.ListPagesResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	pages, err := h.pageService.ListPages(ctx, orgID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list pages", err)
	}
	return &dto.ListPagesResponse{Pages: pagesToSummaries(pages)}, nil
}

// GetPage returns a specific page by ID
func (h *PageHandler) GetPage(ctx context.Context, req dto.GetPageRequest) (*dto.GetPageResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	id, err := decodeID(req.ID, "page_id")
	if err != nil {
		return nil, err
	}
	page, err := h.pageService.GetPage(ctx, orgID, id)
	if err != nil {
		return nil, dto.NotFound("page")
	}
	return &dto.GetPageResponse{
		ID:      page.ID.String(),
		Title:   page.Title,
		Content: page.Content,
	}, nil
}

// CreatePage creates a new page
func (h *PageHandler) CreatePage(ctx context.Context, req dto.CreatePageRequest) (*dto.CreatePageResponse, error) {
	if req.Title == "" {
		return nil, dto.MissingField("title")
	}
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	page, err := h.pageService.CreatePage(ctx, orgID, req.Title, req.Content)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create page", err)
	}
	return &dto.CreatePageResponse{ID: page.ID.String()}, nil
}

// UpdatePage updates an existing page
func (h *PageHandler) UpdatePage(ctx context.Context, req dto.UpdatePageRequest) (*dto.UpdatePageResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	id, err := decodeID(req.ID, "page_id")
	if err != nil {
		return nil, err
	}
	page, err := h.pageService.UpdatePage(ctx, orgID, id, req.Title, req.Content)
	if err != nil {
		return nil, dto.NotFound("page")
	}
	return &dto.UpdatePageResponse{ID: page.ID.String()}, nil
}

// DeletePage deletes a page
func (h *PageHandler) DeletePage(ctx context.Context, req dto.DeletePageRequest) (*dto.DeletePageResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	id, err := decodeID(req.ID, "page_id")
	if err != nil {
		return nil, err
	}
	if err := h.pageService.DeletePage(ctx, orgID, id); err != nil {
		return nil, dto.NotFound("page")
	}
	return &dto.DeletePageResponse{Ok: true}, nil
}

// GetPageHistory returns the history of a page
func (h *PageHandler) GetPageHistory(ctx context.Context, req dto.GetPageHistoryRequest) (*dto.GetPageHistoryResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	id, err := decodeID(req.ID, "page_id")
	if err != nil {
		return nil, err
	}
	history, err := h.pageService.GetPageHistory(ctx, orgID, id)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get page history", err)
	}
	return &dto.GetPageHistoryResponse{History: commitsToDTO(history)}, nil
}

// GetPageVersion returns a specific version of a page
func (h *PageHandler) GetPageVersion(ctx context.Context, req dto.GetPageVersionRequest) (*dto.GetPageVersionResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	id, err := decodeID(req.ID, "page_id")
	if err != nil {
		return nil, err
	}
	content, err := h.pageService.GetPageVersion(ctx, orgID, id, req.Hash)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get page version", err)
	}
	return &dto.GetPageVersionResponse{Content: content}, nil
}
