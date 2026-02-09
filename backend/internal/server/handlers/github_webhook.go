// Handles GitHub webhook events for sync-on-push.

package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/syncsvc"
)

// GitHubWebhookHandler handles incoming GitHub webhook events.
type GitHubWebhookHandler struct {
	WebhookSecret string
	SyncService   *syncsvc.Service
	WsSvc         *identity.WorkspaceService
}

// HandleWebhook processes GitHub webhook events.
// It verifies the signature, and on push events, triggers a pull for matching workspaces.
func (h *GitHubWebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10 MB max
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	// Verify signature if webhook secret is configured.
	if h.WebhookSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifyWebhookSignature(body, sig, h.WebhookSecret) {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Only handle push events.
	event := r.Header.Get("X-GitHub-Event")
	if event != "push" {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"ok":true,"event":"%s","action":"ignored"}`, event)
		return
	}

	var payload struct {
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	repoFullName := payload.Repository.FullName
	if repoFullName == "" {
		http.Error(w, "Missing repository full_name", http.StatusBadRequest)
		return
	}

	// Find workspaces that match this repo.
	parts := strings.SplitN(repoFullName, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "Invalid repository name", http.StatusBadRequest)
		return
	}
	owner, name := parts[0], parts[1]

	// Iterate all workspaces to find matching ones.
	var matched int
	for ws := range h.WsSvc.Iter(0) { //nolint:contextcheck // goroutine uses background context intentionally
		if ws.GitRemote.RepoOwner == owner && ws.GitRemote.RepoName == name {
			matched++
			wsID := ws.ID
			go func() {
				ctx := context.Background()
				if err := h.SyncService.Pull(ctx, wsID); err != nil {
					slog.Error("Webhook pull failed", "wsID", wsID, "repo", repoFullName, "err", err)
				}
			}()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"ok":true,"matched_workspaces":%d}`, matched)
}

// verifyWebhookSignature verifies the HMAC-SHA256 signature of a webhook payload.
func verifyWebhookSignature(body []byte, signature, secret string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	sig, err := hex.DecodeString(signature[7:])
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(sig, expected)
}
