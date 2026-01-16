package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"

	"github.com/maruel/mddb/internal/errors"
	"github.com/maruel/mddb/internal/models"
	"github.com/maruel/mddb/internal/storage"
)

// AssetHandler handles asset/file-related HTTP requests
type AssetHandler struct {
	assetService *storage.AssetService
}

// NewAssetHandler creates a new asset handler
func NewAssetHandler(fileStore *storage.FileStore, gitService *storage.GitService) *AssetHandler {
	return &AssetHandler{
		assetService: storage.NewAssetService(fileStore, gitService),
	}
}

// ListPageAssetsRequest is a request to list assets in a page.
type ListPageAssetsRequest struct {
	OrgID  string `path:"orgID"`
	PageID string `path:"id"`
}

// ListPageAssetsResponse is a response containing a list of assets.
type ListPageAssetsResponse struct {
	Assets []any `json:"assets"`
}

// UploadPageAssetRequest is a request to upload an asset to a page.
// Note: File data is handled separately via multipart form, this is a placeholder for the Wrap handler.
type UploadPageAssetRequest struct {
	OrgID  string `path:"orgID"`
	PageID string `path:"id"`
}

// UploadPageAssetResponse is a response from uploading an asset.
type UploadPageAssetResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
}

// DeletePageAssetRequest is a request to delete an asset from a page.
type DeletePageAssetRequest struct {
	OrgID     string `path:"orgID"`
	PageID    string `path:"id"`
	AssetName string `path:"name"`
}

// DeletePageAssetResponse is a response from deleting an asset.
type DeletePageAssetResponse struct{}

// ServeAssetRequest is a request to serve an asset file directly.
type ServeAssetRequest struct {
	OrgID     string `path:"orgID"`
	PageID    string `path:"id"`
	AssetName string `path:"name"`
}

// ServeAssetResponse wraps the binary asset data.
type ServeAssetResponse struct {
	Data     []byte
	MimeType string
}

// ListPageAssets returns a list of all assets in a page
func (h *AssetHandler) ListPageAssets(ctx context.Context, req ListPageAssetsRequest) (*ListPageAssetsResponse, error) {
	assets, err := h.assetService.ListAssets(ctx, req.PageID)
	if err != nil {
		return nil, errors.NotFound("page")
	}

	assetList := make([]any, len(assets))
	for i, a := range assets {
		assetList[i] = map[string]any{
			"id":        a.ID,
			"name":      a.Name,
			"size":      a.Size,
			"mime_type": a.MimeType,
		}
	}

	return &ListPageAssetsResponse{Assets: assetList}, nil
}

// DeletePageAsset deletes an asset from a page
func (h *AssetHandler) DeletePageAsset(
	ctx context.Context,
	req DeletePageAssetRequest,
) (*DeletePageAssetResponse, error) {
	err := h.assetService.DeleteAsset(ctx, req.PageID, req.AssetName)
	if err != nil {
		return nil, errors.NotFound("asset")
	}

	return &DeletePageAssetResponse{}, nil
}

// ServeAssetFile serves a raw asset file from a page directory.
// Handles GET /assets/{orgID}/{id}/{name}
func (h *AssetHandler) ServeAssetFile(w http.ResponseWriter, r *http.Request) {
	// Extract org ID, page ID and asset name from URL path
	// Pattern: /assets/{orgID}/{id}/{name}
	orgID := r.PathValue("orgID")
	pageID := r.PathValue("id")
	assetName := r.PathValue("name")

	if orgID == "" || pageID == "" || assetName == "" {
		http.NotFound(w, r)
		return
	}

	// For asset serving, we might not have a full user session if it's via <img> tag without credentials
	// but we still want to provide the orgID to the service.
	// We create a minimal user with the orgID if not authenticated.
	ctx := r.Context()
	if models.GetOrgID(ctx) == "" {
		ctx = context.WithValue(ctx, models.UserKey, &models.User{OrganizationID: orgID})
	}

	// Read asset data
	data, err := h.assetService.GetAsset(ctx, pageID, assetName)
	if err != nil {
		slog.ErrorContext(ctx, "failed to read asset", "orgID", orgID, "pageID", pageID, "assetName", assetName, "err", err)
		http.NotFound(w, r)
		return
	}

	// Determine MIME type from file extension
	mimeType := mime.TypeByExtension(filepath.Ext(assetName))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// Serve file
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	_, _ = w.Write(data)
}

// UploadPageAssetHandler handles file uploads with multipart form data.
// Handles POST /api/{orgID}/pages/{id}/assets
// Needs custom http.Handler since Wrap doesn't support multipart file handling.
func (h *AssetHandler) UploadPageAssetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID := models.GetOrgID(ctx)
	pageID := r.PathValue("id")

	if pageID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Missing page ID"})
		return
	}

	// Parse multipart form (32 << 20) max)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to parse multipart form"})
		return
	}

	// Get file from form
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "No file provided"})
		return
	}
	defer func() { _ = file.Close() }()

	// Read file data
	data, err := io.ReadAll(file)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to read file"})
		return
	}

	// Save asset
	asset, err := h.assetService.SaveAsset(ctx, pageID, fileHeader.Filename, data)
	if err != nil {
		slog.ErrorContext(ctx, "failed to save asset", "orgID", orgID, "pageID", pageID, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Failed to save asset"})
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":        asset.ID,
		"name":      asset.Name,
		"size":      asset.Size,
		"mime_type": asset.MimeType,
	})
}
