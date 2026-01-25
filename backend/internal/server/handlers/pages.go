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
	fs *content.FileStore
}

// NewPageHandler creates a new page handler.
func NewPageHandler(fs *content.FileStore) *PageHandler {
	return &PageHandler{fs: fs}
}

// ListPages returns a list of all pages.
func (h *PageHandler) ListPages(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req *dto.ListPagesRequest) (*dto.ListPagesResponse, error) {
	it, err := h.fs.IterPages(orgID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list pages", err)
	}
	return &dto.ListPagesResponse{Pages: pagesToSummaries(slices.Collect(it))}, nil
}

// GetPage returns a specific page by ID.
func (h *PageHandler) GetPage(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req *dto.GetPageRequest) (*dto.GetPageResponse, error) {
	page, err := h.fs.ReadPage(orgID, req.ID)
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
func (h *PageHandler) CreatePage(ctx context.Context, orgID jsonldb.ID, user *identity.User, req *dto.CreatePageRequest) (*dto.CreatePageResponse, error) {
	if req.Title == "" {
		return nil, dto.MissingField("title")
	}
	id := jsonldb.NewID()
	author := git.Author{Name: user.Name, Email: user.Email}
	page, err := h.fs.WritePage(ctx, orgID, id, 0, req.Title, req.Content, author)
	if err != nil {
		return nil, dto.InternalWithError("Failed to create page", err)
	}
	return &dto.CreatePageResponse{ID: page.ID}, nil
}

// UpdatePage updates an existing page.
func (h *PageHandler) UpdatePage(ctx context.Context, orgID jsonldb.ID, user *identity.User, req *dto.UpdatePageRequest) (*dto.UpdatePageResponse, error) {
	author := git.Author{Name: user.Name, Email: user.Email}
	page, err := h.fs.UpdatePage(ctx, orgID, req.ID, req.Title, req.Content, author)
	if err != nil {
		return nil, dto.NotFound("page")
	}
	return &dto.UpdatePageResponse{ID: page.ID}, nil
}

// DeletePage deletes a page.
func (h *PageHandler) DeletePage(ctx context.Context, orgID jsonldb.ID, user *identity.User, req *dto.DeletePageRequest) (*dto.DeletePageResponse, error) {
	author := git.Author{Name: user.Name, Email: user.Email}
	if err := h.fs.DeletePage(ctx, orgID, req.ID, author); err != nil {
		return nil, dto.NotFound("page")
	}
	return &dto.DeletePageResponse{Ok: true}, nil
}

// ListPageVersions returns the version history of a page.
func (h *PageHandler) ListPageVersions(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req *dto.ListPageVersionsRequest) (*dto.ListPageVersionsResponse, error) {
	history, err := h.fs.GetHistory(ctx, orgID, req.ID, req.Limit)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get page history", err)
	}
	return &dto.ListPageVersionsResponse{History: commitsToDTO(history)}, nil
}

// GetPageVersion returns a specific version of a page.
func (h *PageHandler) GetPageVersion(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req *dto.GetPageVersionRequest) (*dto.GetPageVersionResponse, error) {
	path := fmt.Sprintf("%s/pages/%s/index.md", orgID.String(), req.ID.String())
	contentBytes, err := h.fs.GetFileAtCommit(ctx, orgID, req.Hash, path)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get page version", err)
	}
	return &dto.GetPageVersionResponse{Content: string(contentBytes)}, nil
}
