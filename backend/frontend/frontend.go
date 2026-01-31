//go:generate python3 ../../scripts/build_frontend.py

// Package frontend embeds the compiled SolidJS web UI assets.
//
// The dist directory is populated at build time via go:generate, enabling
// single-binary deployment without external file dependencies.
package frontend

import "embed"

// Files contains the embedded web frontend.
//
//go:embed dist/*
var Files embed.FS
