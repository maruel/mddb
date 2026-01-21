package content

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"slices"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/identity"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

var (
	errPageIDEmpty    = errors.New("page id cannot be empty")
	errFileNameEmpty  = errors.New("file name cannot be empty")
	errFileDataEmpty  = errors.New("file data cannot be empty")
	errAssetNameEmpty = errors.New("asset name cannot be empty")
)

// AssetService handles asset business logic.
type AssetService struct {
	FileStore  *FileStore
	gitService *infra.Git
	orgService *identity.OrganizationService
}

// NewAssetService creates a new asset service.
func NewAssetService(fileStore *FileStore, gitService *infra.Git, orgService *identity.OrganizationService) *AssetService {
	return &AssetService{
		FileStore:  fileStore,
		gitService: gitService,
		orgService: orgService,
	}
}

// SaveAsset saves an asset file to a page's directory.
func (s *AssetService) SaveAsset(ctx context.Context, orgID, pageID jsonldb.ID, fileName string, data []byte) (*Asset, error) {
	if pageID.IsZero() {
		return nil, errPageIDEmpty
	}
	if fileName == "" {
		return nil, errFileNameEmpty
	}
	if len(data) == 0 {
		return nil, errFileDataEmpty
	}

	// Check Quota
	if s.orgService != nil {
		org, err := s.orgService.Get(orgID)
		if err == nil && org.Quotas.MaxStorage > 0 {
			_, usage, err := s.FileStore.GetOrganizationUsage(orgID)
			if err == nil && usage+int64(len(data)) > org.Quotas.MaxStorage {
				return nil, fmt.Errorf("storage quota exceeded (%d/%d bytes)", usage+int64(len(data)), org.Quotas.MaxStorage)
			}
		}
	}

	// Verify page exists
	if !s.FileStore.PageExists(orgID, pageID) {
		return nil, errPageNotFound
	}

	path, err := s.FileStore.SaveAsset(orgID, pageID, fileName, data)
	if err != nil {
		return nil, err
	}

	// Detect MIME type from filename
	mimeType := mime.TypeByExtension(filepath.Ext(fileName))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	asset := &Asset{
		ID:       fileName,
		Name:     fileName,
		MimeType: mimeType,
		Size:     int64(len(data)),
		Path:     path,
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "create", "asset", fileName, "in page "+pageID.String()); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return asset, nil
}

// GetAsset retrieves asset file data.
func (s *AssetService) GetAsset(ctx context.Context, orgID, pageID jsonldb.ID, assetName string) ([]byte, error) {
	if pageID.IsZero() {
		return nil, errPageIDEmpty
	}
	if assetName == "" {
		return nil, errAssetNameEmpty
	}

	return s.FileStore.ReadAsset(orgID, pageID, assetName)
}

// DeleteAsset deletes an asset file from a page's directory.
func (s *AssetService) DeleteAsset(ctx context.Context, orgID, pageID jsonldb.ID, assetName string) error {
	if pageID.IsZero() {
		return errPageIDEmpty
	}
	if assetName == "" {
		return errAssetNameEmpty
	}

	if err := s.FileStore.DeleteAsset(orgID, pageID, assetName); err != nil {
		return err
	}

	if s.gitService != nil {
		if err := s.gitService.CommitChange(ctx, orgID, "delete", "asset", assetName, "in page "+pageID.String()); err != nil {
			fmt.Printf("failed to commit change: %v\n", err)
		}
	}

	return nil
}

// ListAssets lists all assets in a page's directory.
func (s *AssetService) ListAssets(ctx context.Context, orgID, pageID jsonldb.ID) ([]*Asset, error) {
	if pageID.IsZero() {
		return nil, errPageIDEmpty
	}

	it, err := s.FileStore.IterAssets(orgID, pageID)
	if err != nil {
		return nil, err
	}
	return slices.Collect(it), nil
}
