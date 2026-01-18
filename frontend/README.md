# mddb Frontend

SolidJS-based frontend for mddb - a markdown document and database system.

## Development

```bash
pnpm install
pnpm dev
```

Frontend runs on http://localhost:5173 and proxies API calls to http://localhost:8080

## Build

```bash
pnpm build
```

Builds optimized frontend to `../backend/frontend/dist/` which is embedded in the Go binary via `go:embed`.

## Files

- `src/` - TypeScript/SolidJS source code
- `vite.config.ts` - Vite build configuration (deterministic builds)
- `tsconfig.json` - TypeScript configuration
