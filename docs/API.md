# mddb API Reference

mddb provides a RESTful JSON API for interacting with nodes (pages and databases), records, and assets, scoped by organization.

## Organizations

Most endpoints require an `{orgID}` in the path to ensure data isolation.

## Nodes (Unified Pages & Databases)

Nodes represent both document pages and databases in a unified hierarchy.

### List Nodes
**GET /api/{orgID}/nodes**
Returns the hierarchical tree of all nodes for the organization.

### Create Node
**POST /api/{orgID}/nodes**
Creates a new node (page or database).
Body: `{"title": "...", "content": "...", "parentId": "optional-id", "type": "page|database"}`

### Get Node
**GET /api/{orgID}/nodes/{id}**
Returns node content, metadata, and schema (if it's a database).

### Update Node
**PUT /api/{orgID}/nodes/{id}**
Updates an existing node.

### Delete Node
**DELETE /api/{orgID}/nodes/{id}**
Deletes a node and all its children/assets.

### Node History
**GET /api/{orgID}/nodes/{id}/history**
Returns git commit history for the node.

### Node Version
**GET /api/{orgID}/nodes/{id}/history/{hash}**
Returns node content at a specific commit.

## Databases & Records

Nodes of type `database` contain structured records.

### Get Database Schema
**GET /api/{orgID}/databases/{id}**
Returns database column definitions and metadata.

### List Records
**GET /api/{orgID}/databases/{id}/records**
Returns records with optional pagination.
Query params: `offset` (int), `limit` (int).

### Create Record
**POST /api/{orgID}/databases/{id}/records**
Adds a new record to the database.

### Update Record
**PUT /api/{orgID}/databases/{id}/records/{recordID}**
Updates an existing record. (Currently returning NotImplemented)

### Delete Record
**DELETE /api/{orgID}/databases/{id}/records/{recordID}**
Deletes a record. (Currently returning NotImplemented)

## Assets

### List Assets
**GET /api/{orgID}/pages/{id}/assets**
Returns a list of all assets in a page's directory.

### Upload Asset
**POST /api/{orgID}/pages/{id}/assets**
Upload a file using multipart form data (field: `file`). Max size: 32 MB.

### Delete Asset
**DELETE /api/{orgID}/pages/{id}/assets/{name}**
Deletes an asset.

### Serve Asset
**GET /assets/{orgID}/{id}/{name}**
Serves raw binary asset file.

## Search

### Global Search
**POST /api/{orgID}/search**
Search across all pages and databases within the organization. Body: `{"query": "..."}`

## Authentication

### Login
**POST /api/auth/login**
Authenticates a user and returns a session token.

### Register
**POST /api/auth/register**
Creates a new user account.

### OAuth2
**GET /api/auth/oauth/{provider}/login**
Initiates OAuth2 flow (provider: `google`).

**GET /api/auth/oauth/{provider}/callback**
OAuth2 callback endpoint.
