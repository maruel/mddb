import { createSignal, createEffect, createMemo, For, Show, onMount, onCleanup } from 'solid-js';
import { createStore, produce, reconcile } from 'solid-js/store';
import Sidebar from './components/Sidebar';
import MarkdownPreview from './components/MarkdownPreview';
import TableTable from './components/TableTable';
import TableGrid from './components/TableGrid';
import TableGallery from './components/TableGallery';
import TableBoard from './components/TableBoard';
import WorkspaceSettings from './components/WorkspaceSettings';
import Onboarding from './components/Onboarding';
import Auth from './components/Auth';
import Privacy from './components/Privacy';
import Terms from './components/Terms';
import PWAInstallBanner from './components/PWAInstallBanner';
import CreateOrgModal from './components/CreateOrgModal';
import CreateWorkspaceModal from './components/CreateWorkspaceModal';
import UserMenu from './components/UserMenu';
import OrgMenu from './components/OrgMenu';
import { debounce } from './utils/debounce';
import { useI18n, type Locale } from './i18n';
import { createApi, APIError } from './useApi';
import {
  OrgRoleAdmin,
  OrgRoleOwner,
  WSRoleAdmin,
  WSRoleEditor,
  type NodeResponse,
  type DataRecordResponse,
  type Commit,
  type UserResponse,
} from './types.gen';
import styles from './App.module.css';

const slugify = (text: string) => {
  return text
    .toString()
    .toLowerCase()
    .trim()
    .replace(/\s+/g, '-')
    .replace(/[^\w-]+/g, '')
    .replace(/--+/g, '-');
};

export default function App() {
  const { t, locale, setLocale } = useI18n();
  const [user, setUser] = createSignal<UserResponse | null>(null);
  const [token, setToken] = createSignal<string | null>(localStorage.getItem('mddb_token'));
  const [nodes, setNodes] = createStore<NodeResponse[]>([]);
  const [records, setRecords] = createSignal<DataRecordResponse[]>([]);
  const [selectedNodeId, setSelectedNodeId] = createSignal<string | null>(null);
  const [showSettings, setShowSettings] = createSignal(false);
  const [isPrivacyPage, setIsPrivacyPage] = createSignal(false);
  const [isTermsPage, setIsTermsPage] = createSignal(false);
  const [viewMode, setViewMode] = createSignal<'table' | 'grid' | 'gallery' | 'board'>('table');
  const [title, setTitle] = createSignal('');
  const [content, setContent] = createSignal('');
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [hasUnsavedChanges, setHasUnsavedChanges] = createSignal(false);
  const [autoSaveStatus, setAutoSaveStatus] = createSignal<'idle' | 'saving' | 'saved'>('idle');
  const [nodeCreationParentId, setNodeCreationParentId] = createSignal<string | null>(null);
  const [showCreateOrg, setShowCreateOrg] = createSignal(false);
  const [showCreateWorkspace, setShowCreateWorkspace] = createSignal(false);
  const [showGitSetup, setShowGitSetup] = createSignal(false);
  const [showMobileSidebar, setShowMobileSidebar] = createSignal(false);
  const [firstLoginCheckDone, setFirstLoginCheckDone] = createSignal(false);

  // History state
  const [showHistory, setShowHistory] = createSignal(false);
  const [history, setHistory] = createSignal<Commit[]>([]);

  // Pagination
  const [hasMore, setHasMore] = createSignal(false);
  const PAGE_SIZE = 50;

  const logout = async () => {
    // Call logout API to revoke the session server-side
    const currentToken = token();
    if (currentToken) {
      try {
        const logoutApi = createApi(
          () => currentToken,
          () => {}
        );
        await logoutApi.auth.logout();
      } catch {
        // Ignore errors - proceed with local logout even if server call fails
      }
    }
    localStorage.removeItem('mddb_token');
    setToken(null);
    setUser(null);
    window.history.pushState(null, '', '/');
  };

  // Create API client with auth
  const api = createMemo(() => createApi(() => token(), logout));

  // Get workspace-scoped API client
  const wsApi = createMemo(() => {
    const wsID = user()?.workspace_id;
    return wsID ? api().ws(wsID) : null;
  });

  const handleLogin = (newToken: string, userData: UserResponse) => {
    localStorage.setItem('mddb_token', newToken);
    setToken(newToken);
    setUser(userData);
  };

  async function switchOrg(orgId: string, redirect = true) {
    try {
      setLoading(true);
      const data = await api().auth.switchOrg({ org_id: orgId });
      if (!data.user) {
        throw new Error('No user data returned');
      }
      handleLogin(data.token, data.user);
      setSelectedNodeId(null);
      if (redirect) {
        window.history.pushState(null, '', '/');
      }
      await loadNodes();
    } catch (err) {
      if (err instanceof APIError) {
        setError(`${t('errors.failedToSwitch')}: ${err.message}`);
      } else {
        setError(`${t('errors.failedToSwitch')}: ${err}`);
      }
      throw err;
    } finally {
      setLoading(false);
    }
  }

  async function switchWorkspace(wsId: string, redirect = true) {
    try {
      setLoading(true);
      const data = await api().auth.switchWorkspace({ ws_id: wsId });
      if (!data.user) {
        throw new Error('No user data returned');
      }
      handleLogin(data.token, data.user);
      setSelectedNodeId(null);
      if (redirect) {
        window.history.pushState(null, '', '/');
      }
      await loadNodes();
    } catch (err) {
      if (err instanceof APIError) {
        setError(`${t('errors.failedToSwitch')}: ${err.message}`);
      } else {
        setError(`${t('errors.failedToSwitch')}: ${err}`);
      }
      throw err;
    } finally {
      setLoading(false);
    }
  }

  async function createOrganization(data: { name: string }) {
    const org = await api().organizations.create({
      name: data.name,
    });
    // Refresh user data and switch to the new org
    // This will trigger first-login check again, showing workspace creation modal
    await switchOrg(org.id);
  }

  async function createWorkspace(data: { name: string }) {
    const u = user();
    if (!u || !u.organization_id) {
      throw new Error('No organization selected');
    }

    // Create workspace via org API
    const ws = await api().org(u.organization_id).workspaces.create({
      name: data.name,
    });

    // Refresh user data to include the new workspace
    const updatedUser = await api().auth.me.get();
    setUser(updatedUser);

    // Switch to the new workspace
    await switchWorkspace(ws.id);
    setShowGitSetup(true);
  }

  // Helper to update node title in local state
  const updateNodeTitle = (nodeId: string, newTitle: string) => {
    setNodes(
      produce((list) => {
        const update = (nodes: NodeResponse[]): boolean => {
          for (const node of nodes) {
            if (node.id === nodeId) {
              node.title = newTitle;
              return true;
            }
            if (node.children) {
              if (update(node.children)) return true;
            }
          }
          return false;
        };
        update(list);
      })
    );
  };

  // Debounced auto-save function
  const debouncedAutoSave = debounce(async () => {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !hasUnsavedChanges() || !ws) return;

    try {
      setAutoSaveStatus('saving');
      await ws.pages.update(nodeId, { title: title(), content: content() });
      setHasUnsavedChanges(false);
      setAutoSaveStatus('saved');

      // Update URL if title changed
      const nodeSlug = slugify(title());
      const wsId = user()?.workspace_id;
      const wsName = user()?.workspace_name;
      if (wsId) {
        const currentPath = window.location.pathname;
        const wsSlug = slugify(wsName || 'workspace');
        const newPath = `/${wsId}+${wsSlug}/${nodeId}${nodeSlug ? '+' + nodeSlug : ''}`;
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

  // First-time login check: ensure user has org and workspace
  createEffect(() => {
    const u = user();
    if (u && !firstLoginCheckDone()) {
      setFirstLoginCheckDone(true);

      // Check if user has any organizations
      const orgs = u.organizations || [];
      if (orgs.length === 0) {
        // User has no organizations, show create org modal
        setShowCreateOrg(true);
        return;
      }

      // User has organizations, check if first org has any workspaces
      const firstOrg = orgs[0];
      if (firstOrg) {
        const orgWorkspaces = u.workspaces?.filter((ws) => ws.organization_id === firstOrg.organization_id) || [];
        if (orgWorkspaces.length === 0) {
          // Only show create workspace modal if user is admin/owner of the org
          if (firstOrg.role === OrgRoleAdmin || firstOrg.role === OrgRoleOwner) {
            setShowCreateWorkspace(true);
            return;
          }
        }
      }

      // User has org and workspace, proceed normally
    }
  });

  // Handle OAuth token from URL on mount (runs once)
  onMount(() => {
    const urlParams = new URLSearchParams(window.location.search);
    const urlToken = urlParams.get('token');
    if (urlToken) {
      localStorage.setItem('mddb_token', urlToken);
      setToken(urlToken);
      // Clean up URL query params but keep pathname
      window.history.replaceState({}, document.title, window.location.pathname);
    }
  });

  // Fetch user when we have a token but no user data
  let fetchingUser = false;
  createEffect(() => {
    const tok = token();
    const u = user();
    if (tok && !u && !fetchingUser) {
      fetchingUser = true;
      (async () => {
        try {
          const data = await api().auth.me.get();
          setUser(data);
        } catch (err) {
          console.error('Failed to load user', err);
          // Clear invalid token on auth failure
          if (err instanceof APIError && err.status === 401) {
            localStorage.removeItem('mddb_token');
            setToken(null);
          }
        } finally {
          fetchingUser = false;
        }
      })();
    }
  });

  // Handle browser back/forward and initial URL
  const handlePopState = async () => {
    const path = window.location.pathname;

    if (path === '/privacy') {
      setIsPrivacyPage(true);
      setIsTermsPage(false);
      return;
    }
    if (path === '/terms') {
      setIsTermsPage(true);
      setIsPrivacyPage(false);
      return;
    }
    setIsPrivacyPage(false);
    setIsTermsPage(false);

    // Check for /wsID+wsSlug/nodeID+nodeSlug format
    const matchWithWs = path.match(/^\/([^+/]+)(?:\+[^/]*)?\/([a-zA-Z0-9_-]+)(?:\+.*)?$/);
    if (matchWithWs && matchWithWs[1] && matchWithWs[2]) {
      const wsId = matchWithWs[1];
      const nodeId = matchWithWs[2];

      // If we are logged in but in wrong workspace, switch
      if (user() && user()?.workspace_id !== wsId) {
        try {
          // Check if we are member of this workspace
          const isMember = user()?.workspaces?.some((m) => m.workspace_id === wsId);
          if (isMember) {
            await switchWorkspace(wsId, false); // Don't redirect to /
          } else {
            setError(t('errors.noAccessToWs') || 'You do not have access to this workspace');
            return;
          }
        } catch {
          return;
        }
      }

      if (nodeId !== selectedNodeId()) {
        loadNode(nodeId, false);
      }
      return;
    }

    setSelectedNodeId(null);
    setTitle('');
    setContent('');
  };

  onMount(() => {
    window.addEventListener('popstate', handlePopState);
    onCleanup(() => window.removeEventListener('popstate', handlePopState));
  });

  // Load nodes and initial route when user is available
  createEffect(() => {
    if (user()) {
      loadNodes();
      // Check URL
      handlePopState();
    } else {
      // If not logged in, we still need to check for /privacy
      handlePopState();
    }
  });

  async function loadNodes() {
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      const data = await ws.nodes.list();
      const loadedNodes = (data.nodes?.filter(Boolean) as NodeResponse[]) || [];

      // If workspace is empty and user has permission, create welcome page
      if (
        loadedNodes.length === 0 &&
        (user()?.workspace_role === WSRoleAdmin || user()?.workspace_role === WSRoleEditor)
      ) {
        const newNode = await ws.pages.create({
          title: t('welcome.welcomePageTitle'),
          content: t('welcome.welcomePageContent'),
        });
        // Reload nodes after creation
        const refreshedData = await ws.nodes.list();
        const refreshedNodes = (refreshedData.nodes?.filter(Boolean) as NodeResponse[]) || [];
        setNodes(reconcile(refreshedNodes));

        // Select the new node
        if (newNode && newNode.id) {
          loadNode(newNode.id);
        }
      } else {
        setNodes(reconcile(loadedNodes));
      }
      setError(null);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function loadNode(id: string, pushState = true) {
    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      setShowHistory(false);
      const nodeData = await ws.nodes.get(id);

      setSelectedNodeId(nodeData.id);
      setTitle(nodeData.title);
      setContent(nodeData.content || '');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);

      // Update URL to include wsID+wsSlug
      const nodeSlug = slugify(nodeData.title);
      const wsId = user()?.workspace_id;
      const wsName = user()?.workspace_name;
      if (wsId) {
        const wsSlug = slugify(wsName || 'workspace');
        const url = `/${wsId}+${wsSlug}/${nodeData.id}${nodeSlug ? '+' + nodeSlug : ''}`;
        if (pushState) {
          if (window.location.pathname !== url) {
            window.history.pushState(null, '', url);
          }
        } else {
          // Canonicalize URL
          if (window.location.pathname !== url) {
            window.history.replaceState(null, '', url);
          }
        }
      }

      // If it's a table or hybrid, load records
      if (nodeData.type === 'table' || nodeData.type === 'hybrid') {
        const recordsData = await ws.tables.records.list(id, { Offset: 0, Limit: PAGE_SIZE });
        const loadedRecords = (recordsData.records || []) as DataRecordResponse[];
        setRecords(loadedRecords);
        setHasMore(loadedRecords.length === PAGE_SIZE);
      } else {
        setRecords([]);
        setHasMore(false);
      }
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function loadHistory(nodeId: string) {
    if (showHistory()) {
      setShowHistory(false);
      return;
    }

    const ws = wsApi();
    if (!ws) return;

    try {
      setLoading(true);
      const data = await ws.pages.history.list(nodeId, { Limit: 100 });
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
      const data = await ws.pages.history.get(nodeId, hash);
      setContent(data.content || '');
      setHasUnsavedChanges(true); // Mark as modified
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
      const newNode = await ws.nodes.create({
        title: title(),
        type,
        parent_id: parentId || nodeCreationParentId() || undefined,
      });
      await loadNodes();
      loadNode(newNode.id);
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

  async function saveNode() {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    try {
      setLoading(true);
      await ws.pages.update(nodeId, { title: title(), content: content() });
      await loadNodes();
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);

      // Update URL if title changed
      const slug = slugify(title());
      const currentPath = window.location.pathname;
      const orgId = user()?.organization_id;
      if (orgId) {
        const newPath = `/${orgId}/${nodeId}${slug ? '+' + slug : ''}`;
        if (currentPath !== newPath) {
          window.history.replaceState(null, '', newPath);
        }
      }
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function deleteCurrentNode() {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    if (!confirm(t('table.confirmDeleteRecord') || 'Delete this record?')) return;

    try {
      setLoading(true);
      await ws.pages.delete(nodeId);
      await loadNodes();
      setSelectedNodeId(null);
      setTitle('');
      setContent('');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);
      window.history.pushState(null, '', '/');
    } catch (err) {
      setError(`${t('errors.failedToDelete')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  const handleNodeClick = (node: NodeResponse) => {
    loadNode(node.id);
    setShowMobileSidebar(false);
  };

  // Recursively find a node by ID in the tree
  const findNodeById = (nodeId: string | null): NodeResponse | undefined => {
    if (!nodeId) return undefined;

    const search = (nodeList: NodeResponse[]): NodeResponse | undefined => {
      for (const node of nodeList) {
        if (node.id === nodeId) return node;
        if (node.children) {
          const found = search(node.children.filter((c): c is NodeResponse => !!c));
          if (found) return found;
        }
      }
      return undefined;
    };

    return search(nodes);
  };

  const getBreadcrumbs = (nodeId: string | null): NodeResponse[] => {
    if (!nodeId) return [];
    const path: NodeResponse[] = [];

    const findPath = (currentNodes: NodeResponse[], targetId: string): boolean => {
      for (const node of currentNodes) {
        if (node.id === targetId) {
          path.push(node);
          return true;
        }
        if (
          node.children &&
          findPath(
            node.children.filter((c): c is NodeResponse => !!c),
            targetId
          )
        ) {
          path.unshift(node);
          return true;
        }
      }
      return false;
    };

    findPath(nodes, nodeId);
    return path;
  };

  async function handleAddRecord(data: Record<string, unknown>) {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    try {
      setLoading(true);
      await ws.tables.records.create(nodeId, { data });
      // Reload records
      loadNode(nodeId);
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
      await ws.tables.records.delete(nodeId, recordId);
      loadNode(nodeId);
      setError(null);
    } catch (err) {
      setError(`${t('errors.failedToDelete')}: ${err}`);
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
      const data = await ws.tables.records.list(nodeId, { Offset: offset, Limit: PAGE_SIZE });
      const newRecords = (data.records || []) as DataRecordResponse[];
      setRecords([...records(), ...newRecords]);
      setHasMore(newRecords.length === PAGE_SIZE);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <Show when={!isPrivacyPage()} fallback={<Privacy />}>
        <Show when={!isTermsPage()} fallback={<Terms />}>
          <Show when={user()} fallback={<Auth onLogin={handleLogin} />}>
            <>
              {/* First org creation for new users with no memberships */}
              <Show when={(user()?.organizations?.length ?? 0) === 0}>
                <CreateOrgModal isFirstOrg={true} onClose={() => {}} onCreate={createOrganization} />
              </Show>
              <Show when={showGitSetup()}>
                <Onboarding
                  user={user() as UserResponse}
                  token={token() as string}
                  onComplete={() => setShowGitSetup(false)}
                />
              </Show>
              <div class={styles.app}>
                <header class={styles.header}>
                  <div class={styles.headerTitle}>
                    <button
                      class={styles.hamburger}
                      onClick={() => setShowMobileSidebar(!showMobileSidebar())}
                      aria-label="Toggle menu"
                    >
                      ☰
                    </button>
                    <h1>{t('app.title')}</h1>
                  </div>
                  <div class={styles.userInfo}>
                    <Show when={(user()?.organizations?.length ?? 0) > 0}>
                      <OrgMenu
                        memberships={user()?.organizations || []}
                        currentOrgId={user()?.organization_id || ''}
                        onSwitchOrg={(orgId) => switchOrg(orgId)}
                        onCreateOrg={() => setShowCreateOrg(true)}
                      />
                    </Show>
                    <UserMenu user={user() as UserResponse} onLogout={logout} />
                  </div>
                </header>

                <div class={styles.container}>
                  <Show when={showMobileSidebar()}>
                    <div class={styles.mobileBackdrop} onClick={() => setShowMobileSidebar(false)} />
                  </Show>
                  <Sidebar
                    isOpen={showMobileSidebar()}
                    loading={loading()}
                    nodes={nodes}
                    selectedNodeId={selectedNodeId()}
                    onToggleSettings={() => {
                      setShowSettings(true);
                      setSelectedNodeId(null);
                      setShowMobileSidebar(false);
                    }}
                    onCreatePage={() => {
                      setShowSettings(false);
                      setNodeCreationParentId(null);
                      createNode('document');
                      setShowMobileSidebar(false);
                    }}
                    onCreateTable={() => {
                      setShowSettings(false);
                      setNodeCreationParentId(null);
                      createNode('table');
                      setShowMobileSidebar(false);
                    }}
                    onCreateChildPage={(parentId: string) => {
                      setShowSettings(false);
                      setNodeCreationParentId(parentId);
                      createNode('document', parentId);
                      setShowMobileSidebar(false);
                    }}
                    onCreateChildTable={(parentId: string) => {
                      setShowSettings(false);
                      setNodeCreationParentId(parentId);
                      createNode('table', parentId);
                      setShowMobileSidebar(false);
                    }}
                    onSelectNode={handleNodeClick}
                    onCloseMobileSidebar={() => setShowMobileSidebar(false)}
                  />

                  <main class={styles.main}>
                    <Show when={showSettings() && user() && token()}>
                      <WorkspaceSettings
                        user={user() as UserResponse}
                        token={token() as string}
                        onClose={() => setShowSettings(false)}
                      />
                    </Show>

                    <Show when={!showSettings()}>
                      <Show when={selectedNodeId()}>
                        <nav class={styles.breadcrumbs}>
                          <For each={getBreadcrumbs(selectedNodeId())}>
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

                      <Show when={error()} fallback={null}>
                        <div class={styles.error}>{error()}</div>
                      </Show>

                      <Show
                        when={selectedNodeId()}
                        fallback={
                          <div class={styles.welcome}>
                            <h2>{t('welcome.title')}</h2>
                            <p>{t('welcome.subtitle')}</p>
                            <div class={styles.createForm}>
                              <input
                                type="text"
                                placeholder={t('editor.titlePlaceholder') || 'Title'}
                                value={title()}
                                onInput={(e) => setTitle(e.target.value)}
                                class={styles.titleInput}
                              />
                              <div class={styles.welcomeActions}>
                                <button onClick={() => createNode('document')} class={styles.createButton}>
                                  {t('welcome.createPage')}
                                </button>
                                <button onClick={() => createNode('table')} class={styles.createButton}>
                                  {t('welcome.createTable')}
                                </button>
                              </div>
                            </div>
                          </div>
                        }
                      >
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
                              <Show when={hasUnsavedChanges()}>
                                <span class={styles.unsavedIndicator}>● {t('editor.unsaved')}</span>
                              </Show>
                              <Show when={autoSaveStatus() === 'saving'}>
                                <span class={styles.savingIndicator}>⟳ {t('common.saving')}</span>
                              </Show>
                              <Show when={autoSaveStatus() === 'saved'}>
                                <span class={styles.savedIndicator}>✓ {t('common.saved')}</span>
                              </Show>
                            </div>
                            <div class={styles.editorActions}>
                              <button
                                onClick={() => {
                                  const id = selectedNodeId();
                                  if (id) loadHistory(id);
                                }}
                                disabled={loading()}
                              >
                                {showHistory() ? t('editor.hideHistory') : t('editor.history')}
                              </button>
                              <button onClick={saveNode} disabled={loading()}>
                                {loading() ? t('common.saving') : t('common.save')}
                              </button>
                              <button onClick={deleteCurrentNode} disabled={loading()}>
                                {t('common.delete')}
                              </button>
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
                            {/* Always show markdown content if it exists or if node is document/hybrid */}
                            <Show when={findNodeById(selectedNodeId())?.type !== 'table'}>
                              <div class={styles.editorContent}>
                                <textarea
                                  value={content()}
                                  onInput={(e) => {
                                    setContent(e.target.value);
                                    setHasUnsavedChanges(true);
                                    debouncedAutoSave();
                                  }}
                                  placeholder={t('editor.contentPlaceholder') || 'Write your content in markdown...'}
                                  class={styles.contentInput}
                                />
                                <MarkdownPreview content={content()} orgId={user()?.organization_id} />
                              </div>
                            </Show>

                            {/* Show table if node is table or hybrid */}
                            <Show when={findNodeById(selectedNodeId())?.type !== 'document'}>
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
                                    columns={findNodeById(selectedNodeId())?.properties || []}
                                    records={records()}
                                    onAddRecord={handleAddRecord}
                                    onDeleteRecord={handleDeleteRecord}
                                    onLoadMore={loadMoreRecords}
                                    hasMore={hasMore()}
                                  />
                                </Show>
                                <Show when={viewMode() === 'grid'}>
                                  <TableGrid
                                    records={records()}
                                    columns={findNodeById(selectedNodeId())?.properties || []}
                                    onDeleteRecord={handleDeleteRecord}
                                  />
                                </Show>
                                <Show when={viewMode() === 'gallery'}>
                                  <TableGallery
                                    records={records()}
                                    columns={findNodeById(selectedNodeId())?.properties || []}
                                    onDeleteRecord={handleDeleteRecord}
                                  />
                                </Show>
                                <Show when={viewMode() === 'board'}>
                                  <TableBoard
                                    records={records()}
                                    columns={findNodeById(selectedNodeId())?.properties || []}
                                    onDeleteRecord={handleDeleteRecord}
                                  />
                                </Show>
                              </div>
                            </Show>
                          </div>
                        </div>
                      </Show>
                    </Show>
                  </main>
                </div>
              </div>
            </>
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
