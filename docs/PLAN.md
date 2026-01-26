# mddb Implementation Plan

This is the high-level roadmap for mddb. For detailed component plans, see:
- [Backend Plan](../backend/docs/PLAN.md)
- [Frontend Plan](../frontend/docs/PLAN.md)

## Status

**Current Phase: Phase 9 (Advanced Sync & Onboarding) - In Progress**

- [x] Phase 1-4: Core Foundation
- [x] Phase 5: Polish & Features
- [x] Phase 6: Multi-tenant Foundation
- [x] Phase 7: Relational Metadata & Identity
- [x] Phase 8: Experience & Scaling
- [ ] Phase 9: Advanced Sync & Onboarding

## Implementation Roadmap

### Completed Milestones
- **Phases 1-4**: Core document management and filesystem storage.
- **Phase 5**: Full-text search and Git history.
- **Phase 6-7**: Multi-tenancy, RBAC, and OAuth2.
- **Phase 8**: Scaling infrastructure and type-safe API contracts.

### Current Objectives (Phase 9)
- **Advanced Git Integration**: Secure remote syncing for organizations.
- **Streamlined Onboarding**: Unified UI and CLI experience for new users.
- **Global Administration**: Tools for system-wide oversight.

### Next Steps (Phases 10-12)
- **Connectivity**: Backlinks and relational data modeling.
- **Intelligence**: Semantic search and AI agent readiness (MCP).
- **Global Presence**: Full localization and offline PWA capabilities.

### Future Evolution
- **Advanced Table Views**: A sophisticated view system with persistent filters and sorts.
- **High-Performance UX**: Virtualization, command palettes, and relationship graphs.
- **Data Integrity**: Migration to SQLite for metadata while preserving the self-describing on-disk format.

## Requirements
See [REQUIREMENTS.md](REQUIREMENTS.md) for complete system requirements.
