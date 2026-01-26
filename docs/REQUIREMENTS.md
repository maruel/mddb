# mddb Requirements

This document outlines the high-level system requirements for mddb. For detailed component-specific requirements, see:
- [Backend Requirements](../backend/docs/REQUIREMENTS.md)
- [Frontend Requirements](../frontend/docs/REQUIREMENTS.md)

## Status

**Project State: Production Prototype (Maturing)**
mddb is a local-first markdown document and database system. The architecture follows a clean 3-layer separation with no layering violations.

## Core System Requirements

### 1. Unified Content Model
- [x] **Nodes**: A single abstraction for documents, tables, and hybrid content.
- [x] **Hierarchy**: Flexible organization through nested nodes.
- [x] **Portability**: Data is stored in human-readable formats (Markdown, JSON, JSONL) for maximum portability.
- [x] **Self-Describing Storage**: On-disk format is designed to be self-describing (e.g., JSONL tables with schema headers), allowing AI agents and external tools to parse and interact with the data directly without requiring the mddb backend code.

### 2. Multi-User & Multi-Tenant
- [x] **Organization Isolation**: Complete data isolation between organizations.
- [x] **Linear Identity Model**: One user account can belong to multiple organizations.
- [x] **Role-Based Access**: Granular permissions (Admin, Editor, Viewer).
- [x] **Hierarchical Authorization**: Workspace access requires organization membership (enforced in middleware).
- [ ] **Membership Cascade**: Removing org membership should cascade to workspace memberships.

### 3. Authentication & Identity
- [x] **OAuth Providers**: Support for Google and Microsoft OAuth.
- [ ] **GitHub OAuth**: Support for GitHub as an OAuth provider.
- [ ] **Multi-Provider Accounts**: Users can link multiple OAuth providers to one account.
- [ ] **Account Reconciliation**: Auto-link accounts by verified email; manual linking for mismatched emails.
- [x] **Email Verification**: Magic link verification for password-based accounts (when SMTP configured).
- [x] **Email Change**: Users can change their primary email (triggers re-verification when SMTP configured).
- [x] **Invitation Emails**: Localized email notifications for organization and workspace invitations.
- **Note**: OAuth emails are trusted as pre-verified by providers. Email features require SMTP configuration.

### 4. Local-First & Versioned
- [x] **Filesystem Persistence**: No external database dependencies.
- [x] **Version Control**: Every change is automatically backed by Git.
- [x] **Audit Trail**: Full history and recovery capabilities.
- [ ] **GitHub App Support**: Integration with GitHub as an App for fine-grained repository access and user-controlled permissions.

### 5. Search & Discovery
- [x] **Global Search**: Full-text search across all accessible content.
- [x] **Performance**: Low-latency indexing and retrieval.

### 6. Deployment
- [x] **Self-Contained**: Distributed as a single, zero-dependency executable.
- [x] **Cross-Platform**: Support for Linux, macOS, and Windows.

### 7. Security
- [x] **API Authorization**: All workspace endpoints verify organization membership before workspace membership.
- [ ] **Asset Authorization**: Asset serving endpoint needs authentication (currently public).
- [ ] **Public Asset Option**: Optional public asset sharing with configurable workspace settings.

### 8. User Experience
- [ ] **Notion-like Editing**: Block-based WYSIWYG editor that feels native and intuitive, while preserving the underlying self-describing Markdown format.

## Globalization
- [x] **i18n**: Support for multiple languages (infrastructure in place).
- [ ] **l10n**: Full regional formatting and localization.

## Roadmap & AI Integration
- [ ] **MCP Server**: Enabling AI agents to interact with the data repository.
- [ ] **Semantic Search**: Vector-based discovery.
- [ ] **Agent Readiness**: Providing context for AI via `AGENTS.md`.
