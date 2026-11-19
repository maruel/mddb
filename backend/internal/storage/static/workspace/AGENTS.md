# Workspace Data Format

## Tenets

- This git repository represents a workspace.
- Each directory in this workspace is a node.
- A page is a index.md file.
- A table is a metadata.json file with its rows in a data.jsonl file.
- Each node must contain either a page (index.md), a table (metadata.json and data.jsonl) or both.
- Each node name must be unique across all the workspace.
- Each change is stored as a git commit.

## File Layout

```
workspace/
├── AGENTS.md
└── <node-id>/
    ├── index.md
    ├── metadata.json
    ├── data.jsonl
    └── data.blobs/
        └── SE/
            └── OC8GKOVGE196NRUJ49IRTP4GJQSGF4CIDP6J54IMCHMU2IN1AG-1024
```

## Tables

`metadata.json` holds the table title, its properties, and its views. Its `version` is `"1.0"`; readers accept any
`1.x` and treat a missing version as `1.0`. Times are Unix seconds.

```
{"title":"People","version":"1.0","properties":[{"name":"name","type":"text"}],"created":1795114562.856,"modified":1795114562.856}
```

`data.jsonl` holds the rows:
- Line 1: Schema header (JSON object with `version` and `columns` array). `version` must start with `1.`.
- Lines 2+: Data rows (one JSON object per line). `data` maps each property name to its value.

This file format is meant to be trivial to process with python, nodejs or whatever tool at your disposal.

Example `data.jsonl`:
```
{"version":"1.0","columns":[{"name":"id","type":"number","required":true},{"name":"data","type":"jsonb","required":true},{"name":"created","type":"number","required":true},{"name":"modified","type":"number","required":true}]}
{"id":"52AN5GIVI000","data":{"name":"Alice"},"created":1795114562.856,"modified":1795114562.856}
{"id":"52AN5GIVJ000","data":{"name":"Bob"},"created":1795114563.104,"modified":1795114563.104}
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
