# Technical Documentation

## Storage Model

mddb uses a directory-per-page structure. Every page (document or database) is assigned a monotonically increasing numeric ID.

```
data/pages/
├── 1/                    # Page ID 1
│   ├── index.md          # Content + YAML Front Matter
│   └── photo.jpg         # Asset local to this page
└── 2/                    # Page ID 2 (Database)
    ├── index.md
    ├── metadata.json     # Schema
    └── data.jsonl        # Records (JSON Lines)
```

### Automatic Versioning
The `data/` directory is an initialized Git repository. Every change (create/update/delete) triggers an automatic commit via the `GitService`. This provides a built-in audit trail and history/restore capability.

## Embedded Build Process

mddb is distributed as a single self-contained binary. This is achieved using Go's `embed` package.

### Workflow
1. **Frontend Build**: The SolidJS frontend is compiled into static assets in `frontend/dist/`.
2. **Go Generate**: The `frontend/frontend.go` file uses `//go:embed dist/*` to include these assets.
3. **Compilation**: The Go compiler includes the static assets in the final binary.

### Reproducibility
The `frontend/dist/` directory is tracked in Git to ensure that the Go binary can be built deterministically without requiring a Node.js environment in the build pipeline (though `make build` will attempt to rebuild it if tools are available).

## Performance Optimizations

### Streaming Reads
Database records are stored in JSONL format, allowing for line-by-line streaming using `bufio.Scanner`. This keeps memory usage low even for large databases.

### API Pagination
Record retrieval supports `offset` and `limit` parameters to handle large datasets efficiently.

## Multi-user Architecture (Planned)

### Identity & Authentication
Authentication will be handled via JWT. User credentials and profile information will be stored in `data/users.json` (encrypted).

### OAuth2 Integration
mddb will support OpenID Connect (OIDC) flows for Google and Microsoft.
- **Callback Handling**: Dedicated `/api/auth/callback/{provider}` endpoints.
- **Account Linking**: Ability to link local accounts with OAuth identities.
- **State Management**: CSRF protection using signed `state` parameters.

### Workspace Isolation
The `data/pages/` directory will be partitioned or tagged with organization IDs to support multiple organizations within a single mddb instance.

### RBAC Model
Role-Based Access Control will define permissions at two levels:
1. **Global/Organization**: Admin, Editor, Viewer roles.
2. **Resource (Page/DB)**: Specific overrides for sensitive content.

### Quota Tracking
Usage metrics (total bytes, page counts) will be cached in organization metadata. The `FileStore` will perform pre-write checks to ensure quotas are not exceeded.

## High-Efficiency Caching (Planned)

### In-Memory Cache
A thread-safe, in-memory cache will be implemented to store:
1. **Metadata**: Database schemas and organization configurations.
2. **Hot Pages**: Frequently accessed markdown content.
3. **Record Indexes**: In-memory maps of record IDs to file positions or small record sets.

### Strategy
- **LRU Policy**: Least Recently Used eviction to maintain a fixed memory footprint.
- **Write-Through/Invalidation**: Cache will be updated or invalidated on every write operation to ensure consistency with the on-disk storage.
- **Lazy Loading**: Data will be loaded into the cache on the first read request.
