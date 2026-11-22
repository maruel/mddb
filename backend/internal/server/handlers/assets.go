// Handles node asset uploads and parent-session-bound retrieval with current membership checks.

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto/v1"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage/content"
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

	wsID, err := ksid.Parse(wsIDStr)
	if err != nil {
		writeErrorResponse(w, dto.BadRequest("invalid_ws_id"))
		return
	}
	nodeID, err := ksid.Parse(nodeIDStr)
	if err != nil {
		writeErrorResponse(w, dto.BadRequest("invalid_node_id"))
		return
	}

	// Body limits govern transport independently of retained asset limits. The
	// storage owner admits net replacement growth, including shrinking recovery.
	maxBytes := h.Cfg.Snapshot().Quotas.MaxRequestBodyBytes
	if maxBytes > 0 {
		if r.ContentLength > maxBytes {
			writeErrorResponse(w, dto.PayloadTooLarge(maxBytes))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	}
	if err := r.ParseMultipartForm(10 << 20); err != nil { //nolint:gosec // G120: MaxBytesReader enforces the configured body limit; zero explicitly permits unlimited requests.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeErrorResponse(w, dto.PayloadTooLarge(maxBytes))
			return
		}
		writeErrorResponse(w, dto.BadRequest("form_parse"))
		return
	}
	defer func(ctx context.Context) {
		if err := r.MultipartForm.RemoveAll(); err != nil {
			slog.ErrorContext(ctx, "Failed to remove multipart staging", "err", err)
		}
	}(r.Context())

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

	// Get user from context (set by WrapAuthRaw middleware)
	user := reqctx.User(r.Context())
	if user == nil {
		writeErrorResponse(w, dto.Internal("user_context"))
		return
	}

	author := GitAuthor(user)
	ws, err := h.Svc.FileStore.GetWorkspaceStore(r.Context(), wsID)
	if err != nil {
		writeErrorResponse(w, dto.Internal("workspace"))
		return
	}

	// This existing pooled precheck is best-effort until aggregate admission.
	// Charge replacements by net growth so lowered pooled limits allow recovery.
	old, err := ws.AssetSize(r.Context(), nodeID, header.Filename)
	if err != nil {
		writeErrorResponse(w, dto.InternalWithError("asset_accounting", err))
		return
	}
	growth := header.Size - old
	maxStorage := h.Cfg.Snapshot().Quotas.MaxTotalStorageBytes
	if growth > 0 {
		if err := h.Svc.FileStore.CheckServerStorageQuota(growth, maxStorage); err != nil {
			if errors.Is(err, content.ErrServerStorageQuotaExceeded) {
				writeErrorResponse(w, dto.QuotaExceededInt64("total storage", maxStorage))
				return
			}
			writeErrorResponse(w, dto.InternalWithError("storage_quota_check", err))
			return
		}
	}

	// Verify node exists before saving asset
	if _, err := ws.ReadNode(r.Context(), nodeID); err != nil {
		writeErrorResponse(w, dto.NotFound("node"))
		return
	}

	asset, err := ws.SaveAsset(r.Context(), nodeID, header.Filename, file, header.Size, author)
	if err != nil {
		slog.Error("Failed to save asset", "error", err, "nodeID", nodeID, "filename", header.Filename, "wsID", wsID, "author", author)
		writeErrorResponse(w, dto.InternalWithError("asset_save", err))
		return
	}

	h.Svc.PublishAssetEvent(r.Context(), wsID, nodeID, user.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	resp := dto.UploadNodeAssetResponse{
		ID:       asset.ID,
		Name:     asset.Name,
		Size:     asset.Size,
		MimeType: asset.MimeType,
		URL:      h.Cfg.GenerateSignedAssetURL(r.Context(), wsID, nodeID, asset.Name),
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("Failed to write asset response", "error", err)
	}
}

// ServeAssetFile serves the binary data of an asset.
// This is a raw http.HandlerFunc for direct file serving.
// The signature binds path, parent session, and expiry. Every read rechecks the
// parent lifecycle and current workspace membership, without authorization caching.
func (h *AssetHandler) ServeAssetFile(w http.ResponseWriter, r *http.Request) {
	wsIDStr := r.PathValue("wsID")
	nodeIDStr := r.PathValue("id")
	assetName := r.PathValue("name")

	// Verify signature
	w.Header().Set("Cache-Control", "no-store")
	sidString := r.URL.Query().Get("sid")
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
	if time.Now().Unix() >= expiry {
		writeErrorResponse(w, dto.Forbidden("expired_url"))
		return
	}

	wsID, err := ksid.Parse(wsIDStr)
	if err != nil {
		writeErrorResponse(w, dto.BadRequest("invalid_ws_id"))
		return
	}
	nodeID, err := ksid.Parse(nodeIDStr)
	if err != nil {
		writeErrorResponse(w, dto.BadRequest("invalid_node_id"))
		return
	}

	// Verify signature matches the path and expiry
	path := fmt.Sprintf("%s/%s/%s", wsIDStr, nodeIDStr, assetName)
	if !h.Cfg.VerifyAssetSignature(path, sidString, sig, expiry) {
		writeErrorResponse(w, dto.Forbidden("invalid_signature"))
		return
	}

	sid, err := ksid.Parse(sidString)
	if err != nil {
		writeErrorResponse(w, dto.Forbidden("invalid_session"))
		return
	}
	session, err := h.Svc.Session.Get(sid)
	if err != nil || !session.RevokedAt.IsZero() || !session.ExpiresAt.AsTime().After(time.Now()) || expiry > session.ExpiresAt.AsTime().Unix() {
		writeErrorResponse(w, dto.Forbidden("session_expired"))
		return
	}
	user, err := h.Svc.User.Get(session.UserID)
	if err != nil {
		writeErrorResponse(w, dto.Forbidden("session_user"))
		return
	}
	if _, err := h.Svc.EffectiveWorkspaceRole(user, wsID); err != nil {
		writeErrorResponse(w, dto.Forbidden("workspace_membership"))
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
	if _, err := w.Write(data); err != nil {
		slog.Error("Failed to write asset data", "error", err, "asset", assetName)
	}
}
