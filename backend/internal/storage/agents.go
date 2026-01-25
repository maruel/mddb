// Package storage provides data persistence for mddb workspaces.
package storage

import _ "embed"

// AgentsMD is the content for AGENTS.md placed at the root of each workspace.
// It documents the jsonldb data format for AI agents and developers.
//
//go:embed AGENTS.md
var AgentsMD string
