package storage

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"

	"github.com/maruel/mddb/internal/models"
)

// AssetService handles asset business logic.
type AssetService struct {
	fileStore  *FileStore
	gitService *GitService
	orgService *OrganizationService
}

// NewAssetService creates a new asset service.
func NewAssetService(fileStore *FileStore, gitService *GitService, orgService *OrganizationService) *AssetService {
	return &AssetService{
		fileStore:  fileStore,
		gitService: gitService,
		orgService: orgService,
	}
}

// SaveAsset saves an asset file to a page's directory.
func (s *AssetService) SaveAsset(ctx context.Context, pageID, fileName string, data []byte) (*models.Asset, error) {
	if pageID == "" {
		return nil, fmt.Errorf("page id cannot be empty")
	}
	if fileName == "" {
		return nil, fmt.Errorf("file name cannot be empty")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("file data cannot be empty")
	}

	orgID := models.GetOrgID(ctx)

	// Check Quota
	if s.orgService != nil {
		org, err := s.orgService.GetOrganization(orgID)
		if err == nil && org.Quotas.MaxStorage > 0 {
			_, usage, err := s.fileStore.GetOrganizationUsage(orgID)
			if err == nil && usage+int64(len(data)) > org.Quotas.MaxStorage {
				return nil, fmt.Errorf("storage quota exceeded (%d/%d bytes)", usage+int64(len(data)), org.Quotas.MaxStorage)
			}
		}
	}

	// Verify page exists
	if !s.fileStore.PageExists(orgID, pageID) {
		return nil, fmt.Errorf("page not found")
	}

	path, err := s.fileStore.SaveAsset(orgID, pageID, fileName, data)
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
		if err := s.gitService.CommitChange(ctx, "create", "asset", fileName, fmt.Sprintf("in page %s", pageID)); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return asset, nil
}

// GetAsset retrieves asset file data.
func (s *AssetService) GetAsset(ctx context.Context, pageID, assetName string) ([]byte, error) {
	if pageID == "" {
		return nil, fmt.Errorf("page id cannot be empty")
	}
	if assetName == "" {
		return nil, fmt.Errorf("asset name cannot be empty")
	}

	orgID := models.GetOrgID(ctx)
	return s.fileStore.ReadAsset(orgID, pageID, assetName)
}

// DeleteAsset deletes an asset file from a page's directory.
func (s *AssetService) DeleteAsset(ctx context.Context, pageID, assetName string) error {
	if pageID == "" {
		return fmt.Errorf("page id cannot be empty")
	}
	if assetName == "" {
		return fmt.Errorf("asset name cannot be empty")
	}

	orgID := models.GetOrgID(ctx)
	if err := s.fileStore.DeleteAsset(orgID, pageID, assetName); err != nil {
		return err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, "delete", "asset", assetName, fmt.Sprintf("in page %s", pageID)); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}

// ListAssets lists all assets in a page's directory.
func (s *AssetService) ListAssets(ctx context.Context, pageID string) ([]*models.Asset, error) {
	if pageID == "" {
		return nil, fmt.Errorf("page id cannot be empty")
	}

	orgID := models.GetOrgID(ctx)
	return s.fileStore.ListAssets(orgID, pageID)
}
