package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAssetService_SaveAsset(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create file store: %v", err)
	}

	// Create a page first
	pageID := EncodeID(1)
	_, err = fs.WritePage("org1", pageID, "Test Page", "Test content")
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}

	as := NewAssetService(fs, nil, nil)

	// Save an asset
	testData := []byte("test image data")
	asset, err := as.SaveAsset(newTestContext("org1"), pageID, "test.png", testData)
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

	// Verify file exists on disk
	assetPath := filepath.Join(fs.pageDir("org1", pageID), "test.png")
	info, err := os.Stat(assetPath)
	if err != nil {
		t.Fatalf("asset file not found: %v", err)
	}

	if info.Size() != int64(len(testData)) {
		t.Errorf("expected file size %d, got %d", len(testData), info.Size())
	}
}

func TestAssetService_GetAsset(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create file store: %v", err)
	}

	// Create a page and save asset
	pageID := EncodeID(1)
	_, err = fs.WritePage("org1", pageID, "Test Page", "Test content")
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}

	as := NewAssetService(fs, nil, nil)
	testData := []byte("test image data")
	_, err = as.SaveAsset(newTestContext("org1"), pageID, "test.png", testData)
	if err != nil {
		t.Fatalf("failed to save asset: %v", err)
	}

	// Retrieve asset
	data, err := as.GetAsset(newTestContext("org1"), pageID, "test.png")
	if err != nil {
		t.Fatalf("failed to get asset: %v", err)
	}

	if !bytes.Equal(data, testData) {
		t.Errorf("asset data mismatch")
	}
}

func TestAssetService_DeleteAsset(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create file store: %v", err)
	}

	// Create a page and save asset
	pageID := EncodeID(1)
	_, err = fs.WritePage("org1", pageID, "Test Page", "Test content")
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}

	as := NewAssetService(fs, nil, nil)
	_, err = as.SaveAsset(newTestContext("org1"), pageID, "test.png", []byte("test data"))
	if err != nil {
		t.Fatalf("failed to save asset: %v", err)
	}

	// Verify file exists
	assetPath := filepath.Join(fs.pageDir("org1", pageID), "test.png")
	if _, err := os.Stat(assetPath); err != nil {
		t.Fatalf("asset file not found before delete: %v", err)
	}

	// Delete asset
	err = as.DeleteAsset(newTestContext("org1"), pageID, "test.png")
	if err != nil {
		t.Fatalf("failed to delete asset: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(assetPath); err == nil {
		t.Fatal("asset file still exists after delete")
	}
}

func TestAssetService_ListAssets(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create file store: %v", err)
	}

	// Create a page
	pageID := EncodeID(1)
	_, err = fs.WritePage("org1", pageID, "Test Page", "Test content")
	if err != nil {
		t.Fatalf("failed to create page: %v", err)
	}

	as := NewAssetService(fs, nil, nil)

	// Save multiple assets
	assets := []string{"image1.png", "image2.jpg", "document.pdf"}
	for _, name := range assets {
		_, err := as.SaveAsset(newTestContext("org1"), pageID, name, []byte("test data"))
		if err != nil {
			t.Fatalf("failed to save asset %s: %v", name, err)
		}
	}

	// List assets
	listed, err := as.ListAssets(newTestContext("org1"), pageID)
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
	fs, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create file store: %v", err)
	}

	as := NewAssetService(fs, nil, nil)

	tests := []struct {
		name    string
		fn      func() error
		wantErr bool
	}{
		{
			name: "empty page id on save",
			fn: func() error {
				_, err := as.SaveAsset(newTestContext("org1"), "", "test.png", []byte("data"))
				return err
			},
			wantErr: true,
		},
		{
			name: "empty file name on save",
			fn: func() error {
				_, err := as.SaveAsset(newTestContext("org1"), EncodeID(1), "", []byte("data"))
				return err
			},
			wantErr: true,
		},
		{
			name: "empty data on save",
			fn: func() error {
				_, err := as.SaveAsset(newTestContext("org1"), EncodeID(1), "test.png", []byte(""))
				return err
			},
			wantErr: true,
		},
		{
			name: "empty page id on get",
			fn: func() error {
				_, err := as.GetAsset(newTestContext("org1"), "", "test.png")
				return err
			},
			wantErr: true,
		},
		{
			name: "empty asset name on get",
			fn: func() error {
				_, err := as.GetAsset(newTestContext("org1"), EncodeID(1), "")
				return err
			},
			wantErr: true,
		},
		{
			name: "empty page id on list",
			fn: func() error {
				_, err := as.ListAssets(newTestContext("org1"), "")
				return err
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if (err != nil) != tt.wantErr {
				t.Errorf("got error %v, want error %v", err, tt.wantErr)
			}
		})
	}
}
