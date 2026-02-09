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

### Git Layer: Pure-Go Migration

Replace `os/exec` git CLI calls with `github.com/go-git/go-git/v5` (stable, tagged) to
eliminate the runtime git binary dependency, yielding a single static binary, typed errors,
and no exec overhead on every mutating HTTP request.

#### Current surface (2 files, ~340 LOC)

**`git.go` — `Repo`** (workspace-level, called on every mutating request):
- `init` / config — `PlainInit` + `Config()`
- `CommitTx` (add + status + commit) — `Worktree.Add` + `Worktree.Status` + `Worktree.Commit`
- `GetHistory` (log) — `repo.Log` iterator
- `GetFileAtCommit` / `FSAtCommit` — `CommitObject` + `Tree.File`
- `CommitCount` — `repo.Log` iterator count
- `SetRemote` — `CreateRemote` / `DeleteRemote` / config edit
- `Push` — `repo.Push`

**`root_repo.go` — `RootRepo`** (data-dir level, submodule management):
- `CommitDBChanges` — same add/commit pattern as Repo
- `AddWorkspaceSubmodule` — `git submodule add --force` + `absorbgitdirs`
- `RemoveWorkspaceSubmodule` — `deinit -f` + rm `.git/modules/` + `git rm`
- `initialCommit` / `migrateExistingWorkspaces` / `knownSubmodules`

#### go-git v5 coverage gaps

go-git v5 supports init, commit, log, push, remote CRUD, and submodule init/update.
It does **not** support:
- `submodule add` (register a new submodule)
- `submodule deinit` / `absorbgitdirs`

These are only used in `RootRepo` for workspace lifecycle. For the go-git backend,
implement via plumbing: write `.gitmodules` + update the index directly via go-git's
plumbing API.

#### Phased plan

- [x] **Phase 1 — Interface extraction**:
    - [x] Extract `Repository` interface from `*Repo` (`CommitTx`, `GetHistory`,
          `GetFileAtCommit`, `FSAtCommit`, `CommitCount`, `SetRemote`, `Push`, `FS`).
    - [x] Rename current `Repo` to `ExecRepo` (os/exec backend).
    - [x] Have `WorkspaceFileStore`, handlers, and `RootRepo` depend on the interface.
    - [x] All existing tests pass unchanged.
- [x] **Phase 2 — go-git backend**:
    - [x] Implement `GoGitRepo` satisfying the `Repository` interface using go-git v5
          (`PlainInit`/`PlainOpen`, `Worktree`, `repo.Log`, `CommitObject`, etc.).
    - [x] Run all existing tests against both `ExecRepo` and `GoGitRepo` (table-driven
          with backend selector).
    - [ ] Submodule add/deinit via plumbing: write `.gitmodules`, update index, commit.
- [x] **Phase 3 — Configuration & wiring**:
    - [x] `Manager` accepts a backend selector via `NewManagerWithBackend`.
    - [x] Default to `ExecRepo`; `GoGitRepo` available via `BackendGoGit`.
    - [x] Update `go.mod` — added `go-git/go-git/v5` v5.16.4.
    - Note: `RootRepo` remains on `ExecRepo` (submodule operations need exec).

### Integration & Future
- [ ] **MCP Server**: Model Context Protocol implementation for AI agents.
- [ ] **SQLite Migration**: Transitioning metadata and global tables to SQLite while maintaining JSONL affinity.
