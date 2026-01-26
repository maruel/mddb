// Handles page management endpoints (documents).

// Package handlers provides HTTP request handlers for the REST API.
//
// Each handler type wraps storage services, validates inputs, and returns
// standardized responses. Handlers accept request models and delegate business
// logic to the storage package.
package handlers

import (
	"context"
	"fmt"
	"slices"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// PageHandler handles page-related HTTP requests.
type PageHandler struct {
	fs *content.FileStoreService
}

// NewPageHandler creates a new page handler.
func NewPageHandler(fs *content.FileStoreService) *PageHandler {
	return &PageHandler{fs: fs}
}

// ListPages returns a list of all pages.
func (h *PageHandler) ListPages(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.ListPagesRequest) (*dto.ListPagesResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	it, err := ws.IterPages()
	if err != nil {
		return nil, dto.InternalWithError("Failed to list pages", err)
	}
	return &dto.ListPagesResponse{Pages: pagesToSummaries(slices.Collect(it))}, nil
}

// GetPage returns a specific page by ID.
func (h *PageHandler) GetPage(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.GetPageRequest) (*dto.GetPageResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	page, err := ws.ReadPage(req.ID)
	if err != nil {
		return nil, dto.NotFound("page")
	}
	return &dto.GetPageResponse{
		ID:      page.ID,
		Title:   page.Title,
		Content: page.Content,
	}, nil
}

// CreatePage creates a new page.
func (h *PageHandler) CreatePage(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.CreatePageRequest) (*dto.CreatePageResponse, error) {
	if req.Title == "" {
		return nil, dto.MissingField("title")
	}
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	id := jsonldb.NewID()
	author := git.Author{Name: user.Name, Email: user.Email}
	page, err := ws.WritePage(ctx, id, 0, req.Title, req.Content, author)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create page", err)
	}
	return &dto.CreatePageResponse{ID: page.ID}, nil
}

// UpdatePage updates an existing page.
func (h *PageHandler) UpdatePage(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.UpdatePageRequest) (*dto.UpdatePageResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := git.Author{Name: user.Name, Email: user.Email}
	page, err := ws.UpdatePage(ctx, req.ID, req.Title, req.Content, author)
	if err != nil {
		return nil, dto.NotFound("page")
	}
	return &dto.UpdatePageResponse{ID: page.ID}, nil
}

// DeletePage deletes a page.
func (h *PageHandler) DeletePage(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.DeletePageRequest) (*dto.DeletePageResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := git.Author{Name: user.Name, Email: user.Email}
	if err := ws.DeletePage(ctx, req.ID, author); err != nil {
		return nil, dto.NotFound("page")
	}
	return &dto.DeletePageResponse{Ok: true}, nil
}

// ListPageVersions returns the version history of a page.
func (h *PageHandler) ListPageVersions(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.ListPageVersionsRequest) (*dto.ListPageVersionsResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	history, err := ws.GetHistory(ctx, req.ID, req.Limit)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get page history", err)
	}
	return &dto.ListPageVersionsResponse{History: commitsToDTO(history)}, nil
}

// GetPageVersion returns a specific version of a page.
func (h *PageHandler) GetPageVersion(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.GetPageVersionRequest) (*dto.GetPageVersionResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	path := fmt.Sprintf("pages/%s/index.md", req.ID.String())
	contentBytes, err := ws.GetFileAtCommit(ctx, req.Hash, path)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get page version", err)
	}
	return &dto.GetPageVersionResponse{Content: string(contentBytes)}, nil
}
