// Downloads and stores assets from Notion (images, files, etc).

package notion

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/maruel/mddb/backend/internal/ksid"
)

// AssetDownloader handles downloading and caching of Notion assets.
type AssetDownloader struct {
	client    *http.Client
	outputDir string
	mu        sync.Mutex

	// downloaded tracks URL -> local path mapping
	downloaded map[string]string

	// stats for reporting
	Downloaded int
	Skipped    int
	Errors     int
}

// NewAssetDownloader creates a new asset downloader.
func NewAssetDownloader(outputDir string) *AssetDownloader {
	return &AssetDownloader{
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		outputDir:  outputDir,
		downloaded: make(map[string]string),
	}
}

// DownloadAsset downloads an asset from URL and returns the local path.
// Returns empty string if the URL is external (not a Notion-hosted file).
// The asset is stored in {nodeDir}/assets/{hash}-{filename}.
func (d *AssetDownloader) DownloadAsset(nodeID ksid.ID, assetURL string) (string, error) {
	if assetURL == "" {
		return "", nil
	}

	// Skip external URLs (not Notion-hosted)
	if !isNotionAssetURL(assetURL) {
		d.Skipped++
		return assetURL, nil // Return original URL for external assets
	}

	d.mu.Lock()
	if localPath, ok := d.downloaded[assetURL]; ok {
		d.mu.Unlock()
		return localPath, nil
	}
	d.mu.Unlock()

	// Parse URL to get filename
	parsed, err := url.Parse(assetURL)
	if err != nil {
		d.Errors++
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Generate unique filename: hash prefix + original filename
	filename := path.Base(parsed.Path)
	if filename == "" || filename == "/" || filename == "." {
		filename = "asset"
	}

	// Remove query parameters from filename
	if idx := strings.Index(filename, "?"); idx > 0 {
		filename = filename[:idx]
	}

	// Create hash prefix from URL for uniqueness
	hash := sha256.Sum256([]byte(assetURL))
	hashPrefix := hex.EncodeToString(hash[:8])
	uniqueFilename := hashPrefix + "-" + filename

	// Create node directory (assets stored alongside index.md)
	nodeDir := filepath.Join(d.outputDir, nodeID.String())
	if err := os.MkdirAll(nodeDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 is intentional
		d.Errors++
		return "", fmt.Errorf("failed to create node dir: %w", err)
	}

	localPath := filepath.Join(nodeDir, uniqueFilename)

	// Download the file
	resp, err := d.client.Get(assetURL)
	if err != nil {
		d.Errors++
		return "", fmt.Errorf("failed to download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		d.Errors++
		return "", fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	// Create local file
	f, err := os.Create(localPath) //nolint:gosec // G304: localPath is constructed from validated nodeID
	if err != nil {
		d.Errors++
		return "", fmt.Errorf("failed to create file: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(localPath) // Clean up partial file
		d.Errors++
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	if err := f.Close(); err != nil {
		d.Errors++
		return "", fmt.Errorf("failed to close file: %w", err)
	}

	// Store relative path for use in markdown (just filename, same directory as index.md)
	relativePath := uniqueFilename

	d.mu.Lock()
	d.downloaded[assetURL] = relativePath
	d.Downloaded++
	d.mu.Unlock()

	return relativePath, nil
}

// isNotionAssetURL checks if a URL is a Notion-hosted asset that needs downloading.
// Notion-hosted files have expiring URLs from specific domains.
func isNotionAssetURL(assetURL string) bool {
	parsed, err := url.Parse(assetURL)
	if err != nil {
		return false
	}

	host := strings.ToLower(parsed.Host)

	// Notion-hosted asset domains
	notionDomains := []string{
		"s3.us-west-2.amazonaws.com",
		"prod-files-secure.s3.us-west-2.amazonaws.com",
		"secure.notion-static.com",
		"www.notion.so",
	}

	for _, domain := range notionDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}

	return false
}

// ProcessMediaBlock downloads assets from a media block and returns the local path.
func (d *AssetDownloader) ProcessMediaBlock(nodeID ksid.ID, media *MediaBlock) (string, error) {
	if media == nil {
		return "", nil
	}

	var assetURL string
	if media.File != nil {
		assetURL = media.File.URL
	} else if media.External != nil {
		assetURL = media.External.URL
	}

	return d.DownloadAsset(nodeID, assetURL)
}

// ProcessFileValue downloads assets from a file property value.
func (d *AssetDownloader) ProcessFileValue(nodeID ksid.ID, fv *FileValue) (string, error) {
	if fv == nil {
		return "", nil
	}

	var assetURL string
	if fv.File != nil {
		assetURL = fv.File.URL
	} else if fv.External != nil {
		assetURL = fv.External.URL
	}

	return d.DownloadAsset(nodeID, assetURL)
}

// ProcessIcon downloads an icon if it's a file and returns the result.
// Returns the emoji string for emoji icons, or local path for file-based icons.
func (d *AssetDownloader) ProcessIcon(nodeID ksid.ID, icon *Icon) (string, error) {
	if icon == nil {
		return "", nil
	}

	switch icon.Type {
	case "emoji":
		return icon.Emoji, nil
	case "file":
		if icon.File != nil {
			return d.DownloadAsset(nodeID, icon.File.URL)
		}
	case "external":
		if icon.External != nil {
			return d.DownloadAsset(nodeID, icon.External.URL)
		}
	}
	return "", nil
}

// ProcessCover downloads a cover image and returns the local path.
func (d *AssetDownloader) ProcessCover(nodeID ksid.ID, cover *File) (string, error) {
	if cover == nil {
		return "", nil
	}
	return d.DownloadAsset(nodeID, cover.URL)
}
