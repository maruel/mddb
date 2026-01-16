# Asset API Endpoints

All assets are stored within page directories and are accessed through the following endpoints:

## List Assets in a Page
**GET /api/pages/{id}/assets**

Returns a list of all assets in a page's directory.

Response:
```json
{
  "assets": [
    {
      "id": "image.png",
      "name": "image.png",
      "size": 12345,
      "mime_type": "image/png"
    }
  ]
}
```

## Upload Asset to a Page
**POST /api/pages/{id}/assets**

Upload a file to a page's asset directory using multipart form data.

Request:
- Form field: `file` (binary file data)
- Max size: 32 MB

Response (201 Created):
```json
{
  "id": "image.png",
  "name": "image.png",
  "size": 12345,
  "mime_type": "image/png"
}
```

## Delete Asset from a Page
**DELETE /api/pages/{id}/assets/{name}**

Delete an asset file from a page's directory.

Response (200 OK):
```json
{}
```

## Serve Asset File
**GET /assets/{id}/{name}**

Serve a binary asset file with appropriate Content-Type header.

Response:
- Content-Type: Detected based on file extension
- Cache-Control: public, max-age=3600
- Body: Raw binary file data

Supported file types:
- Images: PNG, JPG, JPEG, GIF, WebP, AVIF, ICO, SVG
- Documents: PDF, TXT
- Others: Any file extension (default: application/octet-stream)

## Notes

- Assets are stored in page directories: `data/pages/{id}/{filename}`
- Assets are automatically deleted when the parent page is deleted
- Filenames are preserved as-is (URL encoding handled by browser/client)
- File size limit: 32 MB per file
- No duplicate filename checking (files with same name will be overwritten)
