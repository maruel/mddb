// Tests for asset downloading and path generation.

package notion

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maruel/ksid"
)

func TestAssetDownloader(t *testing.T) {
	t.Run("DownloadAsset", func(t *testing.T) {
		t.Run("external URLs pass through", func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "image/png")
				_, err := w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a})
				if err != nil {
					t.Errorf("Failed to write response: %v", err)
				}
			}))
			t.Cleanup(server.Close)

			tempDir := t.TempDir()
			downloader := NewAssetDownloader(tempDir)
			nodeID := ksid.NewID()

			result, err := downloader.DownloadAsset(nodeID, server.URL+"/test.png")
			if err != nil {
				t.Fatalf("DownloadAsset() error = %v", err)
			}

			if result != server.URL+"/test.png" {
				t.Errorf("DownloadAsset() for external URL = %q, want original URL", result)
			}
		})
	})
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
