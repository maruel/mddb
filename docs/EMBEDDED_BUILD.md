# Embedded Build & Distribution

## Overview

mddb uses Go's `go:embed` to include the compiled SolidJS frontend directly in the binary. This creates a single, self-contained executable with no external dependencies.

## Quick Start

```bash
# Build everything (frontend + backend)
make build-all

# Run the single binary
./mddb
```

## How It Works

### 1. Frontend Build (Deterministic)
The Vite config in `frontend/vite.config.ts` ensures deterministic builds (consistent hashing, no source maps).
The build output goes to `frontend/dist/`.

### 2. Go Embedding
In `internal/server/embedded.go`:
```go
//go:embed ../../frontend/dist/*
var WebFS embed.FS
```
This embeds the `frontend/dist/` directory into the binary at compile time.

**Note on Paths:** The path `../../frontend/dist/*` is relative to `internal/server/embedded.go`.
- `../` to `internal/`
- `../` to project root
- `frontend/dist/*` to the target directory

### 3. Runtime Serving
The `EmbeddedSPAHandler` serves files from the embedded filesystem, falling back to `index.html` for SPA routing.

## Development vs. Distribution

| Feature | Development (`make frontend-dev`) | Distribution (`make build-all`) |
|---------|-----------------------------------|---------------------------------|
| **Frontend Source** | Served by Vite dev server | Embedded in Go binary |
| **Hot Reload** | Yes | No |
| **API Proxy** | Proxies to backend (:8080) | Direct internal calls |
| **Assets** | Served from `frontend/src` | Served from memory |

## Git Tracking

The `frontend/dist/` directory is **tracked in git**.
- **Benefit**: reproducible builds without needing a frontend build environment.
- **Workflow**: When frontend code changes, run `make frontend-build` and commit the updated `frontend/dist/` folder.

## Troubleshooting

- **"frontend/dist doesn't exist"**: Run `make frontend-build`.
- **"Binary is too large"**: Normal (~15-25 MB) as it includes the Go runtime and frontend assets.
- **"Changes not reflected"**: Run `make clean` then `make build-all` to force a rebuild.