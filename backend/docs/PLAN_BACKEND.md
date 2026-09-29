# mddb backend runs without a git binary

Workspace and root repositories use go-git, so mddb ships as one static binary
with typed git errors and no process spawn per mutating request. `ExecRepo` and
`GoGitRepo` already share the `Repository` interface in
`internal/storage/git`, and the existing tests run against both backends.

## Phase 1 — gogit-submodules: Manage workspace submodules in pure Go

- **Scope:** Adding and removing workspace submodules in the root repository.
  go-git v5 has submodule init and update, but not add, deinit, or
  absorbgitdirs, so this phase writes `.gitmodules`, the index entry, and the
  module directory through go-git plumbing.
- **Preserve:** Existing data directories created with the git CLI open and
  keep their history.
- **Verify:** `RootRepo` tests pass on both backends. On a root repository
  that go-git edited, `git submodule status` and `git fsck` succeed.

## Phase 2 — gogit-default: Drop the git CLI dependency

- **Scope:** Make go-git the default for `git.NewManager` and `RootRepo`, then
  remove `ExecRepo`.
- **Verify:** `make test` and the e2e suite pass with no `git` on `PATH`.
  Push, pull, and GitHub App sync work against a real remote.

## Later

- GitHub App: a manifest registration flow, and `installation` and
  `installation_repositories` webhook events. Push events and installation
  tokens work today.
- Passkey and WebAuthn sign-in.
- Sharding for very large JSONLDB tables.
- SQLite for metadata and global tables, keeping the self-describing on-disk
  format.
