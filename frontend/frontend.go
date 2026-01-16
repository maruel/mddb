//go:generate sh -c "pnpm install && pnpm build"

// Package frontend provides the embedded SolidJS frontend assets.
package frontend

import "embed"

// Files contains the embedded web frontend.
//
//go:embed dist/*
var Files embed.FS
