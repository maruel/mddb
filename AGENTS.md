# AGENTS.md

Development guidelines and commands for mddb project.

For specific guidelines, see:
- [Backend Guidelines](backend/AGENTS.md)
- [Frontend Guidelines](frontend/AGENTS.md)

## Project Overview

mddb is a Notion-like document and database system. Frontend built with SolidJS, backend with Go. Data stored
as markdown files and images.

**Storage Model:**
- All content lives in the `data/pages/` directory as numbered directories (1, 2, 3, etc.)
- Each directory is a "page" containing:
  - `index.md` - Content with YAML front matter for metadata (title, tags, etc.)
  - `favicon.ico` (optional) - Icon for the page (can be .ico, .avif, or .png)
  - `metadata.json` (databases only) - Database schema and column definitions
  - `data.jsonl` (databases only) - Database records, one per line
  - Other assets (images, files, etc.) - Images, attachments, etc.
- Each page/database is a directory with its own namespace for contained assets
- Hierarchical organization via nested directories (e.g., `1/subfolder/2/index.md`)

See README.md for project overview and docs/PLAN.md for implementation roadmap.

## Directory Structure

```
mddb/
├── backend/                    # Go backend code
│   ├── cmd/mddb/               # Application entry point
│   │   └── main.go             # Server startup and configuration
│   ├── internal/
│   │   ├── models/             # Data models
│   │   ├── server/             # HTTP server and routing
│   │   │   ├── router.go       # Route definitions and SPA handler
│   │   │   ├── embedded.go     # Embedded frontend handler
│   │   │   ├── handler_wrapper.go # Generic handler wrapper with path param extraction
│   │   │   └── handlers/       # HTTP request handlers by feature
│   │   ├── storage/            # File system operations
│   │   └── utils/              # Utilities
│   └── frontend/               # Go embedding for frontend assets
├── frontend/                   # SolidJS frontend
│   ├── src/                    # Frontend source code
│   │   ├── index.tsx           # App entry point
│   │   ├── App.tsx             # Main app component
│   │   └── components/         # UI components
│   ├── dist/                   # Compiled frontend
│   ├── vite.config.ts          # Vite build configuration
│   └── package.json            # Frontend dependencies
├── docs/                       # Project Documentation like PLAN.md and REQUIREMENTS.md
├── Makefile                    # Common development commands
├── AGENTS.md                   # This file - Development guidelines
└── README.md                   # Project overview and API documentation
```

## Git Workflow

### Commit Messages

- Format: `{type}: {description}`
- Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

### Documentation Changes

- **Do NOT** create a markdown file listing changes you made.
- Update `docs/PLAN.md` to reflect status changes.
- Keep `docs/README.md` up to date.

### Pre-Commit Hooks

Install with `make git-hooks`. Runs linters and tests before every commit.

## Useful Resources

- [mddb PLAN.md](docs/PLAN.md)
- [mddb README.md](README.md)