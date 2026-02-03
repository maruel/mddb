// Records context providing table records CRUD and pagination.

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
} from 'solid-js';
import { useAuth } from './AuthContext';
import { useWorkspace } from './WorkspaceContext';
import { useI18n } from '../i18n';
import { debounce } from '../utils/debounce';
import type { DataRecordResponse, View, Filter, Sort, ViewType } from '@sdk/types.gen';

const PAGE_SIZE = 50;
const FILTER_DEBOUNCE_MS = 300;

interface RecordsContextValue {
  records: Accessor<DataRecordResponse[]>;
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
  clearRecords: () => void;

  // View management
  setActiveViewId: (id: string | undefined) => void;
  setFilters: (filters: Filter[]) => void;
  setSorts: (sorts: Sort[]) => void;
  createView: (name: string, type: ViewType) => Promise<void>;
  updateView: (viewId: string, updates: Partial<View>) => Promise<void>;
  deleteView: (viewId: string) => Promise<void>;

  // Clear errors
  clearErrors: () => void;
}

const RecordsContext = createContext<RecordsContextValue>();

export const RecordsProvider: ParentComponent = (props) => {
  const { t } = useI18n();
  const { wsApi } = useAuth();
  const { selectedNodeId, selectedNodeData } = useWorkspace();

  const [records, setRecords] = createSignal<DataRecordResponse[]>([]);
  const [hasMore, setHasMore] = createSignal(false);

  // Full dataset cache for client-side filtering when all records are loaded
  // allRecords stores the unfiltered dataset when hasMore is false
  const [allRecords, setAllRecords] = createSignal<DataRecordResponse[] | null>(null);

  // View state
  const [views, setViews] = createSignal<View[]>([]);
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
    () => loadingRecords() || savingRecordId() !== null || deletingRecordId() !== null || savingView()
  );

  // Clear all errors
  const clearErrors = () => {
    setLoadError(null);
    setSaveError(null);
  };

  // Virtual default view used when no views exist
  const DEFAULT_VIEW_ID = '__default__';

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
      case 'equals':
        return fieldValue === filterValue;
      case 'not_equals':
        return fieldValue !== filterValue;
      case 'contains':
        return (
          typeof fieldValue === 'string' &&
          typeof filterValue === 'string' &&
          fieldValue.toLowerCase().includes(filterValue.toLowerCase())
        );
      case 'not_contains':
        return (
          typeof fieldValue === 'string' &&
          typeof filterValue === 'string' &&
          !fieldValue.toLowerCase().includes(filterValue.toLowerCase())
        );
      case 'starts_with':
        return (
          typeof fieldValue === 'string' &&
          typeof filterValue === 'string' &&
          fieldValue.toLowerCase().startsWith(filterValue.toLowerCase())
        );
      case 'ends_with':
        return (
          typeof fieldValue === 'string' &&
          typeof filterValue === 'string' &&
          fieldValue.toLowerCase().endsWith(filterValue.toLowerCase())
        );
      case 'is_empty':
        return fieldValue === null || fieldValue === undefined || fieldValue === '';
      case 'is_not_empty':
        return fieldValue !== null && fieldValue !== undefined && fieldValue !== '';
      case 'gt':
        return typeof fieldValue === 'number' && typeof filterValue === 'number' && fieldValue > filterValue;
      case 'gte':
        return typeof fieldValue === 'number' && typeof filterValue === 'number' && fieldValue >= filterValue;
      case 'lt':
        return typeof fieldValue === 'number' && typeof filterValue === 'number' && fieldValue < filterValue;
      case 'lte':
        return typeof fieldValue === 'number' && typeof filterValue === 'number' && fieldValue <= filterValue;
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

        let cmp = 0;
        if (aVal === bVal) {
          cmp = 0;
        } else if (aVal === null || aVal === undefined) {
          cmp = 1; // nulls last
        } else if (bVal === null || bVal === undefined) {
          cmp = -1;
        } else if (typeof aVal === 'string' && typeof bVal === 'string') {
          cmp = aVal.localeCompare(bVal);
        } else if (typeof aVal === 'number' && typeof bVal === 'number') {
          cmp = aVal - bVal;
        } else {
          cmp = String(aVal).localeCompare(String(bVal));
        }

        if (cmp !== 0) {
          return sort.direction === 'desc' ? -cmp : cmp;
        }
      }
      return 0;
    });
  }

  // Debounced server reload function
  const debouncedServerReload = debounce((nodeId: string) => {
    loadRecordsFromServer(nodeId);
  }, FILTER_DEBOUNCE_MS);

  // Cleanup debounced function on unmount
  onCleanup(() => {
    debouncedServerReload.cancel();
  });

  // Load records when selected node changes and has table content
  createEffect(() => {
    const node = selectedNodeData();
    if (node?.has_table) {
      batch(() => {
        const nodeViews = node.views || [];

        if (nodeViews.length > 0) {
          setViews(nodeViews);
          // Default to first view or the one marked default
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
            name: t('table.all') || 'All',
            type: 'table',
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
      loadRecords(node.id);
    } else {
      clearRecords();
    }
  });

  function clearRecords() {
    debouncedServerReload.cancel();
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

    try {
      setLoadingRecords(true);

      const filters = activeFilters().length > 0 ? JSON.stringify(activeFilters()) : undefined;
      const sorts = activeSorts().length > 0 ? JSON.stringify(activeSorts()) : undefined;

      // Don't send virtual default view ID to server
      const viewId = activeViewId();
      const serverViewId = viewId === DEFAULT_VIEW_ID ? '' : viewId || '';

      const data = await ws.nodes.table.records.listRecords(nodeId, {
        Offset: 0,
        Limit: PAGE_SIZE,
        ViewID: serverViewId,
        Filters: filters || '',
        Sorts: sorts || '',
      });

      const loadedRecords = (data.records || []) as DataRecordResponse[];
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
    } catch (err) {
      setLoadError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoadingRecords(false);
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

    try {
      setLoadingRecords(true);
      const offset = records().length;

      const filters = activeFilters().length > 0 ? JSON.stringify(activeFilters()) : undefined;
      const sorts = activeSorts().length > 0 ? JSON.stringify(activeSorts()) : undefined;

      // Don't send virtual default view ID to server
      const viewId = activeViewId();
      const serverViewId = viewId === DEFAULT_VIEW_ID ? '' : viewId || '';

      const data = await ws.nodes.table.records.listRecords(nodeId, {
        Offset: offset,
        Limit: PAGE_SIZE,
        ViewID: serverViewId,
        Filters: filters || '',
        Sorts: sorts || '',
      });

      const newRecords = (data.records || []) as DataRecordResponse[];
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
      setLoadError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoadingRecords(false);
    }
  }

  async function addRecord(data: Record<string, unknown>) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    try {
      setSavingRecordId('__new__'); // Special marker for creating new record
      // Invalidate cache since data is changing
      setAllRecords(null);
      await ws.nodes.table.records.createRecord(nodeId, { data });
      await loadRecords(nodeId);
      setSaveError(null);
    } catch (err) {
      setSaveError(`${t('errors.failedToCreate')}: ${err}`);
    } finally {
      setSavingRecordId(null);
    }
  }

  async function updateRecord(recordId: string, data: Record<string, unknown>) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    try {
      setSavingRecordId(recordId);
      // Invalidate cache since data is changing
      setAllRecords(null);
      await ws.nodes.table.records.updateRecord(nodeId, recordId, { data });
      await loadRecords(nodeId);
      setSaveError(null);
    } catch (err) {
      setSaveError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setSavingRecordId(null);
    }
  }

  async function deleteRecord(recordId: string) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;
    if (!confirm(t('table.confirmDeleteRecord') || 'Delete this record?')) return;

    try {
      setDeletingRecordId(recordId);
      // Invalidate cache since data is changing
      setAllRecords(null);
      await ws.nodes.table.records.deleteRecord(nodeId, recordId);
      await loadRecords(nodeId);
      setSaveError(null);
    } catch (err) {
      setSaveError(`${t('errors.failedToDelete')}: ${err}`);
    } finally {
      setDeletingRecordId(null);
    }
  }

  async function createView(name: string, type: ViewType) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    try {
      setSavingView(true);
      const res = await ws.nodes.views.createView(nodeId, { name, type });

      // Ideally reload node to get updated views list, but we can also optimistically update
      // Since we need the full View object which CreateViewResponse doesn't return (only ID),
      // we probably should reload the node.
      // But we can construct a placeholder view.
      const newView: View = {
        id: res.id,
        name,
        type,
        default: false,
        filters: [],
        sorts: [],
        columns: [], // Defaults
        groups: [],
      };

      batch(() => {
        setViews([...views(), newView]);
        setActiveViewIdSignal(res.id);
        setActiveFilters([]);
        setActiveSorts([]);
      });
      setSaveError(null);
      // No need to reload records as filters are empty
    } catch (err) {
      setSaveError(`${t('errors.failedToCreate')}: ${err}`);
    } finally {
      setSavingView(false);
    }
  }

  async function updateView(viewId: string, updates: Partial<View>) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    try {
      setSavingView(true);
      // We only send the fields that are updated.
      // The API expects UpdateViewRequest which matches Partial<View> structure mostly.
      // But we need to map View properties to UpdateViewRequest properties if they differ.
      // They are identical in our DTOs.
      await ws.nodes.views.updateView(nodeId, viewId, updates);

      setViews(views().map((v) => (v.id === viewId ? { ...v, ...updates } : v)));

      // If updating active view's filters/sorts, update local state and apply
      if (viewId === activeViewId()) {
        if (updates.filters) setActiveFilters(updates.filters);
        if (updates.sorts) setActiveSorts(updates.sorts);
        // Apply filters/sorts (use client-side if cached)
        if (updates.filters || updates.sorts) {
          applyFiltersAndSorts();
        }
      }
      setSaveError(null);
    } catch (err) {
      setSaveError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setSavingView(false);
    }
  }

  async function deleteView(viewId: string) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    if (!confirm(t('table.confirmDeleteView') || 'Delete this view?')) return;

    try {
      setSavingView(true);
      await ws.nodes.views.deleteView(nodeId, viewId);

      const newViews = views().filter((v) => v.id !== viewId);
      setViews(newViews);

      if (activeViewId() === viewId) {
        // Switch to another view or clear
        const firstView = newViews[0];
        if (firstView) {
          setActiveViewId(firstView.id);
        } else {
          setActiveViewId(undefined);
        }
      }
      setSaveError(null);
    } catch (err) {
      setSaveError(`${t('errors.failedToDelete')}: ${err}`);
    } finally {
      setSavingView(false);
    }
  }

  const value: RecordsContextValue = {
    records,
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
    clearRecords,
    setActiveViewId,
    setFilters,
    setSorts,
    createView,
    updateView,
    deleteView,
    clearErrors,
  };

  return <RecordsContext.Provider value={value}>{props.children}</RecordsContext.Provider>;
};

export function useRecords(): RecordsContextValue {
  const context = useContext(RecordsContext);
  if (!context) {
    throw new Error('useRecords must be used within a RecordsProvider');
  }
  return context;
}
