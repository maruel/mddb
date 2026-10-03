# mddb owns its instance configuration in one directory

`~/.config/mddb/` holds everything specific to this server instance: a
hand-written `config.toml` for startup and a server-written `settings.json`.
`data/` holds content only, and each workspace owns its own quota. Nothing reads
configuration from the environment except what the platform sets.

## Phase 1 — workspace-quota: A workspace's quota lives in its repository

- **Scope:** `Workspace.Quotas` leaves `data/db/workspaces.jsonl` for
  `data/{wsID}/quota.json`, so a workspace's quota is versioned with its content
  and travels with clone and export. `EffectiveQuotas` reads the workspace layer
  from the file. Workspace creation writes the default file, and the operation
  that changes a workspace's quota writes the file and commits it in the
  workspace repository.
- **Preserve:** `EffectiveQuotas` keeps taking the minimum of the non-inherit
  server, organization, and workspace layers, and the file keeps the workspace
  layer's semantics: -1 inherits, 0 disables, a positive value overrides with a
  stricter limit. The server and organization layers do not move.
- **Verify:** Two workspaces on one server observe different effective quotas.
  Editing one file changes that workspace's effective quota on the next request
  and leaves the other untouched. A workspace created through onboarding
  receives the file. The workspace record no longer carries quotas.
