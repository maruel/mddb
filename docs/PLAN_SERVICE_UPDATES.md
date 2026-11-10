# Timely workspace updates with bounded cost and conflict-safe edits

Keep browser and Android voice current without notification storms or lost edits.
mddb owns content, authorization, and notification policy; Go Mode owns shared
transport, context delivery, and shell capabilities. Include caic in shared changes.
Prefer coordinated breaking changes over compatibility adapters.

## Phase 1 — shared-context: Publish advertised model resources

- **Scope:** Publish the shared contract, deploy its gateway first, and roll both
  Go and npm dependencies together in mddb and caic.
- **Verify:** Both hosts pass standalone gates and browser/native journeys with
  matching published pins, lockfile integrity, and no local overrides. Confirm
  the served runtimes match the published package and retain the measured
  request and wake-up budgets.

## Phase 2 — actionable-alerts: Deliver occurrences with bounded wake-ups

- **Scope:** Expose user-scoped `gomode://notifications` from persisted notifications
  and preferences. mddb owns occurrence identity, relevance, expiry, and withdrawal;
  shared delivery coalesces eligible wake-ups while retaining distinct occurrences.
- **Preserve:** Quiet initial baselines; announcing never marks read. Titles are
  not action identities. Urgent alerts bypass routine delay. Define queued and
  interrupted speech retry without promising exactly-once audible delivery.
- **Verify:** Isolation, preferences, reconnect/truncation, repeated occurrences,
  expiry/withdrawal, and interrupted delivery retain eligible alerts without
  repeating the baseline. Measure burst wake counts and delivery latency.

## Phase 3 — incremental-clients: Refresh only affected state

- **Scope:** Bound browser/Android catalog and resource caches by authorization
  and conversation inputs, including negative discovery and permission changes.
  Route updates to dirty watchers; share subscription demand, single-flight reads,
  and bounded coalescing. Reuse initial snapshots and replace unread polling with
  invalidations. Patch metadata/records or affected parents/query windows instead
  of reloading whole trees/tables. Consume host mutation-error metadata for recovery.
- **Preserve:** Race-free snapshot/subscription handoff and gap resync; query order,
  membership, counts, and pagination. Reject obsolete responses, including
  [per-node metadata refreshes](../frontend/src/contexts/WorkspaceContext.tsx).
  Moves retain dirty/upload-pending drafts and undo. Preserve focus/conversation
  authority and [unknown-save recovery](../frontend/src/contexts/EditorContext.tsx).
  Use bounded jittered retry, stop on terminal authorization failure, and keep
  hidden active voice fresh. No cross-tab leader election is required.
- **Verify:** Alert updates do not reread items; absent resources stay negatively
  cached across focus changes. Equivalent snapshots do not reinject full context.
  Clean editors update, private drafts survive, record bursts stay incremental,
  and idle clients do not poll. Browser/Android journeys cover identity/workspace
  switches and reconnects. Before changing these paths, baseline HTTP/SSE setup
  and recovery, request counts, latency, context bytes, and retained memory;
  compare afterward, including large task records and long navigation/voice sessions.

## Phase 4 — coordinated-release: Ship across shared consumers

- **Scope:** Remove superseded adapters, formats, polling, and lifecycle owners
  from both consumers of the published Go Mode contract. Update owning docs.
  Keep the Vite proxy HTTP test's Origin coverage; isolate its config and temporary
  cache, cleaning after shutdown. Preserve deferred concurrency work in source or
  an existing issue before deleting this plan.
- **Verify:** Required gates, regenerated SDKs/assets, browser/Android journeys,
  and caic context/alert regressions pass with published dependencies, matching
  lockfile integrity, and no workspace overrides. Report application-level
  before/after measurements and remaining budget misses. Live-provider smoke
  remains separate from deterministic gates.

## Later

- `caic-mobile-tail`: investigate intermittent live-tail loss during compacted-task
  navigation and viewport changes; preserve explicit reading intent.
- `table-record-concurrency`: add conditional table/schema/record replacements
  and client recovery; preserve the audit in owning page/table/view handlers.
