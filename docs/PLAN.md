# mddb Implementation Plan

This is the high-level roadmap for mddb. For detailed component plans, see:

- [Backend Plan](../backend/docs/PLAN.md)
- [Frontend Plan](../frontend/docs/PLAN.md)

## Status

**Current Phase: Phase 7 (Intelligence & Polish)**

- [ ] Phase 7: Intelligence & Polish

## Current Objectives (Phase 7)

- **Security Hardening**:
  - [ ] Add authentication to asset serving endpoint (currently public).
  - [ ] Add optional public asset sharing with workspace-level settings.
- **Intelligence**: Go Mode discovery, native bearer handoff, read-only MCP access to nested nodes in the active workspace, and an optional authenticated embedded voice gateway are implemented. Semantic search and write tools remain future work.
- **Global Presence**: Full localization and offline PWA capabilities. The browser now takes language from the authenticated user's saved settings and discards late responses from a previous account or workspace. First service worker installation no longer reloads an active page; later updates still reload it once.
- **Advanced Table Views**: Filter/sort UI (backend + state management done, UI pending).
  - [ ] ViewToolbar with Filter and Sort buttons.
  - [ ] SortMenu dropdown (property + direction per row).
  - [ ] FilterMenu dropdown (property + operator + value per row, compound AND/OR).
  - [ ] Column visibility toggles, view rename/duplicate.

## Future Evolution

- **High-Performance UX**: Virtualization, command palettes, and relationship graphs.
- **Inline Markdown Tables**: Add `table` node type to ProseMirror schema, handle `table_open`/`thead_open`/`tr_open`/`th_open`/`td_open` tokens in `markdown-parser.ts`, and render/edit tables in the flat block editor. Currently table syntax is disabled in markdown-it (`md.disable('table')`) to avoid a crash.
- **Data Integrity**: Migration to SQLite for metadata while preserving the self-describing on-disk format.
- **Large File Uploads**: Streaming uploads for video/large assets (avoid memory buffering), chunked/resumable uploads (tus protocol), progress tracking.

## Requirements

See [REQUIREMENTS.md](REQUIREMENTS.md) for complete system requirements.
