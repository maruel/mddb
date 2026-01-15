package handlers

import (
	"net/http"

	"github.com/maruel/mddb/internal/storage"
	"github.com/maruel/mddb/internal/utils"
)

// AssetHandler handles asset/file-related HTTP requests
type AssetHandler struct {
	fileStore *storage.FileStore
}

// NewAssetHandler creates a new asset handler
func NewAssetHandler(fileStore *storage.FileStore) *AssetHandler {
	return &AssetHandler{fileStore: fileStore}
}

// ListAssets returns a list of all assets
func (h *AssetHandler) ListAssets(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement listing assets
	utils.RespondSuccess(w, http.StatusOK, []interface{}{})
}

// UploadAsset handles file uploads
func (h *AssetHandler) UploadAsset(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement uploading assets
	utils.RespondSuccess(w, http.StatusCreated, map[string]string{"id": "placeholder"})
}

// ServeAsset serves an asset file
func (h *AssetHandler) ServeAsset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// TODO: Implement serving assets
	utils.RespondError(w, http.StatusNotFound, "Asset not found", "NOT_FOUND")
	_ = id
}

// DeleteAsset deletes an asset
func (h *AssetHandler) DeleteAsset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// TODO: Implement deleting assets
	utils.RespondSuccess(w, http.StatusNoContent, nil)
	_ = id
}
