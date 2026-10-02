# mddb owns its instance configuration in one directory

`~/.config/mddb/` holds everything specific to this server instance: a
hand-written `config.toml` for startup and a server-written `settings.json`.
`data/` holds content only, and each workspace owns its own quota. Nothing
reads configuration from the environment except what the platform sets.

## Phase 1 — harness-config: The dev and e2e harness exports no variables

- **Scope:** Test-only behavior stops being reachable at runtime, following caic.
  A new `backend/internal/e2emode` package answers `Enabled()` from the `e2e`
  build tag: `enabled.go` under `//go:build e2e`, `disabled.go` under
  `//go:build !e2e`. It selects the fake OAuth providers in `cmd/mddb/config.go`
  and the raised user quota in `storage.DefaultServerQuotas`. The same pair
  registers the rate-limit multiplier as the flag `-fast-rate-limit`, defaulting
  off: a production binary does not define it, so passing it fails loudly. An
  unset flag means production limits, so a forgotten flag is the safe outcome.
  `ratelimit.ConfigFromStorage` and `ratelimit.DefaultConfig` take the multiplier
  as a parameter instead of reading `os.Getenv`. The dev and e2e server binary is
  built with `-tags e2e`; the default build target stays a production binary.
  The Makefile exports nothing: the fast dev and e2e variants pass
  `-fast-rate-limit`, the slow variant omits it, and the slow variant uses a
  Playwright config of its own. `playwright.config.ts` keeps no
  `process.env.TEST_*` read. `e2e/AGENTS.md` describes the tag and the flag
  instead of the variables.
- **Preserve:** `JOURNAL_STREAM` stays an environment read, because systemd sets
  it. The multipliers keep their values: 50 to 200 users, and a rate multiplier
  of 10000. `make test` still builds without the tag, so unit tests observe
  production behavior. The command line keeps `-config-dir` and `-version`, plus
  flags that exist solely in the e2e build.
- **Verify:** `grep -rn 'TEST_OAUTH\|TEST_FAST_RATE_LIMIT' Makefile
playwright.config.ts e2e/ backend/` returns nothing. `make dev` and
  `make test-e2e` start a server with no variable exported, and the e2e suite
  passes. A test proves the fake providers are absent from a build without the
  tag and present with it, that `-fast-rate-limit` selects the multiplier, and
  that a production binary rejects the flag.

## Phase 2 — workspace-quota: A workspace's quota lives in its repository

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
