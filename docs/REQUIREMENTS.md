# mddb Requirements

## Functional Requirements

### 1. Document Management
- Create, read, update, delete pages
- Hierarchical page organization (folders/nested structure)
- Full markdown editing with preview
- Rich text formatting support
- Link between pages

### 2. Database/Tables
- Define database schemas (columns with types)
- Store database records
- Query and filter records
- Sort and pagination support
- Import/export data

### 3. Media Management
- Upload and store images
- Reference images in documents
- Asset gallery view
- Support common formats (PNG, JPG, GIF, WebP)

### 4. User Experience
- Real-time document editing
- Auto-save functionality
- Search across pages and databases
- Full-text search capability
- Undo/redo support

### 5. API & Integration
- RESTful API for all operations
- Clean error handling and validation
- Rate limiting (optional)

## Non-Functional Requirements

### Performance & Scalability
- Fast startup and load times
- Scalable to thousands of pages/records
- Efficient file I/O operations

### Deployment & Architecture
- Single-user or small team use case initially
- File-system based (no external database required)
- Cross-platform (Linux, macOS, Windows)
- Single executable binary (zero external dependencies for core)

### Data & Security
- Version control friendly (Git-compatible)
- Input validation on all endpoints
- Sanitize markdown before rendering
- File path traversal protection
- No account or internet connection required

### Storage Model
- All content stored as markdown files and JSON
- Directory-based organization with numeric IDs
- Each page is a directory with contained assets
- Human-readable file formats
- Portable and future-proof
