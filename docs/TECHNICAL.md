# Technical Documentation

## Storage Model

mddb uses a multi-tenant directory structure where each organization owns its own data and Git history, managed via submodules.

### Directory Layout
- **Root Repository**: `data/`
  - The `data/` directory is itself a Git repository that manages organizations as **Git submodules**.
  - `data/db/`: System-wide information (Users, Organizations, Memberships) stored in JSON/SQLite.
- **Organization Submodules**: `data/{orgID}/`
  - Each organization directory is an independent Git repository linked as a submodule to the root.
  - `data/{orgID}/pages/`: Hierarchical page structure using numeric IDs.
  - `data/{orgID}/assets/`: Organization-specific assets.

```
data/                     # Root Git Repository
├── .gitmodules           # Submodule definitions
├── db/                   # System Metadata (Global)
│   ├── users.json
│   └── ...
└── {orgID}/              # Organization Submodule (Independent Git Repo)
    └── pages/
        ├── 1/            # Page ID 1
        └── ...
```

### Automatic Versioning
mddb employs a hierarchical versioning strategy:
1. **Organization Level**: Changes within `data/{orgID}/` trigger commits to that organization's independent repository.
2. **Root Level**: The `data/` repository tracks the state of all organizations by updating its submodule pointers. This allows for global backups and state-in-time recovery across the entire system while maintaining tenant isolation.

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

## System Metadata Schema

The following tables are managed in `data/db/` (eventually SQLite).

| Table Name     | Go Symbol (internal/models) | Description                                     |
|----------------|-----------------------------|-------------------------------------------------|
| `users`        | `User`                      | Core identity and global settings               |
| `organizations`| `Organization`              | Workspace/tenant definitions                    |
| `memberships`  | `Membership`                | User-Org relationship, roles, and status       |
| `sessions`     | `Session`                   | Active user sessions and revocation             |

## Unified Node Architecture (Planned)

To achieve a Notion-like experience, mddb is moving towards a unified "Node" concept.

### Data Model Unification
A Node is a directory that can contain:
- `index.md`: Descriptive content (Page).
- `metadata.json`: Structured schema (Database).
- `data.jsonl`: Database records.
- Assets and sub-directories (Sub-pages).

### Hierarchical Tree
The API will provide a tree structure of all nodes, allowing the frontend to render a single, unified sidebar. This replaces the distinct "Pages" and "Databases" silos.

### Component-based Views
The UI will be refactored into modular "Views". A single Page can display its markdown content followed by one or more views (Table, etc.) of its own database records or even linked records from other nodes.

## Multi-user Architecture (Planned)

### Identity & Authentication
Authentication is handled via JWT. User credentials, profile information, and organization memberships are stored in the `data/db/` directory as JSON files.

**Future Note**: There is a planned migration of the `data/db/` contents to **SQLite** to improve query performance and relational integrity.

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
