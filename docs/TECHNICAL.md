# Technical Documentation

## Storage Model

mddb uses a multi-tenant directory structure where each organization owns its own data and Git history, managed via submodules.

### Directory Layout
- **Root Repository**: `data/`
  - The `data/` directory is itself a Git repository that tracks organization directories.
  - `data/db/`: System-wide information (Users, Organizations, Memberships) stored in JSON.
- **Organization Repositories**: `data/{orgID}/`
  - Each organization directory is an independent Git repository.
  - Changes in an organization directory are committed to its local repository, and the state of these repositories is tracked in the root `data/` repository via Git. (Note: Currently implemented as nested repositories rather than formal Git submodules with `.gitmodules`).
  - `data/{orgID}/pages/`: Page storage using flat numeric IDs.
  - `data/{orgID}/assets/`: Organization-specific assets.

```
data/                     # Root Git Repository
├── db/                   # System Metadata (Global)
│   ├── users.json
│   └── ...
└── {orgID}/              # Organization Repository (Independent Git Repo)
    └── pages/
        ├── 1/            # Page ID 1
        └── ...
```

### Automatic Versioning
mddb employs a hierarchical versioning strategy:
1. **Organization Level**: Changes within `data/{orgID}/` trigger commits to that organization's independent repository.
2. **Root Level**: The `data/` repository tracks the state of all organizations by staging and committing the organization directory changes. This allows for global backups and state-in-time recovery across the entire system while maintaining tenant isolation.

## Embedded Build Process

mddb is distributed as a single self-contained binary. This is achieved using Go's `embed` package.

### Workflow
1. **Frontend Build**: The SolidJS frontend is compiled into static assets in `frontend/dist/`.
2. **Go Generate**: The `frontend/frontend.go` file uses `//go:embed dist/*` to include these assets.
3. **Compilation**: The Go compiler includes the static assets in the final binary.

### Type Generation
To maintain type safety between the Go backend and SolidJS frontend, mddb uses **tygo**.
1. **Source**: Go structs in `internal/models/`, `internal/storage/`, and `internal/server/handlers/`.
2. **Process**: `tygo` parses these Go files and generates TypeScript interfaces.
3. **Artifact**: A unified `frontend/src/types.ts` file used by the frontend.
4. **Execution**: Triggered via `make types` or automatically during `make build`.

### Reproducibility
The `frontend/dist/` directory is tracked in Git to ensure that the Go binary can be built deterministically without requiring a Node.js environment in the build pipeline (though `make build` will attempt to rebuild it if tools are available).

## Performance Optimizations

### Streaming Reads
Table records are stored in JSONL format, allowing for line-by-line streaming using `bufio.Scanner`. This keeps memory usage low even for large tables.

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

## Data Model

mddb uses a unified Node-based data model inspired by Notion. All content entities share a common abstraction while supporting different content types.

### Core Entities

| Entity | Description | Storage |
|--------|-------------|---------|
| **Node** | Unified container; can be document, table, or hybrid | Directory at `data/{orgID}/pages/{nodeID}/` |
| **Page** | Node with markdown content | `index.md` with YAML front matter |
| **Table** | Node with schema (Properties) | `metadata.json` for schema |
| **Record** | Row in a Table | Line in `data.jsonl` |
| **Asset** | Binary file attached to a Node | File in node directory |

### Node Types

Defined in `backend/internal/storage/content/types.go`:

- **`document`**: Markdown content only (`Content` field populated)
- **`table`**: Structured data only (`Properties` schema + `DataRecords`)
- **`hybrid`**: Both markdown content and structured data

### Storage Layout

```
data/{orgID}/pages/{nodeID}/
├── index.md          # Markdown content (document/hybrid)
├── metadata.json     # Table schema (table/hybrid)
├── data.jsonl        # Records, one JSON per line (table/hybrid)
├── image.png         # Asset
└── document.pdf      # Asset
```

### Property Types

Table columns support these types:

| Type | Description |
|------|-------------|
| `text` | Plain text |
| `number` | Integer or float |
| `checkbox` | Boolean |
| `date` | ISO8601 date string |
| `select` | Single selection from options |
| `multi_select` | Multiple selections from options |
| `url` | Validated URL |
| `email` | Validated email address |
| `phone` | Phone number |

### Entity Relationships

```
Organization
└── Node (document | table | hybrid)
    ├── Content (markdown, if document/hybrid)
    ├── Properties[] (schema, if table/hybrid)
    ├── Children[] (nested Nodes)
    └── Assets[] (attached files)

Table Node
└── DataRecords[] (stored separately in data.jsonl)
    └── Data: map[string]any (field values keyed by property name)
```

### Key Design Decisions

1. **Polymorphic Nodes**: Pages and Tables share the same API; `Type` field discriminates behavior
2. **Separate Record Storage**: Records stored in JSONL for streaming reads, not embedded in Node
3. **Filename-based Asset IDs**: Assets use original filename as ID (not generated)
4. **Hierarchical Structure**: Nodes support parent-child relationships via `ParentID`

## Multi-user Architecture

### Identity & Authentication

JWT-based authentication with 24-hour token expiry using HS256 signing.

- **Token flow**: Login/register → JWT issued → Bearer token in `Authorization` header
- **Storage**: User credentials and organization memberships in `data/db/` as JSON files
- **Implementation**: `backend/internal/server/handlers/auth.go`

**Future**: Migration of `data/db/` to SQLite for query performance.

### OAuth2 Integration

Supports Google and Microsoft OIDC flows.

| Provider | Endpoint | Scopes |
|----------|----------|--------|
| Google | `/api/auth/oauth/google` | profile, email |
| Microsoft | `/api/auth/oauth/microsoft` | Azure AD common |

- **Callback handling**: `/api/auth/oauth/{provider}/callback`
- **Account linking**: Auto-links OAuth identity to existing user by email
- **CSRF protection**: State token validation
- **Implementation**: `backend/internal/server/handlers/oauth.go`

### Workspace Isolation

Each organization is a separate tenant with isolated data.

- **Storage**: `data/{orgID}/pages/` per organization
- **Git**: Independent Git repository per organization
- **Validation**: All org-scoped endpoints validate membership before access

### RBAC Model

Role-based access control at organization level:

| Role | Permissions |
|------|-------------|
| `viewer` | Read pages, tables, records, assets |
| `editor` | Create/modify content, no user management |
| `admin` | Full org access including user/settings management |
| `globalAdmin` | Server-wide access (first user auto-assigned) |

- **Implementation**: `backend/internal/server/handlers/middleware.go`, `handler_wrapper.go`
- **Enforcement**: `WrapAuth()` middleware validates role before handler execution

### Quota Tracking

Enforced at write time via `FileStore` pre-checks.

| Quota | Default | Scope |
|-------|---------|-------|
| `MaxPages` | 1000 | per org |
| `MaxStorage` | 1 GiB | per org |
| `MaxUsers` | 3 | per org |
| `MaxRecordsPerTable` | 10,000 | per table |
| `MaxAssetSize` | 50 MiB | per asset |
| `MaxOrgs` | 3 | per user |

- **Implementation**: `backend/internal/storage/content/filestore.go`

## High-Efficiency Caching (Planned)

### In-Memory Cache
A thread-safe, in-memory cache will be implemented to store:
1. **Metadata**: Table schemas and organization configurations.
2. **Hot Pages**: Frequently accessed markdown content.
3. **Record Indexes**: In-memory maps of record IDs to file positions or small record sets.

### Strategy
- **LRU Policy**: Least Recently Used eviction to maintain a fixed memory footprint.
- **Write-Through/Invalidation**: Cache will be updated or invalidated on every write operation to ensure consistency with the on-disk storage.
- **Lazy Loading**: Data will be loaded into the cache on the first read request.
