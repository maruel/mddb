// Main application component managing routing and layout.

import { createSignal, createEffect, For, Show, onMount, onCleanup, batch } from 'solid-js';
import Sidebar from './components/Sidebar';
import Editor from './components/editor/Editor';
import TableTable from './components/TableTable';
import TableGrid from './components/TableGrid';
import TableGallery from './components/TableGallery';
import TableBoard from './components/TableBoard';
import WorkspaceSettings from './components/WorkspaceSettings';
import OrganizationSettings from './components/OrganizationSettings';
import UserProfile from './components/UserProfile';
import Auth from './components/Auth';
import Privacy from './components/Privacy';
import Terms from './components/Terms';
import PWAInstallBanner from './components/PWAInstallBanner';
import CreateOrgModal from './components/CreateOrgModal';
import CreateWorkspaceModal from './components/CreateWorkspaceModal';
import NotionImportModal, { type NotionImportData } from './components/NotionImportModal';
import NotionImportBanner from './components/NotionImportBanner';
import UserMenu from './components/UserMenu';
import WorkspaceMenu from './components/WorkspaceMenu';
import AppErrorBoundary from './components/ErrorBoundary';
import {
  AuthProvider,
  useAuth,
  WorkspaceProvider,
  useWorkspace,
  EditorProvider,
  useEditor,
  RecordsProvider,
  useRecords,
} from './contexts';
import {
  workspaceUrl,
  workspaceSettingsUrl,
  orgSettingsUrl,
  parseWorkspaceRoot,
  parseNodeUrl,
  parseWorkspaceSettings,
  parseOrgSettings,
  isStaticRoute,
} from './utils/urls';
import { useI18n, type Locale } from './i18n';
import type { NodeResponse, OrgMembershipResponse, NotionImportStatusResponse } from '@sdk/types.gen';
import styles from './App.module.css';

// Inner app component that uses contexts
function AppContent() {
  const { t, locale, setLocale } = useI18n();
  const { user, login, wsApi, orgApi } = useAuth();
  const {
    nodes,
    selectedNodeId,
    setSelectedNodeId,
    selectedNodeData,
    setSelectedNodeData,
    breadcrumbPath,
    loading,
    setLoading,
    error,
    setError,
    switchWorkspace,
    createOrganization,
    createWorkspace,
    loadNodes,
    loadNode,
    fetchNodeChildren,
  } = useWorkspace();

  const {
    title,
    content,
    hasUnsavedChanges,
    autoSaveStatus,
    showHistory,
    history,
    loadHistory,
    loadVersion,
    flushAutoSave,
    handleTitleChange,
    handleContentChange,
  } = useEditor();

  const { records, hasMore, loadMoreRecords, addRecord, updateRecord, deleteRecord } = useRecords();

  // Page routing state
  const [isSettingsPage, setIsSettingsPage] = createSignal(false);
  const [isOrgSettingsPage, setIsOrgSettingsPage] = createSignal(false);
  const [orgSettingsId, setOrgSettingsId] = createSignal<string | null>(null);
  const [isProfilePage, setIsProfilePage] = createSignal(false);
  const [isPrivacyPage, setIsPrivacyPage] = createSignal(false);
  const [isTermsPage, setIsTermsPage] = createSignal(false);

  // UI state
  const [viewMode, setViewMode] = createSignal<'table' | 'grid' | 'gallery' | 'board'>('table');
  const [showMobileSidebar, setShowMobileSidebar] = createSignal(false);
  const [showCreateOrg, setShowCreateOrg] = createSignal(false);
  const [showCreateWorkspace, setShowCreateWorkspace] = createSignal(false);
  const [showNotionImport, setShowNotionImport] = createSignal(false);
  const [notionImportStatus, setNotionImportStatus] = createSignal<NotionImportStatusResponse | null>(null);
  const [notionImportWsId, setNotionImportWsId] = createSignal<string | null>(null);
  const [nodeCreationParentId, setNodeCreationParentId] = createSignal<string | null>(null);

  // Sync locale with user settings
  createEffect(() => {
    const u = user();
    const userLang = u?.settings?.language as Locale | undefined;
    if (userLang && ['en', 'fr', 'de', 'es'].includes(userLang) && userLang !== locale()) {
      setLocale(userLang);
      localStorage.setItem('mddb_locale', userLang);
    }
  });

  // Handle browser back/forward and initial URL
  const handlePopState = async () => {
    const path = window.location.pathname;

    // Check static routes
    const staticRoute = isStaticRoute(path);
    if (staticRoute === 'privacy') {
      setIsPrivacyPage(true);
      setIsTermsPage(false);
      setIsProfilePage(false);
      return;
    }
    if (staticRoute === 'terms') {
      setIsTermsPage(true);
      setIsPrivacyPage(false);
      setIsProfilePage(false);
      return;
    }
    if (staticRoute === 'profile') {
      setIsProfilePage(true);
      setIsSettingsPage(false);
      setIsPrivacyPage(false);
      setIsTermsPage(false);
      return;
    }
    setIsProfilePage(false);
    setIsPrivacyPage(false);
    setIsTermsPage(false);

    // Check for workspace settings
    const wsSettings = parseWorkspaceSettings(path);
    if (wsSettings) {
      const wsId = wsSettings.id;
      if (user() && user()?.workspace_id !== wsId) {
        try {
          const u = user();
          const isMember = u?.workspaces?.some((m) => m.workspace_id === wsId);
          if (!isMember) {
            setError(t('errors.noAccessToWs') || 'You do not have access to this workspace');
            const currentWsId = u?.workspace_id;
            const currentWsName = u?.workspace_name;
            if (currentWsId) {
              setTimeout(() => window.history.replaceState(null, '', workspaceUrl(currentWsId, currentWsName)), 2000);
            }
            return;
          }
          await switchWorkspace(wsId, false);
        } catch {
          return;
        }
      }
      setIsSettingsPage(true);
      setIsOrgSettingsPage(false);
      return;
    }
    setIsSettingsPage(false);

    // Check for organization settings
    const orgSettings = parseOrgSettings(path);
    if (orgSettings) {
      const orgId = orgSettings.id;
      const u = user();
      const isMember = u?.organizations?.some((m) => m.organization_id === orgId);
      if (!isMember) {
        setError(t('errors.noAccessToOrg') || 'You do not have access to this organization');
        const currentWsId = u?.workspace_id;
        const currentWsName = u?.workspace_name;
        if (currentWsId) {
          setTimeout(() => window.history.replaceState(null, '', workspaceUrl(currentWsId, currentWsName)), 2000);
        }
        return;
      }
      setIsOrgSettingsPage(true);
      setOrgSettingsId(orgId);
      return;
    }
    setIsOrgSettingsPage(false);
    setOrgSettingsId(null);

    // Check for node URL
    const nodeMatch = parseNodeUrl(path);
    if (nodeMatch) {
      const nodeId = nodeMatch.nodeId;
      if (nodeId !== selectedNodeId()) {
        flushAutoSave();
        loadNode(nodeId, false);
      }
      return;
    }

    // Check for workspace root
    const wsRoot = parseWorkspaceRoot(path);
    if (wsRoot) {
      if (nodes.length > 0 && nodes[0]) {
        loadNode(nodes[0].id, false);
      } else {
        setSelectedNodeId(null);
        setSelectedNodeData(null);
      }
      return;
    }

    setSelectedNodeId(null);
    setSelectedNodeData(null);
  };

  onMount(() => {
    window.addEventListener('popstate', handlePopState);
    onCleanup(() => window.removeEventListener('popstate', handlePopState));
  });

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
    // Switch to the new workspace
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

  // Handle initial route when user loads
  createEffect(() => {
    if (user()) {
      handlePopState();
    } else {
      handlePopState();
    }
  });

  async function createNode(type: 'document' | 'table' = 'document', parentId?: string) {
    if (!title().trim()) {
      setError(t('errors.titleRequired') || 'Title is required');
      return;
    }
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      const parent = parentId || nodeCreationParentId() || '0';
      let newNodeId: string | number;
      if (type === 'table') {
        const result = await ws.nodes.table.createTable(parent, { title: title(), properties: [] });
        newNodeId = result.id;
      } else {
        const result = await ws.nodes.page.createPage(parent, { title: title() });
        newNodeId = result.id;
      }
      await loadNodes(true);
      loadNode(String(newNodeId));
      setError(null);
      setNodeCreationParentId(null);
    } catch (err) {
      setError(`${t('errors.failedToCreate')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  const handleNodeClick = (node: NodeResponse) => {
    flushAutoSave();
    loadNode(node.id);
    setShowMobileSidebar(false);
  };

  async function handleDeleteNode(nodeId: string) {
    const ws = wsApi();
    if (!ws) return;
    if (!confirm(t('table.confirmDeleteRecord') || 'Delete this item?')) return;

    try {
      setLoading(true);
      await ws.nodes.deleteNode(nodeId);
      if (nodeId === selectedNodeId()) {
        setSelectedNodeId(null);
        setSelectedNodeData(null);
        const wsId = user()?.workspace_id;
        const wsName = user()?.workspace_name;
        if (wsId) {
          window.history.pushState(null, '', workspaceUrl(wsId, wsName));
        }
      }
      await loadNodes(true);
    } catch (err) {
      setError(`${t('errors.failedToDelete')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <Show when={!isPrivacyPage()} fallback={<Privacy />}>
        <Show when={!isTermsPage()} fallback={<Terms />}>
          <Show when={user()} fallback={<Auth onLogin={login} />}>
            <div class={styles.app}>
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
                    ☰
                  </button>
                  <WorkspaceMenu
                    onOpenSettings={() => {
                      setIsSettingsPage(true);
                      setSelectedNodeId(null);
                      setShowMobileSidebar(false);
                      const wsId = user()?.workspace_id;
                      const wsName = user()?.workspace_name;
                      if (wsId) {
                        window.history.pushState(null, '', workspaceSettingsUrl(wsId, wsName));
                      }
                    }}
                    onCreateWorkspace={() => setShowCreateWorkspace(true)}
                    onImportFromNotion={() => setShowNotionImport(true)}
                  />
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
                  <UserMenu
                    onProfile={() => {
                      setIsProfilePage(true);
                      setIsSettingsPage(false);
                      setSelectedNodeId(null);
                      window.history.pushState(null, '', '/profile');
                    }}
                  />
                </div>
              </header>

              <Show when={isProfilePage()}>
                <UserProfile
                  onBack={() => window.history.back()}
                  onOrgSettings={(org: OrgMembershipResponse) => {
                    batch(() => {
                      setIsOrgSettingsPage(true);
                      setOrgSettingsId(org.organization_id);
                      setIsProfilePage(false);
                    });
                    window.history.pushState(null, '', orgSettingsUrl(org.organization_id, org.organization_name));
                  }}
                />
              </Show>

              <Show when={isSettingsPage()}>
                <WorkspaceSettings
                  onBack={() => window.history.back()}
                  onOpenOrgSettings={() => {
                    const u = user();
                    const orgId = u?.organization_id;
                    const orgMembership = u?.organizations?.find((m) => m.organization_id === orgId);
                    const orgName = orgMembership?.organization_name;
                    if (orgId) {
                      setIsOrgSettingsPage(true);
                      setOrgSettingsId(orgId);
                      setIsSettingsPage(false);
                      window.history.pushState(null, '', orgSettingsUrl(orgId, orgName));
                    }
                  }}
                />
              </Show>

              <Show when={isOrgSettingsPage() && orgSettingsId()}>
                <OrganizationSettings orgId={orgSettingsId() as string} onBack={() => window.history.back()} />
              </Show>

              <Show when={!isProfilePage() && !isSettingsPage() && !isOrgSettingsPage()}>
                <div class={styles.container}>
                  <Show when={showMobileSidebar()}>
                    <div class={styles.mobileBackdrop} onClick={() => setShowMobileSidebar(false)} />
                  </Show>
                  <Sidebar
                    isOpen={showMobileSidebar()}
                    loading={loading()}
                    nodes={nodes}
                    selectedNodeId={selectedNodeId()}
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
                      if (nodeId !== selectedNodeId()) {
                        loadNode(nodeId).then(() => loadHistory(nodeId));
                      } else {
                        loadHistory(nodeId);
                      }
                    }}
                  />

                  <main class={styles.main}>
                    <Show when={error()} fallback={null}>
                      <div class={styles.error} role="alert" aria-live="polite">
                        {error()}
                      </div>
                    </Show>

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
                                      <span class={styles.historyDate}>
                                        {new Date(commit.timestamp).toLocaleString()}
                                      </span>
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
                            <Editor
                              content={content()}
                              pageId={selectedNodeId() ?? undefined}
                              orgId={user()?.organization_id}
                              onChange={handleContentChange}
                            />
                          </Show>

                          <Show when={selectedNodeData()?.has_table}>
                            <div class={styles.tableView}>
                              <div class={styles.tableHeader}>
                                <h3>{t('table.records')}</h3>
                                <div class={styles.viewToggle}>
                                  <button
                                    classList={{ [`${styles.active}`]: viewMode() === 'table' }}
                                    onClick={() => setViewMode('table')}
                                  >
                                    {t('table.table')}
                                  </button>
                                  <button
                                    classList={{ [`${styles.active}`]: viewMode() === 'grid' }}
                                    onClick={() => setViewMode('grid')}
                                  >
                                    {t('table.grid')}
                                  </button>
                                  <button
                                    classList={{ [`${styles.active}`]: viewMode() === 'gallery' }}
                                    onClick={() => setViewMode('gallery')}
                                  >
                                    {t('table.gallery')}
                                  </button>
                                  <button
                                    classList={{ [`${styles.active}`]: viewMode() === 'board' }}
                                    onClick={() => setViewMode('board')}
                                  >
                                    {t('table.board')}
                                  </button>
                                </div>
                              </div>
                              <Show when={viewMode() === 'table'}>
                                <TableTable
                                  tableId={selectedNodeId() || ''}
                                  columns={selectedNodeData()?.properties || []}
                                  records={records()}
                                  onAddRecord={addRecord}
                                  onUpdateRecord={updateRecord}
                                  onDeleteRecord={deleteRecord}
                                  onLoadMore={loadMoreRecords}
                                  hasMore={hasMore()}
                                />
                              </Show>
                              <Show when={viewMode() === 'grid'}>
                                <TableGrid
                                  records={records()}
                                  columns={selectedNodeData()?.properties || []}
                                  onUpdateRecord={updateRecord}
                                  onDeleteRecord={deleteRecord}
                                />
                              </Show>
                              <Show when={viewMode() === 'gallery'}>
                                <TableGallery
                                  records={records()}
                                  columns={selectedNodeData()?.properties || []}
                                  onUpdateRecord={updateRecord}
                                  onDeleteRecord={deleteRecord}
                                />
                              </Show>
                              <Show when={viewMode() === 'board'}>
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
                  </main>
                </div>
              </Show>
            </div>
          </Show>
        </Show>
      </Show>
      <PWAInstallBanner />
      <Show when={showCreateOrg()}>
        <CreateOrgModal onClose={() => setShowCreateOrg(false)} onCreate={createOrganization} />
      </Show>
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
    </>
  );
}

// Root component that provides contexts
export default function App() {
  return (
    <AppErrorBoundary>
      <AuthProvider>
        <WorkspaceProvider>
          <EditorProvider>
            <RecordsProvider>
              <AppContent />
            </RecordsProvider>
          </EditorProvider>
        </WorkspaceProvider>
      </AuthProvider>
    </AppErrorBoundary>
  );
}
