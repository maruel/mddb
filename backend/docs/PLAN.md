# Backend Implementation Plan

## Overview
This document focuses on the backend roadmap for mddb, covering API design, storage engines (JSONLDB), and system-level integrations.

## Core Backend Roadmap

### Phase 9: Advanced Sync & Infrastructure
- [x] **Advanced Git Remotes**:
    - [x] Manual push implementation for organization repositories.
    - [x] Secret management for Git credentials.
- [x] **CLI Evolution**:
    - [x] `mddb -version` build metadata.
    - [x] `.env` based local configuration.
- [x] **Global Admin Infrastructure**:
    - [x] Global admin role and middleware.
    - [x] Admin API endpoints for system oversight.
- [ ] **Operational Safeguards**:
    - [ ] Request rate limiting.
    - [ ] Bandwidth/egress tracking.
- [ ] **User Analytics**:
    - [ ] IP address and GeoIP tracking on signup.
- [ ] **Security Hardening**:
    - [ ] Passkey/WebAuthn support.

### Phase 13: Storage Engine Evolution (JSONLDB)
- [x] **JSONLDB Unification**: Unified `Table[T]` with `Row[T]` interface.
- [x] **ID System**: Sortable 64-bit Base64 IDs.
- [x] **Type Coercion**: SQLite-compatible type affinity system.
- [x] **Parent ID Caching**: O(1) node addressing via in-memory cache.
- [ ] **JSONLDB Sharding**: Support for sharding extremely large datasets.

### Phase 14: URL & Path Standardization
- [ ] **URL Namespace**: Prefix page URLs with `/p/`.
- [ ] **ID Encoding**: Switch to Base64 URL-encoded IDs.
- [ ] **Separator Migration**: Transition from `-` to `+` for Base64 compatibility.

### Phase 16: Table Views & Query Engine
- [ ] **View Model**: Backend storage for persistent filters, sorts, and layouts in `metadata.json`.
- [ ] **Filter/Sort Engine**: Server-side query engine for JSONLDB supporting nested logic.
- [ ] **View API**: CRUD endpoints for managing view configurations.

## Integration & Future
- [ ] **MCP Server**: Model Context Protocol implementation for AI agents.
- [ ] **SQLite Migration**: Transitioning metadata and global tables to SQLite while maintaining JSONL affinity.
- [ ] **Backlinks Indexing**: Background processing for inter-page relationships.
