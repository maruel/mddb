// Owns document drafts, derived dirtiness, save admission, and asynchronous edits through recovery and departure.

import {
  createContext,
  batch,
  useContext,
  createSignal,
  createEffect,
  on,
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
import { EventAspectMetadata, EventNodeUpdated, EventResync, type Commit, type NodeResponse } from "@sdk/types.gen";
import { useUndo } from "../hooks/useUndo";

/** Map of asset filename to signed URL */
export type AssetUrlMap = Record<string, string>;

/** Map of node ID to title for resolving internal page links */
export type NodeTitleMap = Record<string, string>;

interface Draft {
  title: string;
  content: string;
}

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

// A comparison blocks writes without reporting a failed write; a conflict needs recovery.
type SaveOutcome = "blocked" | "conflict" | "error" | "idle" | "saved" | "unknown";

interface EditorContextValue {
  // Editor state
  title: Accessor<string>;
  content: Accessor<string>;
  hasUnsavedChanges: Accessor<boolean>;
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
  comparisonBlocked: Accessor<boolean>;
  editingLocked: Accessor<boolean>;
  copyDraft: () => Promise<void>;
  recovery: Accessor<RecoverySession | null>;
  recoveryLoading: Accessor<boolean>;
  recoveryError: Accessor<string | null>;
  compareChanges: () => Promise<void>;
  closeRecovery: () => void;
  saveRecovery: (session: RecoverySession, result: { title: string; content: string }) => Promise<boolean>;
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
  const location = useLocation();
  let action: object | null = {};
  const navigate = useNavigate();
  let continuingIntent: EditorIntent | null = null;
  const {
    selectedNodeId,
    selectedNodeData,
    selectedDocument,
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

  // Only this provider replaces the draft. Public edits pass through admission,
  // undo tracking, sidebar updates, and autosave scheduling below.
  const [draft, replaceDraft] = createSignal<Draft>({ title: "", content: "" });
  const title = () => draft().title;
  const content = () => draft().content;

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
  const createSaveSession = (node: NodeResponse, spaContent: string) => {
    const [acknowledged, acknowledge] = createSignal({
      title: node.title,
      content: spaContent,
      revision: node.revision ?? "",
      // API responses may omit metadata despite the generated type.
      // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
      modified: node.modified ?? null,
    });
    const [outcome, setOutcome] = createSignal<SaveOutcome>("idle");
    const [pending, setPending] = createSignal<Promise<boolean> | null>(null);
    const [edits, setEdits] = createSignal<readonly Promise<boolean>[]>([]);
    return {
      identity: authIdentity(),
      ws: wsApi(),
      wsId: user()?.workspace_id ?? "",
      nodeId: node.id,
      acknowledged,
      acknowledge,
      outcome,
      setOutcome,
      pending,
      setPending,
      edits,
      setEdits,
      savedTimer: null as number | null,
    };
  };
  const [saveSession, setSaveSession] = createSignal<ReturnType<typeof createSaveSession> | null>(null);
  const saveIssue = (): "conflict" | "unknown" | null => {
    const outcome = saveSession()?.outcome();
    if (outcome === "unknown") return "unknown";
    return outcome === "conflict" ? "conflict" : null;
  };
  const comparisonBlocked = () => saveSession()?.outcome() === "blocked";
  const writesBlocked = () => comparisonBlocked() || saveIssue() !== null;
  const savePending = () => Boolean(saveSession()?.pending());
  const hasUnsavedChanges = () => {
    const acknowledged = saveSession()?.acknowledged();
    return (
      acknowledged !== undefined && (draft().title !== acknowledged.title || draft().content !== acknowledged.content)
    );
  };
  const recoveryMessage = () => t(saveIssue() === "unknown" ? "editor.saveUnknown" : "editor.saveConflict");
  const editingLocked = () => {
    if (switchingWorkspace() || (selectedNodeId() !== null && deletingNodeId() === selectedNodeId())) return true;
    // Returning to the current route cancels departure while its old destination loads.
    const routed = parseNodeUrl(location.pathname);
    if (routed?.nodeId === selectedNodeId()) return false;
    return loadingNodeId() !== null && loadingNodeId() !== selectedNodeId();
  };
  const autoSaveStatus = (): "idle" | "saving" | "saved" | "error" => {
    const session = saveSession();
    if (session === null) return "idle";
    if (session.pending() !== null) return "saving";
    const outcome = session.outcome();
    if (saveIssue() !== null || (outcome === "error" && hasUnsavedChanges())) return "error";
    return outcome === "saved" && !hasUnsavedChanges() ? "saved" : "idle";
  };
  const retireSaveSession = () => {
    const session = untrack(saveSession);
    if (session !== null && session.savedTimer !== null) window.clearTimeout(session.savedTimer);
    setSaveSession(null);
  };
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
  // Covers upload AND reference application, not just the network transfer.
  // Departure keeps the originating editor admitted until this work settles.
  const asyncEditPending = () => (saveSession()?.edits().length ?? 0) > 0;
  const runAsyncEdit: RunAsyncEdit = async (edit, afterSave) => {
    if (editingLocked()) return;
    const scope = captureSave();
    if (!saveCurrent(scope)) return;
    const session = scope.session;
    const intent = captureIntent();
    const pending = Promise.resolve()
      .then(() => (saveCurrent(scope) ? edit() : false))
      .catch((err: unknown) => {
        if (saveCurrent(scope)) setSaveError(`${t("errors.failedToSave")}: ${String(err)}`);
        return false;
      });
    void session.setEdits((current) => [...current, pending]);
    let applied: boolean;
    try {
      applied = await pending;
    } finally {
      void session.setEdits((current) => current.filter((entry) => entry !== pending));
    }
    const mayContinue = () => saveCurrent(scope) && intent.current() && !editingLocked();
    // Never await a flush while this operation itself is still pending.
    if (applied && afterSave && mayContinue()) {
      try {
        if ((await intent.drain()) && mayContinue()) await afterSave(intent);
      } catch (err) {
        if (saveCurrent(scope)) setSaveError(`${t("errors.failedToSave")}: ${String(err)}`);
      }
    }
  };
  const dismissExternalChange = () => setExternalChange(false);
  const localWorkPending = () => hasUnsavedChanges() || savePending() || asyncEditPending() || writesBlocked();
  const reloadFromServer = (automatic: boolean) => {
    if (asyncEditPending()) return;
    const nodeId = selectedNodeId();
    if (nodeId) {
      const node = selectedDocument();
      const captured = {
        draft: draft(),
        icon: icon(),
        cover: cover(),
        acknowledged: saveSession()?.acknowledged(),
        outcome: saveSession()?.outcome(),
      };
      debouncedAutoSave.cancel();
      void loadNode(nodeId, {
        refresh: true,
        canApply: () => {
          if (selectedDocument() !== node) return false;
          if (
            asyncEditPending() ||
            savePending() ||
            (automatic && writesBlocked()) ||
            draft() !== captured.draft ||
            icon() !== captured.icon ||
            cover() !== captured.cover ||
            saveSession()?.acknowledged() !== captured.acknowledged ||
            saveSession()?.outcome() !== captured.outcome
          ) {
            setExternalChange(true);
            return false;
          }
          setExternalChange(false);
          return true;
        },
      }).catch((error: unknown) => console.warn("Node loading failed", error));
    }
  };

  const refreshFromServer = () => reloadFromServer(false);

  // Apply another writer's edit live when the editor is clean, and fall back to
  // the conflict banner when reloading would discard unsaved local work.
  createEffect(() => {
    const evt = lastEvent();
    if (!evt) return;
    untrack(() => {
      if (evt.type !== EventResync && (evt.type !== EventNodeUpdated || evt.node_id !== selectedNodeId())) return;
      const nodeId = selectedNodeId();
      if (!nodeId) return;
      if (evt.type === EventResync || evt.aspects?.includes(EventAspectMetadata)) {
        if (localWorkPending()) setExternalChange(true);
        else reloadFromServer(true);
        return;
      }
      const decision = classifyExternalUpdate(
        evt.modified,
        saveSession()?.acknowledged().modified ?? null,
        localWorkPending(),
        evt.revision ?? null,
        saveSession()?.acknowledged().revision || null,
      );
      if (decision === "ignore") return;
      if (decision === "conflict") {
        setExternalChange(true);
        return;
      }
      reloadFromServer(true);
    });
  });

  // Each draft and queued request stays bound to its loaded editor session.
  const captureSave = () => ({
    session: untrack(saveSession),
    title: title(),
    content: content(),
    expectedRevision: null as string | null,
  });
  type SaveSnapshot = ReturnType<typeof captureSave>;
  const saveCurrent = (
    scope: SaveSnapshot,
  ): scope is SaveSnapshot & { session: NonNullable<SaveSnapshot["session"]> } =>
    scope.session !== null &&
    untrack(saveSession) === scope.session &&
    authIdentity() === scope.session.identity &&
    user()?.workspace_id === scope.session.wsId &&
    selectedNodeId() === scope.session.nodeId;
  const persist = async (scope: SaveSnapshot): Promise<boolean> => {
    if (!saveCurrent(scope)) return false;
    const session = scope.session;
    const { nodeId, ws, wsId } = session;
    if (writesBlocked()) {
      if (saveIssue() !== null) {
        setExternalChange(true);
        setSaveError(recoveryMessage());
      }
      return false;
    }
    if (scope.expectedRevision === null && !hasUnsavedChanges()) return true;
    if (!ws) return false;
    session.setOutcome("idle");
    try {
      const diskContent = spaUrlsToRelativeLinks(scope.content, wsId);
      // Choose the acknowledged revision when this queued request executes.
      const saved = await ws.updatePage(nodeId, {
        title: scope.title,
        content: diskContent,
        expected_revision: scope.expectedRevision ?? session.acknowledged().revision,
      });
      if (!saveCurrent(scope)) return false;
      session.acknowledge({
        title: saved.node.title,
        content: relativeLinksToSpaUrls(saved.node.content ?? "", wsId),
        revision: saved.node.revision ?? "",
        modified: saved.node.modified,
      });
      setSaveError(null);
      if (title() !== scope.title || content() !== scope.content) return true;
      batch(() => {
        replaceDraft({ title: saved.node.title, content: session.acknowledged().content });
        updateNodeTitle(nodeId, saved.node.title);

        if (!writesBlocked()) session.setOutcome("saved");
      });
      if (!saveCurrent(scope)) return false;
      if (wsId) {
        const path = location.pathname;
        const next = nodeUrl(wsId, user()?.workspace_name, nodeId, saved.node.title);
        const parsed = parseNodeUrl(path);
        if (parsed?.wsId === wsId && parsed.nodeId === nodeId && path !== next)
          window.history.replaceState(null, "", next);
      }
      if (session.savedTimer !== null) window.clearTimeout(session.savedTimer);
      session.savedTimer = window.setTimeout(() => {
        session.savedTimer = null;
        if (session.outcome() === "saved") session.setOutcome("idle");
      }, 2000);
      return true;
    } catch (err) {
      if (!saveCurrent(scope)) return false;
      const conflict = err instanceof APIError && err.status === 409;
      const uncertain = !(err instanceof APIError) || err.status >= 500;
      // Settlement cannot reopen admission installed by an explicit comparison.
      const previous = session.outcome();
      session.setOutcome(
        uncertain
          ? "unknown"
          : conflict
            ? "conflict"
            : previous === "blocked" || previous === "unknown" || previous === "conflict"
              ? previous
              : "error",
      );
      if (saveIssue() !== null) setExternalChange(true);
      setSaveError(saveIssue() !== null ? recoveryMessage() : `${t("errors.autoSaveFailed")}: ${String(err)}`);
      return false;
    }
  };
  const enqueueSave = (scope: SaveSnapshot): Promise<boolean> => {
    if (!saveCurrent(scope)) return Promise.resolve(false);
    const session = scope.session;
    const next = (untrack(session.pending) ?? Promise.resolve(true)).then(() => untrack(() => persist(scope)));
    void session.setPending(next);
    return next.finally(() => {
      if (session.pending() === next) session.setPending(null);
    });
  };

  const debouncedAutoSave = debounce((scope: SaveSnapshot) => {
    void enqueueSave(scope);
  }, 2000);
  const scheduleAutoSave = () => {
    if (writesBlocked() || (!hasUnsavedChanges() && !savePending())) {
      debouncedAutoSave.cancel();
      return;
    }
    debouncedAutoSave(captureSave());
  };
  const settleEdits = async (scope: SaveSnapshot): Promise<boolean> => {
    if (!saveCurrent(scope)) return false;
    while (scope.session.edits().length > 0) {
      if ((await Promise.all(scope.session.edits())).some((saved) => !saved)) return false;
      if (!saveCurrent(scope)) return false;
    }
    return saveCurrent(scope);
  };
  const drainAndSave = async (): Promise<boolean> => {
    debouncedAutoSave.cancel();
    if (untrack(saveSession) === null) return !hasUnsavedChanges() && !asyncEditPending();
    const scope = captureSave();
    do {
      if (!(await settleEdits(scope))) return false;
      // Uploads may insert a reference while draining; save their resulting draft.
      if (!(await enqueueSave(captureSave())) || !saveCurrent(scope)) return false;
    } while (hasUnsavedChanges() || asyncEditPending());
    return true;
  };
  // Capture authority without renewing it; async edit admission uses this path.
  const captureIntent = (): EditorIntent => {
    const captured = action;
    const identity = authIdentity();
    const wsId = user()?.workspace_id;
    const current = () =>
      captured !== null && captured === action && identity === authIdentity() && wsId === user()?.workspace_id;
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
    if (action !== null) action = {};
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
    const scope = captureSave();
    if (!saveCurrent(scope)) return;
    if (!localWorkPending()) return;
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
      if (saveCurrent(scope)) setSaveError(`${t("editor.copyFailed")}: ${String(err)}`);
    }
  };
  const closeRecovery = () => {
    setRecoveryState({ kind: "closed" });
  };
  const buildRecovery = (
    owner: NonNullable<SaveSnapshot["session"]>,
    saved: RecoverySession["saved"],
  ): RecoverySession => {
    const acknowledged = owner.acknowledged();
    const local = { title: title(), content: content() };
    return {
      base: { title: acknowledged.title, content: acknowledged.content },
      local,
      saved,
      titleMerge: mergeText(acknowledged.title, local.title, saved.title),
      contentMerge: mergeText(acknowledged.content, local.content, saved.content),
    };
  };
  const compareChanges = async () => {
    const scope = captureSave();
    if (!saveCurrent(scope) || !scope.session.ws) return;
    const intent = beginIntent();
    const request: RecoveryState = { kind: "comparing" };
    debouncedAutoSave.cancel();
    // Block autosaves without relabeling an uncertain write before a current,
    // authorized, versioned snapshot supplies its reconciliation baseline.
    if (!writesBlocked()) scope.session.setOutcome("blocked");
    setRecoveryState(request);
    try {
      if (!(await settleEdits(scope))) return;
      await scope.session.pending();
      if (!saveCurrent(scope) || !intent.current() || recoveryState() !== request) return;
      const latest = await scope.session.ws.getNode(scope.session.nodeId);
      if (!(await settleEdits(scope))) return;
      if (!saveCurrent(scope) || !intent.current() || recoveryState() !== request) return;
      if (!latest.revision) {
        setRecoveryState({ kind: "failed", error: t("editor.compareFailed") });
        return;
      }
      const session = buildRecovery(scope.session, {
        title: latest.title,
        content: relativeLinksToSpaUrls(latest.content ?? "", scope.session.wsId),
        revision: latest.revision,
      });
      batch(() => {
        // A versioned read reconciles uncertainty without turning a manual
        // comparison's admission block into a failed-write indication.
        if (scope.session.outcome() === "unknown") scope.session.setOutcome("conflict");
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
        session: { ...buildRecovery(scope.session, session.saved), previousResult: result },
        error: t("editor.draftChanged"),
      });
      return false;
    }
    debouncedAutoSave.cancel();
    const saving: RecoveryState = { kind: "saving", session };
    batch(() => {
      replaceDraft({ title: result.title, content: result.content });
      updateNodeTitle(scope.session.nodeId, result.title);

      scope.session.setOutcome("idle");
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
      if (scope.session.outcome() !== "unknown") scope.session.setOutcome("conflict");
      setRecoveryState({ kind: "failed", error: t("editor.recoveryRetry") });
    }
    return saved;
  };
  // Ordinary departures are gated before disposal; identity retirement never saves private drafts.
  onCleanup(() => {
    action = null;
    resetEditor();
  });

  // Fetch linked node titles when content has internal links
  async function fetchLinkedNodeTitles(nodeContent: string) {
    const scope = captureSave();
    if (!saveCurrent(scope)) return;
    const { ws, wsId } = scope.session;
    if (!ws || !nodeContent) {
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

  // Only an authoritative document load replaces its save owner and draft.
  createEffect(
    on(selectedDocument, (node) => {
      // Clear undo stack on every node navigation
      undoActions.clear();
      titleBeforeEdit = null;
      if (node) {
        retireSaveSession();
        debouncedAutoSave.cancel();
        setSaveError(null);
        // Convert relative file path links from API to SPA URLs for the editor.
        const wsId = user()?.workspace_id || "";
        const spaContent = relativeLinksToSpaUrls(node.content || "", wsId);
        replaceDraft({ title: node.title, content: spaContent });
        setSaveSession(createSaveSession(node, spaContent));
        closeRecovery();
        setIcon(node.icon || "");
        setCover(node.cover || "");

        setShowHistory(false);
        setExternalChange(false);
        // Fetch linked node titles in the background
        void untrack(() => fetchLinkedNodeTitles(spaContent)).catch((backgroundError: unknown) =>
          reportError(backgroundError),
        );
      } else {
        resetEditor();
      }
    }),
  );

  function resetEditor() {
    retireSaveSession();
    closeRecovery();
    debouncedAutoSave.cancel();
    replaceDraft({ title: "", content: "" });
    setIcon("");
    setCover("");

    setShowHistory(false);
    setHistory(null);
    setLinkedNodeTitles({});
    setExternalChange(false);
  }

  async function handleIconChange(newIcon: string) {
    if (editingLocked()) return;
    const scope = captureSave();
    if (!saveCurrent(scope)) return;
    const { nodeId, ws } = scope.session;
    if (!ws) return;
    setIcon(newIcon);
    updateNodeIcon(nodeId, newIcon);
    try {
      const saved = await ws.updatePageFrontmatter(nodeId, { icon: newIcon, cover: cover() });
      if (!saveCurrent(scope)) return;
      scope.session.acknowledge({ ...scope.session.acknowledged(), modified: saved.modified });
    } catch (err) {
      if (saveCurrent(scope)) setSaveError(`${t("errors.failedToSave")}: ${String(err)}`);
    }
  }

  async function handleCoverChange(newCover: string) {
    if (editingLocked()) return false;
    const scope = captureSave();
    if (!saveCurrent(scope)) return false;
    const { nodeId, ws } = scope.session;
    if (!ws) return false;
    setCover(newCover);
    try {
      const saved = await ws.updatePageFrontmatter(nodeId, { icon: icon(), cover: newCover });
      if (!saveCurrent(scope)) return false;
      scope.session.acknowledge({ ...scope.session.acknowledged(), modified: saved.modified });
      return true;
    } catch (err) {
      if (saveCurrent(scope)) setSaveError(`${t("errors.failedToSave")}: ${String(err)}`);
      return false;
    }
  }

  function handleTitleChange(newTitle: string) {
    if (editingLocked() || !saveCurrent(captureSave()) || newTitle === title()) return;
    // On first edit in this session, capture the pre-edit title for undo.
    if (!applyingTitleUndo && titleBeforeEdit === null) {
      const oldTitle = title();
      titleBeforeEdit = oldTitle;
      undoActions.push({
        description: t("common.undo"),
        undo: () => {
          applyingTitleUndo = true;
          titleBeforeEdit = null;
          handleTitleChange(oldTitle);
          applyingTitleUndo = false;
          return Promise.resolve();
        },
        redo: () => {
          applyingTitleUndo = true;
          titleBeforeEdit = null;
          handleTitleChange(newTitle);
          applyingTitleUndo = false;
          return Promise.resolve();
        },
      });
    }
    replaceDraft((current) => ({ ...current, title: newTitle }));
    const id = selectedNodeId();
    if (id) {
      updateNodeTitle(id, newTitle);
    }

    scheduleAutoSave();
  }

  function handleContentChange(newContent: string) {
    if (editingLocked() || !saveCurrent(captureSave()) || newContent === content()) return;
    replaceDraft((current) => ({ ...current, content: newContent }));

    scheduleAutoSave();
  }

  async function loadHistory(nodeId: string, intent: EditorIntent) {
    if (!intent.current()) return;
    if (showHistory()) {
      setShowHistory(false);
      return;
    }
    // History follows its admitted action and target across document refreshes.
    const identity = authIdentity();
    const wsId = user()?.workspace_id;
    const ws = wsApi();
    const current = () =>
      intent.current() && authIdentity() === identity && user()?.workspace_id === wsId && selectedNodeId() === nodeId;
    if (!ws || !current()) return;

    try {
      const data = await ws.listNodeVersions(nodeId, { Limit: 100 });
      if (!current()) return;
      setHistory({ commits: (data.history ?? []).filter((item): item is Commit => Boolean(item)), intent });
      setShowHistory(true);
    } catch (err) {
      if (current()) setLoadError(`${t("errors.failedToLoad")}: ${String(err)}`);
    }
  }

  const value: EditorContextValue = {
    title,
    content,
    hasUnsavedChanges,
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
    comparisonBlocked,
    editingLocked,
    copyDraft,
    recovery,
    recoveryLoading,
    recoveryError,
    compareChanges,
    closeRecovery,
    saveRecovery,
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
