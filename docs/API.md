# mddb API Reference

mddb provides a RESTful JSON API for interacting with pages, databases, records, and assets.

## Pages

### List Pages
**GET /api/pages**
Returns all document pages.

### Get Page
**GET /api/pages/{id}**
Returns page content and metadata.

### Create Page
**POST /api/pages**
Creates a new page. Body: `{"title": "...", "content": "..."}`

### Update Page
**PUT /api/pages/{id}**
Updates an existing page.

### Delete Page
**DELETE /api/pages/{id}**
Deletes a page and its assets.

### Page History
**GET /api/pages/{id}/history**
Returns git commit history for the page.

### Page Version
**GET /api/pages/{id}/history/{hash}**
Returns page content at a specific commit.

## Databases

### List Databases
**GET /api/databases**
Returns all databases.

### Get Database
**GET /api/databases/{id}**
Returns database schema.

### Create Database
**POST /api/databases**
Creates a new database with schema.

## Records

### List Records
**GET /api/databases/{id}/records**
Returns records with optional pagination.
Query params: `offset` (int), `limit` (int).

### Create Record
**POST /api/databases/{id}/records**
Adds a new record to the database.

## Assets

### List Assets
**GET /api/pages/{id}/assets**
Returns a list of all assets in a page's directory.

### Upload Asset
**POST /api/pages/{id}/assets**
Upload a file using multipart form data (field: `file`). Max size: 32 MB.

### Delete Asset
**DELETE /api/pages/{id}/assets/{name}**
Deletes an asset.

### Serve Asset
**GET /assets/{id}/{name}**
Serves raw binary asset file.

## Search

### Global Search
**POST /api/search**
Search across all pages and databases. Body: `{"query": "..."}`
