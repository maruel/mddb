// Handles file upload and retrieval for page assets.

package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage/content"
	"github.com/maruel/mddb/backend/internal/storage/git"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func init() {
	// Register MIME types not in the standard library.
	for _, pair := range [][2]string{
		{".aac", "audio/aac"},
		{".flac", "audio/flac"},
		{".jsonl", "application/jsonl"},
		{".md", "text/markdown"},
		{".wav", "audio/wav"},
	} {
		if err := mime.AddExtensionType(pair[0], pair[1]); err != nil {
			panic(err)
		}
	}
}

// AssetHandler handles asset/file-related HTTP requests.
type AssetHandler struct {
	fs *content.FileStoreService
}

// NewAssetHandler creates a new asset handler.
func NewAssetHandler(fs *content.FileStoreService) *AssetHandler {
	return &AssetHandler{fs: fs}
}

// ListPageAssets returns a list of assets associated with a page.
func (h *AssetHandler) ListPageAssets(ctx context.Context, wsID jsonldb.ID, _ *identity.User, req *dto.ListPageAssetsRequest) (*dto.ListPageAssetsResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	it, err := ws.IterAssets(req.NodeID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to list assets", err)
	}
	assets := slices.Collect(it)
	return &dto.ListPageAssetsResponse{Assets: assetsToSummaries(assets, wsID.String(), req.NodeID.String())}, nil
}

// UploadPageAssetHandler handles asset uploading (multipart/form-data).
// This is a raw http.HandlerFunc because it handles multipart forms.
func (h *AssetHandler) UploadPageAssetHandler(w http.ResponseWriter, r *http.Request) {
	orgIDStr := r.PathValue("orgID")
	nodeIDStr := r.PathValue("id")

	orgID, err := jsonldb.DecodeID(orgIDStr)
	if err != nil {
		writeErrorResponse(w, dto.BadRequest("invalid_org_id"))
		return
	}
	nodeID, err := jsonldb.DecodeID(nodeIDStr)
	if err != nil {
		writeErrorResponse(w, dto.BadRequest("invalid_node_id"))
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

	// Get user from context (set by auth middleware)
	user, ok := r.Context().Value(userContextKey).(*identity.User)
	if !ok || user == nil {
		writeErrorResponse(w, dto.Internal("user_context"))
		return
	}

	author := git.Author{Name: user.Name, Email: user.Email}
	ws, err := h.fs.GetWorkspaceStore(r.Context(), orgID)
	if err != nil {
		writeErrorResponse(w, dto.Internal("workspace"))
		return
	}
	asset, err := ws.SaveAsset(r.Context(), nodeID, header.Filename, data, author)
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
	nodeIDStr := r.PathValue("id")
	assetName := r.PathValue("name")

	orgID, err := jsonldb.DecodeID(orgIDStr)
	if err != nil {
		writeErrorResponse(w, dto.BadRequest("invalid_org_id"))
		return
	}
	nodeID, err := jsonldb.DecodeID(nodeIDStr)
	if err != nil {
		writeErrorResponse(w, dto.BadRequest("invalid_node_id"))
		return
	}

	ws, err := h.fs.GetWorkspaceStore(r.Context(), orgID)
	if err != nil {
		writeErrorResponse(w, dto.Internal("workspace"))
		return
	}
	data, err := ws.ReadAsset(nodeID, assetName)
	if err != nil {
		writeErrorResponse(w, dto.NotFound("asset"))
		return
	}

	mimeType := mime.TypeByExtension(filepath.Ext(assetName))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if _, err := w.Write(data); err != nil {
		slog.Error("Failed to write asset data", "error", err, "asset", assetName)
	}
}

// DeletePageAsset deletes an asset.
func (h *AssetHandler) DeletePageAsset(ctx context.Context, wsID jsonldb.ID, user *identity.User, req *dto.DeletePageAssetRequest) (*dto.DeletePageAssetResponse, error) {
	ws, err := h.fs.GetWorkspaceStore(ctx, wsID)
	if err != nil {
		return nil, dto.InternalWithError("Failed to get workspace", err)
	}
	author := git.Author{Name: user.Name, Email: user.Email}
	if err := ws.DeleteAsset(ctx, req.NodeID, req.AssetName, author); err != nil {
		return nil, dto.NotFound("asset")
	}
	return &dto.DeletePageAssetResponse{Ok: true}, nil
}

// userContextKey is the context key for the authenticated user.
// This should match what the auth middleware uses.
type contextKey string

const userContextKey contextKey = "user"
