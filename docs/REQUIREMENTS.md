# mddb Requirements

## Status

**Project State: Production Prototype**
Most core functional requirements for a local-first markdown and database system are met.

## Functional Requirements

### 1. Document Management
- [x] **CRUD**: Create, read, update, delete pages.
- [x] **Organization**: Hierarchical page organization (directory-based numeric IDs).
- [x] **Editor**: Full markdown editing with live preview.
- [x] **Assets**: Attach images and files directly to pages.
- [ ] **Interlinking**: Specialized UI for linking between pages (currently manual markdown links).

### 2. Database/Tables
- [x] **Schema**: Define database schemas (columns with types: text, number, select, multi_select, checkbox, date).
- [x] **Records**: Store and manage database records in JSONL format.
- [x] **Pagination**: Support for `offset` and `limit` to handle large datasets.
- [ ] **Advanced Query**: Complex filtering and multi-column sorting.

### 3. Media Management
- [x] **Storage**: Upload and store assets in page-specific namespaces.
- [x] **Serving**: Serve assets with proper MIME detection and caching.
- [x] **Formats**: Support common formats (PNG, JPG, GIF, WebP, SVG, PDF, etc.).
- [ ] **Gallery**: Dedicated asset management and gallery view.

### 4. User Experience
- [x] **Auto-save**: Automatic background saving of documents (2s debounce).
- [x] **Search**: Full-text search across all documents and databases with relevance scoring.
- [x] **History**: View and restore previous versions of pages (leveraging Git).
- [ ] **Real-time**: WebSocket-based real-time sync (future consideration).
### 5. API & Integration
- [x] **REST API**: Comprehensive API for all operations (Pages, DBs, Records, Assets).
- [x] **Error Handling**: Structured error codes and detailed responses.

### 6. Multi-User & Organization
- [ ] **Identity**: User accounts and authentication (JWT/Sessions).
- [ ] **OAuth2**: Login via Google or Microsoft accounts.
- [ ] **Organizations**: Groups of users with shared workspace access.
- [ ] **RBAC**: Role-Based Access Control (Admin, Editor, Viewer) at organization and page levels.
- [ ] **Quotas**: Resource limits per organization (storage space, number of pages, max asset size).

## Non-Functional Requirements

### Performance & Scalability
- [x] **Streaming**: Efficient line-by-line reading of records via `bufio.Scanner`.
- [x] **Scalability**: Designed to handle thousands of pages and large databases via pagination.
- [x] **Lightweight**: Fast startup and low memory footprint.
- [ ] **Caching**: Internal high-efficiency caching for frequently accessed pages, metadata, and database records to minimize disk I/O.

### Deployment & Architecture
- [x] **Self-Contained**: Single executable binary with embedded frontend (`go:embed`).
- [x] **Local-First**: Filesystem-based storage with no external database dependencies.
- [x] **Cross-Platform**: Compatible with Linux, macOS, and Windows.

### Data & Security
- [x] **Versioning**: Automatic Git commits in the `data/` directory for every change.
- [x] **Audit Trail**: Built-in history via standard Git commits.
- [x] **Path Safety**: Protection against file path traversal.
- [ ] **Sanitization**: Improved markdown sanitization (currently permits HTML).

### Storage Model
- [x] **Numeric IDs**: Directory-based organization (1, 2, 3...) for stable paths.
- [x] **Human Readable**: Transparent storage (Markdown, JSON, JSONL).
- [x] **Portable**: Zero-config "copy and paste" portability.