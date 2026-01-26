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
  - [ ] Refactor `OAuthIdentity.Provider` from `string` to typed `OAuthProvider` enum.
  - [ ] Add GitHub OAuth provider support.
  - [ ] Implement multi-provider account linking (manual link/unlink endpoints).
  - [x] Add `EmailVerified` field to User; trust OAuth emails as pre-verified.
  - [x] Support email change with re-verification (when email sending enabled).
  - [x] Email verification via magic links (SMTP-based, optional).
  - [x] Localized invitation emails for org/workspace invitations.
- **Security Hardening**:
  - [ ] Add authentication to asset serving endpoint (currently public).
  - [ ] Fix parameter naming in asset handlers (`orgID` → `wsID`).
  - [ ] Implement membership cascade: deleting org membership removes workspace memberships.
  - [ ] Add optional public asset sharing with workspace-level settings.

### Next Steps (Phase 7+)
- **Connectivity**: Backlinks and relational data modeling.
- **Intelligence**: Semantic search and AI agent readiness (MCP).
- **Global Presence**: Full localization and offline PWA capabilities.

### Future Evolution
- **Advanced Table Views**: A sophisticated view system with persistent filters and sorts.
- **High-Performance UX**: Virtualization, command palettes, and relationship graphs.
- **Notion-like Editing**: A block-based **WYSIWYG editor** for a seamless, intuitive experience.
- **Data Integrity**: Migration to SQLite for metadata while preserving the self-describing on-disk format.

## Requirements
See [REQUIREMENTS.md](REQUIREMENTS.md) for complete system requirements.