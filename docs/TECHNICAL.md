# mddb Technical Architecture and Storage Model

## Storage Model

mddb uses a multi-tenant directory structure where each organization owns its own data and Git history, managed via submodules.

### Directory Layout

- **Root Repository**: `data/`
  - The `data/` directory is itself a Git repository that tracks workspace directories.
  - `data/db/`: System-wide information (Users, Organizations, Workspaces, Memberships) stored in JSON.
- **Configuration Directory**: `~/.config/mddb/`, selected with `-config-dir`
  - `config.toml`: Hand-written startup settings, read once at startup.
  - `device-login-audit.jsonl`: Append-only approval and denial audit, mode 0600, outside the content repositories; contains no login codes or secrets.
  - `settings.json`: Server-written mutable settings (JWT secret, VAPID key pair, SMTP, quotas, rate limits), created mode 0600 on first boot, so the secrets it holds stay out of the content repositories.
- **Workspace Repositories**: `data/{wsID}/`
  - Each workspace directory is an independent Git repository.
  - Changes in a workspace directory are committed to its local repository, and the state of these repositories is tracked in the root `data/` repository via Git.
  - `data/{wsID}/{nodeID}/`: Node storage using time-sortable unique IDs. Each directory contains the node's content, metadata, and assets.

```
data/                     # Root Git Repository
├── db/                   # System Metadata (Global)
│   ├── users.json
│   └── ...
└── {wsID}/               # Workspace Repository (Independent Git Repo)
    └── {nodeID}/         # Node ID (e.g., 01JWAB...)
        ├── index.md      # Page content
        ├── metadata.json # Table schema
        ├── data.jsonl    # Table records
        └── image.png     # Asset
```

### Automatic Versioning

mddb employs a hierarchical versioning strategy:

1. **Workspace Level**: Changes within `data/{wsID}/` trigger commits to that workspace's independent repository.
2. **Root Level**: The `data/` repository tracks the state of all workspaces by staging and committing the workspace directory changes and `data/db/`. Server settings live in the configuration directory, so they stay out of the content history. This allows for global backups and state-in-time recovery across the entire system while maintaining tenant isolation.

## Embedded Build Process

mddb is distributed as a single self-contained binary. This is achieved using Go's `embed` package.

### Workflow

1. **Frontend Build**: The SolidJS frontend is compiled into static assets in `frontend/dist/`.
2. **Go Generate**: The `frontend/frontend.go` file uses `//go:embed dist/*` to include these assets.
3. **Compilation**: The Go compiler includes the static assets in the final binary.

### Type Generation

To maintain type safety between the Go backend and SolidJS frontend, mddb uses **apisdkgen** for DTO types, its TypeScript client, and the API reference. Routes are declared in `backend/internal/server/dto/v1/sdk.go` and checked against router registrations by a backend test.

1. **Source**: Go API structs in `backend/internal/server/dto/v1/`.
2. **Process**: `apisdkgen` follows the route specification and generates TypeScript interfaces and client methods.
3. **Artifact**: The versioned TypeScript SDK in `sdk/mddb/ts/v1/` and API reference in `sdk/mddb/API.v1.md`.
4. **Execution**: Triggered by `make fix` and `make build`.

### Reproducibility

The `frontend/dist/` directory is tracked in Git to ensure that the Go binary can be built deterministically without requiring a Node.js environment in the build pipeline (though `make build` will attempt to rebuild it if tools are available).

## Performance Optimizations

### Streaming Reads

Table records are stored in JSONL format, allowing for line-by-line streaming using `bufio.Scanner`. This keeps memory usage low even for large tables.

### API Pagination

Record retrieval supports `offset` and `limit` parameters to handle large datasets efficiently.

## System Metadata Schema

The following tables are managed in `data/db/` (eventually SQLite).

| Table Name      | Go Symbol (internal/models) | Description                         |
| --------------- | --------------------------- | ----------------------------------- |
| `users`         | `User`                      | Core identity and global settings   |
| `organizations` | `Organization`              | Administrative and billing entity   |
| `workspaces`    | `Workspace`                 | Isolated project container          |
| `memberships`   | `Membership`                | User-Org/WS relationships and roles |
| `sessions`      | `Session`                   | Active user sessions and revocation |

## Data Model

mddb uses a unified Node-based data model inspired by Notion. All content entities share a common abstraction while supporting different content types.

### Core Entities

| Entity     | Description                                          | Storage                              |
| ---------- | ---------------------------------------------------- | ------------------------------------ |
| **Node**   | Unified container; can be document, table, or hybrid | Directory at `data/{wsID}/{nodeID}/` |
| **Page**   | Node with markdown content                           | `index.md` with YAML front matter    |
| **Table**  | Node with schema (Properties)                        | `metadata.json` for schema           |
| **Record** | Row in a Table (`DataRecord` type)                   | Line in `data.jsonl`                 |
| **Asset**  | Binary file attached to a Node                       | File in node directory               |

### Node Content Types

Node content type is inferred from file existence, exposed via `has_page` and `has_table` boolean flags in the API:

| Files Present        | has_page | has_table | Description                         |
| -------------------- | -------- | --------- | ----------------------------------- |
| `index.md` only      | true     | false     | Document with markdown content      |
| `metadata.json` only | false    | true      | Table with schema and records       |
| Both files           | true     | true      | Hybrid node with both content types |

Runtime node types are defined in `backend/internal/storage/content/types.go`; the persisted records, properties,
and views are defined in `backend/internal/storage/content/data/v1`.

### Storage Layout

```
data/{wsID}/{nodeID}/
├── index.md          # Markdown content (document/hybrid)
├── metadata.json     # Table schema (table/hybrid)
├── data.jsonl        # Records, one JSON per line (table/hybrid)
├── image.png         # Asset
└── document.pdf      # Asset
```

### Property Types

Table columns support these types:

| Type           | Description                      |
| -------------- | -------------------------------- |
| `text`         | Plain text                       |
| `number`       | Integer or float                 |
| `checkbox`     | Boolean                          |
| `date`         | ISO8601 date string              |
| `select`       | Single selection from options    |
| `multi_select` | Multiple selections from options |
| `url`          | Validated URL                    |
| `email`        | Validated email address          |
| `phone`        | Phone number                     |

### Property Structure

Each `Property` (table column) has:

| Field      | Type           | Description                          |
| ---------- | -------------- | ------------------------------------ |
| `Name`     | string         | Column header / field key            |
| `Type`     | PropertyType   | One of the types above               |
| `Required` | bool           | Whether records must have this field |
| `Options`  | []SelectOption | For `select`/`multi_select` only     |

`SelectOption` defines allowed values for enumerated types:

| Field   | Type   | Description                      |
| ------- | ------ | -------------------------------- |
| `ID`    | string | Stored value (stable identifier) |
| `Name`  | string | Display name                     |
| `Color` | string | Optional color for UI            |

### Entity Relationships

```
Workspace
└── Node (document | table | hybrid)
    ├── Content (markdown, if document/hybrid)
    ├── Properties[] (schema, if table/hybrid)
    ├── Children[] (nested Nodes)
    └── Assets[] (attached files, filename-based IDs)

Table Node
└── DataRecord[] (stored separately in data.jsonl, NOT embedded in Node)
    ├── ID: jsonldb.ID (uint64)
    ├── Data: map[string]any (field values keyed by Property.Name)
    ├── Created: time.Time
    └── Modified: time.Time
```

### ID Types

| Entity | ID Type               | Example                    |
| ------ | --------------------- | -------------------------- |
| Node   | `jsonldb.ID` (uint64) | `01JWAB...`                |
| Record | `jsonldb.ID` (uint64) | `01JWAC...`                |
| Asset  | `string` (filename)   | `"image.png"`, `"doc.pdf"` |

#### ID Format (jsonldb.ID)

IDs are k-sortable 64-bit unique identifiers encoded as Base32 Extended Hex strings (0-9A-V).

- **Structure**: 48-bit timestamp (10µs intervals since 2026-01-01) + 15-bit monotonic slice counter.
- **Properties**: Case-insensitive, lexicographically sortable, time-ordered, and collision-resistant.
- **Representation**: Stored numerically as `uint64` in JSONL but represented as strings in URLs and filenames.

### Key Design Decisions

1. **Polymorphic Nodes**: Pages and Tables share the same API; content type inferred from file existence (`has_page`, `has_table` flags)
2. **Separate Record Storage**: Records stored in JSONL for streaming reads, not embedded in Node
3. **Filename-based Asset IDs**: Assets use original filename as ID (not generated numeric ID)
4. **Hierarchical Structure**: Nodes support parent-child relationships via directory nesting
5. **Type Coercion**: Record field values are coerced to match Property schema types on write
6. **Default Name Column**: New tables are created with a "Name" text column by default (like Notion)
7. **Virtual Default View**: Tables without saved views display a virtual "All" view of type "table"

## Multi-user Architecture

### Identity & Authentication

Password, provider and device sign-in use the same tracked HS256 JWT session issuer,
account permissions and 31-day expiry. Each request validates the
persisted session, its token hash, account, expiry and revocation state.

- **Browser flow**: Browser login, registration, OAuth and device approval set the same normal credential
  in a host-only `__Host-mddb_session` cookie with Secure, HttpOnly and SameSite=Lax. Browser JSON and
  redirects contain no credential. Production deployments require HTTPS; Chromium also accepts Secure
  cookies on localhost for desktop development. Android WebView requires HTTPS even on localhost;
  use a trusted TLS endpoint for the shell. Plain HTTP outside desktop localhost cannot maintain login.
- **API flow**: `/api/v1/auth/login` and `/api/v1/auth/register` return the normal tracked bearer credential.
  API clients keep it in the Authorization header. Exact matching cookie and bearer credentials are
  accepted together; conflicting credentials are rejected, even for the same account.
- **Request protection**: CrossOriginProtection runs before credential selection. Cross-origin browser
  mutations are rejected even with a forged bearer header. Native clients without Origin or fetch
  metadata may send CookieManager cookies without a JavaScript CSRF nonce. OAuth callback GET requests
  retain their separately validated state cookie. Browser operations pin their originating public session
  ID so an account or session replacement cannot apply stale work to the new session.
- **Storage**: User credentials and organization memberships in `data/db/` as JSON files
- **Implementation**: `backend/internal/server/handlers/auth.go`

**Future**: Migration of `data/db/` to SQLite for query performance.

### OAuth2 Integration

Supports Google and Microsoft OIDC flows.

| Provider  | Endpoint                       | Scopes          |
| --------- | ------------------------------ | --------------- |
| Google    | `/api/v1/auth/oauth/google`    | profile, email  |
| Microsoft | `/api/v1/auth/oauth/microsoft` | Azure AD common |

- **Callback handling**: `/api/v1/auth/oauth/{provider}/callback`
- **Account linking**: A new OAuth identity attaches to the account holding its email, or creates one, only when
  the provider verified the email: Google `verified_email`, a verified GitHub `/user/emails` entry, or a Microsoft
  ID-token `email` with `xms_edov` true. Otherwise the login fails with `EMAIL_NOT_VERIFIED`. A returning identity
  signs in by provider ID and refreshes primary-address verification only from fresh provider proof of the
  current exact primary address.
- **Explicit linking**: `POST /api/v1/auth/oauth/link` starts a flow bound to the signed-in user. Independent
  provider proof verifies the current exact primary address without merging accounts. Its optional `next`
  preserves an invitation landing route and token through the signed OAuth state.
- **CSRF protection**: The flow start sets an HttpOnly, SameSite=Lax cookie with the HMAC-signed state. The
  callback rejects a state that differs from the cookie, so a crafted state can neither log a browser in nor
  name another user to link. The browser must use the `server.base_url` host, which receives the callback.
- **Implementation**: `backend/internal/server/handlers/oauth.go`

### Workspace Isolation

Each organization is a separate tenant with isolated data.

- **Storage**: `data/{wsID}/` per workspace with nodes stored directly as subdirectories
- **Git**: Independent Git repository per workspace
- **Validation**: All workspace-scoped endpoints validate membership before access

### Multi-tenancy Model

mddb uses a two-level hierarchy for multi-tenancy:

1. **Organization**: Billing and administrative entity representing a company or team.
2. **Workspace**: Isolated content container within an organization with its own pages, tables, and Git history.

### RBAC Model

Role-based access control is enforced at both organization and workspace levels.

#### Organization Roles

| Role     | Description                                                           |
| -------- | --------------------------------------------------------------------- |
| `owner`  | Full control including billing and organization deletion              |
| `admin`  | Manage workspaces and members, access all workspaces as admin         |
| `member` | View organization details, access workspaces where explicitly granted |

#### Workspace Roles

| Role     | Permissions                                                         |
| -------- | ------------------------------------------------------------------- |
| `viewer` | Read pages, tables, records, assets                                 |
| `editor` | Create/modify content, no member management                         |
| `admin`  | Full workspace control including member management and Git settings |

#### Server-wide Roles

| Role          | Permissions                                             |
| ------------- | ------------------------------------------------------- |
| `globalAdmin` | Server-wide access to all data and management endpoints |

- **Implicit Permissions**: Organization admins and owners implicitly have `admin` access to all workspaces in their organization.
- **Implementation**: `backend/internal/server/handlers/middleware.go`, `handler_wrapper.go`
- **Enforcement**: `WrapAuth()` and `WrapWSAuth()` middleware validate roles before handler execution.

### Quota Tracking

Enforced at write time via `FileStore` pre-checks.

| Quota                | Default | Scope     |
| -------------------- | ------- | --------- |
| `MaxPages`           | 1000    | per ws    |
| `MaxStorage`         | 1 GiB   | per ws    |
| `MaxUsers`           | 3       | per ws    |
| `MaxRecordsPerTable` | 10,000  | per table |
| `MaxAssetSize`       | 50 MiB  | per asset |
| `MaxOrgs`            | 3       | per user  |

- **Implementation**: `backend/internal/storage/content/filestore.go`

## High-Efficiency Caching

### In-Memory Cache

A thread-safe, in-memory cache is used to store:

1. **Metadata**: Table schemas and organization configurations.
2. **Hot Pages**: Frequently accessed markdown content.
3. **Record Indexes**: In-memory maps of record IDs to file positions or small record sets.
4. **Parent Map**: Node ID to parent ID mapping for fast path resolution.

### Strategy

- **LRU Policy**: Least Recently Used eviction to maintain a fixed memory footprint.
- **Write-Through/Invalidation**: Cache is updated or invalidated on every write operation to ensure consistency with the on-disk storage.
- **Lazy Loading**: Data is loaded into the cache on the first read request.

## Invitation links and account proof

Organization and workspace administrators create copyable invitation links. A
link with an email recipient requires a signed-in account with that exact verified
address. A link without a recipient is a bearer grant: any signed-in account can
claim it. Link possession never authenticates an account, verifies an address,
or links identities. Password registration leaves the address unverified.
Email verification (when SMTP is configured), provider proof, and explicit OAuth
linking prove an address. An OAuth login cannot silently merge into an
unverified password account.

The secret-backed preview describes the scope, role, recipient restriction, and
expiry without changing membership. Administrators can copy pending links again
or revoke them within their owning organization/workspace. Existing membership
roles remain unchanged when an invited account already belongs to the scope.
Workspace acceptance also joins its organization as a member when needed.

Each invitation service serializes acceptance and revocation. It persists the
claimant before granting membership. A failed or interrupted membership grant
can be retried only by that same account, including after restart. Partial
organization membership from a workspace grant remains visible; retry completes
the workspace grant. A failed workspace acceptance reports scope IDs, current
organization/workspace membership presence, and retryability in structured error
details. Failed membership writes mark persistence uncertainty; presence describes
current server state rather than promising a transaction across files. Completion
is persisted before invitation cleanup. If that cleanup fails, a completed retry does not run membership grants again. Revoking
a pending claim prevents further acceptance; removing granted access uses the
normal membership controls. This is single-process file-backed coordination,
not a transaction across all membership and invitation files.

When SMTP is configured, a recipient-bound invitation is also emailed with the
same link; bearer links are never emailed. In-app invitation notifications target
only an existing account that independently verified the named address.

## MCP notification scope

Native subscriptions keep their request's conversation-selected workspace. Released
Streamable HTTP clients carry conversation metadata independently on each POST;
their GET notification stream has no conversation metadata. Use the same explicit
`/api/v1/gomode/mcp?workspaceId=<workspace ID>` endpoint for released initialize,
subscribe, GET, and DELETE when subscribing to documents or conversation context.
The transport session is bound to that workspace and persisted login session.
Changing the account's active workspace does not retarget it. Unscoped GET remains
generic and sessionless; unscoped conversation/document subscription requests fail
with scope guidance instead of silently selecting an account-global workspace.

## Resource policy and settings publication

One server process owns a data directory. External tools can edit files out of
band. Application admission cannot constrain those writes. The server does not yet reject a second process or reconcile all external
usage changes. Atomic application snapshots exclude
arbitrary external in-place writers, as defined by the saved-file transaction
contract in `storage/git`.

The organization owns its workspace content. A workspace resource limit is a
ceiling, not a reserved share of organization or server capacity. Administrators
use the same resource protection as other callers. Increasing an applicable
limit is an explicit settings change; administrative authority does not bypass it.

| Limit family              | Units and scope                                                                                         | Inheritance and zero                                                                                                                             |
| ------------------------- | ------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `ResourceQuotas`          | Pages and tables per workspace; records and columns per table; bytes per asset or workspace             | Server values are positive. Organization/workspace `-1` inherits, `0` permits none, positive values tighten. The smallest applicable value wins. |
| Organization count quotas | Workspaces and organization/workspace memberships                                                       | Positive values only; no inheritance. User organization-count quotas are also positive.                                                          |
| Server count quotas       | Users, organizations, workspaces, active sessions per user                                              | Non-negative; `0` is unlimited, no inheritance. Expired/revoked sessions do not consume active-session capacity.                                 |
| Pooled storage            | Sum of logical content bytes in an organization or across the server's workspaces                       | Organization values are positive. Server `0` is unlimited. Independent of per-workspace ceilings.                                                |
| Request body and egress   | Bytes per request; bytes per second across server responses                                             | Non-negative; `0` is unlimited. These are transport limits, not stored bytes.                                                                    |
| Request rates             | Requests per minute and the configured token-bucket burst, per authenticated user or unauthenticated IP | Non-negative; `0` is unlimited, no inheritance. HTTP/MCP write-rate parity is not yet complete.                                                  |
| Notifications             | Retention age in days and retained count per user                                                       | Non-negative; `0` disables that cleanup. Retention can remove old notifications; it is not an admission quota.                                   |

Logical storage includes the current serialized content: page headers and body,
table schemas and records, and retained assets. Git objects, administrative
configuration, and temporary staging belong to physical disk policy instead.
Replacement charges the difference between old and new stored content, not the
full incoming payload. Deletion releases logical capacity; retained Git history
can continue consuming physical disk. Enforcement does not yet cover every mutation or atomically reserve pooled
capacity across workspaces.

Lowering an admission limit does not delete content, cancel admitted work, or
revoke existing sessions. New growth must fit every applicable ceiling. Reads,
deletion, and changes that do not increase the exceeded resource remain allowed;
another exceeded ceiling can still reject growth. This rule applies to inherited
zero limits as well. Rate and bandwidth changes apply to subsequent checks;
already admitted work can finish under its previous allowance. Logical limits do not cap filesystem growth from Git history or temporary files.

`storage.ServerSettings` owns one immutable published settings snapshot. Updates
are serialized: edit a candidate, validate it completely, save it, update the
infallible runtime rate/bandwidth controls, then publish it atomically. Failed
validation or persistence changes no live configuration. Each snapshot is a
complete old or new configuration; an operation can finish with an earlier
snapshot. Workspace stores refresh their cached effective quotas on their next
lookup after a server resource-quota change. Notification cleanup also reads
the published settings instead of retaining a startup copy.

`settings.json` retains its serialized format and lives in the instance config
directory. A save writes and syncs a private same-directory temporary file, then
renames it over the published file. Interrupted staging leaves the old complete
file. A process restart after replacement loads the new complete file; it ignores
abandoned staging files. This is a single-file publication guarantee, not a
transaction with content or Git history. Filesystem power-loss durability depends
on the host filesystem.
