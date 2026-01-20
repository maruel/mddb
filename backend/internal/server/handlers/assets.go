package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

// AssetHandler handles asset/file-related HTTP requests
type AssetHandler struct {
	fileStore    *infra.FileStore
	assetService *content.AssetService
}

// NewAssetHandler creates a new asset handler
func NewAssetHandler(fileStore *infra.FileStore, git *infra.GitService, orgs *identity.OrganizationService) *AssetHandler {
	return &AssetHandler{
		fileStore:    fileStore,
		assetService: content.NewAssetService(fileStore, git, orgs),
	}
}

// ListPageAssets returns a list of assets associated with a page.
func (h *AssetHandler) ListPageAssets(ctx context.Context, req dto.ListPageAssetsRequest) (*dto.ListPageAssetsResponse, error) {
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	pageID, err := decodeID(req.PageID, "page_id")
	if err != nil {
		return nil, err
	}
	assets, err := h.fileStore.ListAssets(orgID, pageID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list assets", err)
	}
	return &dto.ListPageAssetsResponse{Assets: assetsToSummaries(assets, orgID.String(), req.PageID)}, nil
}

// UploadPageAssetHandler handles asset uploading (multipart/form-data).
func (h *AssetHandler) UploadPageAssetHandler(w http.ResponseWriter, r *http.Request) {
	orgIDStr := r.PathValue("orgID")
	pageIDStr := r.PathValue("id")

	orgID, err := decodeOrgID(orgIDStr)
	if err != nil {
		writeErrorResponse(w, err)
		return
	}
	pageID, err := decodeID(pageIDStr, "page_id")
	if err != nil {
		writeErrorResponse(w, err)
		return
	}

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

	asset, err := h.assetService.SaveAsset(r.Context(), orgID, pageID, header.Filename, data)
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

	orgID, err := decodeOrgID(orgIDStr)
	if err != nil {
		writeErrorResponse(w, err)
		return
	}
	pageID, err := decodeID(pageIDStr, "page_id")
	if err != nil {
		writeErrorResponse(w, err)
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
	orgID, err := decodeOrgID(req.OrgID)
	if err != nil {
		return nil, err
	}
	pageID, err := decodeID(req.PageID, "page_id")
	if err != nil {
		return nil, err
	}
	if err := h.assetService.DeleteAsset(ctx, orgID, pageID, req.AssetName); err != nil {
		return nil, dto.NotFound("asset")
	}
	return &dto.DeletePageAssetResponse{Ok: true}, nil
}
