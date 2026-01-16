# Web Renamed to Frontend

## Changes Made

Renamed the `web/` directory to `frontend/` for cleaner architecture and better naming semantics.

### Directory Structure

**Before:**
```
web/
├── src/
├── dist/
├── package.json
└── vite.config.ts
```

**After:**
```
frontend/
├── src/
├── dist/
├── package.json
├── vite.config.ts
├── frontend.go        (NEW)
└── README.md
```

### New Frontend Package

Created `frontend/frontend.go` as a dedicated Go package:

```go
package frontend

import "embed"

// Files contains the embedded web frontend.
//go:embed dist/*
var Files embed.FS
```

**Benefits:**
- Cleaner architecture: `frontend.Files` is more semantic than `server.WebFS`
- Clear separation: Frontend concerns in `frontend/` package
- Simpler import: `import "github.com/maruel/mddb/frontend"`

### Updated Imports

**router.go (internal/server/router.go):**

```go
import (
    "github.com/maruel/mddb/frontend"
)

// Use it:
mux.Handle("/", NewEmbeddedSPAHandler(frontend.Files))
```

### Updated Build Configuration

**Makefile:**
```makefile
frontend-install:
	cd frontend && pnpm install

frontend-dev: frontend-install
	cd frontend && pnpm dev

frontend-build: frontend-install
	cd frontend && pnpm build
```

### Updated Git Ignore

**.gitignore:**
```
/frontend/node_modules/
/frontend/.env.local
/frontend/.env.*.local
/frontend/.npmrc
```

Note: `frontend/dist/` is NOT ignored - compiled frontend is tracked in git.

### File Mapping

All files from `web/` were copied to `frontend/`:

| Old Path | New Path |
|----------|----------|
| `web/src/` | `frontend/src/` |
| `web/dist/` | `frontend/dist/` |
| `web/package.json` | `frontend/package.json` |
| `web/vite.config.ts` | `frontend/vite.config.ts` |
| `web/tsconfig.json` | `frontend/tsconfig.json` |
| `web/tsconfig.node.json` | `frontend/tsconfig.node.json` |
| `web/.eslintrc.cjs` | `frontend/.eslintrc.cjs` |
| `web/.prettierrc` | `frontend/.prettierrc` |
| `web/.prettierignore` | `frontend/.prettierignore` |
| `web/index.html` | `frontend/index.html` |
| (new) | `frontend/frontend.go` |
| (new) | `frontend/README.md` |

### Old Files

The `web/` directory still exists for reference but is no longer used. All commands now reference `frontend/`.

To clean up:
```bash
rm -rf web/
```

(But keep for now in case we need to revert)

### Build Commands

Development:
```bash
make frontend-dev
# Frontend at http://localhost:5173 (proxies to :8080)
```

Production:
```bash
make build-all
# Builds frontend to frontend/dist/
# Embeds in Go binary via frontend.Files
```

### Summary

✓ Cleaner architecture with dedicated `frontend/` package  
✓ All frontend code in one directory  
✓ Simpler imports: `frontend.Files`  
✓ All build automation updated  
✓ Git configuration updated  
✓ Zero functional changes  

The application works identically - this is purely a structural improvement.
