// Node view component displaying editor or table based on node type.

import { createEffect, createMemo, Show, For, Suspense, lazy, createSignal } from 'solid-js';
import { useParams, useNavigate } from '@solidjs/router';
import TableTable from '../components/TableTable';
import TableGrid from '../components/TableGrid';
import TableGallery from '../components/TableGallery';
import TableBoard from '../components/TableBoard';
import ViewTabs from '../components/table/ViewTabs';
import MarkdownPreview from '../components/MarkdownPreview';
import { useAuth, useWorkspace, useEditor, useRecords } from '../contexts';
import { useI18n } from '../i18n';
import { nodeUrl, stripSlug } from '../utils/urls';
import type { Property, Commit } from '@sdk/types.gen';
import styles from './WorkspaceSection.module.css';

const Editor = lazy(() => import('../components/editor/Editor'));

export default function NodeView() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const params = useParams<{ wsId: string; nodeId: string }>();
  const { user, token, wsApi } = useAuth();
  const { selectedNodeId, selectedNodeData, setSavingNodeId, setLoadError, setSaveError, loadNode } = useWorkspace();
  const {
    title,
    content,
    hasUnsavedChanges,
    autoSaveStatus,
    assetUrls,
    showHistory,
    history,
    flushAutoSave,
    handleTitleChange,
    handleContentChange,
    linkedNodeTitles,
  } = useEditor();
  const { records, hasMore, loadMoreRecords, addRecord, updateRecord, deleteRecord, views, activeViewId } =
    useRecords();

  const [previewContent, setPreviewContent] = createSignal<string | null>(null);
  const [previewCommit, setPreviewCommit] = createSignal<Commit | null>(null);

  // Derive view type from active view in RecordsContext
  const activeView = createMemo(() => views().find((v) => v.id === activeViewId()));
  const viewType = createMemo(() => activeView()?.type || 'table');

  // Load node when nodeId param changes
  createEffect(() => {
    const nodeId = stripSlug(params.nodeId);
    if (nodeId && nodeId !== selectedNodeId()) {
      flushAutoSave();
      loadNode(nodeId);
      setPreviewContent(null);
      setPreviewCommit(null);
    }
  });

  // Clear preview when history is hidden, and auto-load first item when shown
  createEffect(() => {
    if (!showHistory()) {
      setPreviewContent(null);
      setPreviewCommit(null);
    } else if (history().length > 0 && !previewCommit()) {
      const id = selectedNodeId();
      const first = history()[0];
      if (id && first) {
        loadPreview(id, first);
      }
    }
  });

  // Local loading state for preview (operation-specific) - can be used for UI feedback
  const [_loadingPreview, setLoadingPreview] = createSignal(false);

  // Handle adding a column to table
  async function handleAddColumn(column: Property) {
    const nodeId = selectedNodeId();
    const nodeData = selectedNodeData();
    const ws = wsApi();
    if (!nodeId || !nodeData || !ws) return;

    try {
      setSavingNodeId(nodeId);
      const currentProperties: Property[] = nodeData.properties || [];
      const updatedProperties: Property[] = [...currentProperties, column];

      await ws.nodes.table.updateTable(nodeId, {
        title: nodeData.title,
        properties: updatedProperties,
      });

      // Reload node to get updated schema
      await loadNode(nodeId);
      setSaveError(null);
    } catch (err) {
      setSaveError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setSavingNodeId(null);
    }
  }

  async function loadPreview(nodeId: string, commit: Commit) {
    const ws = wsApi();
    if (!ws) return;
    try {
      setLoadingPreview(true);
      const data = await ws.nodes.history.getNodeVersion(nodeId, commit.hash);
      setPreviewContent(data.content || '');
      setPreviewCommit(commit);
    } catch (err) {
      setLoadError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoadingPreview(false);
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
          <Show when={!previewCommit()}>
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
          </Show>

          <Show when={showHistory()}>
            <div class={styles.historyPanel}>
              <h3>{t('editor.versionHistory')}</h3>
              <ul class={styles.historyList}>
                <For each={history()}>
                  {(commit) => (
                    <li
                      class={styles.historyItem}
                      classList={{ [styles.activeHistoryItem as string]: previewCommit()?.hash === commit.hash }}
                      onClick={() => {
                        const id = selectedNodeId();
                        if (id) loadPreview(id, commit);
                      }}
                    >
                      <div class={styles.historyMeta}>
                        <span class={styles.historyDate}>{new Date(commit.timestamp).toLocaleString()}</span>
                        <span class={styles.historyHash}>{commit.hash.substring(0, 7)}</span>
                      </div>
                      <div class={styles.historyMessage}>
                        {commit.author_name} &lt;{commit.author_email}&gt;
                      </div>
                    </li>
                  )}
                </For>
                <Show when={history().length === 0}>
                  <li class={styles.historyItem}>{t('editor.noHistory')}</li>
                </Show>
              </ul>
            </div>
          </Show>

          <Show when={!previewCommit()}>
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
                    onError={setSaveError}
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
          </Show>

          <Show when={previewContent() !== null && previewCommit()}>
            <div class={styles.previewPane}>
              <div class={styles.previewHeader}>
                <div class={styles.previewInfo}>
                  <h4>{t('editor.versionPreview')}</h4>
                  <span class={styles.previewMeta}>
                    {previewCommit()?.author_name} • {new Date(previewCommit()?.timestamp || 0).toLocaleString()}
                  </span>
                </div>
                <button
                  onClick={() => {
                    setPreviewContent(null);
                    setPreviewCommit(null);
                  }}
                  class={styles.closePreviewButton}
                >
                  {t('editor.closePreview')}
                </button>
              </div>
              <div class={styles.previewContent}>
                <MarkdownPreview content={previewContent() || ''} assetUrls={assetUrls()} />
              </div>
            </div>
          </Show>
        </div>
      </Show>
    </>
  );
}
