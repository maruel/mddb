// Main application component managing global state, routing, and layout.

import { createSignal, createEffect, createMemo, For, Show, onMount, onCleanup } from 'solid-js';
import { createStore, produce, reconcile } from 'solid-js/store';
import Sidebar from './components/Sidebar';
import MarkdownPreview from './components/MarkdownPreview';
import TableTable from './components/TableTable';
import TableGrid from './components/TableGrid';
import TableGallery from './components/TableGallery';
import TableBoard from './components/TableBoard';
import WorkspaceSettings from './components/WorkspaceSettings';
import UserProfile from './components/UserProfile';
import Auth from './components/Auth';
import Privacy from './components/Privacy';
import Terms from './components/Terms';
import PWAInstallBanner from './components/PWAInstallBanner';
import CreateOrgModal from './components/CreateOrgModal';
import CreateWorkspaceModal from './components/CreateWorkspaceModal';
import UserMenu from './components/UserMenu';
import OrgMenu from './components/OrgMenu';
import WorkspaceMenu from './components/WorkspaceMenu';
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
  const [selectedNodeData, setSelectedNodeData] = createSignal<NodeResponse | null>(null);
  const [isSettingsPage, setIsSettingsPage] = createSignal(false);
  const [isProfilePage, setIsProfilePage] = createSignal(false);
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
  const [showMobileSidebar, setShowMobileSidebar] = createSignal(false);
  const [firstLoginCheckDone, setFirstLoginCheckDone] = createSignal(false);
  const [firstLoginInProgress, setFirstLoginInProgress] = createSignal(false);

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
      setSelectedNodeData(null);
      loadedNodeId = null;
      loadedForWorkspace = null; // Force reload for new org
      if (redirect) {
        const wsId = data.user.workspace_id;
        const wsName = data.user.workspace_name;
        if (wsId) {
          const wsSlug = slugify(wsName || 'workspace');
          window.history.pushState(null, '', `/w/${wsId}+${wsSlug}/`);
        } else {
          window.history.pushState(null, '', '/');
        }
      }
      await loadNodes();
    } catch (err) {
      if (err instanceof APIError) {
        setError(`${t('errors.failedToSwitch')}: ${err.message}`);
      } else {
        setError(`${t('errors.failedToSwitch')}: ${err}`);
      }
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
      setSelectedNodeData(null);
      loadedNodeId = null;
      loadedForWorkspace = null; // Force reload for new workspace
      if (redirect) {
        const newWsId = data.user.workspace_id;
        const wsName = data.user.workspace_name;
        if (newWsId) {
          const wsSlug = slugify(wsName || 'workspace');
          window.history.pushState(null, '', `/w/${newWsId}+${wsSlug}/`);
        } else {
          window.history.pushState(null, '', '/');
        }
      }
      await loadNodes();
    } catch (err) {
      if (err instanceof APIError) {
        setError(`${t('errors.failedToSwitch')}: ${err.message}`);
      } else {
        setError(`${t('errors.failedToSwitch')}: ${err}`);
      }
    } finally {
      setLoading(false);
    }
  }

  async function createOrganization(data: { name: string }) {
    const org = await api().organizations.createOrganization({
      name: data.name,
    });
    // Refresh user data and switch to the new org
    // This will trigger first-login check again, which will auto-create workspace
    await switchOrg(org.id);
  }

  async function createWorkspace(data: { name: string }) {
    const u = user();
    if (!u || !u.organization_id) {
      throw new Error('No organization selected');
    }

    // Create workspace via org API
    const ws = await api().org(u.organization_id).workspaces.createWorkspace({
      name: data.name,
    });

    // Refresh user data to include the new workspace
    const updatedUser = await api().auth.getMe();
    setUser(updatedUser);

    // Switch to the new workspace
    await switchWorkspace(ws.id);
  }

  // Get user's first name for default naming
  function getUserFirstName(): string {
    const u = user();
    if (!u?.name) return '';
    const firstName = u.name.split(' ')[0];
    return firstName || u.name;
  }

  // Auto-create organization for first-time users
  async function autoCreateOrganization() {
    try {
      setLoading(true);
      const firstName = getUserFirstName();
      const orgName = firstName
        ? t('onboarding.defaultOrgName', { name: firstName })
        : t('onboarding.defaultOrgNameFallback');
      await createOrganization({ name: orgName || 'My Organization' });
    } catch (err) {
      setError(`${t('errors.failedToCreate')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  // Auto-create workspace for users with org but no workspace
  async function autoCreateWorkspace() {
    try {
      setLoading(true);
      const firstName = getUserFirstName();
      const wsName = firstName
        ? t('onboarding.defaultWorkspaceName', { name: firstName })
        : t('onboarding.defaultWorkspaceNameFallback');
      await createWorkspace({ name: wsName || 'Main' });
    } catch (err) {
      setError(`${t('errors.failedToCreate')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  // Auto-create welcome page if no root page exists
  // Returns the new page ID if created, null otherwise
  async function createWelcomePageIfNeeded(): Promise<string | null> {
    const ws = wsApi();
    const u = user();
    if (!ws || !u) return null;

    // Only create if user has permission
    if (u.workspace_role !== WSRoleAdmin && u.workspace_role !== WSRoleEditor) {
      return null;
    }

    try {
      // No pages exist, create a welcome page
      const newPage = await ws.nodes.page.createPage('0', {
        title: t('welcome.welcomePageTitle'),
      });
      return newPage?.id ? String(newPage.id) : null;
    } catch (err) {
      setError(`${t('errors.failedToCreate')}: ${err}`);
      return null;
    }
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

  // First-time login check: ensure user has org, workspace, and welcome page (auto-create if needed)
  createEffect(() => {
    const u = user();
    if (!u || firstLoginCheckDone() || firstLoginInProgress()) return;

    // Check if user has any organizations
    const orgs = u.organizations || [];
    if (orgs.length === 0) {
      // User has no organizations, auto-create one
      setFirstLoginInProgress(true);
      autoCreateOrganization().finally(() => setFirstLoginInProgress(false));
      return;
    }

    // User has organizations, check if first org has any workspaces
    const firstOrg = orgs[0];
    if (firstOrg) {
      const orgWorkspaces = u.workspaces?.filter((ws) => ws.organization_id === firstOrg.organization_id) || [];
      if (orgWorkspaces.length === 0) {
        // Only auto-create workspace if user is admin/owner of the org
        if (firstOrg.role === OrgRoleAdmin || firstOrg.role === OrgRoleOwner) {
          setFirstLoginInProgress(true);
          autoCreateWorkspace().finally(() => setFirstLoginInProgress(false));
          return;
        }
      }
    }

    // Check for welcome page and redirect
    const wsId = u.workspace_id;
    const wsName = u.workspace_name;
    if (!wsId) {
      // Workspace exists in list but not yet switched to - wait for switchWorkspace
      return;
    }

    // User has org and active workspace - mark first login check as done
    setFirstLoginCheckDone(true);

    // Note: autoCreateWelcomePage() is called by loadNodes() after nodes are loaded

    if (window.location.pathname === '/') {
      const wsSlug = slugify(wsName || 'workspace');
      window.history.replaceState(null, '', `/w/${wsId}+${wsSlug}/`);
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
          const data = await api().auth.getMe();
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
    if (path === '/settings') {
      setIsSettingsPage(true);
      setIsProfilePage(false);
      setIsPrivacyPage(false);
      setIsTermsPage(false);
      return;
    }
    setIsProfilePage(false);
    setIsSettingsPage(false);
    setIsPrivacyPage(false);
    setIsTermsPage(false);

    // Check for /w/wsID+wsSlug/nodeID+nodeSlug format
    const matchWithWs = path.match(/^\/w\/([^+/]+)(?:\+[^/]*)?\/([a-zA-Z0-9_-]+)(?:\+.*)?$/);
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

    // Check for /w/wsID+wsSlug/ format (workspace root without node)
    const matchWsRoot = path.match(/^\/w\/([^+/]+)(?:\+[^/]*)?\/?$/);
    if (matchWsRoot && matchWsRoot[1]) {
      const wsId = matchWsRoot[1];

      // If we are logged in but in wrong workspace, switch
      if (user() && user()?.workspace_id !== wsId) {
        try {
          const isMember = user()?.workspaces?.some((m) => m.workspace_id === wsId);
          if (isMember) {
            await switchWorkspace(wsId, false);
          } else {
            setError(t('errors.noAccessToWs') || 'You do not have access to this workspace');
            return;
          }
        } catch {
          return;
        }
      }

      // At workspace root, auto-select the first node if available
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

  let loadingNodes = false;
  let loadedForWorkspace: string | null = null;
  async function loadNodes(force = false) {
    const ws = wsApi();
    const wsId = user()?.workspace_id;
    if (!ws || loadingNodes) return;
    // Skip if already loaded for this workspace (unless forced)
    if (!force && wsId && loadedForWorkspace === wsId && nodes.length > 0) return;

    try {
      loadingNodes = true;
      setLoading(true);
      // Get top-level nodes (children of workspace root, id=0)
      const resp = await ws.nodes.listNodeChildren('0');
      let loadedNodes = resp?.nodes || [];

      // Auto-create welcome page if workspace is empty (first-time user)
      if (loadedNodes.length === 0 && firstLoginCheckDone()) {
        const newPageId = await createWelcomePageIfNeeded();
        if (newPageId) {
          // Re-fetch nodes after creating welcome page
          const resp2 = await ws.nodes.listNodeChildren('0');
          loadedNodes = resp2?.nodes || [];
        }
      }

      setNodes(reconcile(loadedNodes));
      loadedForWorkspace = wsId || null;
      setError(null);

      // Auto-select first node if at workspace root and no node is selected
      const path = window.location.pathname;
      const matchWsRoot = path.match(/^\/w\/([^+/]+)(?:\+[^/]*)?\/?$/);
      if (matchWsRoot && !selectedNodeId() && loadedNodes.length > 0 && loadedNodes[0]) {
        loadNode(loadedNodes[0].id, false);
      }
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      loadingNodes = false;
      setLoading(false);
    }
  }

  let loadingNodeId: string | null = null;
  let loadedNodeId: string | null = null;
  async function loadNode(id: string, pushState = true) {
    const ws = wsApi();
    if (!ws) return;
    // Skip if already loading or loaded this node
    if (loadingNodeId === id || loadedNodeId === id) return;

    // Flush any pending auto-save before switching nodes to prevent data loss
    debouncedAutoSave.flush();

    try {
      loadingNodeId = id;
      setLoading(true);
      setShowHistory(false);
      const nodeData = await ws.nodes.getNode(id);

      setSelectedNodeId(nodeData.id);
      setSelectedNodeData(nodeData);
      loadedNodeId = nodeData.id;
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
        const url = `/w/${wsId}+${wsSlug}/${nodeData.id}${nodeSlug ? '+' + nodeSlug : ''}`;
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

      // If it has table content, load records
      if (nodeData.has_table) {
        const recordsData = await ws.nodes.table.records.listRecords(id, { Offset: 0, Limit: PAGE_SIZE });
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
      loadingNodeId = null;
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
      const parent = parentId || nodeCreationParentId() || '0';
      let newNodeId: string | number;
      if (type === 'table') {
        const result = await ws.nodes.table.createTable(parent, {
          title: title(),
          properties: [],
        });
        newNodeId = result.id;
      } else {
        const result = await ws.nodes.page.createPage(parent, {
          title: title(),
        });
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

  async function saveNode() {
    const nodeId = selectedNodeId();
    const ws = wsApi();
    if (!nodeId || !ws) return;

    try {
      setLoading(true);
      await ws.nodes.page.updatePage(nodeId, { title: title(), content: content() });
      await loadNodes(true);
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);

      // Update URL if title changed
      const slug = slugify(title());
      const currentPath = window.location.pathname;
      const wsId = user()?.workspace_id;
      const wsName = user()?.workspace_name;
      if (wsId) {
        const wsSlug = slugify(wsName || 'workspace');
        const newPath = `/w/${wsId}+${wsSlug}/${nodeId}${slug ? '+' + slug : ''}`;
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
      await ws.nodes.deleteNode(nodeId);
      await loadNodes(true);
      setSelectedNodeId(null);
      setSelectedNodeData(null);
      setTitle('');
      setContent('');
      setHasUnsavedChanges(false);
      setAutoSaveStatus('idle');
      setError(null);
      const wsId = user()?.workspace_id;
      const wsName = user()?.workspace_name;
      if (wsId) {
        const wsSlug = slugify(wsName || 'workspace');
        window.history.pushState(null, '', `/w/${wsId}+${wsSlug}/`);
      } else {
        window.history.pushState(null, '', '/');
      }
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

  // Fetch children for a node (used for lazy loading in sidebar)
  async function fetchNodeChildren(nodeId: string): Promise<NodeResponse[]> {
    const ws = wsApi();
    if (!ws) return [];

    try {
      const data = await ws.nodes.listNodeChildren(nodeId);
      return (data.nodes?.filter(Boolean) as NodeResponse[]) || [];
    } catch (err) {
      console.error('Failed to fetch children:', err);
      return [];
    }
  }

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
      await ws.nodes.table.records.createRecord(nodeId, { data });
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
      await ws.nodes.table.records.deleteRecord(nodeId, recordId);
      loadNode(nodeId);
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
      // Reload records to reflect changes
      const recordsData = await ws.nodes.table.records.listRecords(nodeId, { Offset: 0, Limit: PAGE_SIZE });
      setRecords((recordsData.records || []) as DataRecordResponse[]);
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

  return (
    <>
      <Show when={!isPrivacyPage()} fallback={<Privacy />}>
        <Show when={!isTermsPage()} fallback={<Terms />}>
          <Show when={user()} fallback={<Auth onLogin={handleLogin} />}>
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
                  <Show when={(user()?.organizations?.length ?? 0) > 1}>
                    <OrgMenu
                      memberships={user()?.organizations || []}
                      currentOrgId={user()?.organization_id || ''}
                      onSwitchOrg={(orgId) => switchOrg(orgId)}
                      onCreateOrg={() => setShowCreateOrg(true)}
                    />
                  </Show>
                  <WorkspaceMenu
                    workspaces={user()?.workspaces || []}
                    currentOrgId={user()?.organization_id || ''}
                    currentWsId={user()?.workspace_id || ''}
                    onSwitchWorkspace={(wsId) => switchWorkspace(wsId)}
                    onOpenSettings={() => {
                      setIsSettingsPage(true);
                      setSelectedNodeId(null);
                      setShowMobileSidebar(false);
                      window.history.pushState(null, '', '/settings');
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
                  onBack={() => {
                    window.history.back();
                  }}
                />
              </Show>

              <Show when={isSettingsPage()}>
                <WorkspaceSettings
                  user={user() as UserResponse}
                  token={token() as string}
                  onBack={() => {
                    window.history.back();
                  }}
                />
              </Show>

              <Show when={!isProfilePage() && !isSettingsPage()}>
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
                  />

                  <main class={styles.main}>
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
                          {/* Always show markdown content if it exists or if node has page content */}
                          <Show when={selectedNodeData()?.has_page}>
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

                          {/* Show table if node has table content */}
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
