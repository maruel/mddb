# Phase 4: Assets & Media - Implementation Summary

## Overview
Phase 4 implements complete asset management for mddb, enabling file uploads, storage, retrieval, and serving of assets within page directories.

## Architecture

### Storage Model
```
data/pages/
├── 1/
│   ├── index.md          # Page content
│   ├── image.png         # Asset
│   ├── diagram.svg       # Asset
│   └── favicon.ico       # Asset (optional)
├── 2/
│   ├── index.md          # Page content
│   ├── photo.jpg         # Asset
│   └── favicon.png       # Asset (optional)
```

**Key Design Decision:** Assets live in page directories, creating a natural namespace. When a page is deleted, all its assets are automatically cleaned up.

## Backend Implementation

### FileStore Methods (internal/storage/filestore.go)
```go
SaveAsset(pageID, assetName string, data []byte) (string, error)
  // Saves asset file to page directory
  // Returns relative path from page directory

ReadAsset(pageID, assetName string) ([]byte, error)
  // Reads asset file from page directory
  // Returns binary data

DeleteAsset(pageID, assetName string) error
  // Deletes asset file from page directory

ListAssets(pageID string) ([]*models.Asset, error)
  // Lists all asset files in page directory
  // Excludes index.md, metadata.json, data.jsonl
```

### AssetService (internal/storage/asset_service.go)
Validation and business logic layer:
- Input validation (empty checks, data validation)
- MIME type detection based on file extension
- Service-level error handling
- Asset metadata creation

### AssetHandler (internal/server/handlers/assets.go)
HTTP endpoints:

| Method | Endpoint | Handler |
|--------|----------|---------|
| GET | /api/pages/{id}/assets | ListPageAssets |
| POST | /api/pages/{id}/assets | UploadPageAssetHandler |
| DELETE | /api/pages/{id}/assets/{name} | DeletePageAsset |
| GET | /assets/{id}/{name} | ServeAssetFile |

#### Upload Handler (Multipart Form)
```
POST /api/pages/1/assets
Content-Type: multipart/form-data

file: <binary data>

Response (201 Created):
{
  "id": "image.png",
  "name": "image.png",
  "size": 12345,
  "mime_type": "image/png"
}
```

#### Serve Handler (Raw Binary)
```
GET /assets/1/image.png

Response (200 OK):
Content-Type: image/png
Cache-Control: public, max-age=3600

<binary file data>
```

### Router Configuration (internal/server/router.go)
```go
// List assets in a page
mux.Handle("GET /api/pages/{id}/assets", Wrap(ah.ListPageAssets))

// Upload asset (multipart form - needs direct http.Handler)
mux.HandleFunc("POST /api/pages/{id}/assets", ah.UploadPageAssetHandler)

// Delete asset
mux.Handle("DELETE /api/pages/{id}/assets/{name}", Wrap(ah.DeletePageAsset))

// Serve raw asset file
mux.HandleFunc("GET /assets/{id}/{name}", ah.ServeAssetFile)
```

## Frontend Integration Points

The backend is ready for frontend integration via these endpoints:

1. **Upload**: `POST /api/pages/{id}/assets` with multipart form data
2. **List**: `GET /api/pages/{id}/assets` to get asset metadata
3. **Delete**: `DELETE /api/pages/{id}/assets/{name}` to remove assets
4. **Serve**: `GET /assets/{id}/{name}` to display assets in pages

## MIME Type Support

Automatic detection based on file extension:

| Extension | MIME Type |
|-----------|-----------|
| .png | image/png |
| .jpg, .jpeg | image/jpeg |
| .gif | image/gif |
| .webp | image/webp |
| .avif | image/avif |
| .ico | image/x-icon |
| .svg | image/svg+xml |
| .pdf | application/pdf |
| .txt | text/plain |
| (others) | application/octet-stream |

## Model Changes

### Page Model (internal/models/models.go)
Added favicon URL field:
```go
type Page struct {
    ID         string
    Title      string
    Content    string
    Created    time.Time
    Modified   time.Time
    Tags       []string
    Path       string
    FaviconURL string // NEW: URL to favicon in page directory
}
```

## Testing

### Asset Service Tests (internal/storage/asset_service_test.go)
- `TestAssetService_SaveAsset`: Save asset file to page directory
- `TestAssetService_GetAsset`: Retrieve asset file data
- `TestAssetService_DeleteAsset`: Delete asset from page directory
- `TestAssetService_ListAssets`: List all assets in page directory
- `TestAssetService_Validation`: Input validation for all methods (6 test cases)

**Total: 9 test cases, 100% passing**

### Test Coverage
- File I/O operations
- MIME type detection
- Validation edge cases
- Asset metadata
- Directory cleanup

## Key Features

### 1. Per-Page Asset Namespaces
- Each page has its own directory for assets
- Clean separation of concerns
- Automatic cleanup when page is deleted

### 2. Multipart File Upload
- Standard HTTP multipart form data
- 32 MB per file limit
- Preserves original filenames
- Automatic MIME type detection

### 3. Asset Serving
- Efficient binary file serving
- Proper Content-Type headers
- Cache-Control headers for performance
- Support for all common file types

### 4. Validation
- Empty page ID check
- Empty filename check
- Empty file data check
- Page existence verification

## Constraints & Limitations

1. **Filename Collision**: Files with same name overwrite previous versions
2. **File Size**: 32 MB limit per file (configurable in handler)
3. **Character Support**: Filenames should be URL-safe (escaping not implemented)
4. **No Versioning**: Assets are not versioned (current limitation)

## Future Enhancements

### Phase 4.5: Frontend Asset Integration
- Upload UI component
- Asset preview gallery
- Drag-and-drop file upload
- Image embedding in markdown
- Favicon preview and upload

### Beyond Phase 4
- Image resizing and optimization
- Thumbnail generation
- Asset versioning/history
- S3/cloud storage support
- Asset search and tagging
- Batch upload operations

## Performance Characteristics

- **Upload**: O(1) - Direct file write
- **Serve**: O(1) - Direct file read
- **List**: O(n) - Directory scan where n = assets in page
- **Delete**: O(1) - Direct file delete

## Security Considerations

- File path traversal: Assets must stay within page directory
- File size limits: 32 MB per file
- MIME type detection: Prevents executable uploads as images
- No script execution: Assets served as static files

## Deployment Notes

- No additional dependencies required
- File storage uses standard Go libraries
- Compatible with Docker and file system mounts
- Assets included in directory backups
- Assets can be git-tracked if needed
