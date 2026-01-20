package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/maruel/mddb/backend/internal/storage/entity"
)

// AssetHandler handles asset/file-related HTTP requests
type AssetHandler struct {
	fileStore *storage.FileStore
	git       *storage.GitService
	orgs      *storage.OrganizationService
}

// NewAssetHandler creates a new asset handler
func NewAssetHandler(fileStore *storage.FileStore, git *storage.GitService, orgs *storage.OrganizationService) *AssetHandler {
	return &AssetHandler{
		fileStore: fileStore,
		git:       git,
		orgs:      orgs,
	}
}

// ListPageAssets returns a list of assets associated with a page.
func (h *AssetHandler) ListPageAssets(ctx context.Context, req dto.ListPageAssetsRequest) (*dto.ListPageAssetsResponse, error) {
	orgID := entity.GetOrgID(ctx)
	pageID, err := jsonldb.DecodeID(req.PageID)
	if err != nil {
		return nil, dto.BadRequest("invalid_page_id")
	}

	assets, err := h.fileStore.ListAssets(orgID, pageID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list assets", err)
	}

	assetList := make([]any, len(assets))
	for i, a := range assets {
		assetList[i] = map[string]any{
			"id":        a.ID,
			"name":      a.Name,
			"size":      a.Size,
			"mime_type": a.MimeType,
			"created":   a.Created,
			"url":       fmt.Sprintf("/api/%s/assets/%s/%s", orgID.String(), req.PageID, a.Name),
		}
	}

	return &dto.ListPageAssetsResponse{Assets: assetList}, nil
}

// UploadPageAssetHandler handles asset uploading (multipart/form-data).
func (h *AssetHandler) UploadPageAssetHandler(w http.ResponseWriter, r *http.Request) {
	pageID := r.PathValue("id")

	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB
		writeErrorResponse(w, dto.BadRequest("form_parse"))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeErrorResponse(w, dto.MissingField("file"))
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		writeErrorResponse(w, dto.Internal("file_read"))
		return
	}

	as := storage.NewAssetService(h.fileStore, h.git, h.orgs)
	asset, err := as.SaveAsset(r.Context(), pageID, header.Filename, data)
	if err != nil {
		writeErrorResponse(w, dto.Internal("asset_save"))
		return
	}

	w.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprintf(w, `{"id":"%s","name":"%s"}`, asset.ID, asset.Name)
}

// ServeAssetFile serves the binary data of an asset.
func (h *AssetHandler) ServeAssetFile(w http.ResponseWriter, r *http.Request) {
	orgIDStr := r.PathValue("orgID")
	pageIDStr := r.PathValue("id")
	assetName := r.PathValue("name")

	orgID, err := jsonldb.DecodeID(orgIDStr)
	if err != nil {
		writeErrorResponse(w, dto.BadRequest("invalid_org_id"))
		return
	}
	pageID, err := jsonldb.DecodeID(pageIDStr)
	if err != nil {
		writeErrorResponse(w, dto.BadRequest("invalid_page_id"))
		return
	}

	data, err := h.fileStore.ReadAsset(orgID, pageID, assetName)
	if err != nil {
		writeErrorResponse(w, dto.NotFound("asset"))
		return
	}

	// Simple MIME detection
	mime := "application/octet-stream"
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	_, _ = w.Write(data)
}

// DeletePageAsset deletes an asset.
func (h *AssetHandler) DeletePageAsset(ctx context.Context, req dto.DeletePageAssetRequest) (*dto.DeletePageAssetResponse, error) {
	orgID := entity.GetOrgID(ctx)
	pageID, err := jsonldb.DecodeID(req.PageID)
	if err != nil {
		return nil, dto.BadRequest("invalid_page_id")
	}

	err = h.fileStore.DeleteAsset(orgID, pageID, req.AssetName)
	if err != nil {
		return nil, dto.NotFound("asset")
	}

	if h.git != nil {
		_ = h.git.CommitChange(ctx, "delete", "asset", req.AssetName, "Deleted asset from page "+req.PageID)
	}

	return &dto.DeletePageAssetResponse{}, nil
}
