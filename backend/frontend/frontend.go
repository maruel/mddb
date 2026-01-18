//go:generate sh -c "cd ../../frontend && pnpm install && pnpm build && rm -rf ../backend/frontend/dist && cp -r dist ../backend/frontend/dist"

// Package frontend provides the embedded SolidJS frontend assets.
package frontend

import "embed"

// Files contains the embedded web frontend.
//
//go:embed dist/*
var Files embed.FS
