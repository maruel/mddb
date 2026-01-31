# mddb Frontend Architecture and Setup

SolidJS-based frontend for mddb - a markdown document and table system.

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

## Icons

We use Material Design icon and symbols from https://fonts.google.com/icons

