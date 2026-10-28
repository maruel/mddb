// Editor context providing title, content, auto-save, and history management.

import {
  createContext,
  useContext,
  createSignal,
  createEffect,
  onCleanup,
  untrack,
  type ParentComponent,
  type Accessor,
} from "solid-js";
import { useAuth } from "./AuthContext";
import { useEventSource } from "./EventSourceContext";
import { useWorkspace } from "./WorkspaceContext";
import { useI18n } from "../i18n";
import { debounce } from "../utils/debounce";
import { nodeUrl, parseNodeUrl } from "../utils/urls";
import { extractLinkedNodeIds, relativeLinksToSpaUrls, spaUrlsToRelativeLinks } from "../utils/markdown-utils";
import { classifyExternalUpdate } from "./editorSync";
import { EventNodeUpdated, type Commit } from "@sdk/types.gen";
import { useUndo } from "../hooks/useUndo";

/** Map of asset filename to signed URL */
export type AssetUrlMap = Record<string, string>;

/** Map of node ID to title for resolving internal page links */
export type NodeTitleMap = Record<string, string>;

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
  setShowHistory: (show: boolean) => void;
  history: Accessor<Commit[]>;
  loadHistory: (nodeId: string) => Promise<void>;

  // External change detection (SSE)
  externalChange: Accessor<boolean>;
  dismissExternalChange: () => void;
  refreshFromServer: () => void;

  // Frontmatter (icon and cover)
  icon: Accessor<string>;
  cover: Accessor<string>;
  handleIconChange: (newIcon: string) => Promise<void>;
  handleCoverChange: (newCover: string) => Promise<void>;

  // Actions
  triggerAutoSave: () => void;
  flushAutoSave: () => void;
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
  const { selectedNodeId, selectedNodeData, updateNodeTitle, updateNodeIcon, loadNode, setLoadError, setSaveError } =
    useWorkspace();
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
  const [history, setHistory] = createSignal<Commit[]>([]);

  // External change detection (SSE)
  const [externalChange, setExternalChange] = createSignal(false);
  // syncedModified is the node revision this editor already holds: the revision
  // it loaded, or the one its own last write returned. An SSE event at or below
  // it is this editor's own echo rather than another writer's edit.
  const [syncedModified, setSyncedModified] = createSignal<number | null>(null);
  const dismissExternalChange = () => setExternalChange(false);
  const refreshFromServer = () => {
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
          if (current.some((entry, index) => entry !== draft[index])) {
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
      const decision = classifyExternalUpdate(evt.modified, syncedModified(), hasUnsavedChanges());
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
  });
  const saveCurrent = (scope: ReturnType<typeof captureSave>) =>
    !disposed &&
    authIdentity() === scope.identity &&
    user()?.workspace_id === scope.wsId &&
    selectedNodeId() === scope.nodeId;
  // eslint-disable-next-line solid/reactivity -- delayed saves validate their captured owner at execution time
  const debouncedAutoSave = debounce(async (scope: ReturnType<typeof captureSave>) => {
    const { nodeId, ws, wsId, wsName } = scope;
    if (!saveCurrent(scope) || !nodeId || !hasUnsavedChanges() || !ws) return;

    try {
      setAutoSaveStatus("saving");
      const diskContent = spaUrlsToRelativeLinks(scope.content, wsId);
      const saved = await ws.updatePage(nodeId, { title: scope.title, content: diskContent });
      if (!saveCurrent(scope)) return;
      setSyncedModified(saved.modified);
      if (title() !== scope.title || content() !== scope.content) return;
      setHasUnsavedChanges(false);
      setAutoSaveStatus("saved");

      // Update URL if title changed
      if (wsId) {
        const currentPath = window.location.pathname;
        const newPath = nodeUrl(wsId, wsName, nodeId, scope.title);
        const currentNode = parseNodeUrl(currentPath);
        if (currentNode?.wsId === wsId && currentNode.nodeId === nodeId && currentPath !== newPath) {
          window.history.replaceState(null, "", newPath);
        }
      }

      if (savedStatusTimer !== undefined) {
        window.clearTimeout(savedStatusTimer);
      }
      savedStatusTimer = window.setTimeout(() => {
        savedStatusTimer = undefined;
        if (untrack(autoSaveStatus) === "saved") {
          setAutoSaveStatus("idle");
        }
      }, 2000);
    } catch (err) {
      if (!saveCurrent(scope)) return;
      setSaveError(`${t("errors.autoSaveFailed")}: ${err}`);
      setAutoSaveStatus("error");
    }
  }, 2000);

  const scheduleAutoSave = () => debouncedAutoSave(captureSave());

  // Flush ordinary navigation; stale scheduled identities are discarded by saveCurrent.
  onCleanup(() => {
    debouncedAutoSave.flush();
    disposed = true;
    if (savedStatusTimer !== undefined) {
      window.clearTimeout(savedStatusTimer);
    }
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
      setTitle(node.title);
      // Convert relative file path links from API to SPA URLs for the editor.
      const wsId = user()?.workspace_id || "";
      const spaContent = relativeLinksToSpaUrls(node.content || "", wsId);
      setContent(spaContent);
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
    setTitle("");
    setContent("");
    setIcon("");
    setCover("");
    setHasUnsavedChanges(false);
    setAutoSaveStatus("idle");
    setShowHistory(false);
    setHistory([]);
    setLinkedNodeTitles({});
    setExternalChange(false);
    setSyncedModified(null);
  }

  async function handleIconChange(newIcon: string) {
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
    const nodeId = selectedNodeId();
    const scope = captureSave();
    const ws = scope.ws;
    if (!saveCurrent(scope)) return;
    if (!nodeId || !ws) return;
    setCover(newCover);
    try {
      const saved = await ws.updatePageFrontmatter(nodeId, { icon: icon(), cover: newCover });
      if (!saveCurrent(scope)) return;
      setSyncedModified(saved.modified);
    } catch (err) {
      setSaveError(`${t("errors.failedToSave")}: ${err}`);
    }
  }

  function handleTitleChange(newTitle: string) {
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
    setContent(newContent);
    setHasUnsavedChanges(true);
    scheduleAutoSave();
  }

  // Local loading state for history - can be exposed later for UI feedback
  const [_loadingHistory, setLoadingHistory] = createSignal(false);

  async function loadHistory(nodeId: string) {
    if (showHistory()) {
      setShowHistory(false);
      return;
    }
    const scope = captureSave();
    const ws = scope.ws;
    if (!saveCurrent(scope)) return;
    if (!ws) return;

    try {
      setLoadingHistory(true);
      const data = await ws.listNodeVersions(nodeId, { Limit: 100 });
      if (!saveCurrent(scope)) return;
      setHistory((data.history?.filter(Boolean) as Commit[]) || []);
      setShowHistory(true);
    } catch (err) {
      setLoadError(`${t("errors.failedToLoad")}: ${err}`);
    } finally {
      setLoadingHistory(false);
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
    setShowHistory,
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
    flushAutoSave: debouncedAutoSave.flush,
    resetEditor,
    handleTitleChange,
    handleContentChange,
    undo: undoActions.undo,
    redo: undoActions.redo,
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
