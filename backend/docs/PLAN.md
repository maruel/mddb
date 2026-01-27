# Backend Implementation Plan

## Overview
This document focuses on the backend roadmap for mddb, covering API design, storage engines (JSONLDB), and system-level integrations.

## Core Backend Roadmap

### Phase 1: Advanced Sync & Infrastructure
- [x] **Advanced Git Remotes**:
    - [x] Manual push implementation for organization repositories.
    - [x] Secret management for Git credentials.
- [ ] **GitHub App Integration**:
    - [ ] Implementation of GitHub App manifest and installation flow.
    - [ ] Per-installation token management and scoped API client.
    - [ ] Webhook handler for repository sync and permission changes.
- [ ] **Template Services**:
    - [ ] API for initializing workspaces from remote template repositories.
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

### Phase 2: AI Intelligence
- [ ] **Advanced Search**:
    - [ ] **BM25 Full-Text Search**: High-quality keyword search implementation.
    - [ ] **Vector Semantic Search**: Embedding generation for documents and records to support semantic search.
    - [ ] **LLM Reranking**: Use LLM to rerank search results for relevance.
    - **Inspiration**: https://github.com/tobi/qmd for search implementation patterns.

### Phase 3: Storage Engine Evolution (JSONLDB)
- [x] **JSONLDB Unification**: Unified `Table[T]` with `Row[T]` interface.
- [x] **ID System**: Sortable 64-bit Base64 IDs.
- [x] **Type Coercion**: SQLite-compatible type affinity system.
- [x] **Parent ID Caching**: O(1) node addressing via in-memory cache.
- [ ] **JSONLDB Sharding**: Support for sharding extremely large datasets.

### Phase 4: Table Views & Query Engine
- [ ] **View Model**: Backend storage for persistent filters, sorts, and layouts in `metadata.json`.
- [ ] **Filter/Sort Engine**: Server-side query engine for JSONLDB supporting nested logic.
- [ ] **View API**: CRUD endpoints for managing view configurations.

## Integration & Future
- [ ] **MCP Server**: Model Context Protocol implementation for AI agents.
- [ ] **SQLite Migration**: Transitioning metadata and global tables to SQLite while maintaining JSONL affinity.
- [ ] **Backlinks Indexing**: Background processing for inter-page relationships.
