// Workspace layout with header, sidebar, and content outlet.

import { createSignal, Show, For, onCleanup, type ParentComponent } from 'solid-js';
import { useNavigate } from '@solidjs/router';
import Sidebar from '../components/Sidebar';
import UserMenu from '../components/UserMenu';
import NotificationBell from '../components/NotificationBell';
import NotionImportBanner from '../components/NotionImportBanner';
import CreateWorkspaceModal from '../components/CreateWorkspaceModal';
import NotionImportModal, { type NotionImportData } from '../components/NotionImportModal';
import { useAuth, useWorkspace, useEditor } from '../contexts';
import { useI18n } from '../i18n';
import { settingsUrl, nodeUrl } from '../utils/urls';
import type { NodeResponse, NotionImportStatusResponse } from '@sdk/types.gen';
import styles from './WorkspaceSection.module.css';

import MenuIcon from '@material-symbols/svg-400/outlined/menu.svg?solid';

const WorkspaceLayout: ParentComponent = (props) => {
  const { t } = useI18n();
  const navigate = useNavigate();
  const { user, wsApi, orgApi } = useAuth();
  const {
    nodes,
    selectedNodeId,
    breadcrumbPath,
    loading,
    setCreatingNode,
    setDeletingNodeId,
    loadError,
    saveError,
    setSaveError,
    switchWorkspace,
    createWorkspace,
    loadNodes,
    fetchNodeChildren,
    removeNode,
    moveNode,
  } = useWorkspace();
  const { flushAutoSave, loadHistory } = useEditor();

  // UI state - sidebar open by default on desktop, closed on mobile
  const [showMobileSidebar, setShowMobileSidebar] = createSignal(window.innerWidth > 768);
  const [showCreateWorkspace, setShowCreateWorkspace] = createSignal(false);
  const [showNotionImport, setShowNotionImport] = createSignal(false);
  const [notionImportStatus, setNotionImportStatus] = createSignal<NotionImportStatusResponse | null>(null);
  const [notionImportWsId, setNotionImportWsId] = createSignal<string | null>(null);
  const [nodeCreationParentId, setNodeCreationParentId] = createSignal<string | null>(null);

  // Notion import polling
  let importPollInterval: number | undefined;

  const startNotionImportPolling = (wsId: string) => {
    setNotionImportWsId(wsId);
    importPollInterval = window.setInterval(async () => {
      const org = orgApi();
      if (!org) return;
      try {
        const status = await org.notion.getStatus(wsId);
        setNotionImportStatus(status);
        if (['completed', 'failed', 'cancelled'].includes(status.status)) {
          window.clearInterval(importPollInterval);
          importPollInterval = undefined;
          // Reload nodes if import completed
          if (status.status === 'completed') {
            loadNodes(true);
          }
        }
      } catch {
        window.clearInterval(importPollInterval);
        importPollInterval = undefined;
      }
    }, 2000);
  };

  const handleNotionImport = async (data: NotionImportData) => {
    const org = orgApi();
    if (!org) return;
    const result = await org.notion.startImport({
      notion_token: data.notionToken,
    });
    // Switch to the new workspace (switchWorkspace navigates automatically)
    await switchWorkspace(result.workspace_id);
    // Start polling for import status
    startNotionImportPolling(result.workspace_id);
    setNotionImportStatus({ status: 'running', progress: 0, total: 0 });
  };

  const handleCancelNotionImport = async () => {
    const ws = wsApi();
    const wsId = notionImportWsId();
    if (!ws || !wsId) return;
    try {
      await ws.notion.cancelImport();
      setNotionImportStatus((prev) => (prev ? { ...prev, status: 'cancelled' } : null));
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
    if (importPollInterval) {
      window.clearInterval(importPollInterval);
    }
  });

  // Find a node's parent and siblings in the tree
  function findNodeContext(
    nodeId: string,
    nodeList: NodeResponse[] = nodes,
    parent: NodeResponse | null = null
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
  async function createNode(type: 'document' | 'table' = 'document', parentId?: string) {
    const ws = wsApi();
    if (!ws) return;

    try {
      setCreatingNode(true);
      const parent = parentId || nodeCreationParentId() || '0';
      let newNodeId: string | number;
      const defaultTitle =
        type === 'table'
          ? t('welcome.untitledTable') || 'Untitled Table'
          : t('welcome.untitledPage') || 'Untitled Page';
      if (type === 'table') {
        const result = await ws.nodes.table.createTable(parent, {
          title: defaultTitle,
          properties: [{ name: 'Name', type: 'text', required: false }],
        });
        newNodeId = result.id;
      } else {
        const result = await ws.nodes.page.createPage(parent, { title: defaultTitle });
        newNodeId = result.id;
      }
      await loadNodes(true);
      const u = user();
      const wsId = u?.workspace_id;
      const wsName = u?.workspace_name;
      if (wsId) {
        navigate(nodeUrl(wsId, wsName, String(newNodeId), defaultTitle));
      }
      setSaveError(null);
      setNodeCreationParentId(null);
    } catch (err) {
      setSaveError(`${t('errors.failedToCreate')}: ${err}`);
    } finally {
      setCreatingNode(false);
    }
  }

  const handleNodeClick = (node: NodeResponse) => {
    flushAutoSave();
    const u = user();
    const wsId = u?.workspace_id;
    const wsName = u?.workspace_name;
    if (wsId) {
      navigate(nodeUrl(wsId, wsName, node.id, node.title));
    }
    // Only close sidebar on mobile
    if (window.innerWidth <= 768) {
      setShowMobileSidebar(false);
    }
  };

  async function handleDeleteNode(nodeId: string) {
    const ws = wsApi();
    if (!ws) return;
    if (!confirm(t('table.confirmDeleteRecord') || 'Delete this item?')) return;

    // Find navigation target BEFORE deleting (if this is the selected node)
    let nextNode: NodeResponse | null = null;
    if (nodeId === selectedNodeId()) {
      const ctx = findNodeContext(nodeId);
      if (ctx) {
        const { parent, siblings, index } = ctx;
        if (parent && parent.id !== '0') {
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
      await ws.nodes.deleteNode(nodeId);
      removeNode(nodeId);

      if (nodeId === selectedNodeId()) {
        const u = user();
        const wsId = u?.workspace_id;
        const wsName = u?.workspace_name;
        if (nextNode && wsId) {
          navigate(nodeUrl(wsId, wsName, nextNode.id, nextNode.title));
        } else if (wsId) {
          await loadNodes(true);
          // Navigate to workspace root, let WorkspaceRoot handle redirect
          navigate(`/w/@${wsId}+${wsName || 'workspace'}/`);
        }
      }
      setSaveError(null);
    } catch (err) {
      setSaveError(`${t('errors.failedToDelete')}: ${err}`);
    } finally {
      setDeletingNodeId(null);
    }
  }

  async function handleMoveNode(nodeId: string, newParentId: string) {
    await moveNode(nodeId, newParentId);
  }

  return (
    <div class={`${styles.app} ${showMobileSidebar() ? styles.sidebarOpen : ''}`}>
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
            onClick={() => setShowMobileSidebar(!showMobileSidebar())}
            aria-label="Toggle menu"
          >
            <MenuIcon />
          </button>
          <Show when={selectedNodeId()}>
            <nav class={styles.breadcrumbs}>
              <For each={breadcrumbPath()}>
                {(crumb, i) => (
                  <>
                    <Show when={i() > 0}>
                      <span class={styles.breadcrumbSeparator}>/</span>
                    </Show>
                    <span class={styles.breadcrumbItem} onClick={() => handleNodeClick(crumb)}>
                      {crumb.title}
                    </span>
                  </>
                )}
              </For>
            </nav>
          </Show>
        </div>
        <div class={styles.userInfo}>
          <NotificationBell />
          <UserMenu
            onProfile={() => {
              navigate(settingsUrl('user'));
            }}
          />
        </div>
      </header>

      <div class={styles.container}>
        <div
          class={`${styles.mobileBackdrop} ${showMobileSidebar() ? styles.mobileBackdropVisible : ''}`}
          onClick={() => setShowMobileSidebar(false)}
        />
        <Sidebar
          isOpen={showMobileSidebar()}
          loading={loading()}
          nodes={nodes}
          selectedNodeId={selectedNodeId()}
          ancestorIds={breadcrumbPath().map((n) => n.id)}
          onCreatePage={() => {
            setNodeCreationParentId(null);
            createNode('document');
            setShowMobileSidebar(false);
          }}
          onCreateTable={() => {
            setNodeCreationParentId(null);
            createNode('table');
            setShowMobileSidebar(false);
          }}
          onCreateChildPage={(parentId: string) => {
            setNodeCreationParentId(parentId);
            createNode('document', parentId);
            setShowMobileSidebar(false);
          }}
          onCreateChildTable={(parentId: string) => {
            setNodeCreationParentId(parentId);
            createNode('table', parentId);
            setShowMobileSidebar(false);
          }}
          onSelectNode={handleNodeClick}
          onCloseMobileSidebar={() => setShowMobileSidebar(false)}
          onFetchChildren={fetchNodeChildren}
          onDeleteNode={handleDeleteNode}
          onShowHistory={(nodeId: string) => {
            const u = user();
            const wsId = u?.workspace_id;
            const wsName = u?.workspace_name;
            if (nodeId !== selectedNodeId() && wsId) {
              navigate(nodeUrl(wsId, wsName, nodeId));
              // Load history after navigation settles
              setTimeout(() => loadHistory(nodeId), 100);
            } else {
              loadHistory(nodeId);
            }
          }}
          onOpenSettings={() => {
            const wsId = user()?.workspace_id;
            const wsName = user()?.workspace_name;
            if (wsId) {
              setShowMobileSidebar(false);
              navigate(settingsUrl('workspace', wsId, wsName));
            }
          }}
          onCreateWorkspace={() => setShowCreateWorkspace(true)}
          onImportFromNotion={() => setShowNotionImport(true)}
          onMoveNode={handleMoveNode}
        />

        <main class={styles.main}>
          <Show when={loadError() || saveError()}>
            <div class={styles.error} role="alert" aria-live="polite">
              {loadError() || saveError()}
            </div>
          </Show>
          {props.children}
        </main>
      </div>

      <Show when={showCreateWorkspace()}>
        <CreateWorkspaceModal
          onClose={() => setShowCreateWorkspace(false)}
          onCreate={createWorkspace}
          isFirstWorkspace={true}
        />
      </Show>
      <Show when={showNotionImport()}>
        <NotionImportModal onClose={() => setShowNotionImport(false)} onImport={handleNotionImport} />
      </Show>
    </div>
  );
};

export default WorkspaceLayout;
