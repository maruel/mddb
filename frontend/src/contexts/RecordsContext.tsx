// Records context providing table records CRUD and pagination.
import { rebaseRecordEdit } from "./recordEdit";
import {
  createContext,
  useContext,
  createSignal,
  createEffect,
  createMemo,
  type ParentComponent,
  type Accessor,
  batch,
  onCleanup,
  untrack,
} from "solid-js";
import { APIError } from "../useApi";
import { useAuth } from "./AuthContext";
import { useRecordDrafts } from "./RecordDraftContext";
import { useEventSource } from "./EventSourceContext";
import { useWorkspace } from "./WorkspaceContext";
import { useI18n } from "../i18n";
import { debounce } from "../utils/debounce";
import { useUndo } from "../hooks/useUndo";
import {
  EventRecordsChanged,
  EventTableUpdated,
  EventResync,
  PropertyTypeUser,
  type DataRecordResponse,
  type View,
  type Filter,
  type Sort,
  type ViewType,
  type WorkspaceMemberResponse,
  type ResolvedUser,
} from "@sdk/types.gen";

const PAGE_SIZE = 50;
const FILTER_DEBOUNCE_MS = 300;

/** Virtual view ID used when no saved views exist. */
export const DEFAULT_VIEW_ID = "__default__";

export interface RecordsContextValue {
  records: Accessor<DataRecordResponse[]>;
  workspaceMembers: Accessor<WorkspaceMemberResponse[]>;
  // Map from user ID string to resolved user info (includes ghosts).
  resolvedUsers: Accessor<Map<string, ResolvedUser>>;
  hasMore: Accessor<boolean>;
  views: Accessor<View[]>;
  activeViewId: Accessor<string | undefined>;
  activeFilters: Accessor<Filter[]>;
  activeSorts: Accessor<Sort[]>;

  // Operation-specific loading states
  loadingRecords: Accessor<boolean>;
  savingRecordId: Accessor<string | null>; // ID of record being saved/created (null for create)
  deletingRecordId: Accessor<string | null>; // ID of record being deleted
  savingView: Accessor<boolean>;

  // Operation-specific error states
  loadError: Accessor<string | null>;
  setLoadError: (error: string | null) => void;
  saveError: Accessor<string | null>;
  setSaveError: (error: string | null) => void;

  // Combined loading (any operation in progress) - for backward compatibility
  loading: Accessor<boolean>;

  loadRecords: (nodeId: string) => Promise<void>;
  loadMoreRecords: () => Promise<void>;
  addRecord: (data: Record<string, unknown>) => Promise<void>;
  updateRecord: (recordId: string, data: Record<string, unknown>) => Promise<void>;
  deleteRecord: (recordId: string) => Promise<void>;
  duplicateRecord: (recordId: string) => Promise<void>;
  clearRecords: () => void;

  viewErrors: Accessor<Array<{ id: string; name: string; code: string }>>;
  discardViewError: (id: string) => Promise<void>;

  // View management
  setActiveViewId: (id: string | undefined) => void;
  setFilters: (filters: Filter[]) => void;
  setSorts: (sorts: Sort[]) => void;
  createView: (name: string, type: ViewType) => Promise<void>;
  updateView: (viewId: string, updates: Partial<View>) => Promise<void>;
  deleteView: (viewId: string) => Promise<void>;

  // Clear errors
  clearErrors: () => void;

  // Application-level undo/redo (record CRUD)
  undo: () => Promise<void>;
  redo: () => Promise<void>;
  canUndo: () => boolean;
  canRedo: () => boolean;
}

// Exported so tests can provide canned records state without mocking the module.
export const RecordsContext = createContext<RecordsContextValue>();

export const RecordsProvider: ParentComponent = (props) => {
  const { t, translateError } = useI18n();
  const { user, identity: authIdentity, wsApi } = useAuth();
  const { selectedNodeId, selectedNodeData } = useWorkspace();

  // Application-level undo stack for record CRUD operations.
  const undoActions = useUndo();
  let historyAcknowledgement: { done: Promise<void>; record?: DataRecordResponse; beforeRevision?: string } | undefined;
  // Successful local edits share a revision so successive undo actions can
  // follow one another without accepting changes made by another client.
  const historyRecords = new Map<string, { id: string; revision: string }>();
  function remapHistoryRecord(record: { id: string; revision: string }, id: string, revision: string) {
    historyRecords.delete(record.id);
    record.id = id;
    record.revision = revision;
    historyRecords.set(id, record);
  }

  const [records, setRecords] = createSignal<DataRecordResponse[]>([]);
  const recordDrafts = useRecordDrafts();
  const [hasMore, setHasMore] = createSignal(false);

  // Workspace members for user-type column picker and ghost resolution.
  const [workspaceMembers, setWorkspaceMembers] = createSignal<WorkspaceMemberResponse[]>([]);
  // Map from user ID → resolved user (active or ghost).
  const [resolvedUsers, setResolvedUsers] = createSignal<Map<string, ResolvedUser>>(new Map());

  // Full dataset cache for client-side filtering when all records are loaded
  // allRecords stores the unfiltered dataset when hasMore is false
  const [allRecords, setAllRecords] = createSignal<DataRecordResponse[] | null>(null);
  const workspaceIdentity = () => `${authIdentity()}:${user()?.id ?? ""}:${user()?.workspace_id ?? ""}`;
  const recordIdentity = (nodeId: string) => `${workspaceIdentity()}:${nodeId}`;
  let recordsRequest = 0;
  let disposed = false;

  // View state
  const [views, setViews] = createSignal<View[]>([]);
  const [viewFailures, setViewFailures] = createSignal(new Map<string, { id: string; name: string; code: string }>());
  const viewErrorKey = (identity: string, id: string) => `${identity}:${id}`;
  const viewErrors = createMemo(() => {
    const identity = recordIdentity(selectedNodeId() ?? "");
    return [...viewFailures()].filter(([key]) => key.startsWith(`${identity}:`)).map(([, error]) => error);
  });
  function setViewFailure(identity: string, id: string, error: unknown) {
    const view = untrack(views).find((item) => item.id === id);
    setViewFailures((previous) =>
      new Map(previous).set(viewErrorKey(identity, id), {
        id,
        name: view?.name ?? id,
        code: error instanceof APIError ? error.code : "unknown",
      }),
    );
  }
  // A recovery owns its view until its read completes; overlapping reads could
  // otherwise restore an older snapshot after a subsequent edit.
  const recoveringViews = new Set<string>();
  async function discardViewError(id: string) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;
    const identity = recordIdentity(nodeId);
    const key = viewErrorKey(identity, id);
    if (recoveringViews.has(key) || !viewFailures().has(key)) return;
    recoveringViews.add(key);
    try {
      const node = await ws.getNode(nodeId);
      if (disposed || identity !== recordIdentity(selectedNodeId() ?? "")) return;
      const current = node.views?.find((view) => view.id === id);
      setViews((previous) =>
        current ? previous.map((view) => (view.id === id ? current : view)) : previous.filter((view) => view.id !== id),
      );
      if (activeViewId() === id) {
        if (current) {
          setActiveFilters(current.filters ?? []);
          setActiveSorts(current.sorts ?? []);
          applyFiltersAndSorts();
        } else {
          setActiveViewId(views()[0]?.id);
        }
      }
      setViewFailures((previous) => {
        const next = new Map(previous);
        next.delete(viewErrorKey(identity, id));
        return next;
      });
    } catch (error) {
      if (!disposed && identity === recordIdentity(selectedNodeId() ?? "")) setViewFailure(identity, id, error);
    } finally {
      recoveringViews.delete(key);
    }
  }
  const [activeViewId, setActiveViewIdSignal] = createSignal<string | undefined>();
  const [activeFilters, setActiveFilters] = createSignal<Filter[]>([]);
  const [activeSorts, setActiveSorts] = createSignal<Sort[]>([]);

  // Operation-specific loading states
  const [loadingRecords, setLoadingRecords] = createSignal(false);
  const [savingRecordId, setSavingRecordId] = createSignal<string | null>(null);
  const [deletingRecordId, setDeletingRecordId] = createSignal<string | null>(null);
  const [savingView, setSavingView] = createSignal(false);

  // Operation-specific error states
  const [loadError, setLoadError] = createSignal<string | null>(null);
  const [saveError, setSaveError] = createSignal<string | null>(null);

  // Combined loading (any operation in progress) - derived for backward compatibility
  const loading = createMemo(
    () => loadingRecords() || savingRecordId() !== null || deletingRecordId() !== null || savingView(),
  );

  // Clear all errors
  const clearErrors = () => {
    setLoadError(null);
    setSaveError(null);
  };

  // Fetch workspace members whenever the API client changes (i.e. on workspace switch).
  createEffect(() => {
    const ws = wsApi();
    if (!ws) return;
    const identity = workspaceIdentity();
    setWorkspaceMembers([]);
    setResolvedUsers(new Map());
    ws.listWorkspaceMembers()
      .then((res) => {
        if (identity === workspaceIdentity()) setWorkspaceMembers(res.members ?? []);
      })
      .catch(() => {
        // Non-critical: the member list feeds the user-type column picker, and a
        // logout or workspace switch cancels the request that was in flight.
        if (identity === workspaceIdentity()) setWorkspaceMembers([]);
      });
  });

  // Resolve ghost users: called after loading records to surface removed members.
  async function resolveGhostUsers(loadedRecords: DataRecordResponse[]) {
    const ws = wsApi();
    const node = selectedNodeData();
    if (!ws || !node?.properties) return;
    const identity = recordIdentity(node.id);

    // Collect user-type column names.
    const userCols = node.properties.filter((p) => p.type === PropertyTypeUser).map((p) => p.name);
    if (userCols.length === 0) return;

    // Gather unique user IDs referenced in records.
    const memberIds = new Set(workspaceMembers().map((m) => m.id));
    const unknownIds = new Set<string>();
    for (const rec of loadedRecords) {
      for (const col of userCols) {
        const v = rec.data?.[col];
        if (typeof v === "string" && v && !memberIds.has(v)) {
          unknownIds.add(v);
        }
      }
    }

    // Build the merged map from current members + any previously cached resolved users.
    const merged = new Map(resolvedUsers());
    for (const m of workspaceMembers()) {
      merged.set(m.id, { id: m.id, name: m.name, email: m.email, avatar_url: m.avatar_url, is_ghost: false });
    }

    if (unknownIds.size === 0) {
      if (identity === recordIdentity(selectedNodeId() ?? "")) setResolvedUsers(merged);
      return;
    }

    // Only resolve IDs not already cached.
    const toFetch = [...unknownIds].filter((id) => !merged.has(id));
    if (toFetch.length > 0) {
      try {
        const res = await ws.resolveUsers({ ids: toFetch });

        for (const u of res.users ?? []) {
          merged.set(u.id, u);
        }
      } catch {
        // Non-critical: ghost users remain unresolved.
      }
    }
    if (identity === recordIdentity(selectedNodeId() ?? "")) setResolvedUsers(merged);
  }

  // Virtual default view used when no views exist (uses module-level constant)

  // Client-side filter matching
  function matchesFilter(record: DataRecordResponse, filter: Filter): boolean {
    // Handle compound filters (and/or)
    if (filter.and && filter.and.length > 0) {
      return filter.and.every((f) => matchesFilter(record, f));
    }
    if (filter.or && filter.or.length > 0) {
      return filter.or.some((f) => matchesFilter(record, f));
    }

    // Simple filter
    if (!filter.property) return true;

    const fieldValue = record.data?.[filter.property];
    const filterValue = filter.value;

    switch (filter.operator) {
      case "equals":
        return fieldValue === filterValue;
      case "not_equals":
        return fieldValue !== filterValue;
      case "contains":
        return (
          typeof fieldValue === "string" &&
          typeof filterValue === "string" &&
          fieldValue.toLowerCase().includes(filterValue.toLowerCase())
        );
      case "not_contains":
        return (
          typeof fieldValue === "string" &&
          typeof filterValue === "string" &&
          !fieldValue.toLowerCase().includes(filterValue.toLowerCase())
        );
      case "starts_with":
        return (
          typeof fieldValue === "string" &&
          typeof filterValue === "string" &&
          fieldValue.toLowerCase().startsWith(filterValue.toLowerCase())
        );
      case "ends_with":
        return (
          typeof fieldValue === "string" &&
          typeof filterValue === "string" &&
          fieldValue.toLowerCase().endsWith(filterValue.toLowerCase())
        );
      case "is_empty":
        return fieldValue === null || fieldValue === undefined || fieldValue === "";
      case "is_not_empty":
        return fieldValue !== null && fieldValue !== undefined && fieldValue !== "";
      case "gt":
        return typeof fieldValue === "number" && typeof filterValue === "number" && fieldValue > filterValue;
      case "gte":
        return typeof fieldValue === "number" && typeof filterValue === "number" && fieldValue >= filterValue;
      case "lt":
        return typeof fieldValue === "number" && typeof filterValue === "number" && fieldValue < filterValue;
      case "lte":
        return typeof fieldValue === "number" && typeof filterValue === "number" && fieldValue <= filterValue;
      default:
        return true;
    }
  }

  // Apply filters to records client-side
  function applyFiltersClientSide(recs: DataRecordResponse[], filters: Filter[]): DataRecordResponse[] {
    if (filters.length === 0) return recs;
    return recs.filter((record) => filters.every((filter) => matchesFilter(record, filter)));
  }

  // Apply sorts to records client-side
  function applySortsClientSide(recs: DataRecordResponse[], sorts: Sort[]): DataRecordResponse[] {
    if (sorts.length === 0) return recs;

    return [...recs].sort((a, b) => {
      for (const sort of sorts) {
        const aVal = a.data?.[sort.property];

        const bVal = b.data?.[sort.property];

        let cmp: number;
        if (aVal === bVal) {
          cmp = 0;
        } else if (aVal === null || aVal === undefined) {
          cmp = 1; // nulls last
        } else if (bVal === null || bVal === undefined) {
          cmp = -1;
        } else if (typeof aVal === "string" && typeof bVal === "string") {
          cmp = aVal.localeCompare(bVal);
        } else if (typeof aVal === "number" && typeof bVal === "number") {
          cmp = aVal - bVal;
        } else {
          // eslint-disable-next-line @typescript-eslint/no-base-to-string -- Preserve JavaScript display for arbitrary record values.
          cmp = String(aVal).localeCompare(String(bVal));
        }

        if (cmp !== 0) {
          return sort.direction === "desc" ? -cmp : cmp;
        }
      }
      return 0;
    });
  }

  // Debounced server reload function
  const debouncedServerReload = debounce((nodeId: string) => {
    void loadRecordsFromServer(nodeId).catch((backgroundError: unknown) => reportError(backgroundError));
  }, FILTER_DEBOUNCE_MS);

  // Cleanup debounced function on unmount
  onCleanup(() => {
    disposed = true;
    debouncedServerReload.cancel();
  });

  // Load records when selected node changes and has table content.
  // Track only the node ID to avoid re-running when other node data updates.
  let prevNodeId: string | undefined;
  createEffect(() => {
    const node = selectedNodeData();
    const nodeId = node?.id;
    if (nodeId === prevNodeId) return;
    prevNodeId = nodeId;
    undoActions.clear();
    clearRecords();

    if (node?.has_table) {
      batch(() => {
        const nodeViews = node.views || [];

        if (nodeViews.length > 0) {
          setViews(nodeViews);
          const defaultView = nodeViews.find((v) => v.default) || nodeViews[0];
          if (defaultView) {
            setActiveViewIdSignal(defaultView.id);
            setActiveFilters(defaultView.filters || []);
            setActiveSorts(defaultView.sorts || []);
          }
        } else {
          // Create a virtual default view when no views exist
          const virtualDefault: View = {
            id: DEFAULT_VIEW_ID,
            revision: "",
            name: t("table.all") || "All",
            type: "table",
            default: true,
            filters: [],
            sorts: [],
            columns: [],
            groups: [],
          };
          setViews([virtualDefault]);
          setActiveViewIdSignal(DEFAULT_VIEW_ID);
          setActiveFilters([]);
          setActiveSorts([]);
        }
      });
      void loadRecords(node.id).catch((backgroundError: unknown) => reportError(backgroundError));
    }
  });

  // Auto-refetch records when SSE reports external record/table changes
  const { lastEvent } = useEventSource();
  createEffect(() => {
    const evt = lastEvent();
    if (!evt) return;
    const nodeId = selectedNodeId();
    if (!nodeId || (evt.type !== EventResync && evt.node_id !== nodeId)) return;
    if (evt.type === EventResync || evt.type === EventRecordsChanged || evt.type === EventTableUpdated) {
      void loadRecords(nodeId).catch((backgroundError: unknown) => reportError(backgroundError));
    }
  });

  function clearRecords() {
    historyRecords.clear();
    debouncedServerReload.cancel();
    recordsRequest++;
    setLoadingRecords(false);
    setRecords([]);
    setHasMore(false);
    setAllRecords(null);
    setViews([]);
    setActiveViewIdSignal(undefined);
    setActiveFilters([]);
    setActiveSorts([]);
  }

  function setActiveViewId(id: string | undefined) {
    if (id === activeViewId()) return;

    const view = views().find((v) => v.id === id);
    batch(() => {
      setActiveViewIdSignal(id);
      if (view) {
        setActiveFilters(view.filters || []);
        setActiveSorts(view.sorts || []);
      } else {
        setActiveFilters([]);
        setActiveSorts([]);
      }
    });

    // Use optimized filtering (client-side if all records cached)
    applyFiltersAndSorts();
  }

  function setFilters(filters: Filter[]) {
    setActiveFilters(filters);
    applyFiltersAndSorts();
  }

  function setSorts(sorts: Sort[]) {
    setActiveSorts(sorts);
    applyFiltersAndSorts();
  }

  // Internal function to load records from server (always reloads)
  async function loadRecordsFromServer(nodeId: string) {
    const ws = wsApi();
    if (!ws) return;
    const identity = recordIdentity(nodeId);
    const request = ++recordsRequest;

    try {
      setLoadingRecords(true);

      const filters = activeFilters().length > 0 ? JSON.stringify(activeFilters()) : undefined;
      const sorts = activeSorts().length > 0 ? JSON.stringify(activeSorts()) : undefined;

      // Don't send virtual default view ID to server
      const viewId = activeViewId();
      const serverViewId = viewId === DEFAULT_VIEW_ID ? "" : viewId || "";

      const data = await ws.listRecords(nodeId, {
        Offset: 0,
        Limit: PAGE_SIZE,
        ViewID: serverViewId,
        Filters: filters || "",
        Sorts: sorts || "",
      });
      if (request !== recordsRequest || identity !== recordIdentity(selectedNodeId() ?? "")) return;

      const loadedRecords = data.records || [];
      setRecords(loadedRecords);
      const more = loadedRecords.length === PAGE_SIZE;
      setHasMore(more);

      // If we have all records and no filters/sorts, cache them for client-side filtering
      if (!more && activeFilters().length === 0 && activeSorts().length === 0) {
        setAllRecords(loadedRecords);
      } else {
        // Clear cache if filters/sorts are active or not all records loaded
        setAllRecords(null);
      }
      setLoadError(null);
      // Resolve any ghost user references in the loaded records.
      void resolveGhostUsers(loadedRecords).catch((backgroundError: unknown) => reportError(backgroundError));
    } catch (err) {
      if (request === recordsRequest && identity === recordIdentity(selectedNodeId() ?? "")) {
        setLoadError(`${t("errors.failedToLoad")}: ${String(err)}`);
      }
    } finally {
      if (request === recordsRequest) setLoadingRecords(false);
    }
  }

  // Public function that may use client-side filtering when possible
  async function loadRecords(nodeId: string) {
    debouncedServerReload.cancel();
    await loadRecordsFromServer(nodeId);
  }

  // Apply filters/sorts, using client-side when all data is cached
  function applyFiltersAndSorts() {
    const cached = allRecords();
    if (cached !== null) {
      // Client-side filtering: we have all records cached
      const filtered = applyFiltersClientSide(cached, activeFilters());
      const sorted = applySortsClientSide(filtered, activeSorts());
      setRecords(sorted);
      setHasMore(false);
    } else {
      // Need server reload (debounced)
      const nodeId = selectedNodeId();
      if (nodeId) {
        debouncedServerReload(nodeId);
      }
    }
  }

  async function loadMoreRecords() {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || loadingRecords() || !ws) return;
    const identity = recordIdentity(nodeId);
    const request = ++recordsRequest;

    try {
      setLoadingRecords(true);
      const offset = records().length;

      const filters = activeFilters().length > 0 ? JSON.stringify(activeFilters()) : undefined;
      const sorts = activeSorts().length > 0 ? JSON.stringify(activeSorts()) : undefined;

      // Don't send virtual default view ID to server
      const viewId = activeViewId();
      const serverViewId = viewId === DEFAULT_VIEW_ID ? "" : viewId || "";

      const data = await ws.listRecords(nodeId, {
        Offset: offset,
        Limit: PAGE_SIZE,
        ViewID: serverViewId,
        Filters: filters || "",
        Sorts: sorts || "",
      });
      if (request !== recordsRequest || identity !== recordIdentity(selectedNodeId() ?? "")) return;

      const newRecords = data.records || [];
      const allRecs = [...records(), ...newRecords];
      setRecords(allRecs);
      const more = newRecords.length === PAGE_SIZE;
      setHasMore(more);

      // Update cache if we now have all records and no filters/sorts active
      if (!more && activeFilters().length === 0 && activeSorts().length === 0) {
        setAllRecords(allRecs);
      }
      setLoadError(null);
    } catch (err) {
      if (request === recordsRequest && identity === recordIdentity(selectedNodeId() ?? "")) {
        setLoadError(`${t("errors.failedToLoad")}: ${String(err)}`);
      }
    } finally {
      if (request === recordsRequest) setLoadingRecords(false);
    }
  }

  function beginRecordMutation() {
    let release!: () => void;
    const pending = {
      done: new Promise<void>((resolve) => {
        release = resolve;
      }),
      record: undefined as DataRecordResponse | undefined,
      beforeRevision: undefined as string | undefined,
    };
    historyAcknowledgement = pending;
    return () => {
      if (historyAcknowledgement === pending) historyAcknowledgement = undefined;
      release();
    };
  }

  function applyCommittedRecord(record: DataRecordResponse, beforeRevision: string) {
    // Retire reads started before this acknowledgement; they cannot replace it.
    recordsRequest++;
    if (historyAcknowledgement) {
      historyAcknowledgement.record = record;
      historyAcknowledgement.beforeRevision = beforeRevision;
    }
    setRecords((current) => current.map((item) => (item.id === record.id ? record : item)));
    setAllRecords(null);
  }

  async function addRecord(data: Record<string, unknown>) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;
    const identity = recordIdentity(nodeId);
    const operationCurrent = () => !disposed && identity === recordIdentity(selectedNodeId() ?? "");
    while (historyAcknowledgement) await historyAcknowledgement.done;
    if (!operationCurrent()) return;

    const finish = beginRecordMutation();
    try {
      setSavingRecordId("__new__"); // Special marker for creating new record
      // Invalidate cache since data is changing
      setAllRecords(null);
      const result = await ws.createRecord(nodeId, { data });
      if (!operationCurrent()) return;
      setSaveError(null);

      const historyRecord = { id: result.id, revision: result.revision };
      historyRecords.set(historyRecord.id, historyRecord);
      const capturedNodeId = nodeId;
      const capturedData = { ...data };
      undoActions.push({
        description: t("common.undo"),
        undo: async () => {
          if (!operationCurrent()) return;
          const ws2 = ws;
          await ws2.deleteRecord(capturedNodeId, historyRecord.id, { expected_revision: historyRecord.revision });
          if (!operationCurrent()) return;
          void loadRecords(capturedNodeId);
        },
        redo: async () => {
          if (!operationCurrent()) return;
          const ws2 = ws;
          const created = await ws2.createRecord(capturedNodeId, { data: capturedData });
          if (!operationCurrent()) return;
          remapHistoryRecord(historyRecord, created.id, created.revision);
          void loadRecords(capturedNodeId);
        },
      });
      finish();
      await loadRecords(nodeId);
    } catch (err) {
      setSaveError(`${t("errors.failedToCreate")}: ${String(err)}`);
    } finally {
      finish();
      setSavingRecordId(null);
    }
  }

  async function updateRecord(recordId: string, data: Record<string, unknown>) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;
    const identity = recordIdentity(nodeId);
    const owner = authIdentity();
    const submitted = structuredClone(data);
    const workspaceId = user()?.workspace_id;
    if (!workspaceId) return;
    const operationCurrent = () => !disposed && identity === recordIdentity(selectedNodeId() ?? "");
    // Bind the submitted values to what the user saw, before waiting for history.
    let prevRecord = records().find((r) => r.id === recordId);
    const observedData = prevRecord?.data ? structuredClone(prevRecord.data) : {};
    const schemaRevision = selectedNodeData()?.schema_revision;
    while (historyAcknowledgement) {
      const pending = historyAcknowledgement;
      await pending.done;
      if (prevRecord && pending.record?.id === recordId && pending.beforeRevision === prevRecord.revision) {
        prevRecord = pending.record;
      }
    }
    if (!operationCurrent() || !prevRecord || !schemaRevision) {
      recordDrafts.add(owner, workspaceId, nodeId, recordId, submitted);
      return;
    }
    const prevData: Record<string, unknown> = prevRecord.data ? structuredClone(prevRecord.data) : {};
    const rebased = rebaseRecordEdit(observedData, submitted, prevData);
    let historyRecord = historyRecords.get(recordId);
    if (historyRecord && historyRecord.revision !== prevRecord.revision) {
      undoActions.clear();
      historyRecords.clear();
      historyRecord = undefined;
    }
    const trackedRecord = historyRecord ?? { id: recordId, revision: prevRecord.revision };

    const finish = beginRecordMutation();
    let committed = false;
    try {
      setSavingRecordId(recordId);
      // Invalidate cache since data is changing
      setAllRecords(null);
      const saved = await ws.updateRecord(nodeId, recordId, {
        data: rebased,
        expected_revision: prevRecord.revision,
        expected_schema_revision: schemaRevision,
      });
      committed = true;
      if (!operationCurrent()) return;
      trackedRecord.revision = saved.record.revision;
      applyCommittedRecord(saved.record, prevRecord.revision);
      historyRecords.set(trackedRecord.id, trackedRecord);
      setSaveError(null);

      const capturedNodeId = nodeId;
      const capturedPrevData = prevData;
      const capturedNewData = rebased;
      undoActions.push({
        description: t("common.undo"),
        undo: async () => {
          if (!operationCurrent()) return;
          const ws2 = ws;
          const beforeRevision = trackedRecord.revision;
          const result = await ws2.updateRecord(capturedNodeId, trackedRecord.id, {
            data: capturedPrevData,
            expected_revision: trackedRecord.revision,
            expected_schema_revision: schemaRevision,
          });
          if (!operationCurrent()) return;
          trackedRecord.revision = result.record.revision;
          applyCommittedRecord(result.record, beforeRevision);
          void loadRecords(capturedNodeId);
        },
        redo: async () => {
          if (!operationCurrent()) return;
          const ws2 = ws;
          const beforeRevision = trackedRecord.revision;
          const result = await ws2.updateRecord(capturedNodeId, trackedRecord.id, {
            data: capturedNewData,
            expected_revision: trackedRecord.revision,
            expected_schema_revision: schemaRevision,
          });
          if (!operationCurrent()) return;
          trackedRecord.revision = result.record.revision;
          applyCommittedRecord(result.record, beforeRevision);
          void loadRecords(capturedNodeId);
        },
      });
      finish();
      await loadRecords(nodeId);
    } catch (err) {
      if (!committed) recordDrafts.add(owner, workspaceId, nodeId, recordId, rebased);
      else if (operationCurrent())
        setSaveError(`${t("errors.failedToSave")}: ${translateError(err instanceof APIError ? err.code : "unknown")}`);
    } finally {
      finish();
      setSavingRecordId(null);
    }
  }

  async function deleteRecord(recordId: string) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;
    const identity = recordIdentity(nodeId);
    const operationCurrent = () => !disposed && identity === recordIdentity(untrack(selectedNodeId) ?? "");
    while (historyAcknowledgement) await historyAcknowledgement.done;
    if (!operationCurrent()) return;
    if (!confirm(t("table.confirmDeleteRecord") || "Delete this record?")) return;

    // Capture full record data before deletion for undo.
    const record = records().find((r) => r.id === recordId);
    if (!record) {
      setSaveError(t("errors.failedToDelete"));
      return;
    }
    const savedData: Record<string, unknown> = record.data ? structuredClone(record.data) : {};
    let historyRecord = historyRecords.get(recordId);
    if (historyRecord && historyRecord.revision !== record.revision) {
      undoActions.clear();
      historyRecords.clear();
      historyRecord = undefined;
    }
    const trackedRecord = historyRecord ?? { id: recordId, revision: record.revision };

    const finish = beginRecordMutation();
    try {
      setDeletingRecordId(recordId);
      // Invalidate cache since data is changing
      setAllRecords(null);
      await ws.deleteRecord(nodeId, recordId, { expected_revision: record.revision });
      if (!operationCurrent()) return;
      setSaveError(null);

      const capturedNodeId = nodeId;
      const capturedData = savedData;
      historyRecords.set(trackedRecord.id, trackedRecord);
      undoActions.push({
        description: t("common.undo"),
        undo: async () => {
          if (!operationCurrent()) return;
          const ws2 = ws;
          const result = await ws2.createRecord(capturedNodeId, {
            data: capturedData,
          });
          if (!operationCurrent()) return;
          remapHistoryRecord(trackedRecord, result.id, result.revision);
          void loadRecords(capturedNodeId);
        },
        redo: async () => {
          if (!operationCurrent()) return;
          const ws2 = ws;
          await ws2.deleteRecord(capturedNodeId, trackedRecord.id, { expected_revision: trackedRecord.revision });
          if (!operationCurrent()) return;
          void loadRecords(capturedNodeId);
        },
      });
      finish();
      await loadRecords(nodeId);
    } catch (err) {
      setSaveError(`${t("errors.failedToDelete")}: ${String(err)}`);
    } finally {
      finish();
      setDeletingRecordId(null);
    }
  }

  async function duplicateRecord(recordId: string) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;
    const identity = recordIdentity(nodeId);
    const operationCurrent = () => !disposed && identity === recordIdentity(selectedNodeId() ?? "");
    while (historyAcknowledgement) await historyAcknowledgement.done;
    if (!operationCurrent()) return;

    const record = records().find((r) => r.id === recordId);
    if (!record) return;

    const finish = beginRecordMutation();
    try {
      setSavingRecordId("__new__"); // Special marker for creating new record
      // Invalidate cache since data is changing
      setAllRecords(null);

      await ws.createRecord(nodeId, { data: record.data || {} });
      if (!operationCurrent()) return;
      finish();
      await loadRecords(nodeId);
      if (!operationCurrent()) return;
      setSaveError(null);
    } catch (err) {
      setSaveError(`${t("errors.failedToCreate")}: ${String(err)}`);
    } finally {
      finish();
      setSavingRecordId(null);
    }
  }

  async function createView(name: string, type: ViewType) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;
    const identity = recordIdentity(nodeId);
    const operationCurrent = () => !disposed && identity === recordIdentity(selectedNodeId() ?? "");

    try {
      setSavingView(true);
      const res = await ws.createView(nodeId, { name, type });
      if (!operationCurrent()) return;

      batch(() => {
        setViews([...views(), res.view]);
        setActiveViewIdSignal(res.view.id);
        setActiveFilters([]);
        setActiveSorts([]);
      });
      setSaveError(null);
      // No need to reload records as filters are empty
    } catch (err) {
      setSaveError(`${t("errors.failedToCreate")}: ${translateError(err instanceof APIError ? err.code : "unknown")}`);
    } finally {
      setSavingView(false);
    }
  }

  // Serialize writes for each view so a later local edit uses the revision from
  // the preceding save. A failed save rejects all already queued edits.
  const viewWrites = new Map<string, Promise<void>>();
  function queueViewWrite(key: string, write: () => Promise<void>): Promise<void> {
    const previous = viewWrites.get(key) ?? Promise.resolve();
    const pending = previous.then(write);
    viewWrites.set(key, pending);
    void pending
      .finally(() => {
        if (viewWrites.get(key) === pending) viewWrites.delete(key);
      })
      .catch(() => undefined);
    return pending;
  }

  async function updateView(viewId: string, updates: Partial<View>) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;
    const identity = recordIdentity(nodeId);
    const operationCurrent = () => !disposed && identity === recordIdentity(untrack(selectedNodeId) ?? "");
    if (viewFailures().has(viewErrorKey(identity, viewId))) return;
    setViews(views().map((v) => (v.id === viewId ? { ...v, ...updates } : v)));
    try {
      setSavingView(true);
      await queueViewWrite(`${identity}:${viewId}`, async () => {
        if (!operationCurrent()) throw new Error("View changed while saving");
        const current = untrack(views).find((v) => v.id === viewId);
        if (!current) throw new Error("View no longer available");
        const res = await ws.updateView(nodeId, viewId, { ...updates, expected_revision: current.revision });
        if (operationCurrent())
          setViews(untrack(views).map((v) => (v.id === viewId ? { ...v, revision: res.view.revision } : v)));
      });
    } catch (err) {
      if (operationCurrent()) setViewFailure(identity, viewId, err);
    } finally {
      if (operationCurrent()) setSavingView(false);
    }
  }

  async function deleteView(viewId: string) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;
    const identity = recordIdentity(nodeId);
    const operationCurrent = () => !disposed && identity === recordIdentity(untrack(selectedNodeId) ?? "");
    if (viewFailures().has(viewErrorKey(identity, viewId))) return;
    if (!confirm(t("table.confirmDeleteView"))) return;
    try {
      setSavingView(true);
      await queueViewWrite(`${identity}:${viewId}`, async () => {
        if (!operationCurrent()) throw new Error("View changed while deleting");
        const current = untrack(views).find((v) => v.id === viewId);
        if (!current) throw new Error("View no longer available");
        await ws.deleteView(nodeId, viewId, { expected_revision: current.revision });
      });
      if (!operationCurrent()) return;
      const newViews = views().filter((v) => v.id !== viewId);
      setViews(newViews);
      if (activeViewId() === viewId) setActiveViewId(newViews[0]?.id);
    } catch (err) {
      if (operationCurrent()) setViewFailure(identity, viewId, err);
    } finally {
      if (operationCurrent()) setSavingView(false);
    }
  }

  async function runHistory(redo: boolean) {
    const identity = recordIdentity(selectedNodeId() ?? "");
    const current = () => !disposed && identity === recordIdentity(selectedNodeId() ?? "");
    while (historyAcknowledgement) await historyAcknowledgement.done;
    if (!current()) return;
    const finish = beginRecordMutation();
    try {
      await (redo ? undoActions.redo() : undoActions.undo());
      if (current()) setSaveError(null);
    } catch (err) {
      if (!current()) return;
      setSaveError(
        err instanceof APIError && err.status === 409
          ? t("errors.recordHistoryConflict")
          : `${t("errors.failedToSave")}: ${translateError(err instanceof APIError ? err.code : "unknown")}`,
      );
    } finally {
      finish();
    }
  }

  const value: RecordsContextValue = {
    records,
    viewErrors,
    discardViewError,
    workspaceMembers,
    resolvedUsers,
    hasMore,
    views,
    activeViewId,
    activeFilters,
    activeSorts,
    loadingRecords,
    savingRecordId,
    deletingRecordId,
    savingView,
    loadError,
    setLoadError,
    saveError,
    setSaveError,
    loading,
    loadRecords,
    loadMoreRecords,
    addRecord,
    updateRecord,
    deleteRecord,
    duplicateRecord,
    clearRecords,
    setActiveViewId,
    setFilters,
    setSorts,
    createView,
    updateView,
    deleteView,
    clearErrors,
    undo: () => runHistory(false),
    redo: () => runHistory(true),
    canUndo: undoActions.canUndo,
    canRedo: undoActions.canRedo,
  };

  return <RecordsContext.Provider value={value}>{props.children}</RecordsContext.Provider>;
};

export function useRecords(): RecordsContextValue {
  const context = useContext(RecordsContext);
  if (!context) {
    throw new Error("useRecords must be used within a RecordsProvider");
  }
  return context;
}
