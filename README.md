# mddb - Markdown Document & Database System

A Notion-like document and database system where everything is stored as markdown files. Built with Go backend and SolidJS frontend.

**Quick Links**: [Quick Start](docs/QUICKSTART.md) • [Demo](docs/DEMO.md) • [Implementation Plan](docs/PLAN.md) • [Progress](docs/PROGRESS.md)

## Architecture

- **Backend**: Go with standard library HTTP server
- **Frontend**: SolidJS with Vite
- **Storage**: File-system based markdown with YAML front matter
- **Data Model**: Pages (`.md`), Databases (`.db.md`), Assets (any other file)

## Project Structure

```
mddb/
├── cmd/mddb/                    # Application entry point
├── internal/
│   ├── server/                  # HTTP server and routing
│   │   ├── router.go
│   │   ├── handler_wrapper.go   # Generic handler wrapper
│   │   └── handlers/            # HTTP request handlers
│   ├── storage/                 # File system operations
│   │   ├── filestore.go         # Low-level file operations
│   │   └── page_service.go      # Page business logic
│   ├── models/                  # Data models
│   ├── errors/                  # Error types
│   └── utils/                   # Utilities
├── web/                         # SolidJS frontend
│   ├── src/                     # Frontend source
│   ├── public/                  # Static files
│   ├── index.html
│   ├── vite.config.ts
│   └── package.json
├── data/                        # Data directory (created at runtime)
│   └── pages/                   # All markdown content
└── PLAN.md                      # Implementation roadmap
```

## Getting Started

### Backend

Build and run the server:

```bash
go build -o mddb ./cmd/mddb
./mddb -data-dir ./data -port 8080
```

Available flags:
- `-port`: Server port (default: 8080)
- `-data-dir`: Data directory (default: ./data)
- `-log-level`: Log level - debug, info, warn, error (default: info)

### Frontend

Development:

```bash
cd web
pnpm install
pnpm dev
```

Build for production:

```bash
cd web
pnpm install
pnpm build
```

The build outputs to `web/dist/`.

**Note**: Uses pnpm for faster, more efficient package management. If you don't have pnpm installed: `npm install -g pnpm`

## API Endpoints

### Health Check

```
GET /api/health
```

### Pages

```
GET    /api/pages              # List all pages
GET    /api/pages/:id          # Get page by ID
POST   /api/pages              # Create new page
PUT    /api/pages/:id          # Update page
DELETE /api/pages/:id          # Delete page
```

**Create/Update Page Request:**
```json
{
  "title": "Page Title",
  "content": "Markdown content here..."
}
```

**Get Page Response:**
```json
{
  "id": "uuid-string",
  "title": "Page Title",
  "content": "Markdown content..."
}
```

### Databases (Planned)

```
GET    /api/databases          # List all databases
GET    /api/databases/:id      # Get database schema
POST   /api/databases          # Create database
PUT    /api/databases/:id      # Update database
DELETE /api/databases/:id      # Delete database
```

### Records (Planned)

```
GET    /api/databases/:id/records         # List records
POST   /api/databases/:id/records         # Create record
PUT    /api/databases/:id/records/:rid    # Update record
DELETE /api/databases/:id/records/:rid    # Delete record
```

### Assets (Planned)

```
GET    /api/assets             # List assets
POST   /api/assets             # Upload asset
GET    /assets/:id             # Serve asset
DELETE /api/assets/:id         # Delete asset
```

## Development

### Handler Pattern

All HTTP handlers follow this pattern:

```go
func (h *Handler) MyEndpoint(ctx context.Context, req MyRequest) (*MyResponse, error) {
    // Handler logic
    return &MyResponse{...}, nil
}
```

The `Wrap()` function automatically:
- Reads and parses JSON request bodies
- Extracts path parameters via struct tags: `path:"paramName"`
- Calls the handler function
- Handles errors and sets appropriate HTTP status codes
- Encodes the response as JSON

### Path Parameters

Mark request struct fields with `path:"paramName"` tags:

```go
type GetPageRequest struct {
    ID string `path:"id"`
}
```

### Error Handling

Return errors implementing `ErrorWithStatus`:

```go
return nil, errors.NewAPIError(404, "Page not found")
```

## Frontend Development

The frontend is built with SolidJS and uses:

- `createSignal` for reactive state
- `createEffect` for side effects
- Component-based architecture
- CSS modules for styling

### File Organization

- `src/App.tsx` - Main application component
- `src/App.module.css` - Application styles

### Development Server

The dev server proxies API requests to `http://localhost:8080`:

```
/api/* → http://localhost:8080/api/*
/assets/* → http://localhost:8080/assets/*
```

## Data Storage

All data is stored in the `pages/` directory as markdown files with YAML front matter:

```markdown
---
id: uuid-string
title: Page Title
created: 2024-01-15T10:30:00Z
modified: 2024-01-15T10:30:00Z
tags: [tag1, tag2]
---

# Page content in markdown
```

### File Types

- **Pages**: Files ending with `.md` - Regular markdown documents
- **Databases**: Files ending with `.db.md` - Database records in markdown format
- **Assets**: Any file not ending with `.md` - Images, attachments, etc.

## Implementation Phases

See [PLAN.md](docs/PLAN.md) for detailed implementation roadmap.

### Phase 1: Core Foundation (In Progress)

- [x] Go server setup with routing
- [x] Handler wrapper with automatic path parameter extraction
- [x] Error handling with ErrorWithStatus interface
- [x] Request/Response typing for all endpoints
- [x] FileStore abstraction layer
- [x] Basic page CRUD operations
- [ ] Static file serving for SolidJS frontend
- [ ] Basic SolidJS frontend structure

### Phase 2: Page Editor

- [ ] Markdown editor component
- [ ] Live preview functionality
- [ ] Auto-save mechanism
- [ ] Page linking with autocomplete

### Phase 3: Databases

- [ ] Database schema definition
- [ ] Record storage format
- [ ] Table UI component
- [ ] CRUD operations for records

### Phase 4: Assets & Media

- [ ] File upload endpoint
- [ ] Image storage and serving
- [ ] Asset gallery UI

### Phase 5: Polish & Features

- [ ] Full-text search
- [ ] Performance optimization
- [ ] Documentation

## Testing

### Run Backend Tests

```bash
go test ./...
```

### Run Frontend Tests

```bash
cd web
npm test
```

## Deployment

### Build and Release

1. Build frontend:
   ```bash
   cd web && npm run build && cd ..
   ```

2. Ensure frontend is copied to `web/public/dist/` if embedding

3. Build binary:
   ```bash
   go build -o mddb ./cmd/mddb
   ```

4. Run with:
   ```bash
   ./mddb -data-dir ./data -port 8080
   ```

## License

See LICENSE file
