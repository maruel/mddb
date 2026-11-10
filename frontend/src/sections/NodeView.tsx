// Owns live editing, conflict recovery, and historical previews under their departure intent.
import { createEffect, createMemo, Show, For, Suspense, lazy, createSignal, on, untrack } from "solid-js";
import { useParams, useNavigate, useSearchParams } from "@solidjs/router";
import TableTable from "../components/TableTable";
import TableList from "../components/TableList";
import TableGallery from "../components/TableGallery";
import TableBoard from "../components/TableBoard";
import RecordDetail from "../components/RecordDetail";
import ViewTabs from "../components/table/ViewTabs";
import { AddColumnDropdown } from "../components/table/AddColumnDropdown";
import MarkdownPreview from "../components/MarkdownPreview";
import { ConflictRecovery } from "../components/editor/ConflictRecovery";
import { PageHeader } from "../components/editor/PageHeader";
import { useAssetUpload } from "../components/editor/useAssetUpload";
import { useAuthScope, useAuth, useWorkspace, useEditor, useRecords, DEFAULT_VIEW_ID } from "../contexts";
import { useI18n } from "../i18n";
import { nodeUrl, stripSlug } from "../utils/urls";
import { relativeLinksToSpaUrls } from "../utils/markdown-utils";
import type { EditorIntent } from "../contexts/EditorContext";
import type { Property, Commit } from "@sdk/types.gen";
import styles from "./WorkspaceSection.module.css";
import EditIcon from "@material-symbols/svg-400/outlined/edit.svg?solid";
import SyncIcon from "@material-symbols/svg-400/outlined/sync.svg?solid";
import CloudDoneIcon from "@material-symbols/svg-400/outlined/cloud_done.svg?solid";

const Editor = lazy(() => import("../components/editor/Editor"));

export default function NodeView() {
  const { t } = useI18n();
  let titleInput: HTMLInputElement | undefined;
  const navigate = useNavigate();
  const params = useParams<{ wsId: string; nodeId: string }>();
  const { user, wsApi } = useAuth();
  const captureScope = useAuthScope();
  const {
    selectedNodeId,
    selectedNodeData,
    setSelectedNodeData,
    savingNodeId,
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
    handleTitleChange,
    handleContentChange,
    linkedNodeTitles,
    externalChange,
    saveIssue,
    comparisonBlocked,
    editingLocked,
    copyDraft,
    compareChanges,
    dismissExternalChange,
    refreshFromServer,
    icon,
    cover,
    handleIconChange,
    handleCoverChange,
    beginIntent,
    runAsyncEdit,
    asyncEditPending,
  } = useEditor();

  const coverSession = user()?.session_id;
  const coverUpload = useAssetUpload({
    sessionID: coverSession,
    get wsId() {
      return user()?.workspace_id ?? "";
    },
    get nodeId() {
      return selectedNodeId() ?? "";
    },
  });
  const {
    records,
    failedRecordDrafts,
    dismissRecordDraft,
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
  const clearPreview = () => {
    setPreviewContent(null);
    setPreviewCommit(null);
  };
  const [openRecordId, setOpenRecordId] = createSignal<string | null>(null);

  // Derive view type from active view in RecordsContext
  const activeView = createMemo(() => views().find((v) => v.id === activeViewId()));
  const viewType = createMemo(() => activeView()?.type || "table");

  // Signal: true once the URL ?view= param has been applied for the current node.
  // This gates the URL sync effect so it cannot fire before initialization is complete,
  // preventing it from clearing the incoming ?view= param during node transitions.
  const [viewParamApplied, setViewParamApplied] = createSignal(false);

  // 1. Reset on every node navigation so the apply effect runs again for the new node.
  createEffect(
    on(
      () => stripSlug(params.nodeId),
      () => {
        setViewParamApplied(false);
        clearPreview();
      },
    ),
  );

  // 2. Apply URL ?view= param and track back/forward navigation.
  // Pre-init: tracks views() to wait for them to load, applies the initial URL param, then
  // marks initialization done. Post-init: tracks only searchParams.view for back/forward —
  // views() is untracked so createView() additions don't re-trigger stale URL param application.
  createEffect(() => {
    const paramNodeId = stripSlug(params.nodeId);
    const nodeId = selectedNodeId();
    if (!paramNodeId || !nodeId || paramNodeId !== nodeId) return;
    const paramId = searchParams.view; // Always tracked for back/forward
    if (!viewParamApplied()) {
      const loadedViews = views(); // Tracked only pre-init
      if (loadedViews.length === 0) return;
      if (paramId && loadedViews.some((v) => v.id === paramId)) {
        setActiveViewId(paramId);
      }
      setViewParamApplied(true);
      return;
    }
    // Post-init: handle back/forward navigation; untrack views/activeViewId to avoid
    // re-running when createView() or deleteView() mutates views().
    if (paramId) {
      const loadedViews = untrack(() => views());
      if (loadedViews.some((v) => v.id === paramId) && paramId !== untrack(() => activeViewId())) {
        setActiveViewId(paramId);
      }
    }
  });

  // 3. Sync activeViewId → URL. Gated on viewParamApplied so it only runs after the
  // apply effect above has finished, preventing it from clearing the incoming ?view= param.
  createEffect(() => {
    if (!viewParamApplied()) return;
    const id = activeViewId();
    if (!id || id === DEFAULT_VIEW_ID) {
      setSearchParams({ view: undefined });
    } else {
      setSearchParams({ view: id });
    }
  });

  // Load node when nodeId param changes
  createEffect(
    on(
      () => stripSlug(params.nodeId),
      (nodeId) => {
        if (nodeId) {
          void loadNode(nodeId, {
            refresh: false,
            navigation: true,
            canApply: () => stripSlug(params.nodeId) === nodeId,
          });
        }
      },
    ),
  );

  // Opening history loads its first version; dismissing a preview must not reopen it.
  createEffect(
    on([showHistory, history], ([visible, snapshot]) => {
      clearPreview();
      const id = selectedNodeId();
      const first = snapshot?.commits[0];
      if (visible && id && first) void loadPreview(id, first, snapshot.intent);
    }),
  );

  async function updateTableProperties(nodeId: string, updatedProperties: Property[]) {
    const isCurrent = captureScope();
    const nodeData = selectedNodeData();
    const ws = wsApi();
    if (!nodeData || nodeData.id !== nodeId || selectedNodeId() !== nodeId || !ws) return;
    if (!nodeData.schema_revision) {
      setSaveError(t("errors.failedToSave"));
      return;
    }
    const ownsNode = () => isCurrent() && selectedNodeId() === nodeId && selectedNodeData() === nodeData;
    try {
      setSavingNodeId(nodeId);
      const saved = await ws.updateTable(nodeId, {
        title: nodeData.title,
        properties: updatedProperties,
        expected_title: nodeData.title,
        expected_schema_revision: nodeData.schema_revision,
      });
      if (!ownsNode()) return;
      setSelectedNodeData({
        ...nodeData,
        title: saved.title,
        properties: saved.properties ?? undefined,
        views: saved.views ?? undefined,
        schema_revision: saved.schema_revision,
      });
      setSaveError(null);
    } catch (err) {
      if (ownsNode()) setSaveError(`${t("errors.failedToSave")}: ${String(err)}`);
    } finally {
      if (isCurrent() && savingNodeId() === nodeId) setSavingNodeId(null);
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
    const newColumn: Property = { name: "New Column", type: "text", required: false };
    currentProperties.splice(beforeIndex, 0, newColumn);
    await updateTableProperties(nodeId, currentProperties);
  }

  const handleUploadCover = (file: File): Promise<void> =>
    runAsyncEdit(async () => {
      const isCurrent = captureScope();
      const nodeId = selectedNodeId();
      const result = await coverUpload.uploadFile(file);
      if (!isCurrent() || selectedNodeId() !== nodeId) return false;
      if (!result) {
        const err = coverUpload.error();
        throw new Error(err ?? t("errors.failedToSave"));
      }
      if (!(await handleCoverChange(result.name))) return false;
      if (!isCurrent() || selectedNodeId() !== nodeId) return false;
      // Reload the originating node to get updated asset URLs
      if (nodeId)
        void loadNode(nodeId, { refresh: false }).catch((backgroundError: unknown) => reportError(backgroundError));
      return true;
    });

  async function loadPreview(nodeId: string, commit: Commit, intent: EditorIntent) {
    const isCurrent = captureScope();
    const ws = wsApi();
    const wsId = user()?.workspace_id;
    if (
      !ws ||
      !wsId ||
      !showHistory() ||
      editingLocked() ||
      selectedNodeId() !== nodeId ||
      stripSlug(params.nodeId) !== nodeId
    )
      return;
    const ownsPreview = () =>
      isCurrent() &&
      intent.current() &&
      showHistory() &&
      !editingLocked() &&
      selectedNodeId() === nodeId &&
      stripSlug(params.nodeId) === nodeId;
    try {
      // Preview retires the live editor. Drain admitted work both before fetching
      // and before applying: uploads and newer drafts may arrive during the read.
      if (!(await intent.drain()) || !ownsPreview()) return;
      const data = await ws.getNodeVersion(nodeId, commit.hash);
      if (!ownsPreview() || !(await intent.drain()) || !ownsPreview()) return;
      setPreviewContent(relativeLinksToSpaUrls(data.content || "", wsId));
      setPreviewCommit(commit);
    } catch (err) {
      if (ownsPreview()) setLoadError(`${t("errors.failedToLoad")}: ${String(err)}`);
    }
  }

  // Handle navigation to another node from editor links
  const handleNavigateToNode = (nodeId: string) => {
    const u = user();
    const wsId = u?.workspace_id;
    const wsName = u?.workspace_name;
    if (wsId) {
      navigate(nodeUrl(wsId, wsName, nodeId));
    }
  };

  return (
    <>
      <ConflictRecovery onSaved={() => titleInput?.focus()} />
      <Show when={selectedNodeId()}>
        <div class={styles.editor} inert={editingLocked()} aria-busy={editingLocked()}>
          <Show when={externalChange() || saveIssue() !== null || comparisonBlocked()}>
            <div class={styles.conflictBanner}>
              <span>
                {saveIssue() === "unknown"
                  ? t("editor.saveUnknown")
                  : saveIssue() === "conflict"
                    ? t("editor.saveConflict")
                    : comparisonBlocked()
                      ? t("editor.reviewRequired")
                      : t("sse.externalChange")}
              </span>
              <Show
                when={
                  (hasUnsavedChanges() || saveIssue() === "conflict" || comparisonBlocked()) &&
                  saveIssue() !== "unknown"
                }
              >
                <button onClick={compareChanges}>{t("editor.compareChanges")}</button>
              </Show>
              <Show when={saveIssue() !== null || comparisonBlocked()}>
                <button onClick={copyDraft}>{t("editor.copyDraft")}</button>
              </Show>
              <button
                onClick={() => {
                  beginIntent();
                  refreshFromServer();
                }}
                disabled={asyncEditPending()}
              >
                {saveIssue() !== null || comparisonBlocked() ? t("editor.reloadLatest") : t("sse.refreshContent")}
              </button>
              <Show when={saveIssue() === null && !comparisonBlocked()}>
                <button onClick={dismissExternalChange}>{t("sse.dismissNotice")}</button>
              </Show>
            </div>
          </Show>
          <Show when={!previewCommit()}>
            <Show when={selectedNodeData()?.has_page}>
              <PageHeader
                icon={icon()}
                cover={cover()}
                coverUrl={cover() ? (assetUrls()[cover()] ?? "") : ""}
                onIconChange={handleIconChange}
                onCoverChange={handleCoverChange}
                onUploadCover={handleUploadCover}
              />
            </Show>
            <div class={styles.editorHeader}>
              <input
                type="text"
                ref={(el) => (titleInput = el)}
                placeholder={t("editor.titlePlaceholder") || "Title"}
                value={title()}
                disabled={editingLocked()}
                onInput={(e) => handleTitleChange(e.target.value)}
                class={styles.titleInput}
              />
              <div class={styles.editorStatus}>
                <Show when={hasUnsavedChanges() && autoSaveStatus() === "idle"}>
                  <span class={styles.unsavedIndicator} role="status" aria-live="polite" aria-atomic="true">
                    <EditIcon aria-hidden="true" />
                    <span>{t("editor.unsaved") || "Unsaved"}</span>
                  </span>
                </Show>
                <Show when={autoSaveStatus() === "saving"}>
                  <span class={styles.savingIndicator} role="status" aria-live="polite" aria-atomic="true">
                    <SyncIcon aria-hidden="true" />
                    <span>{t("common.saving") || "Saving..."}</span>
                  </span>
                </Show>
                <Show when={autoSaveStatus() === "saved"}>
                  <span class={styles.savedIndicator} role="status" aria-live="polite" aria-atomic="true">
                    <CloudDoneIcon aria-hidden="true" />
                    <span>{t("common.saved") || "Saved"}</span>
                  </span>
                </Show>
                <Show when={autoSaveStatus() === "error"}>
                  <span class={styles.saveErrorIndicator} role="status" aria-live="assertive" aria-atomic="true">
                    <span aria-hidden="true">!</span>
                    <span>{t("errors.autoSaveFailed") || "Auto-save failed"}</span>
                  </span>
                </Show>
              </div>
            </div>
          </Show>

          <Show when={showHistory()}>
            <div class={styles.historyPanel}>
              <h3>{t("editor.versionHistory")}</h3>
              <ul class={styles.historyList}>
                <For each={history()?.commits}>
                  {(commit) => (
                    <li
                      class={styles.historyItem}
                      classList={{ [styles.activeHistoryItem]: previewCommit()?.hash === commit.hash }}
                    >
                      <button
                        type="button"
                        class={styles.historyButton}
                        aria-pressed={previewCommit()?.hash === commit.hash}
                        onClick={() => {
                          const id = selectedNodeId();
                          if (id && !editingLocked()) void loadPreview(id, commit, beginIntent());
                        }}
                      >
                        <div class={styles.historyMeta}>
                          <span class={styles.historyDate}>{new Date(commit.timestamp).toLocaleString()}</span>
                          <span class={styles.historyHash}>{commit.hash.substring(0, 7)}</span>
                        </div>
                        <div class={styles.historyMessage}>
                          {commit.author_name} &lt;{commit.author_email}&gt;
                        </div>
                      </button>
                    </li>
                  )}
                </For>
                <Show when={history()?.commits.length === 0}>
                  <li class={styles.noHistory}>{t("editor.noHistory")}</li>
                </Show>
              </ul>
            </div>
          </Show>

          <Show when={!previewCommit()}>
            <div class={styles.nodeContent}>
              <Show when={selectedNodeData()?.has_page}>
                <Suspense fallback={<div class={styles.editorLoading}>{t("common.loading")}</div>}>
                  <Editor
                    readOnly={editingLocked()}
                    content={content()}
                    nodeId={selectedNodeId() ?? undefined}
                    assetUrls={assetUrls()}
                    linkedNodeTitles={linkedNodeTitles()}
                    onChange={handleContentChange}
                    sessionID={user()?.session_id}
                    wsId={user()?.workspace_id}
                    onAssetUploaded={() => {
                      const nodeId = selectedNodeId();
                      if (nodeId)
                        void loadNode(nodeId, { refresh: false }).catch((backgroundError: unknown) =>
                          reportError(backgroundError),
                        );
                    }}
                    runAsyncEdit={runAsyncEdit}
                    asyncEditPending={asyncEditPending()}
                    onNavigateToNode={handleNavigateToNode}
                  />
                </Suspense>
              </Show>

              <Show when={selectedNodeData()?.has_table}>
                <div class={styles.tableView}>
                  <For each={failedRecordDrafts()}>
                    {(draft) => (
                      <section class={styles.recordDraft} role="alert">
                        <p>{t("errors.recordDraft")}</p>
                        <pre class={styles.recordDraftContent}>{draft.data}</pre>
                        <button type="button" onClick={() => dismissRecordDraft(draft.id)}>
                          {t("common.close")}
                        </button>
                      </section>
                    )}
                  </For>
                  <div class={styles.viewBar}>
                    <ViewTabs />
                    <Show when={viewType() !== "table"}>
                      <div class={styles.viewBarAddField}>
                        <AddColumnDropdown onAddColumn={handleAddColumn} />
                      </div>
                    </Show>
                  </div>
                  <Show when={viewType() === "table"}>
                    <TableTable
                      tableId={selectedNodeId() || ""}
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
                      onUpdateColumns={(cols) => {
                        const nodeId = selectedNodeId();
                        if (nodeId) return updateTableProperties(nodeId, cols);
                        return Promise.resolve();
                      }}
                      onLoadMore={loadMoreRecords}
                      hasMore={hasMore()}
                      onOpenRecord={(id) => setOpenRecordId(id)}
                    />
                  </Show>
                  <Show when={viewType() === "list"}>
                    <TableList
                      records={records()}
                      columns={selectedNodeData()?.properties || []}
                      onAddRecord={() => addRecord({})}
                      onUpdateRecord={updateRecord}
                      onDeleteRecord={deleteRecord}
                      onDuplicateRecord={duplicateRecord}
                      onOpenRecord={(id) => setOpenRecordId(id)}
                    />
                  </Show>
                  <Show when={viewType() === "calendar"}>
                    <div class={styles.calendarPlaceholder}>{t("table.calendarNotAvailable")}</div>
                  </Show>
                  <Show when={viewType() === "gallery"}>
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
                  <Show when={viewType() === "board"}>
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
                        if (id)
                          void updateView(id, { groups: [{ property: columnName }] }).catch(
                            (backgroundError: unknown) => reportError(backgroundError),
                          );
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
                  <h4>{t("editor.versionPreview")}</h4>
                  <span class={styles.previewMeta}>
                    {previewCommit()?.author_name} • {new Date(previewCommit()?.timestamp || 0).toLocaleString()}
                  </span>
                </div>
                <button
                  onClick={() => {
                    beginIntent();
                    clearPreview();
                  }}
                  class={styles.closePreviewButton}
                >
                  {t("editor.closePreview")}
                </button>
              </div>
              <div class={styles.previewContent}>
                <MarkdownPreview content={previewContent() || ""} assetUrls={assetUrls()} onNavigate={navigate} />
              </div>
            </div>
          </Show>
        </div>
      </Show>
    </>
  );
}
