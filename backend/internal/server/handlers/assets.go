package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// AssetHandler handles asset/file-related HTTP requests.
type AssetHandler struct {
	fileStore    *content.FileStore
	assetService *content.AssetService
}

// NewAssetHandler creates a new asset handler.
func NewAssetHandler(fileStore *content.FileStore, gitClient *git.Client, orgs *identity.OrganizationService) *AssetHandler {
	return &AssetHandler{
		fileStore:    fileStore,
		assetService: content.NewAssetService(fileStore, gitClient, orgs),
	}
}

// ListPageAssets returns a list of assets associated with a page.
func (h *AssetHandler) ListPageAssets(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req dto.ListPageAssetsRequest) (*dto.ListPageAssetsResponse, error) {
	pageID, err := decodeID(req.PageID, "page_id")
	if err != nil {
		return nil, err
	}
	it, err := h.fileStore.IterAssets(orgID, pageID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list assets", err)
	}
	assets := slices.Collect(it)
	return &dto.ListPageAssetsResponse{Assets: assetsToSummaries(assets, orgID.String(), req.PageID)}, nil
}

// UploadPageAssetHandler handles asset uploading (multipart/form-data).
// This is a raw http.HandlerFunc because it handles multipart forms.
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
	defer func() {
		if err := file.Close(); err != nil {
			slog.Error("Failed to close uploaded file", "error", err)
		}
	}()

	data, err := io.ReadAll(file)
	if err != nil {
		writeErrorResponse(w, dto.Internal("file_read"))
		return
	}

	asset, err := h.assetService.Save(r.Context(), orgID, pageID, header.Filename, data)
	if err != nil {
		writeErrorResponse(w, dto.Internal("asset_save"))
		return
	}

	w.WriteHeader(http.StatusCreated)
	if _, err := fmt.Fprintf(w, `{"id":"%s","name":"%s"}`, asset.ID, asset.Name); err != nil {
		slog.Error("Failed to write asset response", "error", err)
	}
}

// ServeAssetFile serves the binary data of an asset.
// This is a raw http.HandlerFunc for direct file serving.
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
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if _, err := w.Write(data); err != nil {
		slog.Error("Failed to write asset data", "error", err, "asset", assetName)
	}
}

// DeletePageAsset deletes an asset.
func (h *AssetHandler) DeletePageAsset(ctx context.Context, orgID jsonldb.ID, _ *identity.User, req dto.DeletePageAssetRequest) (*dto.DeletePageAssetResponse, error) {
	pageID, err := decodeID(req.PageID, "page_id")
	if err != nil {
		return nil, err
	}
	if err := h.assetService.Delete(ctx, orgID, pageID, req.AssetName); err != nil {
		return nil, dto.NotFound("asset")
	}
	return &dto.DeletePageAssetResponse{Ok: true}, nil
}
