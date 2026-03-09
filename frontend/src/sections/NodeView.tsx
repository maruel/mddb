// Node view component displaying editor or table based on node type.

import { createEffect, createMemo, Show, For, Suspense, lazy, createSignal, on, untrack } from 'solid-js';
import { useParams, useNavigate, useSearchParams } from '@solidjs/router';
import TableTable from '../components/TableTable';
import TableGrid from '../components/TableGrid';
import TableGallery from '../components/TableGallery';
import TableBoard from '../components/TableBoard';
import RecordDetail from '../components/RecordDetail';
import ViewTabs from '../components/table/ViewTabs';
import { AddColumnDropdown } from '../components/table/AddColumnDropdown';
import MarkdownPreview from '../components/MarkdownPreview';
import { PageHeader } from '../components/editor/PageHeader';
import { useAssetUpload } from '../components/editor/useAssetUpload';
import { useAuth, useWorkspace, useEditor, useRecords, DEFAULT_VIEW_ID } from '../contexts';
import { useI18n } from '../i18n';
import { nodeUrl, stripSlug } from '../utils/urls';
import { relativeLinksToSpaUrls } from '../utils/markdown-utils';
import type { Property, Commit } from '@sdk/types.gen';
import styles from './WorkspaceSection.module.css';
import EditIcon from '@material-symbols/svg-400/outlined/edit.svg?solid';
import SyncIcon from '@material-symbols/svg-400/outlined/sync.svg?solid';
import CloudDoneIcon from '@material-symbols/svg-400/outlined/cloud_done.svg?solid';

const Editor = lazy(() => import('../components/editor/Editor'));

export default function NodeView() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const params = useParams<{ wsId: string; nodeId: string }>();
  const { user, token, wsApi } = useAuth();
  const {
    selectedNodeId,
    selectedNodeData,
    setSelectedNodeData,
    setSavingNodeId,
    setLoadError,
    setSaveError,
    loadNode,
  } = useWorkspace();
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
    externalChange,
    dismissExternalChange,
    refreshFromServer,
    icon,
    cover,
    handleIconChange,
    handleCoverChange,
  } = useEditor();

  const coverUpload = useAssetUpload({
    get wsId() {
      return user()?.workspace_id ?? '';
    },
    get nodeId() {
      return selectedNodeId() ?? '';
    },
    getToken: () => token(),
  });
  const {
    records,
    hasMore,
    loadMoreRecords,
    addRecord,
    updateRecord,
    deleteRecord,
    duplicateRecord,
    views,
    activeViewId,
    setActiveViewId,
    updateView,
  } = useRecords();
  const [searchParams, setSearchParams] = useSearchParams<{ view?: string }>();

  const [previewContent, setPreviewContent] = createSignal<string | null>(null);
  const [previewCommit, setPreviewCommit] = createSignal<Commit | null>(null);
  const [openRecordId, setOpenRecordId] = createSignal<string | null>(null);

  // Derive view type from active view in RecordsContext
  const activeView = createMemo(() => views().find((v) => v.id === activeViewId()));
  const viewType = createMemo(() => activeView()?.type || 'table');

  // Apply the URL ?view= param exactly once per node navigation (not on createView()).
  // A non-reactive variable tracks which nodeId we've already applied the param for,
  // preventing createView() from triggering a re-application that would override
  // the newly-created view's activeViewId with the stale URL param.
  let urlParamAppliedForNode: string | undefined;
  createEffect(
    on(
      () => stripSlug(params.nodeId),
      () => {
        urlParamAppliedForNode = undefined; // Reset on node navigation
      }
    )
  );
  createEffect(() => {
    const paramNodeId = stripSlug(params.nodeId);
    const nodeId = selectedNodeId();
    const loadedViews = views();
    // Only apply when the URL node matches the loaded node and views are ready.
    if (!paramNodeId || !nodeId || paramNodeId !== nodeId || loadedViews.length === 0) return;
    if (urlParamAppliedForNode === paramNodeId) return; // Already applied for this node
    urlParamAppliedForNode = paramNodeId;
    const paramId = untrack(() => searchParams.view);
    if (paramId && loadedViews.some((v) => v.id === paramId) && paramId !== untrack(() => activeViewId())) {
      setActiveViewId(paramId);
    }
  });

  // When the URL ?view= param changes due to browser back/forward navigation, apply it.
  createEffect(
    on(
      () => searchParams.view,
      (paramId) => {
        if (!paramId) return;
        const loadedViews = untrack(() => views());
        if (loadedViews.some((v) => v.id === paramId) && paramId !== untrack(() => activeViewId())) {
          setActiveViewId(paramId);
        }
      },
      { defer: true } // Initial mount is handled by the node-load effect above
    )
  );

  // Keep ?view= param in sync with the active view ID.
  // defer:true prevents this from running on initial mount with activeViewId()=undefined,
  // which would clear the incoming ?view= param before the apply effect above can read it.
  createEffect(
    on(
      () => activeViewId(),
      (id) => {
        if (!id || id === DEFAULT_VIEW_ID) {
          setSearchParams({ view: undefined });
        } else {
          setSearchParams({ view: id });
        }
      },
      { defer: true }
    )
  );

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

  async function updateTableProperties(nodeId: string, updatedProperties: Property[]) {
    const nodeData = selectedNodeData();
    const ws = wsApi();
    if (!nodeData || !ws) return;
    try {
      setSavingNodeId(nodeId);
      await ws.nodes.table.updateTable(nodeId, {
        title: nodeData.title,
        properties: updatedProperties,
      });
      // Update selectedNodeData directly — loadNode() skips re-fetching the current node
      // due to a loadedNodeId guard, so we apply the new properties optimistically.
      setSelectedNodeData({ ...nodeData, properties: updatedProperties });
      setSaveError(null);
    } catch (err) {
      setSaveError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setSavingNodeId(null);
    }
  }

  // Handle adding a column to table
  async function handleAddColumn(column: Property) {
    const nodeId = selectedNodeId();
    const nodeData = selectedNodeData();
    if (!nodeId || !nodeData) return;
    const currentProperties: Property[] = nodeData.properties || [];
    await updateTableProperties(nodeId, [...currentProperties, column]);
  }

  // Handle renaming or editing a column
  async function handleUpdateColumn(index: number, column: Property) {
    const nodeId = selectedNodeId();
    const nodeData = selectedNodeData();
    if (!nodeId || !nodeData) return;
    const currentProperties: Property[] = [...(nodeData.properties || [])];
    currentProperties[index] = column;
    await updateTableProperties(nodeId, currentProperties);
  }

  // Handle deleting a column
  async function handleDeleteColumn(index: number) {
    const nodeId = selectedNodeId();
    const nodeData = selectedNodeData();
    if (!nodeId || !nodeData) return;
    const currentProperties: Property[] = [...(nodeData.properties || [])];
    currentProperties.splice(index, 1);
    await updateTableProperties(nodeId, currentProperties);
  }

  // Handle reordering columns via drag-and-drop
  async function handleReorderColumns(newProperties: Property[]) {
    const nodeId = selectedNodeId();
    if (!nodeId) return;
    await updateTableProperties(nodeId, newProperties);
  }

  // Handle inserting a new text column at a position
  async function handleInsertColumn(beforeIndex: number) {
    const nodeId = selectedNodeId();
    const nodeData = selectedNodeData();
    if (!nodeId || !nodeData) return;
    const currentProperties: Property[] = [...(nodeData.properties || [])];
    const newColumn: Property = { name: 'New Column', type: 'text', required: false };
    currentProperties.splice(beforeIndex, 0, newColumn);
    await updateTableProperties(nodeId, currentProperties);
  }

  async function handleUploadCover(file: File): Promise<string> {
    const result = await coverUpload.uploadFile(file);
    if (!result) return '';
    // Reload node to get updated asset URLs
    const nodeId = selectedNodeId();
    if (nodeId) loadNode(nodeId);
    return result.name;
  }

  async function loadPreview(nodeId: string, commit: Commit) {
    const ws = wsApi();
    if (!ws) return;
    try {
      setLoadingPreview(true);
      const data = await ws.nodes.history.getNodeVersion(nodeId, commit.hash);
      const wsId = user()?.workspace_id || '';
      setPreviewContent(relativeLinksToSpaUrls(data.content || '', wsId));
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
          <Show when={externalChange()}>
            <div class={styles.conflictBanner}>
              <span>{t('sse.externalChange')}</span>
              <button onClick={refreshFromServer}>{t('sse.refreshContent')}</button>
              <button onClick={dismissExternalChange}>{t('sse.dismissNotice')}</button>
            </div>
          </Show>
          <Show when={!previewCommit()}>
            <Show when={selectedNodeData()?.has_page}>
              <PageHeader
                icon={icon()}
                cover={cover()}
                coverUrl={cover() ? (assetUrls()[cover()] ?? '') : ''}
                onIconChange={handleIconChange}
                onCoverChange={handleCoverChange}
                onUploadCover={handleUploadCover}
              />
            </Show>
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
                  <span class={styles.unsavedIndicator} title={t('editor.unsaved')}>
                    <EditIcon />
                  </span>
                </Show>
                <Show when={autoSaveStatus() === 'saving'}>
                  <span class={styles.savingIndicator} title={t('common.saving')}>
                    <SyncIcon />
                  </span>
                </Show>
                <Show when={autoSaveStatus() === 'saved'}>
                  <span class={styles.savedIndicator} title={t('common.saved')}>
                    <CloudDoneIcon />
                  </span>
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
                  <div class={styles.viewBar}>
                    <ViewTabs />
                    <Show when={viewType() !== 'table'}>
                      <div class={styles.viewBarAddField}>
                        <AddColumnDropdown onAddColumn={handleAddColumn} />
                      </div>
                    </Show>
                  </div>
                  <Show when={viewType() === 'table'}>
                    <TableTable
                      tableId={selectedNodeId() || ''}
                      columns={selectedNodeData()?.properties || []}
                      records={records()}
                      onAddRecord={addRecord}
                      onUpdateRecord={updateRecord}
                      onDeleteRecord={deleteRecord}
                      onDuplicateRecord={duplicateRecord}
                      onAddColumn={handleAddColumn}
                      onUpdateColumn={handleUpdateColumn}
                      onDeleteColumn={handleDeleteColumn}
                      onInsertColumn={handleInsertColumn}
                      onReorderColumns={handleReorderColumns}
                      onLoadMore={loadMoreRecords}
                      hasMore={hasMore()}
                      onOpenRecord={(id) => setOpenRecordId(id)}
                    />
                  </Show>
                  <Show when={viewType() === 'list'}>
                    <TableGrid
                      records={records()}
                      columns={selectedNodeData()?.properties || []}
                      onAddRecord={() => addRecord({})}
                      onUpdateRecord={updateRecord}
                      onDeleteRecord={deleteRecord}
                      onDuplicateRecord={duplicateRecord}
                      onOpenRecord={(id) => setOpenRecordId(id)}
                    />
                  </Show>
                  <Show when={viewType() === 'gallery'}>
                    <TableGallery
                      records={records()}
                      columns={selectedNodeData()?.properties || []}
                      onAddRecord={() => addRecord({})}
                      onUpdateRecord={updateRecord}
                      onDeleteRecord={deleteRecord}
                      onDuplicateRecord={duplicateRecord}
                      onOpenRecord={(id) => setOpenRecordId(id)}
                    />
                  </Show>
                  <Show when={viewType() === 'board'}>
                    <TableBoard
                      records={records()}
                      columns={selectedNodeData()?.properties || []}
                      onAddRecord={addRecord}
                      onUpdateRecord={updateRecord}
                      onDeleteRecord={deleteRecord}
                      onDuplicateRecord={duplicateRecord}
                      onOpenRecord={(id) => setOpenRecordId(id)}
                      groupByColumn={activeView()?.groups?.[0]?.property}
                      onGroupByChange={(columnName) => {
                        const id = activeViewId();
                        if (id) updateView(id, { groups: [{ property: columnName }] });
                      }}
                    />
                  </Show>
                </div>
              </Show>
            </div>
          </Show>
          <Show when={openRecordId()}>
            {(id) => (
              <RecordDetail
                recordId={id()}
                records={records()}
                columns={selectedNodeData()?.properties || []}
                onUpdate={updateRecord}
                onClose={() => setOpenRecordId(null)}
                onDelete={(rid) => deleteRecord(rid)}
                onDuplicate={(rid) => duplicateRecord(rid)}
              />
            )}
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
                <MarkdownPreview content={previewContent() || ''} assetUrls={assetUrls()} onNavigate={navigate} />
              </div>
            </div>
          </Show>
        </div>
      </Show>
    </>
  );
}
