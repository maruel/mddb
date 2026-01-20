// Package handlers provides HTTP request handlers for the REST API.
//
// Each handler type wraps storage services, validates inputs, and returns
// standardized responses. Handlers accept request models and delegate business
// logic to the storage package.
package handlers

import (
	"context"

	"github.com/maruel/mddb/backend/internal/jsonldb"
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
	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
	}
	nodes, err := h.pageService.ListPages(ctx, orgID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list pages", err)
	}

	pageList := make([]any, len(nodes))
	for i, n := range nodes {
		pageList[i] = map[string]any{
			"id":       n.ID.String(),
			"title":    n.Title,
			"created":  n.Created,
			"modified": n.Modified,
		}
	}

	return &dto.ListPagesResponse{Pages: pageList}, nil
}

// GetPage returns a specific page by ID
func (h *PageHandler) GetPage(ctx context.Context, req dto.GetPageRequest) (*dto.GetPageResponse, error) {
	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
	}
	id, err := jsonldb.DecodeID(req.ID)
	if err != nil {
		return nil, dto.BadRequest("invalid_page_id")
	}
	node, err := h.pageService.GetPage(ctx, orgID, id)
	if err != nil {
		return nil, dto.NotFound("page")
	}

	return &dto.GetPageResponse{
		ID:      node.ID.String(),
		Title:   node.Title,
		Content: node.Content,
	}, nil
}

// CreatePage creates a new page
func (h *PageHandler) CreatePage(ctx context.Context, req dto.CreatePageRequest) (*dto.CreatePageResponse, error) {
	if req.Title == "" {
		return nil, dto.MissingField("title")
	}

	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
	}
	node, err := h.pageService.CreatePage(ctx, orgID, req.Title, req.Content)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create page", err)
	}

	return &dto.CreatePageResponse{ID: node.ID.String()}, nil
}

// UpdatePage updates an existing page
func (h *PageHandler) UpdatePage(ctx context.Context, req dto.UpdatePageRequest) (*dto.UpdatePageResponse, error) {
	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
	}
	id, err := jsonldb.DecodeID(req.ID)
	if err != nil {
		return nil, dto.BadRequest("invalid_page_id")
	}
	node, err := h.pageService.UpdatePage(ctx, orgID, id, req.Title, req.Content)
	if err != nil {
		return nil, dto.NotFound("page")
	}

	return &dto.UpdatePageResponse{ID: node.ID.String()}, nil
}

// DeletePage deletes a page
func (h *PageHandler) DeletePage(ctx context.Context, req dto.DeletePageRequest) (*dto.DeletePageResponse, error) {
	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
	}
	id, err := jsonldb.DecodeID(req.ID)
	if err != nil {
		return nil, dto.BadRequest("invalid_page_id")
	}
	err = h.pageService.DeletePage(ctx, orgID, id)
	if err != nil {
		return nil, dto.NotFound("page")
	}

	return &dto.DeletePageResponse{}, nil
}

// GetPageHistory returns the history of a page
func (h *PageHandler) GetPageHistory(ctx context.Context, req dto.GetPageHistoryRequest) (*dto.GetPageHistoryResponse, error) {
	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
	}
	id, err := jsonldb.DecodeID(req.ID)
	if err != nil {
		return nil, dto.BadRequest("invalid_page_id")
	}
	history, err := h.pageService.GetPageHistory(ctx, orgID, id)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get page history", err)
	}

	return &dto.GetPageHistoryResponse{History: commitsToDTO(history)}, nil
}

// GetPageVersion returns a specific version of a page
func (h *PageHandler) GetPageVersion(ctx context.Context, req dto.GetPageVersionRequest) (*dto.GetPageVersionResponse, error) {
	orgID, err := jsonldb.DecodeID(req.OrgID)
	if err != nil {
		return nil, dto.BadRequest("invalid_org_id")
	}
	id, err := jsonldb.DecodeID(req.ID)
	if err != nil {
		return nil, dto.BadRequest("invalid_page_id")
	}
	content, err := h.pageService.GetPageVersion(ctx, orgID, id, req.Hash)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get page version", err)
	}

	return &dto.GetPageVersionResponse{Content: content}, nil
}
