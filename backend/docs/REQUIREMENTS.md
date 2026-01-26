# Backend Requirements

## Functional Requirements

### 1. Document Management (Backend)
- [x] **CRUD**: API and storage logic for creating, reading, updating, deleting pages.
- [x] **Organization**: Hierarchical storage via nested numeric directories in the filesystem.
- [x] **Assets**: Backend handling of multipart file uploads and storage in page-specific namespaces.
- [ ] **Backlinks**: Logic to parse markdown and track inter-page references.

### 2. Tables & JSONLDB
- [x] **Schema Management**: Logic to define and store table schemas.
- [x] **Record Storage**: High-performance JSONL storage engine with concurrent-safe access.
- [x] **Pagination**: Server-side support for `offset` and `limit`.
- [x] **Type Coercion**: SQLite-compatible type affinity system implemented in Go.
- [/] **Advanced Query**: Server-side filtering and sorting logic.
- [ ] **Relations & Rollups**: Backend logic for cross-table record linking and aggregation.

### 3. Media Management
- [x] **Storage**: Content-addressed or page-scoped asset storage.
- [x] **Serving**: Efficient file serving with proper MIME detection and cache headers.

### 4. API & Integration
- [x] **REST API**: Implementation of all functional endpoints.
- [x] **Error Handling**: Centralized `ErrorCode` system with HTTP status mapping.
- [x] **Type Generation**: `tygo` integration for synchronizing models with frontend.

### 5. Multi-User & Security
- [x] **Identity & Auth**: JWT and Session management logic.
- [x] **OAuth2**: Integration with Google and Microsoft OIDC providers.
- [x] **RBAC**: Server-side enforcement of Admin, Editor, and Viewer roles.
- [x] **Git Integration**: Backend logic for managing per-organization Git remotes and automated syncing.

### 6. Data Interoperability
- [ ] **Importers**: Logic to ingest Notion (zip) and Obsidian (vault) exports, mapping metadata to mddb structure.
- [ ] **Portable Export**: Automated generation of standard Markdown + CSV + Assets packages for zero lock-in.

### 7. Advanced Search & Intelligence
- [ ] **Semantic Indexing**: Vector-based indexing of nodes to support natural language queries and "related content" discovery.

### 8. Storage Resilience
- [ ] **Conflict Resolution**: Backend engine to detect and expose Git merge conflicts via the API for user reconciliation.
- [ ] **Snapshotting**: Logic to create named workspace versions using Git tags.

## Non-Functional Requirements

### Performance & Scalability
- [x] **Streaming**: Line-by-line JSONL processing.
- [x] **Caching**: In-memory caching for hot metadata and records.
- [x] **Concurrency**: Thread-safe storage access using RWMutex.

### Architecture
- [x] **Self-Contained**: Go `embed` for frontend assets.
- [x] **Local-First**: Filesystem-based persistence with no external DB dependency.
- [x] **ID Generation**: LUCI-inspired sortable 64-bit IDs.
