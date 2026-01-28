// Records context providing table records CRUD and pagination.

import { createContext, useContext, createSignal, createEffect, type ParentComponent, type Accessor } from 'solid-js';
import { useAuth } from './AuthContext';
import { useWorkspace } from './WorkspaceContext';
import { useI18n } from '../i18n';
import type { DataRecordResponse } from '@sdk/types.gen';

const PAGE_SIZE = 50;

interface RecordsContextValue {
  records: Accessor<DataRecordResponse[]>;
  hasMore: Accessor<boolean>;
  loadRecords: (nodeId: string) => Promise<void>;
  loadMoreRecords: () => Promise<void>;
  addRecord: (data: Record<string, unknown>) => Promise<void>;
  updateRecord: (recordId: string, data: Record<string, unknown>) => Promise<void>;
  deleteRecord: (recordId: string) => Promise<void>;
  clearRecords: () => void;
}

const RecordsContext = createContext<RecordsContextValue>();

export const RecordsProvider: ParentComponent = (props) => {
  const { t } = useI18n();
  const { wsApi } = useAuth();
  const { selectedNodeId, selectedNodeData, loading, setLoading, setError } = useWorkspace();

  const [records, setRecords] = createSignal<DataRecordResponse[]>([]);
  const [hasMore, setHasMore] = createSignal(false);

  // Load records when selected node changes and has table content
  createEffect(() => {
    const node = selectedNodeData();
    if (node?.has_table) {
      loadRecords(node.id);
    } else {
      clearRecords();
    }
  });

  function clearRecords() {
    setRecords([]);
    setHasMore(false);
  }

  async function loadRecords(nodeId: string) {
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      const data = await ws.nodes.table.records.listRecords(nodeId, { Offset: 0, Limit: PAGE_SIZE });
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
      const data = await ws.nodes.table.records.listRecords(nodeId, { Offset: offset, Limit: PAGE_SIZE });
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

  const value: RecordsContextValue = {
    records,
    hasMore,
    loadRecords,
    loadMoreRecords,
    addRecord,
    updateRecord,
    deleteRecord,
    clearRecords,
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
