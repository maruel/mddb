// Main application component managing routing, editor state, and layout.

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
import UserMenu from './components/UserMenu';
import WorkspaceMenu from './components/WorkspaceMenu';
import { AuthProvider, useAuth, WorkspaceProvider, useWorkspace, slugify } from './contexts';
import { debounce } from './utils/debounce';
import { useI18n, type Locale } from './i18n';
import type { NodeResponse, DataRecordResponse, Commit, OrgMembershipResponse, UserResponse } from '@sdk/types.gen';
import styles from './App.module.css';

// Inner app component that uses contexts
function AppContent() {
  const { t, locale, setLocale } = useI18n();
  const { user, token, wsApi, login, logout } = useAuth();
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
    updateNodeTitle,
  } = useWorkspace();

  // Editor state
  const [title, setTitle] = createSignal('');
  const [content, setContent] = createSignal('');
  const [hasUnsavedChanges, setHasUnsavedChanges] = createSignal(false);
  const [autoSaveStatus, setAutoSaveStatus] = createSignal<'idle' | 'saving' | 'saved'>('idle');

  // Table records state
  const [records, setRecords] = createSignal<DataRecordResponse[]>([]);
  const [hasMore, setHasMore] = createSignal(false);
  const PAGE_SIZE = 50;

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
  const [nodeCreationParentId, setNodeCreationParentId] = createSignal<string | null>(null);

  // History state
  const [showHistory, setShowHistory] = createSignal(false);
  const [history, setHistory] = createSignal<Commit[]>([]);

  // Debounced auto-save function
  const debouncedAutoSave = debounce(async () => {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !hasUnsavedChanges() || !ws) return;

    try {
      setAutoSaveStatus('saving');
      await ws.nodes.page.updatePage(nodeId, { title: title(), content: content() });
      setHasUnsavedChanges(false);
      setAutoSaveStatus('saved');

      // Update URL if title changed
      const nodeSlug = slugify(title());
      const wsId = user()?.workspace_id;
      const wsName = user()?.workspace_name;
      if (wsId) {
        const currentPath = window.location.pathname;
        const wsSlug = slugify(wsName || 'workspace');
        const newPath = `/w/${wsId}+${wsSlug}/${nodeId}${nodeSlug ? '+' + nodeSlug : ''}`;
        if (currentPath !== newPath) {
          window.history.replaceState(null, '', newPath);
        }
      }

      setTimeout(() => {
        if (autoSaveStatus() === 'saved') {
          setAutoSaveStatus('idle');
        }
      }, 2000);
    } catch (err) {
      setError(`${t('errors.autoSaveFailed')}: ${err}`);
      setAutoSaveStatus('idle');
    }
  }, 2000);

  // Sync locale with user settings
  createEffect(() => {
    const u = user();
    const userLang = u?.settings?.language as Locale | undefined;
    if (userLang && ['en', 'fr', 'de', 'es'].includes(userLang) && userLang !== locale()) {
      setLocale(userLang);
      localStorage.setItem('mddb_locale', userLang);
    }
  });

  // Sync editor state when selected node changes
  createEffect(() => {
    const node = selectedNodeData();
    if (node) {
      setTitle(node.title);
      setContent(node.content || '');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setShowHistory(false);

      // Load records if it has table content
      if (node.has_table) {
        loadRecords(node.id);
      } else {
        setRecords([]);
        setHasMore(false);
      }
    } else {
      setTitle('');
      setContent('');
      setRecords([]);
    }
  });

  async function loadRecords(nodeId: string) {
    const ws = wsApi();
    if (!ws) return;

    try {
      const recordsData = await ws.nodes.table.records.listRecords(nodeId, { Offset: 0, Limit: PAGE_SIZE });
      const loadedRecords = (recordsData.records || []) as DataRecordResponse[];
      setRecords(loadedRecords);
      setHasMore(loadedRecords.length === PAGE_SIZE);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    }
  }

  // Handle browser back/forward and initial URL
  const handlePopState = async () => {
    const path = window.location.pathname;

    if (path === '/privacy') {
      setIsPrivacyPage(true);
      setIsTermsPage(false);
      setIsProfilePage(false);
      return;
    }
    if (path === '/terms') {
      setIsTermsPage(true);
      setIsPrivacyPage(false);
      setIsProfilePage(false);
      return;
    }
    if (path === '/profile') {
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
    const matchWsSettings = path.match(/^\/w\/([^+/]+)(?:\+[^/]*)?\/settings\/?$/);
    if (matchWsSettings && matchWsSettings[1]) {
      const wsId = matchWsSettings[1];
      if (user() && user()?.workspace_id !== wsId) {
        try {
          const u = user();
          const isMember = u?.workspaces?.some((m) => m.workspace_id === wsId);
          if (!isMember) {
            setError(t('errors.noAccessToWs') || 'You do not have access to this workspace');
            const currentWsId = u?.workspace_id;
            const currentWsName = u?.workspace_name;
            if (currentWsId) {
              const wsSlug = slugify(currentWsName || 'workspace');
              setTimeout(() => window.history.replaceState(null, '', `/w/${currentWsId}+${wsSlug}/`), 2000);
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
    const matchOrgSettings = path.match(/^\/o\/([^+/]+)(?:\+[^/]*)?\/settings\/?$/);
    if (matchOrgSettings && matchOrgSettings[1]) {
      const orgId = matchOrgSettings[1];
      const u = user();
      const isMember = u?.organizations?.some((m) => m.organization_id === orgId);
      if (!isMember) {
        setError(t('errors.noAccessToOrg') || 'You do not have access to this organization');
        const currentWsId = u?.workspace_id;
        const currentWsName = u?.workspace_name;
        if (currentWsId) {
          const wsSlug = slugify(currentWsName || 'workspace');
          setTimeout(() => window.history.replaceState(null, '', `/w/${currentWsId}+${wsSlug}/`), 2000);
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
    const matchWithWs = path.match(/^\/w\/([^+/]+)(?:\+[^/]*)?\/([a-zA-Z0-9_-]+)(?:\+.*)?$/);
    if (matchWithWs && matchWithWs[1] && matchWithWs[2]) {
      const nodeId = matchWithWs[2];
      if (nodeId !== selectedNodeId()) {
        // Flush auto-save before switching nodes
        debouncedAutoSave.flush();
        loadNode(nodeId, false);
      }
      return;
    }

    // Check for workspace root
    const matchWsRoot = path.match(/^\/w\/([^+/]+)(?:\+[^/]*)?\/?$/);
    if (matchWsRoot && matchWsRoot[1]) {
      if (nodes.length > 0 && nodes[0]) {
        loadNode(nodes[0].id, false);
      } else {
        setSelectedNodeId(null);
        setSelectedNodeData(null);
        setTitle('');
        setContent('');
      }
      return;
    }

    setSelectedNodeId(null);
    setSelectedNodeData(null);
    setTitle('');
    setContent('');
  };

  onMount(() => {
    window.addEventListener('popstate', handlePopState);
    onCleanup(() => window.removeEventListener('popstate', handlePopState));
  });

  // Handle initial route when user loads
  createEffect(() => {
    if (user()) {
      handlePopState();
    } else {
      handlePopState();
    }
  });

  async function loadHistory(nodeId: string) {
    if (showHistory()) {
      setShowHistory(false);
      return;
    }
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      const data = await ws.nodes.history.listNodeVersions(nodeId, { Limit: 100 });
      setHistory((data.history?.filter(Boolean) as Commit[]) || []);
      setShowHistory(true);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function loadVersion(nodeId: string, hash: string) {
    if (!confirm(t('editor.restoreConfirm') || 'This will replace current editor content. Continue?')) return;
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      const data = await ws.nodes.history.getNodeVersion(nodeId, hash);
      setContent(data.content || '');
      setHasUnsavedChanges(true);
      setShowHistory(false);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

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
      setTitle('');
      setContent('');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);
      setNodeCreationParentId(null);
    } catch (err) {
      setError(`${t('errors.failedToCreate')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  const handleNodeClick = (node: NodeResponse) => {
    debouncedAutoSave.flush();
    loadNode(node.id);
    setShowMobileSidebar(false);
  };

  async function handleAddRecord(data: Record<string, unknown>) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    try {
      setLoading(true);
      await ws.nodes.table.records.createRecord(nodeId, { data });
      await loadRecords(nodeId);
      setError(null);
    } catch (err) {
      setError(`${t('errors.failedToCreate')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function handleDeleteRecord(recordId: string) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;
    if (!confirm(t('table.confirmDeleteRecord') || 'Delete this record?')) return;

    try {
      setLoading(true);
      await ws.nodes.table.records.deleteRecord(nodeId, recordId);
      await loadRecords(nodeId);
      setError(null);
    } catch (err) {
      setError(`${t('errors.failedToDelete')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function handleUpdateRecord(recordId: string, data: Record<string, unknown>) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    try {
      setLoading(true);
      await ws.nodes.table.records.updateRecord(nodeId, recordId, { data });
      await loadRecords(nodeId);
      setError(null);
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function loadMoreRecords() {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || loading() || !ws) return;

    try {
      setLoading(true);
      const offset = records().length;
      const data = await ws.nodes.table.records.listRecords(nodeId, { Offset: offset, Limit: PAGE_SIZE });
      const newRecords = (data.records || []) as DataRecordResponse[];
      setRecords([...records(), ...newRecords]);
      setHasMore(newRecords.length === PAGE_SIZE);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function handleDeleteNode(nodeId: string) {
    if (!confirm(t('table.confirmDeleteRecord') || 'Delete this item?')) return;
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      await ws.nodes.deleteNode(nodeId);
      if (nodeId === selectedNodeId()) {
        setSelectedNodeId(null);
        setSelectedNodeData(null);
        setTitle('');
        setContent('');
        setRecords([]);
        const wsId = user()?.workspace_id;
        const wsName = user()?.workspace_name;
        if (wsId) {
          const wsSlug = slugify(wsName || 'workspace');
          window.history.pushState(null, '', `/w/${wsId}+${wsSlug}/`);
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
              <header class={styles.header}>
                <div class={styles.headerLeft}>
                  <button
                    class={styles.hamburger}
                    onClick={() => setShowMobileSidebar(!showMobileSidebar())}
                    aria-label="Toggle menu"
                  >
                    ☰
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
                  <WorkspaceMenu
                    workspaces={user()?.workspaces || []}
                    organizations={user()?.organizations || []}
                    currentWsId={user()?.workspace_id || ''}
                    onSwitchWorkspace={(wsId) => switchWorkspace(wsId)}
                    onOpenSettings={() => {
                      setIsSettingsPage(true);
                      setSelectedNodeId(null);
                      setShowMobileSidebar(false);
                      const wsId = user()?.workspace_id;
                      const wsName = user()?.workspace_name;
                      if (wsId) {
                        const wsSlug = slugify(wsName || 'workspace');
                        window.history.pushState(null, '', `/w/${wsId}+${wsSlug}/settings`);
                      }
                    }}
                    onCreateWorkspace={() => setShowCreateWorkspace(true)}
                  />
                  <UserMenu
                    user={user() as UserResponse}
                    onLogout={logout}
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
                  user={user() as UserResponse}
                  token={token() as string}
                  onBack={() => window.history.back()}
                  onOrgSettings={(org: OrgMembershipResponse) => {
                    batch(() => {
                      setIsOrgSettingsPage(true);
                      setOrgSettingsId(org.organization_id);
                      setIsProfilePage(false);
                    });
                    const orgSlug = slugify(org.organization_name || 'organization');
                    window.history.pushState(null, '', `/o/${org.organization_id}+${orgSlug}/settings`);
                  }}
                />
              </Show>

              <Show when={isSettingsPage()}>
                <WorkspaceSettings
                  user={user() as UserResponse}
                  token={token() as string}
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
                      const orgSlug = slugify(orgName || 'organization');
                      window.history.pushState(null, '', `/o/${orgId}+${orgSlug}/settings`);
                    }
                  }}
                />
              </Show>

              <Show when={isOrgSettingsPage() && orgSettingsId()}>
                <OrganizationSettings
                  user={user() as UserResponse}
                  token={token() as string}
                  orgId={orgSettingsId() as string}
                  onBack={() => window.history.back()}
                />
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
                      <div class={styles.error}>{error()}</div>
                    </Show>

                    <Show when={selectedNodeId()}>
                      <div class={styles.editor}>
                        <div class={styles.editorHeader}>
                          <input
                            type="text"
                            placeholder={t('editor.titlePlaceholder') || 'Title'}
                            value={title()}
                            onInput={(e) => {
                              setTitle(e.target.value);
                              const id = selectedNodeId();
                              if (id) {
                                updateNodeTitle(id, e.target.value);
                              }
                              setHasUnsavedChanges(true);
                              debouncedAutoSave();
                            }}
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
                              onChange={(md) => {
                                setContent(md);
                                setHasUnsavedChanges(true);
                                debouncedAutoSave();
                              }}
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
                                  onAddRecord={handleAddRecord}
                                  onUpdateRecord={handleUpdateRecord}
                                  onDeleteRecord={handleDeleteRecord}
                                  onLoadMore={loadMoreRecords}
                                  hasMore={hasMore()}
                                />
                              </Show>
                              <Show when={viewMode() === 'grid'}>
                                <TableGrid
                                  records={records()}
                                  columns={selectedNodeData()?.properties || []}
                                  onUpdateRecord={handleUpdateRecord}
                                  onDeleteRecord={handleDeleteRecord}
                                />
                              </Show>
                              <Show when={viewMode() === 'gallery'}>
                                <TableGallery
                                  records={records()}
                                  columns={selectedNodeData()?.properties || []}
                                  onUpdateRecord={handleUpdateRecord}
                                  onDeleteRecord={handleDeleteRecord}
                                />
                              </Show>
                              <Show when={viewMode() === 'board'}>
                                <TableBoard
                                  records={records()}
                                  columns={selectedNodeData()?.properties || []}
                                  onUpdateRecord={handleUpdateRecord}
                                  onDeleteRecord={handleDeleteRecord}
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
    </>
  );
}

// Root component that provides contexts
export default function App() {
  return (
    <AuthProvider>
      <WorkspaceProvider>
        <AppContent />
      </WorkspaceProvider>
    </AuthProvider>
  );
}
