package content

import (
	"bytes"
	"testing"

	"github.com/maruel/mddb/backend/internal/jsonldb"
	"github.com/maruel/mddb/backend/internal/storage/infra"
)

func TestAssetService_SaveAsset(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := infra.NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create file store: %v", err)
	}

	// Create a page first
	orgID := jsonldb.ID(100)
	pageID := jsonldb.ID(1)
	_, err = fs.WritePage(orgID, pageID, "Test Page", "Test content")
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}

	as := NewAssetService(fs, nil, nil)

	// Save an asset
	testData := []byte("test image data")
	asset, err := as.SaveAsset(newTestContext(t, orgID.String()), orgID, pageID, "test.png", testData)
	if err != nil {
		t.Fatalf("failed to save asset: %v", err)
	}

	if asset.ID != "test.png" {
		t.Errorf("expected asset ID 'test.png', got '%s'", asset.ID)
	}

	if asset.Name != "test.png" {
		t.Errorf("expected asset name 'test.png', got '%s'", asset.Name)
	}

	if asset.MimeType != "image/png" {
		t.Errorf("expected mime type 'image/png', got '%s'", asset.MimeType)
	}

	if asset.Size != int64(len(testData)) {
		t.Errorf("expected size %d, got %d", len(testData), asset.Size)
	}

	// Verify file exists and can be retrieved
	retrievedData, err := as.GetAsset(newTestContext(t, orgID.String()), orgID, pageID, "test.png")
	if err != nil {
		t.Fatalf("asset file not found: %v", err)
	}

	if !bytes.Equal(retrievedData, testData) {
		t.Errorf("retrieved data does not match saved data")
	}
}

func TestAssetService_GetAsset(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := infra.NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create file store: %v", err)
	}

	// Create a page and save asset
	orgID := jsonldb.ID(100)
	pageID := jsonldb.ID(1)
	_, err = fs.WritePage(orgID, pageID, "Test Page", "Test content")
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}

	as := NewAssetService(fs, nil, nil)
	testData := []byte("test image data")
	_, err = as.SaveAsset(newTestContext(t, orgID.String()), orgID, pageID, "test.png", testData)
	if err != nil {
		t.Fatalf("failed to save asset: %v", err)
	}

	// Retrieve asset
	data, err := as.GetAsset(newTestContext(t, orgID.String()), orgID, pageID, "test.png")
	if err != nil {
		t.Fatalf("failed to get asset: %v", err)
	}

	if !bytes.Equal(data, testData) {
		t.Errorf("asset data mismatch")
	}
}

func TestAssetService_DeleteAsset(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := infra.NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create file store: %v", err)
	}

	// Create a page and save asset
	orgID := jsonldb.ID(100)
	pageID := jsonldb.ID(1)
	_, err = fs.WritePage(orgID, pageID, "Test Page", "Test content")
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}

	as := NewAssetService(fs, nil, nil)
	_, err = as.SaveAsset(newTestContext(t, orgID.String()), orgID, pageID, "test.png", []byte("test data"))
	if err != nil {
		t.Fatalf("failed to save asset: %v", err)
	}

	// Verify file exists via public API
	ctx := newTestContext(t, orgID.String())
	if _, err := as.GetAsset(ctx, orgID, pageID, "test.png"); err != nil {
		t.Fatalf("asset file not found before delete: %v", err)
	}

	// Delete asset
	err = as.DeleteAsset(ctx, orgID, pageID, "test.png")
	if err != nil {
		t.Fatalf("failed to delete asset: %v", err)
	}

	// Verify file is gone
	if _, err := as.GetAsset(ctx, orgID, pageID, "test.png"); err == nil {
		t.Fatal("asset file still exists after delete")
	}
}

func TestAssetService_ListAssets(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := infra.NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create file store: %v", err)
	}

	// Create a page
	orgID := jsonldb.ID(100)
	pageID := jsonldb.ID(1)
	_, err = fs.WritePage(orgID, pageID, "Test Page", "Test content")
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}

	as := NewAssetService(fs, nil, nil)

	// Save multiple assets
	assets := []string{"image1.png", "image2.jpg", "document.pdf"}
	for _, name := range assets {
		_, err := as.SaveAsset(newTestContext(t, orgID.String()), orgID, pageID, name, []byte("test data"))
		if err != nil {
			t.Fatalf("failed to save asset %s: %v", name, err)
		}
	}

	// List assets
	listed, err := as.ListAssets(newTestContext(t, orgID.String()), orgID, pageID)
	if err != nil {
		t.Fatalf("failed to list assets: %v", err)
	}

	if len(listed) != len(assets) {
		t.Errorf("expected %d assets, got %d", len(assets), len(listed))
	}

	// Verify assets are in the list
	assetMap := make(map[string]bool)
	for _, a := range listed {
		assetMap[a.Name] = true
	}

	for _, name := range assets {
		if !assetMap[name] {
			t.Errorf("asset %s not found in list", name)
		}
	}
}

func TestAssetService_Validation(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := infra.NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create file store: %v", err)
	}
	as := NewAssetService(fs, nil, nil)
	orgID := jsonldb.ID(100)
	var zeroID jsonldb.ID
	t.Run("empty page id on save", func(t *testing.T) {
		if _, err := as.SaveAsset(newTestContext(t, orgID.String()), orgID, zeroID, "test.png", []byte("data")); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("empty file name on save", func(t *testing.T) {
		if _, err := as.SaveAsset(newTestContext(t, orgID.String()), orgID, jsonldb.ID(1), "", []byte("data")); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("empty data on save", func(t *testing.T) {
		if _, err := as.SaveAsset(newTestContext(t, orgID.String()), orgID, jsonldb.ID(1), "test.png", []byte("")); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("empty page id on get", func(t *testing.T) {
		if _, err := as.GetAsset(newTestContext(t, orgID.String()), orgID, zeroID, "test.png"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("empty asset name on get", func(t *testing.T) {
		if _, err := as.GetAsset(newTestContext(t, orgID.String()), orgID, jsonldb.ID(1), ""); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("empty page id on list", func(t *testing.T) {
		if _, err := as.ListAssets(newTestContext(t, orgID.String()), orgID, zeroID); err == nil {
			t.Error("expected error")
		}
	})
}
