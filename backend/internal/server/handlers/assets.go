// Handles file upload and retrieval for node assets.

package handlers

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

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
	Svc *Services
	Cfg *Config
}

// UploadNodeAssetHandler handles asset uploading (multipart/form-data).
// This is a raw http.HandlerFunc because it handles multipart forms.
func (h *AssetHandler) UploadNodeAssetHandler(w http.ResponseWriter, r *http.Request) {
	wsIDStr := r.PathValue("wsID")
	nodeIDStr := r.PathValue("id")

	wsID, err := jsonldb.DecodeID(wsIDStr)
	if err != nil {
		writeErrorResponse(w, dto.BadRequest("invalid_ws_id"))
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

	// Check server-wide storage quota before saving
	maxStorage := h.Cfg.Quotas.MaxTotalStorageBytes
	if err := h.Svc.FileStore.CheckServerStorageQuota(int64(len(data)), maxStorage); err != nil {
		if errors.Is(err, content.ErrServerStorageQuotaExceeded) {
			writeErrorResponse(w, dto.QuotaExceededInt64("total storage", maxStorage))
			return
		}
		writeErrorResponse(w, dto.Internal("storage_quota_check"))
		return
	}

	author := git.Author{Name: user.Name, Email: user.Email}
	ws, err := h.Svc.FileStore.GetWorkspaceStore(r.Context(), wsID)
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
// Requires valid signature query parameters: sig (HMAC signature) and exp (expiry timestamp).
func (h *AssetHandler) ServeAssetFile(w http.ResponseWriter, r *http.Request) {
	wsIDStr := r.PathValue("wsID")
	nodeIDStr := r.PathValue("id")
	assetName := r.PathValue("name")

	// Verify signature
	sig := r.URL.Query().Get("sig")
	expStr := r.URL.Query().Get("exp")
	if sig == "" || expStr == "" {
		writeErrorResponse(w, dto.Forbidden("missing_signature"))
		return
	}

	expiry, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		writeErrorResponse(w, dto.Forbidden("invalid_expiry"))
		return
	}

	// Check if URL has expired
	if time.Now().Unix() > expiry {
		writeErrorResponse(w, dto.Forbidden("expired_url"))
		return
	}

	wsID, err := jsonldb.DecodeID(wsIDStr)
	if err != nil {
		writeErrorResponse(w, dto.BadRequest("invalid_ws_id"))
		return
	}
	nodeID, err := jsonldb.DecodeID(nodeIDStr)
	if err != nil {
		writeErrorResponse(w, dto.BadRequest("invalid_node_id"))
		return
	}

	// Verify signature matches the path and expiry
	path := fmt.Sprintf("%s/%s/%s", wsIDStr, nodeIDStr, assetName)
	if !h.Cfg.VerifyAssetSignature(path, sig, expiry) {
		writeErrorResponse(w, dto.Forbidden("invalid_signature"))
		return
	}

	ws, err := h.Svc.FileStore.GetWorkspaceStore(r.Context(), wsID)
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
	// Cache asset for the duration of the URL validity
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if _, err := w.Write(data); err != nil {
		slog.Error("Failed to write asset data", "error", err, "asset", assetName)
	}
}

// userContextKey is the context key for the authenticated user.
// This should match what the auth middleware uses.
type contextKey string

const userContextKey contextKey = "user"
