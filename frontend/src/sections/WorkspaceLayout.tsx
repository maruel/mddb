// Workspace layout with persistent preparation recovery, draft-gated navigation, and route-owned sidebar History.

import {
  createEffect,
  createMemo,
  createSignal,
  on,
  Show,
  For,
  onCleanup,
  onMount,
  type ParentComponent,
} from "solid-js";
import type { EditorIntent } from "../contexts/EditorContext";
import { useLocation, useNavigate } from "@solidjs/router";
import Sidebar from "../components/Sidebar";
import UserMenu from "../components/UserMenu";
import NotificationBell from "../components/NotificationBell";
import NotionImportBanner from "../components/NotionImportBanner";
import CreateWorkspaceModal from "../components/CreateWorkspaceModal";
import NotionImportModal, { type NotionImportData } from "../components/NotionImportModal";
import KeyboardShortcutsDialog from "../components/KeyboardShortcutsDialog";
import { TransientFeedback } from "../components/TransientFeedback";
import { Button } from "../components/shared";
import { useWorkspaceData } from "../contexts/WorkspaceDataContext";
import { useWorkspaceNavigation } from "../contexts/WorkspaceNavigationContext";
import { useGoMode } from "../gomode/GoMode";
import { useAuth, useWorkspace, useEditor, useEventSource, useRecords } from "../contexts";
import { useI18n } from "../i18n";
import { settingsUrl, nodeUrl, parseNodeUrl } from "../utils/urls";
import type { NodeResponse, NotionImportStatusResponse } from "@sdk/types.gen";
import styles from "./WorkspaceSection.module.css";

import MenuIcon from "@material-symbols/svg-400/outlined/menu.svg?solid";

const WorkspaceLayout: ParentComponent = (props) => {
  const { t } = useI18n();
  const workspaceData = useWorkspaceData();
  const navigate = useNavigate();
  const location = useLocation();
  const { user, wsApi, orgApi, identity: authIdentity } = useAuth();
  let disposed = false;
  const {
    nodes,
    selectedNodeId,
    selectedNodeData,
    breadcrumbPath,
    loading,
    failedNodeId,
    setCreatingNode,
    setDeletingNodeId,
    loadError,
    setLoadError,
    saveError,
    setSaveError,
    switchWorkspace,
    createWorkspace,
    refreshRoots,
    fetchNodeChildren,
    removeNode,
    moveNode,
  } = useWorkspace();
  const { connected, lastEvent } = useEventSource();
  const { beginIntent, prepareDeparture, editingLocked, loadHistory, undo: editorUndo, redo: editorRedo } = useEditor();
  const {
    undo: recordsUndo,
    redo: recordsRedo,
    saveError: recordSaveError,
    setSaveError: setRecordSaveError,
  } = useRecords();

  const goMode = useGoMode();
  const voiceOwner = createMemo(() => `${authIdentity()}:${user()?.workspace_id ?? ""}`);
  createEffect(
    on(voiceOwner, () => {
      goMode.shareWorkspaceEvents(lastEvent);
    }),
  );

  // Global Ctrl-Z / Ctrl-Shift-Z handler for application-level undo/redo.
  // ProseMirror and native inputs handle their own undo — skip if an editable
  // element has focus so we do not interfere.
  onMount(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!(e.ctrlKey || e.metaKey) || e.key.toLowerCase() !== "z") return;
      const active = document.activeElement as HTMLElement | null;
      if (active?.isContentEditable || active?.tagName === "INPUT" || active?.tagName === "TEXTAREA") return;

      e.preventDefault();
      const isTable = selectedNodeData()?.has_table ?? false;
      if (e.shiftKey) {
        if (isTable) {
          void recordsRedo().catch((backgroundError: unknown) => reportError(backgroundError));
        } else {
          void editorRedo().catch((backgroundError: unknown) => reportError(backgroundError));
        }
      } else {
        if (isTable) {
          void recordsUndo().catch((backgroundError: unknown) => reportError(backgroundError));
        } else {
          void editorUndo().catch((backgroundError: unknown) => reportError(backgroundError));
        }
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    onCleanup(() => window.removeEventListener("keydown", handleKeyDown));
  });

  // Keep desktop collapse and mobile overlay state independent across breakpoint changes.
  const [isMobileLayout, setIsMobileLayout] = createSignal(window.innerWidth <= 768);
  const [desktopSidebarOpen, setDesktopSidebarOpen] = createSignal(true);
  const [mobileSidebarOpen, setMobileSidebarOpen] = createSignal(false);
  const [showCreateWorkspace, setShowCreateWorkspace] = createSignal(false);
  const [showNotionImport, setShowNotionImport] = createSignal(false);
  const [showKeyboardShortcuts, setShowKeyboardShortcuts] = createSignal(false);
  const [notionImportStatus, setNotionImportStatus] = createSignal<NotionImportStatusResponse | null>(null);
  const [notionImportWsId, setNotionImportWsId] = createSignal<string | null>(null);
  const [nodeCreationParentId, setNodeCreationParentId] = createSignal<string | null>(null);

  const isSidebarOpen = () => (isMobileLayout() ? mobileSidebarOpen() : desktopSidebarOpen());
  const toggleSidebar = () => {
    if (isMobileLayout()) {
      setMobileSidebarOpen(!mobileSidebarOpen());
    } else {
      setDesktopSidebarOpen(!desktopSidebarOpen());
    }
  };
  const closeSidebar = () => {
    if (isMobileLayout()) {
      setMobileSidebarOpen(false);
    } else {
      setDesktopSidebarOpen(false);
    }
  };
  const navigation = useWorkspaceNavigation();
  navigation.registerView({
    beginNavigation() {
      const intent = beginIntent();
      return {
        ...intent,
        async drain() {
          const saved = await intent.drain();
          if (saved && isMobileLayout()) setMobileSidebarOpen(false);
          return saved;
        },
      };
    },
    selectedNodeId,
    selectedNodeData,
    failedNodeId,
    async beforeNavigate() {
      if (!(await prepareDeparture())) return false;
      if (isMobileLayout()) setMobileSidebarOpen(false);
      return true;
    },
  });

  // Sidebar History follows the route-owned load, including a slow draft flush.
  const [historyTarget, setHistoryTarget] = createSignal<{
    id: string;
    identity: number;
    wsId: string;
    from: string;
    intent: EditorIntent;
  } | null>(null);
  createEffect(
    on(
      [historyTarget, selectedNodeId, failedNodeId, () => location.pathname, authIdentity, () => user()?.workspace_id],
      () => {
        const target = historyTarget();
        if (!target) return;
        const route = parseNodeUrl(location.pathname);
        const matches = route?.wsId === target.wsId && route.nodeId === target.id;
        if (
          !target.intent.current() ||
          authIdentity() !== target.identity ||
          user()?.workspace_id !== target.wsId ||
          (!matches && location.pathname !== target.from) ||
          failedNodeId() === target.id
        ) {
          setHistoryTarget(null);
        } else if (matches && selectedNodeId() === target.id) {
          setHistoryTarget(null);
          void loadHistory(target.id, target.intent);
        }
      },
    ),
  );

  async function handleShowHistory(nodeId: string) {
    if (editingLocked()) return;
    setHistoryTarget(null);
    const intent = beginIntent();
    const u = user();
    const wsId = u?.workspace_id;
    if (nodeId !== selectedNodeId() && wsId) {
      // Install the route-following intent only after THIS departure succeeds.
      if (!(await intent.drain())) return;
      setHistoryTarget({ id: nodeId, identity: authIdentity(), wsId, from: location.pathname, intent });
      intent.navigate(nodeUrl(wsId, u.workspace_name, nodeId));
    } else {
      setHistoryTarget(null);
      void loadHistory(nodeId, intent);
    }
  }

  const isFirstWorkspace = () => {
    const currentUser = user();
    return !currentUser?.workspace_id && (currentUser?.workspaces ?? []).length === 0;
  };

  const isEditingText = (target: EventTarget | null) => {
    if (!(target instanceof HTMLElement)) return false;
    return (
      target.isContentEditable ||
      target.matches('input, textarea, select, [role="textbox"]') ||
      target.closest('[contenteditable="true"], input, textarea, select, [role="textbox"]') !== null
    );
  };

  const hasOpenModal = () => document.querySelector('[aria-modal="true"]') !== null;

  const focusWorkspaceTree = () => {
    if (isMobileLayout()) {
      setMobileSidebarOpen(true);
    } else {
      setDesktopSidebarOpen(true);
    }

    let attempts = 0;
    const focusActiveTreeItem = () => {
      const tree = document.querySelector<HTMLElement>('[data-testid="workspace-tree"]');
      const activeTreeItem = tree?.querySelector<HTMLButtonElement>('[role="treeitem"][tabindex="0"]');
      if (activeTreeItem) {
        activeTreeItem.focus();
        return;
      }
      attempts += 1;
      if (attempts < 3) requestAnimationFrame(focusActiveTreeItem);
    };
    requestAnimationFrame(focusActiveTreeItem);
  };

  onMount(() => {
    const handleResize = () => {
      const nextIsMobile = window.innerWidth <= 768;
      if (nextIsMobile !== isMobileLayout()) {
        setIsMobileLayout(nextIsMobile);
        setMobileSidebarOpen(false);
      }
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented) return;
      if (event.key === "Escape" && isMobileLayout() && mobileSidebarOpen()) {
        event.preventDefault();
        setMobileSidebarOpen(false);
      }
    };

    window.addEventListener("resize", handleResize);
    window.addEventListener("keydown", handleKeyDown);
    onCleanup(() => {
      window.removeEventListener("resize", handleResize);
      window.removeEventListener("keydown", handleKeyDown);
    });
  });

  onMount(() => {
    const handleKeyboardShortcut = (event: KeyboardEvent) => {
      if (
        event.defaultPrevented ||
        event.isComposing ||
        event.ctrlKey ||
        event.metaKey ||
        event.altKey ||
        isEditingText(event.target) ||
        hasOpenModal()
      ) {
        return;
      }

      if (event.key === "?") {
        event.preventDefault();
        setShowKeyboardShortcuts(true);
        return;
      }

      if (event.key.toLowerCase() === "g" && !event.shiftKey) {
        event.preventDefault();
        focusWorkspaceTree();
      }
    };

    window.addEventListener("keydown", handleKeyboardShortcut);
    onCleanup(() => window.removeEventListener("keydown", handleKeyboardShortcut));
  });

  // Notion import polling
  let importPollInterval: number | undefined;

  const startNotionImportPolling = (wsId: string, org: NonNullable<ReturnType<typeof orgApi>>, identity: number) => {
    setNotionImportWsId(wsId);
    const pollImport = async () => {
      if (disposed || authIdentity() !== identity) {
        window.clearInterval(importPollInterval);
        importPollInterval = undefined;
        return;
      }
      try {
        const status = await org.getStatus(wsId);
        // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- Callbacks and asynchronous work can change this state after narrowing.
        if (disposed || authIdentity() !== identity) return;
        setNotionImportStatus(status);
        if (["completed", "failed", "cancelled"].includes(status.status)) {
          window.clearInterval(importPollInterval);
          importPollInterval = undefined;
          // Reload nodes if import completed
          if (status.status === "completed") {
            void refreshRoots().catch((error: unknown) => console.warn("Workspace refresh failed", error));
          }
        }
      } catch {
        window.clearInterval(importPollInterval);
        importPollInterval = undefined;
      }
    };
    importPollInterval = window.setInterval(() => {
      void pollImport().catch((error: unknown) => console.error("Import polling failed:", error));
    }, 2000);
  };

  const handleNotionImport = async (data: NotionImportData): Promise<string | null> => {
    const org = orgApi();
    if (!org) return null;
    const identity = authIdentity();
    try {
      const result = await org.startImport({ notion_token: data.notionToken });
      return !disposed && authIdentity() === identity ? result.workspace_id : null;
    } catch (error) {
      if (disposed || authIdentity() !== identity) return null;
      throw error;
    }
  };

  const handleOpenImportedWorkspace = async (wsId: string): Promise<boolean> => {
    const org = orgApi();
    if (!org) return false;
    const identity = authIdentity();
    if (!(await switchWorkspace(wsId)) || disposed || authIdentity() !== identity) return false;
    startNotionImportPolling(wsId, org, identity);
    setNotionImportStatus({ status: "running", progress: 0, total: 0 });
    return true;
  };

  const handleCancelNotionImport = async () => {
    const ws = wsApi();
    const wsId = notionImportWsId();
    if (!ws || !wsId) return;
    try {
      await ws.cancelImport();
      setNotionImportStatus((prev) => (prev ? { ...prev, status: "cancelled" } : null));
    } catch {
      // Ignore
    }
  };

  const dismissNotionImportBanner = () => {
    if (importPollInterval) {
      window.clearInterval(importPollInterval);
      importPollInterval = undefined;
    }
    setNotionImportStatus(null);
    setNotionImportWsId(null);
  };

  onCleanup(() => {
    disposed = true;
    if (importPollInterval) {
      window.clearInterval(importPollInterval);
    }
  });

  // Find a node's parent and siblings in the tree
  function findNodeContext(
    nodeId: string,
    nodeList: NodeResponse[],
    parent: NodeResponse | null,
  ): { parent: NodeResponse | null; siblings: NodeResponse[]; index: number } | null {
    const index = nodeList.findIndex((n) => n.id === nodeId);
    if (index !== -1) {
      return { parent, siblings: nodeList, index };
    }
    for (const node of nodeList) {
      if (node.children) {
        const result = findNodeContext(nodeId, node.children, node);
        if (result) return result;
      }
    }
    return null;
  }

  // Create node helper
  async function createNode(type: "document" | "table", parentId: string | null) {
    const intent = await prepareDeparture();
    if (!intent) return;
    const identity = authIdentity();
    const initialWorkspace = user()?.workspace_id;
    const isCurrent = () => !disposed && authIdentity() === identity && user()?.workspace_id === initialWorkspace;
    const ws = wsApi();
    if (!ws) return;

    try {
      setCreatingNode(true);
      const parent = parentId || nodeCreationParentId() || "0";
      let newNodeId: string | number;
      const defaultTitle =
        type === "table"
          ? t("welcome.untitledTable") || "Untitled Table"
          : t("welcome.untitledPage") || "Untitled Page";
      if (type === "table") {
        const result = await ws.createTable(parent, {
          title: defaultTitle,
          properties: [{ name: "Name", type: "text", required: false }],
        });
        if (!isCurrent()) return;
        newNodeId = result.id;
      } else {
        const result = await ws.createPage(parent, { title: defaultTitle });
        if (!isCurrent()) return;
        newNodeId = result.id;
      }
      // The mutation is acknowledged. Canonical refresh has its own error and
      // retry; a failed refresh must not invite another creation.
      await refreshRoots().catch((error: unknown) => console.warn("Workspace refresh failed", error));
      if (!isCurrent()) return;
      const u = user();
      const wsId = u?.workspace_id;
      const wsName = u?.workspace_name;
      if (wsId && intent.current()) {
        intent.navigate(nodeUrl(wsId, wsName, String(newNodeId), defaultTitle));
      }
      setSaveError(null);
      setNodeCreationParentId(null);
    } catch (err) {
      if (isCurrent()) setSaveError(`${t("errors.failedToCreate")}: ${String(err)}`);
    } finally {
      if (isCurrent()) setCreatingNode(false);
    }
  }

  const handleNodeClick = (node: NodeResponse) => {
    setHistoryTarget(null);
    const u = user();
    const wsId = u?.workspace_id;
    const wsName = u?.workspace_name;
    if (wsId) {
      navigate(nodeUrl(wsId, wsName, node.id, node.title));
    }
    // Only close sidebar on mobile
    if (isMobileLayout()) {
      setMobileSidebarOpen(false);
    }
  };

  async function handleDeleteNode(nodeId: string) {
    const selected = nodeId === selectedNodeId();
    const intent = selected ? await prepareDeparture() : null;
    if (selected && !intent) return;
    const identity = authIdentity();
    const initialWorkspace = user()?.workspace_id;
    const isCurrent = () => !disposed && authIdentity() === identity && user()?.workspace_id === initialWorkspace;
    const ws = wsApi();
    if (!ws) return;
    if (!confirm(t("table.confirmDeleteRecord") || "Delete this item?")) return;

    // Find navigation target BEFORE deleting (if this is the selected node)
    let nextNode: NodeResponse | null = null;
    if (nodeId === selectedNodeId()) {
      const ctx = findNodeContext(nodeId, nodes, null);
      if (ctx) {
        const { parent, siblings, index } = ctx;
        if (parent && parent.id !== "0") {
          nextNode = parent;
        } else {
          if (index > 0) {
            nextNode = siblings[index - 1] ?? null;
          } else if (siblings.length > 1) {
            nextNode = siblings[1] ?? null;
          }
        }
      }
    }

    try {
      setDeletingNodeId(nodeId);
      await ws.deleteNode(nodeId);
      if (!isCurrent()) return;
      removeNode(nodeId);

      if (nodeId === selectedNodeId() && intent?.current()) {
        const u = user();
        const wsId = u?.workspace_id;
        const wsName = u?.workspace_name;
        if (nextNode && wsId) {
          intent.navigate(nodeUrl(wsId, wsName, nextNode.id, nextNode.title));
        } else if (wsId) {
          // Deletion is already acknowledged; canonical retry owns refresh.
          await refreshRoots().catch((error: unknown) => console.warn("Workspace refresh failed", error));
          if (!isCurrent()) return;
          // Navigate to workspace root, let WorkspaceRoot handle redirect
          intent.navigate(`/w/@${wsId}+${wsName || "workspace"}/`);
        }
      }
      setSaveError(null);
    } catch (err) {
      if (isCurrent()) setSaveError(`${t("errors.failedToDelete")}: ${String(err)}`);
    } finally {
      if (isCurrent()) setDeletingNodeId(null);
    }
  }

  async function handleMoveNode(nodeId: string, newParentId: string): Promise<boolean> {
    try {
      if (!(await moveNode(nodeId, newParentId))) return false;
      setSaveError(null);
      return true;
    } catch (error) {
      setSaveError(`${t("errors.failedToMove")}: ${String(error)}`);
      return false;
    }
  }

  async function handleFetchChildren(nodeId: string): Promise<void> {
    try {
      await fetchNodeChildren(nodeId);
    } catch (error) {
      setLoadError(`${t("errors.failedToLoad")}: ${String(error)}`);
    }
  }

  return (
    <div class={`${styles.app} ${isSidebarOpen() ? styles.sidebarOpen : ""}`}>
      <Show when={notionImportStatus()} keyed>
        {(status) => (
          <NotionImportBanner
            status={status}
            onCancel={handleCancelNotionImport}
            onDismiss={dismissNotionImportBanner}
          />
        )}
      </Show>
      <header class={styles.header}>
        <div class={styles.headerLeft}>
          <button
            class={styles.hamburger}
            onClick={toggleSidebar}
            aria-label="Toggle menu"
            aria-expanded={isSidebarOpen()}
          >
            <MenuIcon />
          </button>
          <Show when={selectedNodeId()}>
            <nav class={styles.breadcrumbs} aria-label={t("app.breadcrumbs")}>
              <ol class={styles.breadcrumbList}>
                <For each={breadcrumbPath()}>
                  {(crumb, i) => (
                    <li class={styles.breadcrumbSegment}>
                      <Show when={i() > 0}>
                        <span class={styles.breadcrumbSeparator} aria-hidden="true">
                          /
                        </span>
                      </Show>
                      <Show
                        when={i() < breadcrumbPath().length - 1}
                        fallback={
                          <span class={styles.breadcrumbCurrent} aria-current="page">
                            {crumb.title}
                          </span>
                        }
                      >
                        <button type="button" class={styles.breadcrumbItem} onClick={() => handleNodeClick(crumb)}>
                          {crumb.title}
                        </button>
                      </Show>
                    </li>
                  )}
                </For>
              </ol>
            </nav>
          </Show>
        </div>
        <div class={styles.userInfo}>
          <span
            class={styles.connectionStatus}
            classList={{ [styles.connectionReconnecting]: !connected() }}
            role="status"
            aria-live="polite"
            aria-atomic="true"
            data-testid="connection-status"
          >
            <span class={styles.connectionIndicator} aria-hidden="true" />
            {connected() ? t("sse.connected") || "Connected" : t("sse.reconnecting") || "Reconnecting"}
          </span>
          <NotificationBell />
          <UserMenu
            onProfile={() => {
              navigate(settingsUrl("user"));
            }}
          />
        </div>
      </header>

      <div class={styles.container}>
        <Show when={isMobileLayout()}>
          <div
            class={`${styles.mobileBackdrop} ${mobileSidebarOpen() ? styles.mobileBackdropVisible : ""}`}
            data-testid="workspace-sidebar-backdrop"
            onClick={() => setMobileSidebarOpen(false)}
          />
        </Show>
        <Sidebar
          isOpen={isSidebarOpen()}
          loading={loading()}
          nodes={nodes}
          selectedNodeId={selectedNodeId()}
          ancestorIds={breadcrumbPath().map((n) => n.id)}
          onCreatePage={() => {
            setNodeCreationParentId(null);
            void createNode("document", null).catch((backgroundError: unknown) => reportError(backgroundError));
            closeSidebar();
          }}
          onCreateTable={() => {
            setNodeCreationParentId(null);
            void createNode("table", null).catch((backgroundError: unknown) => reportError(backgroundError));
            closeSidebar();
          }}
          onCreateChildPage={(parentId: string) => {
            setNodeCreationParentId(parentId);
            void createNode("document", parentId).catch((backgroundError: unknown) => reportError(backgroundError));
            closeSidebar();
          }}
          onCreateChildTable={(parentId: string) => {
            setNodeCreationParentId(parentId);
            void createNode("table", parentId).catch((backgroundError: unknown) => reportError(backgroundError));
            closeSidebar();
          }}
          onSelectNode={handleNodeClick}
          onCloseMobileSidebar={closeSidebar}
          onFetchChildren={handleFetchChildren}
          onDeleteNode={handleDeleteNode}
          onShowHistory={handleShowHistory}
          onOpenSettings={() => {
            const wsId = user()?.workspace_id;
            const wsName = user()?.workspace_name;
            if (wsId) {
              closeSidebar();
              navigate(settingsUrl("workspace", wsId, wsName));
            }
          }}
          onCreateWorkspace={() => setShowCreateWorkspace(true)}
          onImportFromNotion={() => setShowNotionImport(true)}
          onShowKeyboardShortcuts={() => setShowKeyboardShortcuts(true)}
          onMoveNode={handleMoveNode}
        />

        <main class={styles.main}>
          <Show
            when={workspaceData.preparationError()}
            fallback={
              <Show
                when={loadError()}
                fallback={
                  <Show when={saveError() || recordSaveError()}>
                    {(error) => (
                      <TransientFeedback
                        message={error()}
                        onDismiss={() => {
                          if (saveError()) setSaveError(null);
                          else setRecordSaveError(null);
                        }}
                      />
                    )}
                  </Show>
                }
              >
                {(error) => <TransientFeedback message={error()} onDismiss={() => setLoadError(null)} />}
              </Show>
            }
          >
            {(error) => (
              <div class={styles.preparationError} role="alert">
                <span class={styles.preparationMessage}>{error()}</span>
                <Button
                  variant="primary"
                  class={styles.preparationRetry}
                  onClick={() =>
                    void workspaceData
                      .retryPreparation()
                      .catch((failure: unknown) => console.warn("Workspace preparation retry failed", failure))
                  }
                >
                  {t("recovery.retry")}
                </Button>
              </div>
            )}
          </Show>
          {props.children}
        </main>
      </div>

      <Show when={showCreateWorkspace()}>
        <CreateWorkspaceModal
          onClose={() => setShowCreateWorkspace(false)}
          onCreate={createWorkspace}
          onOpen={switchWorkspace}
          isFirstWorkspace={isFirstWorkspace()}
        />
      </Show>
      <Show when={showNotionImport()}>
        <NotionImportModal
          onClose={() => setShowNotionImport(false)}
          onImport={handleNotionImport}
          onOpen={handleOpenImportedWorkspace}
        />
      </Show>
      <Show when={showKeyboardShortcuts()}>
        <KeyboardShortcutsDialog onClose={() => setShowKeyboardShortcuts(false)} />
      </Show>
    </div>
  );
};

export default WorkspaceLayout;
