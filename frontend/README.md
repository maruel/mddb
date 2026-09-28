# mddb Frontend Architecture and Setup

SolidJS-based frontend for mddb - a markdown document and table system.

Run these commands from the repository root; `frontend/` has no separate
`package.json`.

## Development

```bash
pnpm install
pnpm dev
```

The frontend runs on http://localhost:5173 and proxies API calls to
http://localhost:8080.

## Build

```bash
pnpm build
```

`pnpm build` writes assets to `backend/frontend/dist/`. Run `make build` to
precompress those assets and embed them in the Go binary.

## Icons

We use Material Design icon and symbols from https://fonts.google.com/icons
