// Records context providing table records CRUD and pagination.

import {
  createContext,
  useContext,
  createSignal,
  createEffect,
  type ParentComponent,
  type Accessor,
  batch,
} from 'solid-js';
import { useAuth } from './AuthContext';
import { useWorkspace } from './WorkspaceContext';
import { useI18n } from '../i18n';
import type { DataRecordResponse, View, Filter, Sort, ViewType } from '@sdk/types.gen';

const PAGE_SIZE = 50;

interface RecordsContextValue {
  records: Accessor<DataRecordResponse[]>;
  hasMore: Accessor<boolean>;
  views: Accessor<View[]>;
  activeViewId: Accessor<string | undefined>;
  activeFilters: Accessor<Filter[]>;
  activeSorts: Accessor<Sort[]>;

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
}

const RecordsContext = createContext<RecordsContextValue>();

export const RecordsProvider: ParentComponent = (props) => {
  const { t } = useI18n();
  const { wsApi } = useAuth();
  const { selectedNodeId, selectedNodeData, loading, setLoading, setError } = useWorkspace();

  const [records, setRecords] = createSignal<DataRecordResponse[]>([]);
  const [hasMore, setHasMore] = createSignal(false);

  // View state
  const [views, setViews] = createSignal<View[]>([]);
  const [activeViewId, setActiveViewIdSignal] = createSignal<string | undefined>();
  const [activeFilters, setActiveFilters] = createSignal<Filter[]>([]);
  const [activeSorts, setActiveSorts] = createSignal<Sort[]>([]);

  // Load records when selected node changes and has table content
  createEffect(() => {
    const node = selectedNodeData();
    if (node?.has_table) {
      batch(() => {
        setViews(node.views || []);
        // Reset view state when node changes
        // Note: we might want to remember the last used view for this node in the future
        if (node.views && node.views.length > 0) {
          // Default to first view or the one marked default
          const defaultView = node.views.find((v) => v.default) || node.views[0];
          if (defaultView) {
            setActiveViewIdSignal(defaultView.id);
            setActiveFilters(defaultView.filters || []);
            setActiveSorts(defaultView.sorts || []);
          }
        } else {
          setActiveViewIdSignal(undefined);
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
    setRecords([]);
    setHasMore(false);
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

    const nodeId = selectedNodeId();
    if (nodeId) {
      loadRecords(nodeId);
    }
  }

  function setFilters(filters: Filter[]) {
    setActiveFilters(filters);
    const nodeId = selectedNodeId();
    if (nodeId) loadRecords(nodeId);
  }

  function setSorts(sorts: Sort[]) {
    setActiveSorts(sorts);
    const nodeId = selectedNodeId();
    if (nodeId) loadRecords(nodeId);
  }

  async function loadRecords(nodeId: string) {
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);

      const filters = activeFilters().length > 0 ? JSON.stringify(activeFilters()) : undefined;
      const sorts = activeSorts().length > 0 ? JSON.stringify(activeSorts()) : undefined;

      const data = await ws.nodes.table.records.listRecords(nodeId, {
        Offset: 0,
        Limit: PAGE_SIZE,
        ViewID: activeViewId() || '',
        Filters: filters || '',
        Sorts: sorts || '',
      });

      const loadedRecords = (data.records || []) as DataRecordResponse[];
      setRecords(loadedRecords);
      setHasMore(loadedRecords.length === PAGE_SIZE);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function loadMoreRecords() {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || loading() || !ws) return;

    try {
      setLoading(true);
      const offset = records().length;

      const filters = activeFilters().length > 0 ? JSON.stringify(activeFilters()) : undefined;
      const sorts = activeSorts().length > 0 ? JSON.stringify(activeSorts()) : undefined;

      const data = await ws.nodes.table.records.listRecords(nodeId, {
        Offset: offset,
        Limit: PAGE_SIZE,
        ViewID: activeViewId() || '',
        Filters: filters || '',
        Sorts: sorts || '',
      });

      const newRecords = (data.records || []) as DataRecordResponse[];
      setRecords([...records(), ...newRecords]);
      setHasMore(newRecords.length === PAGE_SIZE);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function addRecord(data: Record<string, unknown>) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    try {
      setLoading(true);
      await ws.nodes.table.records.createRecord(nodeId, { data });
      await loadRecords(nodeId);
      setError(null);
    } catch (err) {
      setError(`${t('errors.failedToCreate')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function updateRecord(recordId: string, data: Record<string, unknown>) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    try {
      setLoading(true);
      await ws.nodes.table.records.updateRecord(nodeId, recordId, { data });
      await loadRecords(nodeId);
      setError(null);
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function deleteRecord(recordId: string) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;
    if (!confirm(t('table.confirmDeleteRecord') || 'Delete this record?')) return;

    try {
      setLoading(true);
      await ws.nodes.table.records.deleteRecord(nodeId, recordId);
      await loadRecords(nodeId);
      setError(null);
    } catch (err) {
      setError(`${t('errors.failedToDelete')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function createView(name: string, type: ViewType) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    try {
      setLoading(true);
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
      // No need to reload records as filters are empty
    } catch (err) {
      setError(`${t('errors.failedToCreate')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function updateView(viewId: string, updates: Partial<View>) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    try {
      setLoading(true);
      // We only send the fields that are updated.
      // The API expects UpdateViewRequest which matches Partial<View> structure mostly.
      // But we need to map View properties to UpdateViewRequest properties if they differ.
      // They are identical in our DTOs.
      await ws.nodes.views.updateView(nodeId, viewId, updates);

      setViews(views().map((v) => (v.id === viewId ? { ...v, ...updates } : v)));

      // If updating active view's filters/sorts, update local state
      if (viewId === activeViewId()) {
        if (updates.filters) setActiveFilters(updates.filters);
        if (updates.sorts) setActiveSorts(updates.sorts);
        // Reload records if filters/sorts changed
        if (updates.filters || updates.sorts) {
          await loadRecords(nodeId);
        }
      }
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function deleteView(viewId: string) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    if (!confirm(t('table.confirmDeleteView') || 'Delete this view?')) return;

    try {
      setLoading(true);
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
    } catch (err) {
      setError(`${t('errors.failedToDelete')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  const value: RecordsContextValue = {
    records,
    hasMore,
    views,
    activeViewId,
    activeFilters,
    activeSorts,
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
