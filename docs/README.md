# mddb Documentation Index

## For Developers
- [API.md](API.md) - REST API reference for pages, tables, and assets.
- [TECHNICAL.md](TECHNICAL.md) - Storage model, build process, and optimizations.
- [REQUIREMENTS.md](REQUIREMENTS.md) - Original project specifications.
- [PLAN.md](PLAN.md) - Implementation roadmap and design decisions.
- [AGENTS.md](../AGENTS.md) - Detailed development guidelines and code standards.

## Quick Reference
- **Data Model**: Node (unified container) → Page (markdown) | Table (schema+records) | Hybrid (both). Assets attach to nodes.
- **Storage**: Numeric directories in `data/{orgID}/pages/{nodeID}/`.
- **Backend**: Go with `http.ServeMux`.
- **Frontend**: SolidJS + TypeScript.
- **Versioning**: Automatic Git commits in `data/`.
