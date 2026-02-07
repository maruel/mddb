# Workspace Data Format

## Tenets

- This git repository represents a workspace.
- Each directory in this workspace is a node.
- A page is a index.md file.
- A table is a data.jsonl file, which contains columns and rows. It's a JSONL table.
- Each node must contain either a page (index.md), a table (data.jsonl) or both.
- Each node name must be unique across all the workspace.
- Each change is stored as a git commit.

## File Layout

```
workspace/
├── AGENTS.md
└── <node-id>/
    ├── index.md
    ├── data.jsonl
    └── data.blobs/
        └── SE/
            └── OC8GKOVGE196NRUJ49IRTP4GJQSGF4CIDP6J54IMCHMU2IN1AG-1024
```

## JSONL Tables

Each `.jsonl` file is a table:
- Line 1: Schema header (JSON object with `version` and `columns` array)
- Lines 2+: Data rows (one JSON object per line)

The schema header row describes the columns of the table. The remaining rows are the data.

This file format is meant to be trivial to process with python, nodejs or whatever tool at your disposal.

Example `data.jsonl`:
```
{"version":"1","columns":[{"name":"id","type":"id"},{"name":"name","type":"string"}]}
{"id":"01JWAB","name":"Alice"}
{"id":"01JWAC","name":"Bob"}
```

## Blob Storage

Binary data is stored in a sibling `.blobs/` directory:
- `data.jsonl` → `data.blobs/`

Blob references in JSON: `sha256:<BASE32HEX>-<size>`
- 52 uppercase base32 hex characters (0-9, A-V) encoding SHA-256 hash
- Decimal byte size after the dash

File path: `<table>.blobs/<char0-1>/<char2-51>-<size>`

Example:
- Reference: `sha256:SEOC8GKOVGE196NRUJ49IRTP4GJQSGF4CIDP6J54IMCHMU2IN1AG-1024`
- File path: `data.blobs/SE/OC8GKOVGE196NRUJ49IRTP4GJQSGF4CIDP6J54IMCHMU2IN1AG-1024`
