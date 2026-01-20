package storage

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/entity"
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
func (s *AssetService) SaveAsset(ctx context.Context, orgID, pageID jsonldb.ID, fileName string, data []byte) (*entity.Asset, error) {
	if pageID.IsZero() {
		return nil, fmt.Errorf("page id cannot be empty")
	}
	if fileName == "" {
		return nil, fmt.Errorf("file name cannot be empty")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("file data cannot be empty")
	}

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

	asset := &entity.Asset{
		ID:       fileName,
		Name:     fileName,
		MimeType: mimeType,
		Size:     int64(len(data)),
		Path:     path,
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "create", "asset", fileName, fmt.Sprintf("in page %s", pageID.String())); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return asset, nil
}

// GetAsset retrieves asset file data.
func (s *AssetService) GetAsset(ctx context.Context, orgID, pageID jsonldb.ID, assetName string) ([]byte, error) {
	if pageID.IsZero() {
		return nil, fmt.Errorf("page id cannot be empty")
	}
	if assetName == "" {
		return nil, fmt.Errorf("asset name cannot be empty")
	}

	return s.fileStore.ReadAsset(orgID, pageID, assetName)
}

// DeleteAsset deletes an asset file from a page's directory.
func (s *AssetService) DeleteAsset(ctx context.Context, orgID, pageID jsonldb.ID, assetName string) error {
	if pageID.IsZero() {
		return fmt.Errorf("page id cannot be empty")
	}
	if assetName == "" {
		return fmt.Errorf("asset name cannot be empty")
	}

	if err := s.fileStore.DeleteAsset(orgID, pageID, assetName); err != nil {
		return err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "delete", "asset", assetName, fmt.Sprintf("in page %s", pageID.String())); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}

// ListAssets lists all assets in a page's directory.
func (s *AssetService) ListAssets(ctx context.Context, orgID, pageID jsonldb.ID) ([]*entity.Asset, error) {
	if pageID.IsZero() {
		return nil, fmt.Errorf("page id cannot be empty")
	}

	return s.fileStore.ListAssets(orgID, pageID)
}
