// Workspace context providing node tree, navigation, and workspace switching.

import {
  createContext,
  useContext,
  createSignal,
  createEffect,
  createMemo,
  batch,
  type ParentComponent,
  type Accessor,
} from 'solid-js';
import { createStore, produce, reconcile } from 'solid-js/store';
import { useAuth } from './AuthContext';
import { useI18n } from '../i18n';
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

  // Operation-specific loading states
  loadingNodes: Accessor<boolean>;
  loadingNodeId: Accessor<string | null>; // ID of node being loaded
  switchingWorkspace: Accessor<boolean>;
  savingNodeId: Accessor<string | null>; // ID of node being saved (for column updates, etc.)
  setSavingNodeId: (id: string | null) => void;
  deletingNodeId: Accessor<string | null>; // ID of node being deleted
  setDeletingNodeId: (id: string | null) => void;
  creatingNode: Accessor<boolean>;
  setCreatingNode: (creating: boolean) => void;

  // Operation-specific error states
  loadError: Accessor<string | null>;
  setLoadError: (error: string | null) => void;
  saveError: Accessor<string | null>;
  setSaveError: (error: string | null) => void;

  // Combined loading (any operation in progress) - for backward compatibility
  loading: Accessor<boolean>;

  // First login state
  firstLoginCheckDone: Accessor<boolean>;
  setFirstLoginCheckDone: (done: boolean) => void;

  // Operations
  switchWorkspace: (wsId: string) => Promise<void>;
  createOrganization: (data: { name: string }) => Promise<void>;
  createWorkspace: (data: { name: string }) => Promise<void>;
  loadNodes: (force?: boolean) => Promise<void>;
  loadNode: (id: string) => Promise<NodeResponse | undefined>;
  fetchNodeChildren: (nodeId: string) => Promise<void>;
  removeNode: (nodeId: string) => void;
  updateNodeTitle: (nodeId: string, newTitle: string) => void;
  moveNode: (nodeId: string, newParentId: string) => Promise<void>;

  // Clear errors
  clearErrors: () => void;
}

const WorkspaceContext = createContext<WorkspaceContextValue>();

export const WorkspaceProvider: ParentComponent = (props) => {
  const { t, ready: i18nReady } = useI18n();
  const { user, setUser, api, wsApi, login } = useAuth();

  // Node state
  const [nodes, setNodesStore] = createStore<NodeResponse[]>([]);
  const [selectedNodeId, setSelectedNodeId] = createSignal<string | null>(null);
  const [selectedNodeData, setSelectedNodeData] = createSignal<NodeResponse | null>(null);
  const [breadcrumbPath, setBreadcrumbPath] = createSignal<NodeResponse[]>([]);

  // Operation-specific loading states
  const [loadingNodes, setLoadingNodes] = createSignal(false);
  const [loadingNodeId, setLoadingNodeId] = createSignal<string | null>(null);
  const [switchingWorkspace, setSwitchingWorkspace] = createSignal(false);
  const [savingNodeId, setSavingNodeId] = createSignal<string | null>(null);
  const [deletingNodeId, setDeletingNodeId] = createSignal<string | null>(null);
  const [creatingNode, setCreatingNode] = createSignal(false);

  // Operation-specific error states
  const [loadError, setLoadError] = createSignal<string | null>(null);
  const [saveError, setSaveError] = createSignal<string | null>(null);

  // Combined loading (any operation in progress) - derived for backward compatibility
  const loading = createMemo(
    () =>
      loadingNodes() ||
      loadingNodeId() !== null ||
      switchingWorkspace() ||
      savingNodeId() !== null ||
      deletingNodeId() !== null ||
      creatingNode()
  );

  // First login state
  const [firstLoginCheckDone, setFirstLoginCheckDone] = createSignal(false);
  const [firstLoginInProgress, setFirstLoginInProgress] = createSignal(false);

  // Track loaded state to prevent duplicate calls
  const [loadedForWorkspace, setLoadedForWorkspace] = createSignal<string | null>(null);
  const [loadedNodeId, setLoadedNodeId] = createSignal<string | null>(null);

  // Clear all errors
  const clearErrors = () => {
    setLoadError(null);
    setSaveError(null);
  };

  // Wrapper for setNodes using produce
  const setNodes = (fn: (nodes: NodeResponse[]) => void) => {
    setNodesStore(produce(fn));
  };

  // Helper to update node title in local state (sidebar and breadcrumbs)
  const updateNodeTitle = (nodeId: string, newTitle: string) => {
    setNodesStore(
      produce((list) => {
        const update = (nodeList: NodeResponse[]): boolean => {
          for (const node of nodeList) {
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

  // Remove a node from the store (used after deletion)
  const removeNode = (nodeId: string) => {
    setNodesStore(
      produce((list) => {
        const removeFromList = (nodeList: NodeResponse[]): boolean => {
          const index = nodeList.findIndex((n) => n.id === nodeId);
          if (index !== -1) {
            nodeList.splice(index, 1);
            return true;
          }
          for (const node of nodeList) {
            if (node.children && removeFromList(node.children)) {
              // Update has_children if parent now has no children
              if (node.children.length === 0) {
                node.has_children = false;
              }
              return true;
            }
          }
          return false;
        };
        removeFromList(list);
      })
    );
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
      setCreatingNode(true);
      const firstName = getUserFirstName();
      const orgName = firstName
        ? t('onboarding.defaultOrgName', { name: firstName })
        : t('onboarding.defaultOrgNameFallback');
      await createOrganization({ name: orgName as string });
    } catch (err) {
      setSaveError(`${t('errors.failedToCreate')}: ${err}`);
    } finally {
      setCreatingNode(false);
    }
  }

  // Auto-create workspace for users with org but no workspace
  async function autoCreateWorkspace() {
    try {
      setCreatingNode(true);
      const firstName = getUserFirstName();
      const wsName = firstName
        ? t('onboarding.defaultWorkspaceName', { name: firstName })
        : t('onboarding.defaultWorkspaceNameFallback');
      await createWorkspace({ name: wsName as string });
    } catch (err) {
      setSaveError(`${t('errors.failedToCreate')}: ${err}`);
    } finally {
      setCreatingNode(false);
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
      setSaveError(`${t('errors.failedToCreate')}: ${err}`);
      return null;
    }
  }

  async function switchWorkspace(wsId: string) {
    try {
      setSwitchingWorkspace(true);
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
      setLoadedNodeId(null);
      setLoadedForWorkspace(null);
      await loadNodes();
    } catch (err) {
      setLoadError(`${t('errors.failedToSwitch')}: ${err}`);
    } finally {
      setSwitchingWorkspace(false);
    }
  }

  async function loadNodes(force = false) {
    const ws = wsApi();
    const wsId = user()?.workspace_id;
    if (!ws || loadingNodes()) return;
    if (!force && wsId && loadedForWorkspace() === wsId && nodes.length > 0) return;

    try {
      setLoadingNodes(true);
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
      setLoadedForWorkspace(wsId || null);
      setLoadError(null);
      // Navigation to first node is handled by WorkspaceRoot component
    } catch (err) {
      setLoadError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoadingNodes(false);
    }
  }

  async function loadNode(id: string): Promise<NodeResponse | undefined> {
    const ws = wsApi();
    if (!ws) return undefined;
    if (loadingNodeId() === id || loadedNodeId() === id) return undefined;

    try {
      setLoadingNodeId(id);
      const nodeData = await ws.nodes.getNode(id);

      batch(() => {
        setSelectedNodeId(nodeData.id);
        setSelectedNodeData(nodeData);
      });
      setLoadedNodeId(nodeData.id);
      setLoadError(null);

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
      setLoadError(`${t('errors.failedToLoad')}: ${err}`);
      return undefined;
    } finally {
      setLoadingNodeId(null);
    }
  }

  async function fetchNodeChildren(nodeId: string): Promise<void> {
    const ws = wsApi();
    if (!ws) return;

    try {
      const data = await ws.nodes.listNodeChildren(nodeId);
      const children = (data.nodes?.filter(Boolean) as NodeResponse[]) || [];

      setNodesStore(
        produce((list) => {
          const updateChildren = (nodeList: NodeResponse[]): boolean => {
            for (const node of nodeList) {
              if (node.id === nodeId) {
                node.children = children;
                node.has_children = children.length > 0;
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
    } catch (err) {
      console.error('Failed to fetch children:', err);
    }
  }

  // First-time login check (wait for translations to be ready)
  // Navigation to workspace is handled by App.tsx RootRedirect component
  createEffect(() => {
    const u = user();
    if (!u || !i18nReady() || firstLoginCheckDone() || firstLoginInProgress()) return;

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
    if (!wsId) return;

    setFirstLoginCheckDone(true);
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
    if (wsId && loadedForWorkspace() !== wsId) {
      setLoadedNodeId(null);
    }
  });

  // Move a node to a new parent (or root if newParentId === "0")
  async function moveNode(nodeId: string, newParentId: string): Promise<void> {
    const ws = wsApi();
    if (!ws) return;

    try {
      await ws.nodes.moveNode(nodeId, { new_parent_id: newParentId });

      setNodesStore(
        produce((list) => {
          // Find and remove node from old location
          let movedNode: NodeResponse | null = null;

          const removeFromList = (nodeList: NodeResponse[]): boolean => {
            const index = nodeList.findIndex((n) => n.id === nodeId);
            if (index !== -1) {
              movedNode = { ...nodeList[index] } as NodeResponse;
              nodeList.splice(index, 1);
              return true;
            }
            for (const node of nodeList) {
              if (node.children && removeFromList(node.children)) {
                if (node.children.length === 0) {
                  node.has_children = false;
                }
                return true;
              }
            }
            return false;
          };
          removeFromList(list);

          if (!movedNode) return;
          const nodeToMove: NodeResponse = movedNode;

          // Add to new parent
          if (newParentId === '0') {
            list.push(nodeToMove);
          } else {
            const addToParent = (nodeList: NodeResponse[]): boolean => {
              for (const node of nodeList) {
                if (node.id === newParentId) {
                  if (!node.children) node.children = [];
                  node.children.push(nodeToMove);
                  node.has_children = true;
                  return true;
                }
                if (node.children && addToParent(node.children)) return true;
              }
              return false;
            };
            addToParent(list);
          }
        })
      );
      setSaveError(null);
    } catch (err) {
      setSaveError(`${t('errors.failedToMove')}: ${err}`);
    }
  }

  const value: WorkspaceContextValue = {
    nodes,
    setNodes,
    selectedNodeId,
    setSelectedNodeId,
    selectedNodeData,
    setSelectedNodeData,
    breadcrumbPath,
    setBreadcrumbPath,
    loadingNodes,
    loadingNodeId,
    switchingWorkspace,
    savingNodeId,
    setSavingNodeId,
    deletingNodeId,
    setDeletingNodeId,
    creatingNode,
    setCreatingNode,
    loadError,
    setLoadError,
    saveError,
    setSaveError,
    loading,
    firstLoginCheckDone,
    setFirstLoginCheckDone,
    switchWorkspace,
    createOrganization,
    createWorkspace,
    loadNodes,
    loadNode,
    fetchNodeChildren,
    removeNode,
    updateNodeTitle,
    moveNode,
    clearErrors,
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
