# mddb Implementation Plan

This is the high-level roadmap for mddb. For detailed component plans, see:
- [Backend Plan](../backend/docs/PLAN.md)
- [Frontend Plan](../frontend/docs/PLAN.md)

## Status

**Current Phase: Phase 6 (Advanced Sync & Onboarding) - In Progress**

- [x] Phase 1: Core Foundation
- [x] Phase 2: Polish & Features
- [x] Phase 3: Multi-tenant Foundation
- [x] Phase 4: Relational Metadata & Identity
- [x] Phase 5: Experience & Scaling
- [ ] Phase 6: Advanced Sync & Onboarding

## Implementation Roadmap

### Completed Milestones
- **Phase 1**: Core document management and filesystem storage.
- **Phase 2**: Full-text search and Git history.
- **Phase 3**: Multi-tenancy, RBAC, and OAuth2.
- **Phase 4**: Relational Metadata & Identity.
- **Phase 5**: Scaling infrastructure and type-safe API contracts.

### Current Objectives (Phase 6)
- **Advanced Git Integration**: Secure remote syncing for organizations and **GitHub App support** for simplified access control.
- **Streamlined Onboarding**: Unified UI and CLI experience for new users, including **template repository starters**.
- **Global Administration**: Tools for system-wide oversight.
- **Enhanced Authentication**:
  - [x] Refactor `OAuthIdentity.Provider` from `string` to typed `OAuthProvider` enum.
  - [x] Add GitHub OAuth provider support.
  - [x] Implement multi-provider account linking (manual link/unlink endpoints).
  - [x] Add `EmailVerified` field to User; trust OAuth emails as pre-verified.
  - [x] Support email change with re-verification (when email sending enabled).
  - [x] Email verification via magic links (SMTP-based, optional).
  - [x] Localized invitation emails for org/workspace invitations.
- **Security Hardening**:
  - [ ] Add authentication to asset serving endpoint (currently public).
  - [x] Fix parameter naming in asset handlers (`orgID` → `wsID`).
  - [x] Implement membership cascade: deleting org membership removes workspace memberships.
  - [ ] Add optional public asset sharing with workspace-level settings.

### Next Steps (Phase 7+)
- **Intelligence**: Semantic search and AI agent readiness (MCP).
- **Global Presence**: Full localization and offline PWA capabilities.

### Future Evolution
- **Advanced Table Views**: Filter/sort UI dropdowns, column visibility toggles, view rename/duplicate.
- **High-Performance UX**: Virtualization, command palettes, and relationship graphs.
- **Inline Markdown Tables**: Add `table` node type to ProseMirror schema, handle `table_open`/`thead_open`/`tr_open`/`th_open`/`td_open` tokens in `markdown-parser.ts`, and render/edit tables in the flat block editor. Currently table syntax is disabled in markdown-it (`md.disable('table')`) to avoid a crash.
- **Data Integrity**: Migration to SQLite for metadata while preserving the self-describing on-disk format.
- **Large File Uploads**: Streaming uploads for video/large assets (avoid memory buffering), chunked/resumable uploads (tus protocol), progress tracking.

### Completed Table Features
- **View Tabs**: Horizontal tabs for switching between saved views (table, list, gallery, board types).
- **Default View**: Virtual "All" view auto-created when table has no saved views.
- **Column Management**: Add columns via "+" button in table header with type selection.
- **Default Name Column**: New tables start with a "Name" text column like Notion.
- **View CRUD API**: Backend endpoints for creating, updating, and deleting views.
- **Filter/Sort Engine**: Server-side query engine with 12 operators and nested AND/OR logic.
- **Backlinks**: Bidirectional link cache with lazy indexing and API integration.
- **Block Editor**: ProseMirror-based WYSIWYG with slash commands, drag-and-drop, and Markdown serialization.
- **Undo/Redo**: ProseMirror history plugin (Ctrl+Z / Ctrl+Shift+Z).
- **Inline Cell Editing**: Click-to-edit table cells with type-specific inputs.

## Requirements
See [REQUIREMENTS.md](REQUIREMENTS.md) for complete system requirements.