// Package storage provides data persistence for mddb workspaces.
package storage

// AgentsMD is the content for AGENTS.md placed at the root of each workspace.
// It documents the jsonldb data format for AI agents and developers.
const AgentsMD = `# Workspace Data Format

## JSONL Tables

Each ` + "`" + `.jsonl` + "`" + ` file is a table:
- Line 1: Schema header (JSON object with ` + "`" + `version` + "`" + ` and ` + "`" + `columns` + "`" + ` array)
- Lines 2+: Data rows (one JSON object per line)

Example ` + "`" + `users.jsonl` + "`" + `:
` + "```" + `
{"version":"1","columns":[{"name":"id","type":"id"},{"name":"name","type":"string"}]}
{"id":"01JWAB","name":"Alice"}
{"id":"01JWAC","name":"Bob"}
` + "```" + `

## Blob Storage

Binary data is stored in a sibling ` + "`" + `.blobs/` + "`" + ` directory:
- ` + "`" + `users.jsonl` + "`" + ` → ` + "`" + `users.blobs/` + "`" + `

Blob references in JSON: ` + "`" + `sha256:<BASE32HEX>-<size>` + "`" + `
- 52 uppercase base32 hex characters (0-9, A-V) encoding SHA-256 hash
- Decimal byte size after the dash

File path: ` + "`" + `<table>.blobs/<char0-1>/<char2-51>-<size>` + "`" + `

Example:
- Reference: ` + "`" + `sha256:SEOC8GKOVGE196NRUJ49IRTP4GJQSGF4CIDP6J54IMCHMU2IN1AG-1024` + "`" + `
- File path: ` + "`" + `users.blobs/SE/OC8GKOVGE196NRUJ49IRTP4GJQSGF4CIDP6J54IMCHMU2IN1AG-1024` + "`" + `

## File Layout

` + "```" + `
workspace/
├── AGENTS.md
├── users.jsonl
├── users.blobs/
│   └── SE/
│       └── OC8GKOVGE196NRUJ49IRTP4GJQSGF4CIDP6J54IMCHMU2IN1AG-1024
└── documents.jsonl
` + "```" + `
`
