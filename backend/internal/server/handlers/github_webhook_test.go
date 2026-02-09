// Tests for GitHub webhook handler: signature verification and push event processing.

package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maruel/mddb/backend/internal/storage/identity"
)

func TestVerifyWebhookSignature(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"action":"push"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	t.Run("valid", func(t *testing.T) {
		if !verifyWebhookSignature(body, validSig, secret) {
			t.Fatal("expected valid signature")
		}
	})

	t.Run("wrong_secret", func(t *testing.T) {
		if verifyWebhookSignature(body, validSig, "wrong") {
			t.Fatal("expected invalid signature")
		}
	})

	t.Run("missing_prefix", func(t *testing.T) {
		if verifyWebhookSignature(body, "abc123", secret) {
			t.Fatal("expected invalid without sha256= prefix")
		}
	})

	t.Run("invalid_hex", func(t *testing.T) {
		if verifyWebhookSignature(body, "sha256=not-valid-hex!", secret) {
			t.Fatal("expected invalid for bad hex")
		}
	})

	t.Run("wrong_body", func(t *testing.T) {
		if verifyWebhookSignature([]byte("different"), validSig, secret) {
			t.Fatal("expected invalid for different body")
		}
	})
}

func TestHandleWebhook_MethodNotAllowed(t *testing.T) {
	h := &GitHubWebhookHandler{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/github", http.NoBody)
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleWebhook_InvalidSignature(t *testing.T) {
	h := &GitHubWebhookHandler{WebhookSecret: "secret"}
	body := `{"repository":{"full_name":"org/repo"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandleWebhook_NonPushEvent(t *testing.T) {
	h := &GitHubWebhookHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", strings.NewReader(`{}`))
	req.Header.Set("X-GitHub-Event", "ping")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"action":"ignored"`) {
		t.Fatalf("expected ignored action, got %s", w.Body.String())
	}
}

func TestHandleWebhook_InvalidJSON(t *testing.T) {
	h := &GitHubWebhookHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", strings.NewReader(`not json`))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleWebhook_MissingRepoName(t *testing.T) {
	h := &GitHubWebhookHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", strings.NewReader(`{"repository":{}}`))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleWebhook_PushNoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	h := setupWebhookHandler(t, tmpDir, "")

	body := `{"repository":{"full_name":"other/repo"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", strings.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"matched_workspaces":0`) {
		t.Fatalf("expected 0 matches, got %s", w.Body.String())
	}
}

func TestHandleWebhook_PushWithSignature(t *testing.T) {
	tmpDir := t.TempDir()
	secret := "webhook-secret"
	h := setupWebhookHandler(t, tmpDir, secret)

	body := []byte(`{"repository":{"full_name":"other/repo"}}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", strings.NewReader(string(body)))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", sig)
	w := httptest.NewRecorder()

	h.HandleWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// setupWebhookHandler creates a GitHubWebhookHandler with a real WorkspaceService for testing.
func setupWebhookHandler(t *testing.T, tmpDir, secret string) *GitHubWebhookHandler {
	t.Helper()
	wsSvc, err := identity.NewWorkspaceService(filepath.Join(tmpDir, "workspaces.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return &GitHubWebhookHandler{
		WebhookSecret: secret,
		WsSvc:         wsSvc,
		// SyncService is nil — matching workspaces won't actually pull,
		// but we can still verify matching/routing logic.
	}
}
