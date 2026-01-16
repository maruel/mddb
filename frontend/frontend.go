package frontend

import "embed"

// Files contains the embedded web frontend.
//
//go:embed dist/*
var Files embed.FS
