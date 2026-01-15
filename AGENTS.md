# AGENTS.md

Development guidelines and commands for mddb project.

## Project Overview

mddb is a Notion-like document and database system. Frontend built with SolidJS, backend with Go. Data stored
as markdown files and images.

**Storage Model:**
- All content lives in the `pages/` directory
- Pages: Files ending with `.md` (e.g., `getting-started.md`)
- Databases: Files ending with `.db.md` (e.g., `tasks.db.md`, `contacts.db.md`)
- Assets: Any file that doesn't end with `.md` (e.g., `diagram.png`, `chart.svg`)
- Everything can be organized in subdirectories for hierarchical organization

See README.md for project overview and PLAN.md for implementation roadmap.

## Directory Structure

TODO: add when files are created.

## Go Development

Use Go best practices.

## Frontend Development (SolidJS)

### Code Organization

- Components in `web/src/components/` (reusable, single-responsibility)
- Page layouts in `web/src/pages/`
- Global state in `web/src/stores/`
- Utilities in `web/src/utils/`

### Naming Conventions

- Components: PascalCase (e.g., `PageEditor.tsx`, `DatabaseTable.tsx`)
- Stores: camelCase (e.g., `pageStore.ts`, `editorStore.ts`)
- Props interfaces: `{ComponentName}Props`
- Event handlers: `handleXyz` pattern

### SolidJS Patterns

- Use `createSignal` for reactive state
- Use `createEffect` for side effects
- Use Context API for global state
- Prefer stores over props drilling
- Keep components as functions returning JSX

### Common Commands

Development server:
```bash
cd web
npm run dev
```

Build production:
```bash
cd web
npm run build
```

Type checking:
```bash
cd web
npm run typecheck
```

## API Development

### Endpoint Conventions

- RESTful: Use HTTP verbs (GET, POST, PUT, DELETE) correctly
- Response format: Always JSON
- Error responses: Include `error` field with message
- Success responses: Include `data` field with result
- Status codes: 200 OK, 201 Created, 400 Bad Request, 404 Not Found, 500 Server Error

### Error Response Format

```json
{
  "error": "Descriptive error message",
  "code": "ERROR_CODE"
}
```

### Success Response Format

```json
{
  "data": {
    "id": "...",
    "...": "..."
  }
}
```

## File Operations

### Markdown Handling

- Front matter (YAML) for metadata
- UTF-8 encoding always
- Normalize line endings (LF)

## Testing

### Go Tests

- Use table-driven tests for multiple cases

### Frontend Tests

- Use Vitest
- Filename: `{component}.test.tsx`
- Test component rendering and interactions

## Git Workflow

### Commit Messages

- Format: `{type}: {description}`
- Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`
- Keep commits focused and atomic
- Write descriptive messages

## Performance Considerations

### Go Backend

- Use file caching for frequently accessed pages
- Implement pagination for large result sets
- Optimize markdown parsing (consider pre-processing)
- Monitor file I/O operations

### Frontend

- Code splitting for routes
- Lazy load components
- Debounce auto-save operations
- Cache rendered markdown

## Documentation

### Code Comments

- Explain the "why", not the "what"
- Document complex algorithms
- Add examples for tricky functions
- Keep comments up-to-date

### API Documentation

- Document endpoint purpose and parameters
- Include request/response examples
- Document error cases
- Keep in-sync with implementation

## Deployment

### Build Process

1. Ensure tests pass: `go test ./...`
2. Build frontend: `cd web && npm run build`
3. Embed frontend in Go binary (if applicable)
4. Build Go binary: `go build -o mddb ./cmd/mddb`
5. Test binary with sample data

### Configuration

- Use environment variables for configuration
- Support config file (optional)
- Document all config options
- Provide sensible defaults

## Development Checklist

When implementing features:

- [ ] Update PLAN.md if requirements change
- [ ] Write tests first or alongside code
- [ ] Follow naming conventions
- [ ] Run formatters and linters
- [ ] Document complex logic
- [ ] Test error cases
- [ ] Update relevant README sections
- [ ] Commit with descriptive message

## Useful Resources

- [Go Effective Go](https://golang.org/doc/effective_go)
- [SolidJS Docs](https://docs.solidjs.com)
- [mddb PLAN.md](PLAN.md) - Implementation roadmap
- [mddb README.md](README.md) - Project overview
