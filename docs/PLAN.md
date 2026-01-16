# mddb Implementation Plan

## Overview

mddb is a Notion-like collaborative document and database system where all data is persisted as markdown files. The frontend (SolidJS) provides a rich user experience while the backend (Go) handles file operations, API endpoints, and business logic.

## Requirements

### Functional Requirements

1. **Document Management**
   - Create, read, update, delete pages
   - Hierarchical page organization (folders/nested structure)
   - Full markdown editing with preview
   - Rich text formatting support
   - Link between pages

2. **Database/Tables**
   - Define database schemas (columns with types)
   - Store database records as markdown
   - Query and filter records
   - Sort and pagination support
   - Import/export data

3. **Media Management**
   - Upload and store images
   - Reference images in documents
   - Asset gallery view
   - Support common formats (PNG, JPG, GIF, WebP)

4. **User Experience**
   - Real-time document editing
   - Auto-save functionality
   - Search across pages and databases
   - Full-text search capability
   - Undo/redo support

5. **API & Integration**
   - RESTful API for all operations
   - Clean error handling and validation
   - Rate limiting (optional)

### Non-Functional Requirements

- Fast startup and load times
- Scalable to thousands of pages/records
- Single-user or small team use case initially
- File-system based (no external database required)
- Cross-platform (Linux, macOS, Windows)

## Design Fundamentals

### Data Model

```
Project Root/
└── pages/
    ├── index.md              # Pages end with .md
    ├── getting-started.md
    ├── tasks.db.md           # Databases end with .db.md
    ├── contacts.db.md
    ├── image-1.png           # Assets are anything else
    ├── image-2.jpg
    └── subfolder/
        ├── nested-page.md
        ├── notes.db.md
        └── diagram.svg
```

### Storage Format

**Pages (Markdown)**
```markdown
---
id: uuid
title: Page Title
created: 2024-01-15
modified: 2024-01-15
tags: [tag1, tag2]
---

Page content in markdown...

[Link to another page](./other-page)
![Image](../assets/images/image.png)
```

**Databases (Markdown with .db.md extension)**
```markdown
---
id: uuid
title: Tasks
columns:
  - name: title
    type: text
  - name: status
    type: select
    options: [todo, in-progress, done]
  - name: created
    type: date
---

## Records

### Record 1
- title: Task A
- status: in-progress
- created: 2024-01-15

### Record 2
- title: Task B
- status: todo
- created: 2024-01-14
```

### API Architecture

**Core Endpoints**

```
Pages
  GET    /api/pages              - List all pages
  GET    /api/pages/:id          - Get page content
  POST   /api/pages              - Create page
  PUT    /api/pages/:id          - Update page
  DELETE /api/pages/:id          - Delete page
  GET    /api/pages/search       - Search pages

Databases
  GET    /api/databases          - List databases
  GET    /api/databases/:id      - Get database schema
  POST   /api/databases          - Create database
  PUT    /api/databases/:id      - Update schema
  DELETE /api/databases/:id      - Delete database

Records
  GET    /api/databases/:id/records         - List records
  POST   /api/databases/:id/records         - Create record
  PUT    /api/databases/:id/records/:rid    - Update record
  DELETE /api/databases/:id/records/:rid    - Delete record

Assets
  GET    /api/assets             - List assets
  POST   /api/assets             - Upload asset
  GET    /assets/:id             - Serve asset (redirect to file)
  DELETE /api/assets/:id         - Delete asset
```

### Frontend Architecture (SolidJS)

**Core Components**
- `Layout` - Main app shell (sidebar, editor area, preview)
- `PageEditor` - Markdown editor with preview
- `DatabaseTable` - Table view for database records
- `PageBrowser` - Navigation tree of pages
- `AssetGallery` - Image/asset browser
- `SearchPanel` - Full-text search

**State Management**
- Use SolidJS Context API or stores for global state
- Document/database content state
- Editor state (current page, unsaved changes)
- UI state (sidebar open/closed, view mode)

**Key Features**
- Real-time markdown preview
- Debounced auto-save to backend
- Drag-and-drop file uploads
- Page linking with autocomplete
- Database table inline editing

### Backend Architecture (Go)

**Project Structure**
```
cmd/
  mddb/
    main.go           - Application entry point
internal/
  server/
    router.go         - Route definitions
    handlers/
      pages.go        - Page endpoints
      databases.go    - Database endpoints
      records.go      - Record endpoints
      assets.go       - Asset endpoints
  storage/
    filestore.go      - File system operations
    page_service.go   - Page business logic
    database_service.go - Database operations
  models/
    page.go
    database.go
    record.go
    asset.go
  utils/
    markdown.go       - Markdown parsing/utilities
    uuid.go           - ID generation
    response.go       - API response formatting
```

**Key Services**
- `FileStore` - Abstract file operations (read/write markdown, images)
- `PageService` - Page CRUD and management
- `DatabaseService` - Database schema and operations
- `AssetService` - Image upload/management
- `SearchService` - Full-text search implementation

## Implementation Phases

### Phase 1: Core Foundation
- [x] Go server setup with basic routing
- [x] HTTP handler wrapper with automatic path parameter extraction
- [x] Structured logging with context support
- [x] Error handling with ErrorWithStatus interface
- [x] Request/Response typing for all API endpoints
- [x] File system abstraction layer (FileStore with CRUD operations)
- [x] Basic page CRUD operations (PageService with full implementation)
- [x] Tests for FileStore operations (unit tests passing)
- [x] Basic SolidJS frontend structure (App.tsx with page management UI)
- [x] Static file serving (SPA handler with fallback to index.html - ready for built frontend)

### Phase 1.5: Code Quality & Linters
- [x] Go linters (golangci-lint with strict rules)
- [x] Frontend linters (ESLint + Prettier)
- [x] Pre-commit hooks to enforce linting on commits
- [x] All code passes linting (0 errors, 0 warnings)
- [x] Makefile targets: `make lint`, `make lint-go`, `make lint-frontend`, `make lint-fix`

### Phase 2: Page Editor
- [ ] Markdown editor component (SolidJS)
- [ ] Live preview functionality
- [ ] Auto-save mechanism
- [ ] Page linking with autocomplete
- [ ] Page creation/deletion UI

### Phase 3: Databases
- [ ] Database schema definition
- [ ] Record storage format
- [ ] Table UI component
- [ ] CRUD operations for records
- [ ] Filtering and sorting

### Phase 4: Assets & Media
- [ ] File upload endpoint
- [ ] Image storage and serving
- [ ] Asset gallery UI
- [ ] Image embedding in pages
- [ ] Asset cleanup/management

### Phase 5: Polish & Features
- [ ] Full-text search
- [ ] Page history/versioning (optional)
- [ ] Better error handling
- [ ] Performance optimization
- [ ] Documentation

## Technical Decisions

### Why Markdown?
- Human-readable, version-control friendly
- No schema migration complexity
- Easy backups and exports
- Portable and future-proof
- Front matter for metadata

### Why File System Storage?
- Zero external dependencies
- Perfect for single/small team use
- Easy to backup and version control
- Simple deployment
- No database licensing/setup

### Why SolidJS?
- Reactive primitives match the UI's needs
- Small bundle size
- Excellent TypeScript support
- Fine-grained reactivity
- Good for real-time editing UX

### Why Go?
- Fast server startup
- Efficient file I/O
- Simple deployment (single binary)
- Strong standard library
- Good concurrent request handling

### Handler Design Pattern (Wrap)
- All HTTP handlers wrapped with `Wrap()` function for consistent behavior
- Handler signature: `func(context.Context, In) (*Out, error)`
- Automatic path parameter extraction via struct tags: `path:"paramName"`
- Request/Response types for all endpoints ensure type safety
- Errors implementing `ErrorWithStatus` automatically get correct HTTP status codes
- Context passed through entire handler chain for logging and cancellation

## Dependencies

### Go
- Standard library only initially
- Consider: `chi` (routing), `cors` (CORS handling)
- Testing: `testify` for assertions

### Frontend (SolidJS)
- `solid-js` - Core framework
- `@solidjs/router` - Client-side routing
- `solid-markdown` - Markdown rendering
- `prismjs` - Code highlighting
- Build tool: Vite

## Deployment Model

- Single executable binary (Go)
- Serves static SolidJS frontend
- File system for persistence
- No external services required
- Suitable for Docker deployment

## Security Considerations

- Input validation on all endpoints
- Sanitize markdown before rendering
- File path traversal protection
- Rate limiting on file operations
- CORS configuration for web access
