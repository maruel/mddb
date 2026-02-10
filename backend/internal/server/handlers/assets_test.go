package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maruel/mddb/backend/internal/ksid"
	"github.com/maruel/mddb/backend/internal/storage"
)

func TestAssetHandler(t *testing.T) {
	cfg := &Config{
		ServerConfig: storage.ServerConfig{
			JWTSecret: []byte("test-secret-key-32-bytes-long!!!"),
		},
		BaseURL: "http://localhost:8080",
	}
	ah := &AssetHandler{Svc: &Services{}, Cfg: cfg}

	t.Run("GenerateSignedAssetURL", func(t *testing.T) {
		wsID := ksid.ID(123)
		nodeID := ksid.ID(456)
		name := "test-image.png"

		url := cfg.GenerateSignedAssetURL(wsID, nodeID, name)

		// Verify URL format (rooted path, no hostname)
		if !strings.HasPrefix(url, "/assets/") {
			t.Errorf("URL should start with /assets/, got %s", url)
		}
		if !strings.Contains(url, "sig=") {
			t.Error("URL should contain sig parameter")
		}
		if !strings.Contains(url, "exp=") {
			t.Error("URL should contain exp parameter")
		}
		if !strings.Contains(url, wsID.String()) {
			t.Error("URL should contain workspace ID")
		}
		if !strings.Contains(url, nodeID.String()) {
			t.Error("URL should contain node ID")
		}
		if !strings.Contains(url, name) {
			t.Error("URL should contain asset name")
		}
	})

	t.Run("SignatureVerification", func(t *testing.T) {
		expiry := time.Now().Add(time.Hour).Unix()
		path := "123/456/test.png"

		t.Run("valid_signature", func(t *testing.T) {
			sig := cfg.generateSignature(path, expiry)
			if !cfg.VerifyAssetSignature(path, sig, expiry) {
				t.Error("Expected valid signature to verify")
			}
		})

		t.Run("invalid_signature", func(t *testing.T) {
			if cfg.VerifyAssetSignature(path, "invalid-signature", expiry) {
				t.Error("Expected invalid signature to fail verification")
			}
		})

		t.Run("wrong_path", func(t *testing.T) {
			sig := cfg.generateSignature(path, expiry)
			if cfg.VerifyAssetSignature("wrong/path/test.png", sig, expiry) {
				t.Error("Expected signature with wrong path to fail verification")
			}
		})

		t.Run("wrong_expiry", func(t *testing.T) {
			sig := cfg.generateSignature(path, expiry)
			if cfg.VerifyAssetSignature(path, sig, expiry+1000) {
				t.Error("Expected signature with wrong expiry to fail verification")
			}
		})
	})

	t.Run("ServeAssetFile", func(t *testing.T) {
		t.Run("missing_signature", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/assets/123/456/test.png", http.NoBody)
			req.SetPathValue("wsID", "123")
			req.SetPathValue("id", "456")
			req.SetPathValue("name", "test.png")

			w := httptest.NewRecorder()
			ah.ServeAssetFile(w, req)

			if w.Code != http.StatusForbidden {
				t.Errorf("Expected status %d, got %d", http.StatusForbidden, w.Code)
			}
		})

		t.Run("expired_signature", func(t *testing.T) {
			expiry := time.Now().Add(-time.Hour).Unix() // Expired
			path := "123/456/test.png"
			sig := cfg.generateSignature(path, expiry)

			req := httptest.NewRequest(http.MethodGet, "/assets/123/456/test.png?sig="+sig+"&exp="+string(rune(expiry)), http.NoBody)
			req.SetPathValue("wsID", "123")
			req.SetPathValue("id", "456")
			req.SetPathValue("name", "test.png")
			q := req.URL.Query()
			q.Set("sig", sig)
			q.Set("exp", time.Unix(expiry, 0).Format("20060102150405"))
			req.URL.RawQuery = q.Encode()

			w := httptest.NewRecorder()
			ah.ServeAssetFile(w, req)

			// Should fail due to expired or invalid expiry format
			if w.Code != http.StatusForbidden {
				t.Errorf("Expected status %d, got %d", http.StatusForbidden, w.Code)
			}
		})

		t.Run("invalid_signature", func(t *testing.T) {
			expiry := time.Now().Add(time.Hour).Unix()

			req := httptest.NewRequest(http.MethodGet, "/assets/123/456/test.png", http.NoBody)
			req.SetPathValue("wsID", "123")
			req.SetPathValue("id", "456")
			req.SetPathValue("name", "test.png")
			q := req.URL.Query()
			q.Set("sig", "invalid-signature")
			q.Set("exp", time.Unix(expiry, 0).Format("20060102150405"))
			req.URL.RawQuery = q.Encode()

			w := httptest.NewRecorder()
			ah.ServeAssetFile(w, req)

			// Should fail due to invalid signature or expiry parsing
			if w.Code != http.StatusForbidden {
				t.Errorf("Expected status %d, got %d", http.StatusForbidden, w.Code)
			}
		})
	})
}
