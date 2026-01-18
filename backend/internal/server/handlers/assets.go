package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/storage"
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
func (h *AssetHandler) ListPageAssets(ctx context.Context, req models.ListPageAssetsRequest) (*models.ListPageAssetsResponse, error) {
	orgID := models.GetOrgID(ctx)
	assets, err := h.fileStore.ListAssets(orgID, req.PageID)
	if err != nil {
		return nil, models.InternalWithError("Failed to list assets", err)
	}

	assetList := make([]any, len(assets))
	for i, a := range assets {
		assetList[i] = map[string]any{
			"id":        a.ID,
			"name":      a.Name,
			"size":      a.Size,
			"mime_type": a.MimeType,
			"created":   a.Created,
			"url":       fmt.Sprintf("/api/%s/assets/%s/%s", orgID, req.PageID, a.Name),
		}
	}

	return &models.ListPageAssetsResponse{Assets: assetList}, nil
}

// UploadPageAssetHandler handles asset uploading (multipart/form-data).
func (h *AssetHandler) UploadPageAssetHandler(w http.ResponseWriter, r *http.Request) {
	pageID := r.PathValue("id")

	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "File is required", http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	as := storage.NewAssetService(h.fileStore, h.git, h.orgs)
	asset, err := as.SaveAsset(r.Context(), pageID, header.Filename, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprintf(w, `{"id":"%s","name":"%s"}`, asset.ID, asset.Name)
}

// ServeAssetFile serves the binary data of an asset.
func (h *AssetHandler) ServeAssetFile(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	pageID := r.PathValue("id")
	assetName := r.PathValue("name")

	data, err := h.fileStore.ReadAsset(orgID, pageID, assetName)
	if err != nil {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}

	// Simple MIME detection
	mime := "application/octet-stream"
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	_, _ = w.Write(data)
}

// DeletePageAsset deletes an asset.
func (h *AssetHandler) DeletePageAsset(ctx context.Context, req models.DeletePageAssetRequest) (*models.DeletePageAssetResponse, error) {
	orgID := models.GetOrgID(ctx)
	err := h.fileStore.DeleteAsset(orgID, req.PageID, req.AssetName)
	if err != nil {
		return nil, models.NotFound("asset")
	}

	if h.git != nil {
		_ = h.git.CommitChange(ctx, "delete", "asset", req.AssetName, "Deleted asset from page "+req.PageID)
	}

	return &models.DeletePageAssetResponse{}, nil
}
