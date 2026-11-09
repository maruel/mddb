// Owns asynchronous conditional saves, acknowledged merge bases, conflict recovery, and editor departure authority.

import {
  createContext,
  batch,
  useContext,
  createSignal,
  createEffect,
  onCleanup,
  untrack,
  type ParentComponent,
  type Accessor,
} from "solid-js";
import { useBeforeLeave, useLocation, useNavigate } from "@solidjs/router";
import { APIError } from "../useApi";
import { useAuth } from "./AuthContext";
import { useEventSource } from "./EventSourceContext";
import { useWorkspace } from "./WorkspaceContext";
import { useI18n } from "../i18n";
import { debounce } from "../utils/debounce";
import { nodeUrl, parseNodeUrl } from "../utils/urls";
import { extractLinkedNodeIds, relativeLinksToSpaUrls, spaUrlsToRelativeLinks } from "../utils/markdown-utils";
import { mergeText, type TextMerge } from "./editorMerge";
import { classifyExternalUpdate } from "./editorSync";
import { EventNodeUpdated, type Commit } from "@sdk/types.gen";
import { useUndo } from "../hooks/useUndo";

/** Map of asset filename to signed URL */
export type AssetUrlMap = Record<string, string>;

/** Map of node ID to title for resolving internal page links */
export type NodeTitleMap = Record<string, string>;

export interface RecoverySession {
  previousResult?: { title: string; content: string };
  base: { title: string; content: string };
  local: { title: string; content: string };
  saved: { title: string; content: string; revision: string };
  titleMerge: TextMerge;
  contentMerge: TextMerge;
}
// Only a user action begins authority. Its asynchronous children inherit it:
// draining or continuing navigation must never supersede a newer user action.
export interface EditorIntent {
  current: () => boolean;
  drain: () => Promise<boolean>;
  navigate: (path: string) => void;
}
interface HistorySnapshot {
  commits: Commit[];
  intent: EditorIntent;
}
// Follow-ups run only after their edit is removed from pending work and saved.
export type SavedEditContinuation = (intent: EditorIntent) => Promise<void>;
export type RunAsyncEdit = (edit: () => Promise<boolean>, afterSave?: SavedEditContinuation) => Promise<void>;

type RecoveryState =
  | { kind: "closed" }
  | { kind: "comparing" }
  | { kind: "reviewing"; session: RecoverySession; error: string | null }
  | { kind: "saving"; session: RecoverySession }
  | { kind: "failed"; error: string };

interface EditorContextValue {
  // Editor state
  title: Accessor<string>;
  setTitle: (title: string) => void;
  content: Accessor<string>;
  setContent: (content: string) => void;
  hasUnsavedChanges: Accessor<boolean>;
  setHasUnsavedChanges: (has: boolean) => void;
  autoSaveStatus: Accessor<"idle" | "saving" | "saved" | "error">;

  // Asset URLs (filename -> signed URL)
  assetUrls: Accessor<AssetUrlMap>;

  // Linked node titles (for resolving page link display text)
  linkedNodeTitles: Accessor<NodeTitleMap>;

  // History
  showHistory: Accessor<boolean>;
  history: Accessor<HistorySnapshot | null>;
  loadHistory: (nodeId: string, intent: EditorIntent) => Promise<void>;

  // External change detection (SSE)
  externalChange: Accessor<boolean>;
  dismissExternalChange: () => void;
  refreshFromServer: () => void;

  // Frontmatter (icon and cover)
  icon: Accessor<string>;
  cover: Accessor<string>;
  handleIconChange: (newIcon: string) => Promise<void>;
  handleCoverChange: (newCover: string) => Promise<boolean>;

  // Actions
  triggerAutoSave: () => void;
  beginIntent: () => EditorIntent;
  runAsyncEdit: RunAsyncEdit;
  prepareDeparture: () => Promise<EditorIntent | null>;
  asyncEditPending: Accessor<boolean>;
  saveIssue: Accessor<"conflict" | "unknown" | null>;
  editingLocked: Accessor<boolean>;
  copyDraft: () => Promise<void>;
  recovery: Accessor<RecoverySession | null>;
  recoveryLoading: Accessor<boolean>;
  recoveryError: Accessor<string | null>;
  compareChanges: () => Promise<void>;
  closeRecovery: () => void;
  saveRecovery: (session: RecoverySession, result: { title: string; content: string }) => Promise<boolean>;
  resetEditor: () => void;
  handleTitleChange: (newTitle: string) => void;
  handleContentChange: (newContent: string) => void;

  // Application-level undo/redo (title changes)
  undo: () => Promise<void>;
  redo: () => Promise<void>;
  canUndo: () => boolean;
  canRedo: () => boolean;
}

const EditorContext = createContext<EditorContextValue>();

export const EditorProvider: ParentComponent = (props) => {
  const { t } = useI18n();
  const { user, wsApi, identity: authIdentity } = useAuth();
  let disposed = false;
  const location = useLocation();
  let departure = 0;
  const navigate = useNavigate();
  let continuingIntent: EditorIntent | null = null;
  const {
    selectedNodeId,
    selectedNodeData,
    loadingNodeId,
    switchingWorkspace,
    deletingNodeId,
    updateNodeTitle,
    updateNodeIcon,
    loadNode,
    setLoadError,
    setSaveError,
  } = useWorkspace();
  const { lastEvent } = useEventSource();

  // Application-level undo stack (title changes; editor content undo is handled by ProseMirror)
  const undoActions = useUndo();
  // Tracks the title value before the first edit in the current editing session.
  // null means no edit has happened since the last node load or undo.
  let titleBeforeEdit: string | null = null;
  // Prevents re-entering undo tracking when undo itself calls handleTitleChange.
  let applyingTitleUndo = false;

  // Editor state
  const [title, setTitle] = createSignal("");
  const [content, setContent] = createSignal("");
  const [hasUnsavedChanges, setHasUnsavedChanges] = createSignal(false);
  const [autoSaveStatus, setAutoSaveStatus] = createSignal<"idle" | "saving" | "saved" | "error">("idle");
  let savedStatusTimer: number | undefined;

  // Frontmatter state
  const [icon, setIcon] = createSignal("");
  const [cover, setCover] = createSignal("");

  // Asset URLs accessor - derived from selectedNodeData
  const assetUrls = (): AssetUrlMap => selectedNodeData()?.asset_urls || {};

  // Linked node titles for resolving page link display text
  const [linkedNodeTitles, setLinkedNodeTitles] = createSignal<NodeTitleMap>({});

  // History state
  const [showHistory, setShowHistory] = createSignal(false);
  const [history, setHistory] = createSignal<HistorySnapshot | null>(null);

  // External change detection (SSE)
  const [externalChange, setExternalChange] = createSignal(false);
  const [saveIssue, setSaveIssue] = createSignal<"conflict" | "unknown" | null>(null);
  const recoveryMessage = () => t(saveIssue() === "unknown" ? "editor.saveUnknown" : "editor.saveConflict");
  const editingLocked = () => {
    if (switchingWorkspace() || (selectedNodeId() !== null && deletingNodeId() === selectedNodeId())) return true;
    // Returning to the current route cancels departure even if the obsolete
    // destination request has not settled yet.
    const routed = parseNodeUrl(location.pathname);
    if (routed?.nodeId === selectedNodeId()) return false;
    return loadingNodeId() !== null && loadingNodeId() !== selectedNodeId();
  };
  let revision = "";
  let acknowledged = { title: "", content: "" };
  const [recoveryState, setRecoveryState] = createSignal<RecoveryState>({ kind: "closed" });
  const recovery = () => {
    const state = recoveryState();
    return state.kind === "reviewing" || state.kind === "saving" ? state.session : null;
  };
  const recoveryLoading = () => recoveryState().kind === "comparing" || recoveryState().kind === "saving";
  const recoveryError = () => {
    const state = recoveryState();
    return state.kind === "reviewing" || state.kind === "failed" ? state.error : null;
  };
  let saveQueue: Promise<boolean> = Promise.resolve(true);
  let pendingSaves = 0;
  // Covers upload AND reference application, not just the network transfer.
  // Departure keeps the originating editor admitted until this work settles.
  const [pendingEdits, setPendingEdits] = createSignal<readonly Promise<boolean>[]>([]);
  const asyncEditPending = () => pendingEdits().length > 0;
  const runAsyncEdit: RunAsyncEdit = async (edit, afterSave) => {
    if (editingLocked()) return;
    const scope = captureSave();
    const intent = captureIntent();
    const pending = Promise.resolve()
      .then(() => (saveCurrent(scope) ? edit() : false))
      .catch((err: unknown) => {
        if (saveCurrent(scope)) setSaveError(`${t("errors.failedToSave")}: ${err}`);
        return false;
      });
    setPendingEdits((current) => [...current, pending]);
    let applied: boolean;
    try {
      applied = await pending;
    } finally {
      setPendingEdits((current) => current.filter((entry) => entry !== pending));
    }
    const mayContinue = () => saveCurrent(scope) && intent.current() && !editingLocked();
    // Never await a flush while this operation itself is still pending.
    if (applied && afterSave && mayContinue()) {
      try {
        if ((await intent.drain()) && mayContinue()) await afterSave(intent);
      } catch (err) {
        if (saveCurrent(scope)) setSaveError(`${t("errors.failedToSave")}: ${err}`);
      }
    }
  };
  // Metadata-only events use the acknowledged timestamp; content events use
  // the canonical revision so edits within one persisted second stay visible.
  const [syncedModified, setSyncedModified] = createSignal<number | null>(null);
  const dismissExternalChange = () => setExternalChange(false);
  const refreshFromServer = () => {
    if (asyncEditPending()) return;
    const nodeId = selectedNodeId();
    if (nodeId) {
      const node = selectedNodeData();
      const draft = [title(), content(), icon(), cover(), hasUnsavedChanges()];
      debouncedAutoSave.cancel();
      loadNode(nodeId, {
        refresh: true,
        canApply: () => {
          if (selectedNodeData() !== node) return false;
          const current = [title(), content(), icon(), cover(), hasUnsavedChanges()];
          if (asyncEditPending() || current.some((entry, index) => entry !== draft[index])) {
            setExternalChange(true);
            return false;
          }
          setExternalChange(false);
          return true;
        },
      });
    }
  };

  // Apply another writer's edit live when the editor is clean, and fall back to
  // the conflict banner when reloading would discard unsaved local work.
  createEffect(() => {
    const evt = lastEvent();
    if (!evt) return;
    untrack(() => {
      if (evt.type !== EventNodeUpdated || evt.node_id !== selectedNodeId()) return;
      const nodeId = selectedNodeId();
      if (!nodeId) return;
      const decision = classifyExternalUpdate(
        evt.modified,
        syncedModified(),
        hasUnsavedChanges() || asyncEditPending(),
        evt.revision ?? null,
        revision || null,
      );
      if (decision === "ignore") return;
      if (decision === "conflict") {
        setExternalChange(true);
        return;
      }
      refreshFromServer();
    });
  });

  // Debounced auto-save
  const captureSave = () => ({
    identity: authIdentity(),
    ws: wsApi(),
    wsId: user()?.workspace_id ?? "",
    wsName: user()?.workspace_name,
    nodeId: selectedNodeId(),
    title: title(),
    content: content(),
    expectedRevision: undefined as string | undefined,
  });
  const saveCurrent = (scope: ReturnType<typeof captureSave>) =>
    !disposed &&
    authIdentity() === scope.identity &&
    user()?.workspace_id === scope.wsId &&
    selectedNodeId() === scope.nodeId;
  const persist = async (scope: ReturnType<typeof captureSave>): Promise<boolean> => {
    const { nodeId, ws, wsId, wsName } = scope;
    if (!saveCurrent(scope)) return false;
    if (!nodeId || !hasUnsavedChanges()) return true;
    if (!ws) return false;
    if (saveIssue() !== null) {
      setExternalChange(true);
      setSaveError(recoveryMessage());
      setAutoSaveStatus("error");
      return false;
    }
    try {
      setAutoSaveStatus("saving");
      const diskContent = spaUrlsToRelativeLinks(scope.content, wsId);
      // A queued save chooses the last acknowledged revision when it executes.
      const saved = await ws.updatePage(nodeId, {
        title: scope.title,
        content: diskContent,
        expected_revision: scope.expectedRevision ?? revision,
      });
      if (!saveCurrent(scope)) return false;
      revision = saved.node.revision ?? "";
      acknowledged = {
        title: saved.node.title,
        content: relativeLinksToSpaUrls(saved.node.content ?? "", wsId),
      };
      setSyncedModified(saved.node.modified);
      setSaveError(null);
      if (title() !== scope.title || content() !== scope.content) return true;
      batch(() => {
        setTitle(saved.node.title);
        setContent(relativeLinksToSpaUrls(saved.node.content ?? "", wsId));
        updateNodeTitle(nodeId, saved.node.title);
        setHasUnsavedChanges(false);
        setSaveError(null);
        setAutoSaveStatus("saved");
      });
      if (!saveCurrent(scope)) return false;
      if (wsId) {
        const path = location.pathname;
        const next = nodeUrl(wsId, wsName, nodeId, saved.node.title);
        const parsed = parseNodeUrl(path);
        if (parsed?.wsId === wsId && parsed.nodeId === nodeId && path !== next)
          window.history.replaceState(null, "", next);
      }
      if (savedStatusTimer !== undefined) window.clearTimeout(savedStatusTimer);
      savedStatusTimer = window.setTimeout(() => {
        savedStatusTimer = undefined;
        if (untrack(autoSaveStatus) === "saved") setAutoSaveStatus("idle");
      }, 2000);
      return true;
    } catch (err) {
      if (!saveCurrent(scope)) return false;
      if (err instanceof APIError && err.status === 409) {
        setSaveIssue("conflict");
        setExternalChange(true);
        setSaveError(recoveryMessage());
      } else if (!(err instanceof APIError) || err.status >= 500) {
        setSaveIssue("unknown");
        setExternalChange(true);
        setSaveError(recoveryMessage());
      } else setSaveError(`${t("errors.autoSaveFailed")}: ${err}`);
      setAutoSaveStatus("error");
      return false;
    }
  };
  const enqueueSave = (scope: ReturnType<typeof captureSave>) => {
    pendingSaves++;
    const next = saveQueue.then(() => untrack(() => persist(scope)));
    saveQueue = next;
    return next.finally(() => {
      pendingSaves--;
    });
  };

  const debouncedAutoSave = debounce((scope: ReturnType<typeof captureSave>) => {
    void enqueueSave(scope);
  }, 2000);
  const scheduleAutoSave = () => debouncedAutoSave(captureSave());
  const settleEdits = async (scope: ReturnType<typeof captureSave>): Promise<boolean> => {
    while (asyncEditPending()) {
      if ((await Promise.all(pendingEdits())).some((saved) => !saved)) return false;
      if (!saveCurrent(scope)) return false;
    }
    return saveCurrent(scope);
  };
  const drainAndSave = async (): Promise<boolean> => {
    debouncedAutoSave.cancel();
    const scope = captureSave();
    if (!(await settleEdits(scope))) return false;
    // Capture after insertion: an upload may have changed the draft while awaited.
    if (!(await enqueueSave(captureSave()))) return false;
    // Edits made while the first request was in flight also have to be saved.
    if (!saveCurrent(scope)) return false;
    if (hasUnsavedChanges() || asyncEditPending()) return drainAndSave();
    return true;
  };
  // Capture authority without renewing it; async edit admission uses this path.
  const captureIntent = (): EditorIntent => {
    const version = departure;
    const identity = authIdentity();
    const wsId = user()?.workspace_id;
    const current = () =>
      !disposed && version === departure && identity === authIdentity() && wsId === user()?.workspace_id;
    const intent: EditorIntent = {
      current,
      async drain() {
        return current() && (await drainAndSave()) && current();
      },
      navigate(path) {
        if (!current()) return;
        // Router admission is synchronous. Carry this same authority through
        // its guard rather than treating a continuation as a new user action.
        continuingIntent = intent;
        try {
          navigate(path);
        } finally {
          continuingIntent = null;
        }
      },
    };
    return intent;
  };
  const beginIntent = () => {
    departure++;
    return captureIntent();
  };
  const prepareDeparture = async () => {
    if (editingLocked()) return null;
    const intent = beginIntent();
    return (await intent.drain()) && !editingLocked() ? intent : null;
  };
  useBeforeLeave((event) => {
    if (event.defaultPrevented) return;
    // Clean route changes also supersede an asynchronous command's follow-up.
    const intent = continuingIntent ?? beginIntent();
    if (!hasUnsavedChanges() && pendingSaves === 0 && !asyncEditPending()) return;
    const scope = captureSave();
    if (!scope.nodeId || !scope.wsId) return;
    event.preventDefault();
    void drainAndSave().then((saved) => {
      if (saved && intent.current() && saveCurrent(scope)) event.retry(true);
    });
  });
  const copyDraft = async () => {
    const scope = captureSave();
    try {
      await navigator.clipboard.writeText(`# ${title()}\n\n${content()}`);
    } catch (err) {
      if (saveCurrent(scope)) setSaveError(`${t("editor.copyFailed")}: ${err}`);
    }
  };
  const closeRecovery = () => {
    setRecoveryState({ kind: "closed" });
  };
  const buildRecovery = (saved: RecoverySession["saved"]): RecoverySession => {
    const local = { title: title(), content: content() };
    return {
      base: { ...acknowledged },
      local,
      saved,
      titleMerge: mergeText(acknowledged.title, local.title, saved.title),
      contentMerge: mergeText(acknowledged.content, local.content, saved.content),
    };
  };
  const compareChanges = async () => {
    const scope = captureSave();
    if (!saveCurrent(scope) || !scope.ws || !scope.nodeId) return;
    const intent = beginIntent();
    const request: RecoveryState = { kind: "comparing" };
    debouncedAutoSave.cancel();
    // Block autosaves without relabeling an uncertain write before a current,
    // authorized, versioned snapshot supplies its reconciliation baseline.
    setSaveIssue((issue) => issue ?? "conflict");
    setRecoveryState(request);
    try {
      if (!(await settleEdits(scope))) return;
      await saveQueue;
      if (!saveCurrent(scope) || !intent.current() || recoveryState() !== request) return;
      const latest = await scope.ws.getNode(scope.nodeId);
      if (!(await settleEdits(scope))) return;
      if (!saveCurrent(scope) || !intent.current() || recoveryState() !== request) return;
      if (!latest.revision) {
        setRecoveryState({ kind: "failed", error: t("editor.compareFailed") });
        return;
      }
      const session = buildRecovery({
        title: latest.title,
        content: relativeLinksToSpaUrls(latest.content ?? "", scope.wsId),
        revision: latest.revision,
      });
      batch(() => {
        setSaveIssue("conflict");
        setRecoveryState({ kind: "reviewing", session, error: null });
      });
    } catch {
      if (saveCurrent(scope) && intent.current() && recoveryState() === request)
        setRecoveryState({ kind: "failed", error: t("editor.compareFailed") });
    } finally {
      if (recoveryState() === request) setRecoveryState({ kind: "closed" });
    }
  };
  const saveRecovery = async (session: RecoverySession, result: { title: string; content: string }) => {
    const scope = captureSave();
    const state = recoveryState();
    if (!saveCurrent(scope) || state.kind !== "reviewing" || state.session !== session) return false;
    const intent = beginIntent();
    if (!(await settleEdits(scope)) || !intent.current() || recoveryState() !== state) return false;
    if (title() !== session.local.title || content() !== session.local.content) {
      // Keep every new keystroke and ask for a fresh explicit review.
      setRecoveryState({
        kind: "reviewing",
        session: { ...buildRecovery(session.saved), previousResult: result },
        error: t("editor.draftChanged"),
      });
      return false;
    }
    debouncedAutoSave.cancel();
    const saving: RecoveryState = { kind: "saving", session };
    batch(() => {
      setTitle(result.title);
      setContent(result.content);
      if (scope.nodeId) updateNodeTitle(scope.nodeId, result.title);
      setHasUnsavedChanges(true);
      setSaveIssue(null);
      setRecoveryState(saving);
    });
    const savingScope = { ...captureSave(), expectedRevision: session.saved.revision };
    const saved = await enqueueSave(savingScope);
    if (!saveCurrent(scope)) return false;
    if (recoveryState() !== saving) return saved;
    if (saved) {
      closeRecovery();
      setExternalChange(false);
      // persist preserves any edits made while the recovery save was pending.
      if (hasUnsavedChanges()) scheduleAutoSave();
    } else {
      setSaveIssue((issue) => issue ?? "conflict");
      setRecoveryState({ kind: "failed", error: t("editor.recoveryRetry") });
    }
    return saved;
  };
  // Ordinary departures are gated before disposal; identity retirement never saves private drafts.
  onCleanup(() => {
    disposed = true;
    departure++;
    resetEditor();
    if (savedStatusTimer !== undefined) window.clearTimeout(savedStatusTimer);
  });

  // Fetch linked node titles when content has internal links
  async function fetchLinkedNodeTitles(nodeContent: string) {
    const scope = captureSave();
    const ws = scope.ws;
    if (!saveCurrent(scope)) return;
    const wsId = user()?.workspace_id;
    if (!ws || !wsId || !nodeContent) {
      setLinkedNodeTitles({});
      return;
    }

    // Extract node IDs from internal links
    const linkedIds = extractLinkedNodeIds(nodeContent, wsId);
    if (linkedIds.length === 0) {
      setLinkedNodeTitles({});
      return;
    }

    try {
      const resp = await ws.getNodeTitles({ IDs: linkedIds.join(",") });
      if (!saveCurrent(scope)) return;
      setLinkedNodeTitles(resp.titles || {});
    } catch {
      // Silent failure - just use stored titles in links
      setLinkedNodeTitles({});
    }
  }

  // Sync editor state when selected node changes
  createEffect(() => {
    const node = selectedNodeData();
    // Clear undo stack on every node navigation
    undoActions.clear();
    titleBeforeEdit = null;
    if (node) {
      revision = node.revision ?? "";
      setSaveIssue(null);
      setSaveError(null);
      setTitle(node.title);
      // Convert relative file path links from API to SPA URLs for the editor.
      const wsId = user()?.workspace_id || "";
      const spaContent = relativeLinksToSpaUrls(node.content || "", wsId);
      setContent(spaContent);
      acknowledged = { title: node.title, content: spaContent };
      closeRecovery();
      setIcon(node.icon || "");
      setCover(node.cover || "");
      setSyncedModified(node.modified ?? null);
      setHasUnsavedChanges(false);
      setAutoSaveStatus("idle");
      setShowHistory(false);
      setExternalChange(false);
      // Fetch linked node titles in the background
      untrack(() => fetchLinkedNodeTitles(spaContent));
    } else {
      resetEditor();
    }
  });

  function resetEditor() {
    revision = "";
    acknowledged = { title: "", content: "" };
    closeRecovery();
    setSaveIssue(null);
    debouncedAutoSave.cancel();
    setTitle("");
    setContent("");
    setIcon("");
    setCover("");
    setHasUnsavedChanges(false);
    setAutoSaveStatus("idle");
    setShowHistory(false);
    setHistory(null);
    setLinkedNodeTitles({});
    setExternalChange(false);
    setSyncedModified(null);
  }

  async function handleIconChange(newIcon: string) {
    if (editingLocked()) return;
    const nodeId = selectedNodeId();
    const scope = captureSave();
    const ws = scope.ws;
    if (!saveCurrent(scope)) return;
    if (!nodeId || !ws) return;
    setIcon(newIcon);
    updateNodeIcon(nodeId, newIcon);
    try {
      const saved = await ws.updatePageFrontmatter(nodeId, { icon: newIcon, cover: cover() });
      if (!saveCurrent(scope)) return;
      setSyncedModified(saved.modified);
    } catch (err) {
      setSaveError(`${t("errors.failedToSave")}: ${err}`);
    }
  }

  async function handleCoverChange(newCover: string) {
    if (editingLocked()) return false;
    const nodeId = selectedNodeId();
    const scope = captureSave();
    const ws = scope.ws;
    if (!saveCurrent(scope)) return false;
    if (!nodeId || !ws) return false;
    setCover(newCover);
    try {
      const saved = await ws.updatePageFrontmatter(nodeId, { icon: icon(), cover: newCover });
      if (!saveCurrent(scope)) return false;
      setSyncedModified(saved.modified);
      return true;
    } catch (err) {
      setSaveError(`${t("errors.failedToSave")}: ${err}`);
      return false;
    }
  }

  function handleTitleChange(newTitle: string) {
    if (editingLocked()) return;
    // On first edit in this session, capture the pre-edit title for undo.
    if (!applyingTitleUndo && titleBeforeEdit === null) {
      const oldTitle = title();
      titleBeforeEdit = oldTitle;
      undoActions.push({
        description: t("common.undo"),
        undo: async () => {
          applyingTitleUndo = true;
          titleBeforeEdit = null;
          handleTitleChange(oldTitle);
          applyingTitleUndo = false;
        },
        redo: async () => {
          applyingTitleUndo = true;
          titleBeforeEdit = null;
          handleTitleChange(newTitle);
          applyingTitleUndo = false;
        },
      });
    }
    setTitle(newTitle);
    const id = selectedNodeId();
    if (id) {
      updateNodeTitle(id, newTitle);
    }
    setHasUnsavedChanges(true);
    scheduleAutoSave();
  }

  function handleContentChange(newContent: string) {
    if (editingLocked()) return;
    setContent(newContent);
    setHasUnsavedChanges(true);
    scheduleAutoSave();
  }

  async function loadHistory(nodeId: string, intent: EditorIntent) {
    if (!intent.current()) return;
    if (showHistory()) {
      setShowHistory(false);
      return;
    }
    const scope = captureSave();
    const ws = scope.ws;
    if (!saveCurrent(scope) || scope.nodeId !== nodeId) return;
    if (!ws) return;

    try {
      const data = await ws.listNodeVersions(nodeId, { Limit: 100 });
      if (!saveCurrent(scope) || !intent.current()) return;
      setHistory({ commits: (data.history?.filter(Boolean) as Commit[]) || [], intent });
      setShowHistory(true);
    } catch (err) {
      if (saveCurrent(scope) && intent.current()) setLoadError(`${t("errors.failedToLoad")}: ${err}`);
    }
  }

  const value: EditorContextValue = {
    title,
    setTitle,
    content,
    setContent,
    hasUnsavedChanges,
    setHasUnsavedChanges,
    autoSaveStatus,
    assetUrls,
    linkedNodeTitles,
    showHistory,
    history,
    loadHistory,
    externalChange,
    dismissExternalChange,
    refreshFromServer,
    icon,
    cover,
    handleIconChange,
    handleCoverChange,
    triggerAutoSave: scheduleAutoSave,
    beginIntent,
    runAsyncEdit,
    prepareDeparture,
    asyncEditPending,
    saveIssue,
    editingLocked,
    copyDraft,
    recovery,
    recoveryLoading,
    recoveryError,
    compareChanges,
    closeRecovery,
    saveRecovery,
    resetEditor,
    handleTitleChange,
    handleContentChange,
    async undo() {
      if (!editingLocked()) await undoActions.undo();
    },
    async redo() {
      if (!editingLocked()) await undoActions.redo();
    },
    canUndo: undoActions.canUndo,
    canRedo: undoActions.canRedo,
  };

  return <EditorContext.Provider value={value}>{props.children}</EditorContext.Provider>;
};

export function useEditor(): EditorContextValue {
  const context = useContext(EditorContext);
  if (!context) {
    throw new Error("useEditor must be used within an EditorProvider");
  }
  return context;
}
