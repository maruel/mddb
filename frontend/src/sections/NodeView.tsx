// Node view component displaying editor or table based on node type.

import { createEffect, createMemo, Show, For, Suspense, lazy } from 'solid-js';
import { useParams, useNavigate } from '@solidjs/router';
import TableTable from '../components/TableTable';
import TableGrid from '../components/TableGrid';
import TableGallery from '../components/TableGallery';
import TableBoard from '../components/TableBoard';
import ViewTabs from '../components/table/ViewTabs';
import { useAuth, useWorkspace, useEditor, useRecords } from '../contexts';
import { useI18n } from '../i18n';
import { nodeUrl, stripSlug } from '../utils/urls';
import type { Property } from '@sdk/types.gen';
import styles from './WorkspaceSection.module.css';

const Editor = lazy(() => import('../components/editor/Editor'));

export default function NodeView() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const params = useParams<{ wsId: string; nodeId: string }>();
  const { user, token, wsApi } = useAuth();
  const { selectedNodeId, selectedNodeData, setLoading, setError, loadNode } = useWorkspace();
  const {
    title,
    content,
    hasUnsavedChanges,
    autoSaveStatus,
    assetUrls,
    showHistory,
    history,
    loadVersion,
    flushAutoSave,
    handleTitleChange,
    handleContentChange,
    linkedNodeTitles,
  } = useEditor();
  const { records, hasMore, loadMoreRecords, addRecord, updateRecord, deleteRecord, views, activeViewId } =
    useRecords();

  // Derive view type from active view in RecordsContext
  const activeView = createMemo(() => views().find((v) => v.id === activeViewId()));
  const viewType = createMemo(() => activeView()?.type || 'table');

  // Load node when nodeId param changes
  createEffect(() => {
    const nodeId = stripSlug(params.nodeId);
    if (nodeId && nodeId !== selectedNodeId()) {
      flushAutoSave();
      loadNode(nodeId);
    }
  });

  // Handle adding a column to table
  async function handleAddColumn(column: Property) {
    const nodeId = selectedNodeId();
    const nodeData = selectedNodeData();
    const ws = wsApi();
    if (!nodeId || !nodeData || !ws) return;

    try {
      setLoading(true);
      const currentProperties: Property[] = nodeData.properties || [];
      const updatedProperties: Property[] = [...currentProperties, column];

      await ws.nodes.table.updateTable(nodeId, {
        title: nodeData.title,
        properties: updatedProperties,
      });

      // Reload node to get updated schema
      await loadNode(nodeId);
      setError(null);
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  // Handle navigation to another node from editor links
  const handleNavigateToNode = (nodeId: string) => {
    flushAutoSave();
    const u = user();
    const wsId = u?.workspace_id;
    const wsName = u?.workspace_name;
    if (wsId) {
      navigate(nodeUrl(wsId, wsName, nodeId));
    }
  };

  return (
    <>
      <Show when={selectedNodeId()}>
        <div class={styles.editor}>
          <div class={styles.editorHeader}>
            <input
              type="text"
              placeholder={t('editor.titlePlaceholder') || 'Title'}
              value={title()}
              onInput={(e) => handleTitleChange(e.target.value)}
              class={styles.titleInput}
            />
            <div class={styles.editorStatus}>
              <Show when={hasUnsavedChanges() && autoSaveStatus() === 'idle'}>
                <span class={styles.unsavedIndicator}>● {t('editor.unsaved')}</span>
              </Show>
              <Show when={autoSaveStatus() === 'saving'}>
                <span class={styles.savingIndicator}>⟳ {t('common.saving')}</span>
              </Show>
              <Show when={autoSaveStatus() === 'saved'}>
                <span class={styles.savedIndicator}>✓ {t('common.saved')}</span>
              </Show>
            </div>
          </div>

          <Show when={showHistory()}>
            <div class={styles.historyPanel}>
              <h3>{t('editor.versionHistory')}</h3>
              <ul class={styles.historyList}>
                <For each={history()}>
                  {(commit) => (
                    <li
                      class={styles.historyItem}
                      onClick={() => {
                        const id = selectedNodeId();
                        if (id) loadVersion(id, commit.hash);
                      }}
                    >
                      <div class={styles.historyMeta}>
                        <span class={styles.historyDate}>{new Date(commit.timestamp).toLocaleString()}</span>
                        <span class={styles.historyHash}>{commit.hash.substring(0, 7)}</span>
                      </div>
                      <div class={styles.historyMessage}>{commit.message}</div>
                    </li>
                  )}
                </For>
                <Show when={history().length === 0}>
                  <li class={styles.historyItem}>{t('editor.noHistory')}</li>
                </Show>
              </ul>
            </div>
          </Show>

          <div class={styles.nodeContent}>
            <Show when={selectedNodeData()?.has_page}>
              <Suspense fallback={<div class={styles.editorLoading}>{t('common.loading')}</div>}>
                <Editor
                  content={content()}
                  nodeId={selectedNodeId() ?? undefined}
                  assetUrls={assetUrls()}
                  linkedNodeTitles={linkedNodeTitles()}
                  onChange={handleContentChange}
                  wsId={user()?.workspace_id}
                  getToken={() => token()}
                  onAssetUploaded={() => {
                    const nodeId = selectedNodeId();
                    if (nodeId) loadNode(nodeId);
                  }}
                  onError={setError}
                  onNavigateToNode={handleNavigateToNode}
                />
              </Suspense>
            </Show>

            <Show when={selectedNodeData()?.has_table}>
              <div class={styles.tableView}>
                <ViewTabs />
                <Show when={viewType() === 'table'}>
                  <TableTable
                    tableId={selectedNodeId() || ''}
                    columns={selectedNodeData()?.properties || []}
                    records={records()}
                    onAddRecord={addRecord}
                    onUpdateRecord={updateRecord}
                    onDeleteRecord={deleteRecord}
                    onAddColumn={handleAddColumn}
                    onLoadMore={loadMoreRecords}
                    hasMore={hasMore()}
                  />
                </Show>
                <Show when={viewType() === 'list'}>
                  <TableGrid
                    records={records()}
                    columns={selectedNodeData()?.properties || []}
                    onUpdateRecord={updateRecord}
                    onDeleteRecord={deleteRecord}
                  />
                </Show>
                <Show when={viewType() === 'gallery'}>
                  <TableGallery
                    records={records()}
                    columns={selectedNodeData()?.properties || []}
                    onUpdateRecord={updateRecord}
                    onDeleteRecord={deleteRecord}
                  />
                </Show>
                <Show when={viewType() === 'board'}>
                  <TableBoard
                    records={records()}
                    columns={selectedNodeData()?.properties || []}
                    onUpdateRecord={updateRecord}
                    onDeleteRecord={deleteRecord}
                  />
                </Show>
              </div>
            </Show>
          </div>
        </div>
      </Show>
    </>
  );
}
