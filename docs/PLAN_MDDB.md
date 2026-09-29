# mddb workspaces searchable by people and agents

Workspace search works in the browser and over MCP. Module plans own
single-module work: [backend](../backend/docs/PLAN_BACKEND.md) and
[frontend](../frontend/docs/PLAN_FRONTEND.md). Requirements:
[REQUIREMENTS.md](REQUIREMENTS.md).

## Phase 1 — keyword-search: Find pages and records by keyword

- **Scope:** The workspace search service behind
  `POST /api/v1/workspaces/{wsID}/search`, which returns "search not
  implemented" today; a search UI; and an MCP search tool on the `workspace`
  skill.
- **Preserve:** Results honor workspace membership and role, like every other
  workspace endpoint. The on-disk format stays self-describing; any index can
  be rebuilt from it.
- **Verify:** Ranked results cover page bodies, titles, and table records.
  Edits appear in results without a restart. An e2e test finds a new page
  through the UI, and an MCP test calls the search tool.

## Phase 2 — semantic-search: Find content by meaning

- **Scope:** Document and record embeddings, and vector retrieval merged with
  keyword results. Patterns: [tobi/qmd](https://github.com/tobi/qmd).
- **Preserve:** Search works without an embedding provider.
- **Verify:** A query without shared keywords finds the matching page. LLM
  reranking is measured against keyword-only ranking before it ships.

## Later

- Workspace initialization from template Git repositories, offered during
  onboarding.
- Optional public asset links, configured per workspace; asset URLs are
  signed today.
- Offline editing with client storage and reconciliation.
- Streaming and resumable (tus) uploads with progress for large assets.
