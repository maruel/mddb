package storage

import (
	"fmt"
	"mime"
	"path/filepath"

	"github.com/maruel/mddb/internal/models"
)

// AssetService handles asset business logic.
type AssetService struct {
	fileStore  *FileStore
	gitService *GitService
}

// NewAssetService creates a new asset service.
func NewAssetService(fileStore *FileStore, gitService *GitService) *AssetService {
	return &AssetService{
		fileStore:  fileStore,
		gitService: gitService,
	}
}

// SaveAsset saves an asset file to a page's directory.
func (s *AssetService) SaveAsset(pageID, fileName string, data []byte) (*models.Asset, error) {
	if pageID == "" {
		return nil, fmt.Errorf("page id cannot be empty")
	}
	if fileName == "" {
		return nil, fmt.Errorf("file name cannot be empty")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("file data cannot be empty")
	}

	// Verify page exists
	if !s.fileStore.PageExists(pageID) {
		return nil, fmt.Errorf("page not found")
	}

	path, err := s.fileStore.SaveAsset(pageID, fileName, data)
	if err != nil {
		return nil, err
	}

	// Detect MIME type from filename
	mimeType := mime.TypeByExtension(filepath.Ext(fileName))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	asset := &models.Asset{
		ID:       fileName,
		Name:     fileName,
		MimeType: mimeType,
		Size:     int64(len(data)),
		Path:     path,
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange("create", "asset", fileName, fmt.Sprintf("in page %s", pageID)); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return asset, nil
}

// GetAsset retrieves asset file data.
func (s *AssetService) GetAsset(pageID, assetName string) ([]byte, error) {
	if pageID == "" {
		return nil, fmt.Errorf("page id cannot be empty")
	}
	if assetName == "" {
		return nil, fmt.Errorf("asset name cannot be empty")
	}

	return s.fileStore.ReadAsset(pageID, assetName)
}

// DeleteAsset deletes an asset file from a page's directory.
func (s *AssetService) DeleteAsset(pageID, assetName string) error {
	if pageID == "" {
		return fmt.Errorf("page id cannot be empty")
	}
	if assetName == "" {
		return fmt.Errorf("asset name cannot be empty")
	}

	if err := s.fileStore.DeleteAsset(pageID, assetName); err != nil {
		return err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange("delete", "asset", assetName, fmt.Sprintf("in page %s", pageID)); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}

// ListAssets lists all assets in a page's directory.
func (s *AssetService) ListAssets(pageID string) ([]*models.Asset, error) {
	if pageID == "" {
		return nil, fmt.Errorf("page id cannot be empty")
	}

	return s.fileStore.ListAssets(pageID)
}
