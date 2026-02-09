# Backend Implementation Plan

## Overview
This document focuses on the backend roadmap for mddb, covering API design, storage engines (JSONLDB), and system-level integrations.

## Roadmap

### Sync & Infrastructure
- [ ] **GitHub App Integration**:
    - [ ] Implementation of GitHub App manifest and installation flow.
    - [ ] Per-installation token management and scoped API client.
    - [ ] Webhook handler for repository sync and permission changes.
- [ ] **Template Services**:
    - [ ] API for initializing workspaces from remote template repositories.
- [ ] **Security Hardening**:
    - [ ] Passkey/WebAuthn support.

### AI Intelligence
- [ ] **Advanced Search**:
    - [ ] **BM25 Full-Text Search**: High-quality keyword search implementation.
    - [ ] **Vector Semantic Search**: Embedding generation for documents and records to support semantic search.
    - [ ] **LLM Reranking**: Use LLM to rerank search results for relevance.
    - **Inspiration**: https://github.com/tobi/qmd for search implementation patterns.

### Storage Engine Evolution (JSONLDB)
- [ ] **JSONLDB Sharding**: Support for sharding extremely large datasets.

### Git Layer
- [ ] **Pure-Go Git**: Option to use `github.com/go-git/go-git/v6` instead of `os/exec` git, removing the runtime dependency on the git binary.

### Integration & Future
- [ ] **MCP Server**: Model Context Protocol implementation for AI agents.
- [ ] **SQLite Migration**: Transitioning metadata and global tables to SQLite while maintaining JSONL affinity.
