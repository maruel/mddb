package handlers

import (
	"context"

	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/storage"
)

// AssetHandler handles asset/file-related HTTP requests
type AssetHandler struct {
	fileStore *storage.FileStore
}

// NewAssetHandler creates a new asset handler
func NewAssetHandler(fileStore *storage.FileStore) *AssetHandler {
	return &AssetHandler{fileStore: fileStore}
}

// ListAssetsRequest is a request to list all assets.
type ListAssetsRequest struct{}

// ListAssetsResponse is a response containing a list of assets.
type ListAssetsResponse struct {
	Assets []any `json:"assets"`
}

// UploadAssetRequest is a request to upload an asset.
type UploadAssetRequest struct{}

// UploadAssetResponse is a response from uploading an asset.
type UploadAssetResponse struct {
	ID string `json:"id"`
}

// ServeAssetRequest is a request to serve an asset.
type ServeAssetRequest struct {
	ID string `path:"id"`
}

// ServeAssetResponse is a response when serving an asset.
type ServeAssetResponse struct {
	Data string `json:"data"`
}

// DeleteAssetRequest is a request to delete an asset.
type DeleteAssetRequest struct {
	ID string `path:"id"`
}

// DeleteAssetResponse is a response from deleting an asset.
type DeleteAssetResponse struct{}

// ListAssets returns a list of all assets
func (h *AssetHandler) ListAssets(ctx context.Context, req ListAssetsRequest) (*ListAssetsResponse, error) {
	// TODO: Implement listing assets
	return &ListAssetsResponse{Assets: []any{}}, nil
}

// UploadAsset handles file uploads
// Note: Multipart form handling needs custom logic, will be implemented separately
func (h *AssetHandler) UploadAsset(ctx context.Context, req UploadAssetRequest) (*UploadAssetResponse, error) {
	// TODO: Implement uploading assets
	return &UploadAssetResponse{ID: "placeholder"}, nil
}

// ServeAsset serves an asset file
func (h *AssetHandler) ServeAsset(ctx context.Context, req ServeAssetRequest) (*ServeAssetResponse, error) {
	// TODO: Implement serving assets (req.ID is populated from path parameter)
	return nil, errors.NewAPIError(404, "Asset not found")
}

// DeleteAsset deletes an asset
func (h *AssetHandler) DeleteAsset(ctx context.Context, req DeleteAssetRequest) (*DeleteAssetResponse, error) {
	// TODO: Implement deleting assets (req.ID is populated from path parameter)
	return &DeleteAssetResponse{}, nil
}
