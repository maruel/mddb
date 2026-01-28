// Workspace context providing node tree, navigation, and workspace switching.

import {
  createContext,
  useContext,
  createSignal,
  createEffect,
  batch,
  type ParentComponent,
  type Accessor,
} from 'solid-js';
import { createStore, produce, reconcile } from 'solid-js/store';
import { useAuth } from './AuthContext';
import { useI18n } from '../i18n';
import { workspaceUrl, nodeUrl } from '../utils/urls';
import { OrgRoleAdmin, OrgRoleOwner, WSRoleAdmin, WSRoleEditor, type NodeResponse } from '@sdk/types.gen';

interface WorkspaceContextValue {
  // Node tree
  nodes: NodeResponse[];
  setNodes: (fn: (nodes: NodeResponse[]) => void) => void;

  // Selected node
  selectedNodeId: Accessor<string | null>;
  setSelectedNodeId: (id: string | null) => void;
  selectedNodeData: Accessor<NodeResponse | null>;
  setSelectedNodeData: (node: NodeResponse | null) => void;

  // Breadcrumbs
  breadcrumbPath: Accessor<NodeResponse[]>;
  setBreadcrumbPath: (path: NodeResponse[] | ((prev: NodeResponse[]) => NodeResponse[])) => void;

  // Loading/error state
  loading: Accessor<boolean>;
  setLoading: (loading: boolean) => void;
  error: Accessor<string | null>;
  setError: (error: string | null) => void;

  // First login state
  firstLoginCheckDone: Accessor<boolean>;
  setFirstLoginCheckDone: (done: boolean) => void;

  // Operations
  switchWorkspace: (wsId: string, redirect?: boolean) => Promise<void>;
  createOrganization: (data: { name: string }) => Promise<void>;
  createWorkspace: (data: { name: string }) => Promise<void>;
  loadNodes: (force?: boolean) => Promise<void>;
  loadNode: (id: string, pushState?: boolean) => Promise<NodeResponse | undefined>;
  fetchNodeChildren: (nodeId: string) => Promise<NodeResponse[]>;
  updateNodeTitle: (nodeId: string, newTitle: string) => void;
}

const WorkspaceContext = createContext<WorkspaceContextValue>();

export const WorkspaceProvider: ParentComponent = (props) => {
  const { t } = useI18n();
  const { user, setUser, api, wsApi, login } = useAuth();

  // Node state
  const [nodes, setNodesStore] = createStore<NodeResponse[]>([]);
  const [selectedNodeId, setSelectedNodeId] = createSignal<string | null>(null);
  const [selectedNodeData, setSelectedNodeData] = createSignal<NodeResponse | null>(null);
  const [breadcrumbPath, setBreadcrumbPath] = createSignal<NodeResponse[]>([]);

  // Loading/error state
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);

  // First login state
  const [firstLoginCheckDone, setFirstLoginCheckDone] = createSignal(false);
  const [firstLoginInProgress, setFirstLoginInProgress] = createSignal(false);

  // Track loading state to prevent duplicate calls
  let loadingNodes = false;
  let loadedForWorkspace: string | null = null;
  let loadingNodeId: string | null = null;
  let loadedNodeId: string | null = null;

  // Wrapper for setNodes using produce
  const setNodes = (fn: (nodes: NodeResponse[]) => void) => {
    setNodesStore(produce(fn));
  };

  // Helper to update node title in local state (sidebar and breadcrumbs)
  const updateNodeTitle = (nodeId: string, newTitle: string) => {
    setNodesStore(
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
    setBreadcrumbPath((path) => path.map((node) => (node.id === nodeId ? { ...node, title: newTitle } : node)));
  };

  // Get user's first name for default naming
  function getUserFirstName(): string {
    const u = user();
    if (!u?.name) return '';
    const firstName = u.name.split(' ')[0];
    return firstName || u.name;
  }

  async function createOrganization(data: { name: string }) {
    await api().organizations.createOrganization({ name: data.name });
    const updatedUser = await api().auth.getMe();
    setUser(updatedUser);
  }

  async function createWorkspace(data: { name: string }) {
    const u = user();
    if (!u || !u.organization_id) {
      throw new Error('No organization selected');
    }
    const ws = await api().org(u.organization_id).workspaces.createWorkspace({ name: data.name });
    const updatedUser = await api().auth.getMe();
    setUser(updatedUser);
    await switchWorkspace(ws.id);
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
  async function createWelcomePageIfNeeded(): Promise<string | null> {
    const ws = wsApi();
    const u = user();
    if (!ws || !u) return null;
    if (u.workspace_role !== WSRoleAdmin && u.workspace_role !== WSRoleEditor) {
      return null;
    }
    try {
      const newPage = await ws.nodes.page.createPage('0', {
        title: t('welcome.welcomePageTitle'),
        content: t('welcome.welcomePageContent'),
      });
      return newPage?.id ? String(newPage.id) : null;
    } catch (err) {
      setError(`${t('errors.failedToCreate')}: ${err}`);
      return null;
    }
  }

  async function switchWorkspace(wsId: string, redirect = true) {
    try {
      setLoading(true);
      const data = await api().auth.switchWorkspace({ ws_id: wsId });
      if (!data.user) {
        throw new Error('No user data returned');
      }
      login(data.token, data.user);
      batch(() => {
        setSelectedNodeId(null);
        setSelectedNodeData(null);
        setBreadcrumbPath([]);
      });
      loadedNodeId = null;
      loadedForWorkspace = null;
      if (redirect) {
        const newWsId = data.user.workspace_id;
        const wsName = data.user.workspace_name;
        if (newWsId) {
          window.history.pushState(null, '', workspaceUrl(newWsId, wsName));
        } else {
          window.history.pushState(null, '', '/');
        }
      }
      await loadNodes();
    } catch (err) {
      setError(`${t('errors.failedToSwitch')}: ${err}`);
    } finally {
      setLoading(false);
    }
  }

  async function loadNodes(force = false) {
    const ws = wsApi();
    const wsId = user()?.workspace_id;
    if (!ws || loadingNodes) return;
    if (!force && wsId && loadedForWorkspace === wsId && nodes.length > 0) return;

    try {
      loadingNodes = true;
      setLoading(true);
      const resp = await ws.nodes.listNodeChildren('0');
      let loadedNodes = resp?.nodes || [];

      if (loadedNodes.length === 0 && firstLoginCheckDone()) {
        const newPageId = await createWelcomePageIfNeeded();
        if (newPageId) {
          const resp2 = await ws.nodes.listNodeChildren('0');
          loadedNodes = resp2?.nodes || [];
        }
      }

      setNodesStore(reconcile(loadedNodes));
      loadedForWorkspace = wsId || null;
      setError(null);

      // Auto-select first node if at workspace root
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

  async function loadNode(id: string, pushState = true): Promise<NodeResponse | undefined> {
    const ws = wsApi();
    if (!ws) return undefined;
    if (loadingNodeId === id || loadedNodeId === id) return undefined;

    try {
      loadingNodeId = id;
      setLoading(true);
      const nodeData = await ws.nodes.getNode(id);

      batch(() => {
        setSelectedNodeId(nodeData.id);
        setSelectedNodeData(nodeData);
      });
      loadedNodeId = nodeData.id;
      setError(null);

      // Update URL
      const wsId = user()?.workspace_id;
      const wsName = user()?.workspace_name;
      if (wsId) {
        const url = nodeUrl(wsId, wsName, nodeData.id, nodeData.title);
        if (pushState) {
          if (window.location.pathname !== url) {
            window.history.pushState(null, '', url);
          }
        } else {
          if (window.location.pathname !== url) {
            window.history.replaceState(null, '', url);
          }
        }
      }

      // Build breadcrumb path
      const path: NodeResponse[] = [nodeData];
      let currentNode = nodeData;
      while (currentNode.parent_id && currentNode.parent_id !== '0') {
        try {
          const parentNode = await ws.nodes.getNode(currentNode.parent_id);
          path.unshift(parentNode);
          currentNode = parentNode;
        } catch {
          break;
        }
      }
      setBreadcrumbPath(path);

      return nodeData;
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
      return undefined;
    } finally {
      loadingNodeId = null;
      setLoading(false);
    }
  }

  async function fetchNodeChildren(nodeId: string): Promise<NodeResponse[]> {
    const ws = wsApi();
    if (!ws) return [];

    try {
      const data = await ws.nodes.listNodeChildren(nodeId);
      const children = (data.nodes?.filter(Boolean) as NodeResponse[]) || [];

      if (children.length > 0) {
        setNodesStore(
          produce((list) => {
            const updateChildren = (nodes: NodeResponse[]): boolean => {
              for (const node of nodes) {
                if (node.id === nodeId) {
                  node.children = children;
                  return true;
                }
                if (node.children && updateChildren(node.children)) {
                  return true;
                }
              }
              return false;
            };
            updateChildren(list);
          })
        );
      }

      return children;
    } catch (err) {
      console.error('Failed to fetch children:', err);
      return [];
    }
  }

  // First-time login check
  createEffect(() => {
    const u = user();
    if (!u || firstLoginCheckDone() || firstLoginInProgress()) return;

    const orgs = u.organizations || [];
    if (orgs.length === 0) {
      setFirstLoginInProgress(true);
      autoCreateOrganization().finally(() => setFirstLoginInProgress(false));
      return;
    }

    const firstOrg = orgs[0];
    if (firstOrg) {
      const orgWorkspaces = u.workspaces?.filter((ws) => ws.organization_id === firstOrg.organization_id) || [];
      if (orgWorkspaces.length === 0) {
        if (firstOrg.role === OrgRoleAdmin || firstOrg.role === OrgRoleOwner) {
          setFirstLoginInProgress(true);
          autoCreateWorkspace().finally(() => setFirstLoginInProgress(false));
          return;
        }
      }
    }

    const wsId = u.workspace_id;
    const wsName = u.workspace_name;
    if (!wsId) return;

    setFirstLoginCheckDone(true);

    if (window.location.pathname === '/') {
      window.history.replaceState(null, '', workspaceUrl(wsId, wsName));
    }
  });

  // Load nodes when user changes
  createEffect(() => {
    if (user()) {
      loadNodes();
    }
  });

  // Reset loadedNodeId when workspace changes
  createEffect(() => {
    const wsId = user()?.workspace_id;
    if (wsId && loadedForWorkspace !== wsId) {
      loadedNodeId = null;
    }
  });

  const value: WorkspaceContextValue = {
    nodes,
    setNodes,
    selectedNodeId,
    setSelectedNodeId,
    selectedNodeData,
    setSelectedNodeData,
    breadcrumbPath,
    setBreadcrumbPath,
    loading,
    setLoading,
    error,
    setError,
    firstLoginCheckDone,
    setFirstLoginCheckDone,
    switchWorkspace,
    createOrganization,
    createWorkspace,
    loadNodes,
    loadNode,
    fetchNodeChildren,
    updateNodeTitle,
  };

  return <WorkspaceContext.Provider value={value}>{props.children}</WorkspaceContext.Provider>;
};

export function useWorkspace(): WorkspaceContextValue {
  const context = useContext(WorkspaceContext);
  if (!context) {
    throw new Error('useWorkspace must be used within a WorkspaceProvider');
  }
  return context;
}
