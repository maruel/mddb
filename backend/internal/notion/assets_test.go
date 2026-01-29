// Tests for asset downloading and path generation.

package notion

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
)

func TestAssetDownloader_ExternalURLsPassThrough(t *testing.T) {
	// Create a test server that returns a small image
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, err := w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a})
		if err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	downloader := NewAssetDownloader(tempDir)
	nodeID := jsonldb.NewID()

	// Test with an external URL (non-Notion) - should return original URL
	result, err := downloader.DownloadAsset(nodeID, server.URL+"/test.png")
	if err != nil {
		t.Fatalf("DownloadAsset() error = %v", err)
	}

	// For external URLs, it should return the original URL
	if result != server.URL+"/test.png" {
		t.Errorf("DownloadAsset() for external URL = %q, want original URL", result)
	}
}

func TestIsNotionAssetURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		// Notion-hosted URLs
		{"https://s3.us-west-2.amazonaws.com/bucket/file.png", true},
		{"https://prod-files-secure.s3.us-west-2.amazonaws.com/abc/def.jpg", true},
		{"https://secure.notion-static.com/path/to/file.png", true},
		{"https://www.notion.so/image/test.png", true},

		// External URLs (should not be downloaded)
		{"https://example.com/image.png", false},
		{"https://cdn.example.com/file.jpg", false},
		{"https://github.com/user/repo/raw/image.png", false},
		{"", false},
		{"invalid-url", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := isNotionAssetURL(tt.url)
			if got != tt.want {
				t.Errorf("isNotionAssetURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestAssetDownloader_RelativePathIsJustFilename(t *testing.T) {
	// The relative path returned should be just the filename.
	// Assets are stored in the same directory as index.md.
	// The frontend transforms filename.png to /assets/{wsId}/{nodeId}/filename.png

	// Expected format: just the hashed filename like "abc123def456-image.png"
	// NOT "./assets/..." or "assets/..." - just the filename itself

	// This matches how workspace_store.go stores assets:
	// filePath := filepath.Join(dir, assetName)  // Same dir as index.md
}
