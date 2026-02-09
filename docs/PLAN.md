# mddb Implementation Plan

This is the high-level roadmap for mddb. For detailed component plans, see:
- [Backend Plan](../backend/docs/PLAN.md)
- [Frontend Plan](../frontend/docs/PLAN.md)

## Status

**Current Phase: Phase 7 (Intelligence & Polish)**

- [x] Phase 1: Core Foundation
- [x] Phase 2: Polish & Features
- [x] Phase 3: Multi-tenant Foundation
- [x] Phase 4: Relational Metadata & Identity
- [x] Phase 5: Experience & Scaling
- [x] Phase 6: Advanced Sync & Onboarding
- [ ] Phase 7: Intelligence & Polish

## Current Objectives (Phase 7)

- **Security Hardening**:
  - [ ] Add authentication to asset serving endpoint (currently public).
  - [ ] Add optional public asset sharing with workspace-level settings.
- **Intelligence**: Semantic search and AI agent readiness (MCP).
- **Global Presence**: Full localization and offline PWA capabilities.

## Future Evolution
- **Advanced Table Views**: Filter/sort UI dropdowns, column visibility toggles, view rename/duplicate.
- **High-Performance UX**: Virtualization, command palettes, and relationship graphs.
- **Inline Markdown Tables**: Add `table` node type to ProseMirror schema, handle `table_open`/`thead_open`/`tr_open`/`th_open`/`td_open` tokens in `markdown-parser.ts`, and render/edit tables in the flat block editor. Currently table syntax is disabled in markdown-it (`md.disable('table')`) to avoid a crash.
- **Data Integrity**: Migration to SQLite for metadata while preserving the self-describing on-disk format.
- **Large File Uploads**: Streaming uploads for video/large assets (avoid memory buffering), chunked/resumable uploads (tus protocol), progress tracking.

## Requirements
See [REQUIREMENTS.md](REQUIREMENTS.md) for complete system requirements.
