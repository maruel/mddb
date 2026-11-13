# mddb - Markdown Document & Table System

A self-hosted, Notion-like document and table system. Pages are Markdown files, tables are JSONL files, and every
edit is a Git commit.

[![codecov](https://codecov.io/gh/maruel/mddb/graph/badge.svg?token=Q4SYDP0B00)](https://codecov.io/gh/maruel/mddb)

- Product: [mddb.app](https://mddb.app)
- Installation, configuration, and integration guides: [docs.mddb.app](https://docs.mddb.app)

## Development

Development requires Go, Node.js 26, and pnpm 12.

```bash
git clone https://github.com/maruel/mddb.git
cd mddb
make git-hooks
make build
```

`make build` installs `mddb` and `notion-import` into your Go binary directory.
`uv run --locked python scripts/run-dev.py` runs a development server with a temporary configuration.
`make test` runs the Go and frontend unit tests, `make test-e2e` runs the browser suite, and `make` lists the
common targets.

[AGENTS.md](AGENTS.md) describes the repository workflow. The HTTP API reference is
[sdk/mddb/API.v1.md](sdk/mddb/API.v1.md).

## License

See [LICENSE](LICENSE) file. May change in the future.
