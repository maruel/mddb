// Workspace context providing node tree, navigation, and workspace switching.

import {
  createContext,
  useContext,
  createSignal,
  createEffect,
  createMemo,
  batch,
  on,
  onCleanup,
  type ParentComponent,
  type Accessor,
} from "solid-js";
import { produce } from "solid-js/store";
import { useNavigate } from "@solidjs/router";
import { useAuth } from "./AuthContext";
import { useWorkspaceData } from "./WorkspaceDataContext";
import { useEventSource } from "./EventSourceContext";
import { useI18n } from "../i18n";
import {
  EventNodeCreated,
  EventNodeDeleted,
  EventNodeMoved,
  EventNodeUpdated,
  OrgRoleAdmin,
  OrgRoleOwner,
  WSRoleAdmin,
  WSRoleEditor,
  type NodeResponse,
} from "@sdk/types.gen";
import { workspaceUrl } from "../utils/urls";
import { APIError, createApi } from "../useApi";
import { findNodePath, reconcileBreadcrumbPath, reconcileMovedNode, reconcileSelectedNodeData } from "./workspaceTree";
import { provisionFirstWorkspace } from "./firstLogin";

interface NodeLoadOptions {
  refresh: boolean;
  canApply?: () => boolean;
}

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
  failedNodeId: Accessor<string | null>;
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
  loadNode: (id: string, options: NodeLoadOptions) => Promise<NodeResponse | undefined>;
  fetchNodeChildren: (nodeId: string) => Promise<void>;
  removeNode: (nodeId: string) => void;
  updateNodeTitle: (nodeId: string, newTitle: string) => void;
  updateNodeIcon: (nodeId: string, newIcon: string) => void;
  moveNode: (nodeId: string, newParentId: string) => Promise<void>;

  // Clear errors
  clearErrors: () => void;
}

const WorkspaceContext = createContext<WorkspaceContextValue>();

export const WorkspaceProvider: ParentComponent = (props) => {
  const { t, ready: i18nReady } = useI18n();
  const navigate = useNavigate();
  const { user, identity: authIdentity, setUser, api, wsApi } = useAuth();

  const rootData = useWorkspaceData();
  const { nodes, setNodesStore, loadingNodes, loadError, setLoadError } = rootData;
  let disposed = false;
  onCleanup(() => {
    disposed = true;
    nodeRequest++;
  });

  // Selected-node and editor ownership remain local to this route.
  const [selectedNodeId, setSelectedNodeId] = createSignal<string | null>(null);
  const [selectedNodeData, setSelectedNodeData] = createSignal<NodeResponse | null>(null);
  const [breadcrumbPath, setBreadcrumbPath] = createSignal<NodeResponse[]>([]);

  // Operation-specific loading states
  const [loadingNodeId, setLoadingNodeId] = createSignal<string | null>(null);
  const [switchingWorkspace, setSwitchingWorkspace] = createSignal(false);
  const [savingNodeId, setSavingNodeId] = createSignal<string | null>(null);
  const [deletingNodeId, setDeletingNodeId] = createSignal<string | null>(null);
  const [creatingNode, setCreatingNode] = createSignal(false);

  // Operation-specific error states
  const [saveError, setSaveError] = createSignal<string | null>(null);

  // Combined loading (any operation in progress) - derived for backward compatibility
  const loading = createMemo(
    () =>
      loadingNodes() ||
      loadingNodeId() !== null ||
      switchingWorkspace() ||
      savingNodeId() !== null ||
      deletingNodeId() !== null ||
      creatingNode(),
  );

  // First login state
  const [firstLoginCheckDone, setFirstLoginCheckDone] = createSignal(false);
  const [firstLoginInProgress, setFirstLoginInProgress] = createSignal(false);

  // Track loaded state to prevent duplicate calls
  const [failedNodeId, setFailedNodeId] = createSignal<string | null>(null);
  const [loadedNodeId, setLoadedNodeId] = createSignal<string | null>(null);
  const workspaceIdentity = () => `${authIdentity()}:${user()?.id ?? ""}:${user()?.workspace_id ?? ""}`;
  let nodeRequest = 0;

  let previousIdentity = workspaceIdentity();
  createEffect(() => {
    const identity = workspaceIdentity();
    if (identity === previousIdentity) return;
    previousIdentity = identity;
    nodeRequest++;
    batch(() => {
      setSelectedNodeId(null);
      setSelectedNodeData(null);
      setBreadcrumbPath([]);
      setLoadedNodeId(null);
      setLoadingNodeId(null);
      setSwitchingWorkspace(false);
      setCreatingNode(false);
    });
  });

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
      }),
    );
    setBreadcrumbPath((path) => path.map((node) => (node.id === nodeId ? { ...node, title: newTitle } : node)));
  };

  const updateNodeIcon = (nodeId: string, newIcon: string) => {
    setNodesStore(
      produce((list) => {
        const update = (nodeList: NodeResponse[]): boolean => {
          for (const node of nodeList) {
            if (node.id === nodeId) {
              node.icon = newIcon;
              return true;
            }
            if (node.children) {
              if (update(node.children)) return true;
            }
          }
          return false;
        };
        update(list);
      }),
    );
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
      }),
    );
  };

  async function createOrganization(data: { name: string }) {
    const client = api();
    const initialIdentity = authIdentity();
    await client.createOrganization({ name: data.name });
    if (authIdentity() !== initialIdentity) return;
    const updatedUser = await client.getMe();
    if (authIdentity() !== initialIdentity) return;
    setUser(updatedUser);
  }

  async function createWorkspace(data: { name: string }) {
    const u = user();
    if (!u || !u.organization_id) {
      throw new Error("No organization selected");
    }
    const client = api();
    const initialIdentity = authIdentity();
    const ws = await client.org(u.organization_id).createWorkspace({ name: data.name });
    if (authIdentity() !== initialIdentity) return;
    const updatedUser = await client.getMe();
    if (authIdentity() !== initialIdentity) return;
    setUser(updatedUser);
    await switchWorkspace(ws.id);
  }

  // Provision the first organization and workspace with server-side idempotency.
  async function autoProvisionFirstLogin() {
    const initialUser = user();
    const initialIdentity = authIdentity();
    if (!initialUser) return;
    try {
      setCreatingNode(true);
      const firstName = initialUser.name.split(" ")[0] || initialUser.name;
      const result = await provisionFirstWorkspace(initialUser.id, createApi(undefined, initialUser.session_id), {
        organization: (firstName
          ? t("onboarding.defaultOrgName", { name: firstName })
          : t("onboarding.defaultOrgNameFallback")) as string,
        workspace: (firstName
          ? t("onboarding.defaultWorkspaceName", { name: firstName })
          : t("onboarding.defaultWorkspaceNameFallback")) as string,
      });
      if (authIdentity() !== initialIdentity) return;
      setUser(result.user);
      if (result.user.workspace_id) navigate(workspaceUrl(result.user.workspace_id, result.user.workspace_name));
    } catch (err) {
      if (authIdentity() === initialIdentity) {
        setSaveError(`${t("errors.failedToCreate")}: ${err}`);
        setFirstLoginCheckDone(true);
      }
    } finally {
      if (authIdentity() === initialIdentity) setCreatingNode(false);
    }
  }

  // Auto-create welcome page if no root page exists
  async function createWelcomePageIfNeeded(): Promise<void> {
    const ws = wsApi();
    const u = user();
    if (!ws || !u?.workspace_id) return;
    if (u.workspace_role !== WSRoleAdmin && u.workspace_role !== WSRoleEditor) {
      return;
    }
    try {
      await ws.createPage("0", {
        title: t("welcome.welcomePageTitle"),
        content: t("welcome.welcomePageContent"),
        ensure_root_if_empty: true,
      });
    } catch (err) {
      setSaveError(`${t("errors.failedToCreate")}: ${err}`);
    }
  }

  async function switchWorkspace(wsId: string) {
    const initialIdentity = authIdentity();
    const client = api();
    try {
      setSwitchingWorkspace(true);
      const data = await client.switchWorkspace({ ws_id: wsId });
      if (authIdentity() !== initialIdentity) return;
      if (!data.user) {
        throw new Error("No user data returned");
      }
      setUser(data.user);
      batch(() => {
        setSelectedNodeId(null);
        setSelectedNodeData(null);
        setBreadcrumbPath([]);
      });
      setLoadedNodeId(null);
      // Navigate to the new workspace root to clear any stale node ID from the URL.
      const u = user();
      if (u?.workspace_id) {
        navigate(workspaceUrl(u.workspace_id, u.workspace_name));
      }
    } catch (err) {
      if (authIdentity() === initialIdentity) setLoadError(`${t("errors.failedToSwitch")}: ${err}`);
    } finally {
      if (authIdentity() === initialIdentity) setSwitchingWorkspace(false);
    }
  }

  const loadNodes = async (force?: boolean) => {
    try {
      await rootData.loadNodes(force === true);
    } catch (error) {
      console.warn("Workspace refresh failed", error);
    }
  };

  async function loadNode(id: string, options: NodeLoadOptions): Promise<NodeResponse | undefined> {
    const ws = wsApi();
    if (!ws) return undefined;
    if (!options.refresh && (loadingNodeId() === id || loadedNodeId() === id)) return undefined;

    const identity = workspaceIdentity();
    const request = ++nodeRequest;

    try {
      setFailedNodeId(null);
      setLoadingNodeId(id);
      const nodeData = await ws.getNode(id);
      if (request !== nodeRequest || identity !== workspaceIdentity() || disposed) return undefined;
      if (options.canApply && !options.canApply()) return undefined;

      batch(() => {
        setSelectedNodeId(nodeData.id);
        setSelectedNodeData(nodeData);
      });
      setLoadedNodeId(nodeData.id);
      setLoadError(null);

      // Build breadcrumb path
      const path: NodeResponse[] = [nodeData];
      let currentNode = nodeData;
      while (currentNode.parent_id && currentNode.parent_id !== "0") {
        try {
          const parentNode = await ws.getNode(currentNode.parent_id);
          if (request !== nodeRequest || identity !== workspaceIdentity()) return undefined;
          path.unshift(parentNode);
          currentNode = parentNode;
        } catch {
          break;
        }
      }
      if (request !== nodeRequest || identity !== workspaceIdentity()) return undefined;
      setBreadcrumbPath(path);

      return nodeData;
    } catch (err) {
      if (!disposed && request === nodeRequest && identity === workspaceIdentity()) {
        setFailedNodeId(id);
        setLoadError(`${t("errors.failedToLoad")}: ${err}`);
      }
      return undefined;
    } finally {
      if (request === nodeRequest) setLoadingNodeId(null);
    }
  }

  async function fetchNodeChildren(nodeId: string): Promise<void> {
    const ws = wsApi();
    if (!ws) return;
    const identity = workspaceIdentity();

    try {
      const data = await ws.listNodeChildren(nodeId);
      if (identity !== workspaceIdentity()) return;
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
        }),
      );
    } catch (err) {
      console.error("Failed to fetch children:", err);
    }
  }

  // First-time login check (wait for translations to be ready)
  // Navigation to workspace is handled by App.tsx RootRedirect component
  createEffect(() => {
    const u = user();
    if (!u || !i18nReady() || firstLoginCheckDone() || firstLoginInProgress()) return;

    const firstOrg = u.organizations?.[0];
    const orgWorkspaces = u.workspaces?.filter((ws) => ws.organization_id === firstOrg?.organization_id) ?? [];
    if (
      !firstOrg ||
      (!u.workspace_id && !!u.workspaces?.length) ||
      (orgWorkspaces.length === 0 && (firstOrg.role === OrgRoleAdmin || firstOrg.role === OrgRoleOwner))
    ) {
      setFirstLoginInProgress(true);
      void autoProvisionFirstLogin().finally(() => setFirstLoginInProgress(false));
      return;
    }

    const wsId = u.workspace_id;
    if (!wsId) return;

    setFirstLoginCheckDone(true);
  });

  // The route owns welcome initialization; the root owns each canonical read.
  let initializationOwner: string | null = null;
  const initialize = () => {
    const captured = rootData.owner();
    if (!firstLoginCheckDone() || captured === null) return;
    if (initializationOwner === captured) return;
    initializationOwner = captured;
    const current = () => !disposed && rootData.owner() === captured;
    void (async () => {
      const snapshot = await rootData.loadNodes(false);
      if (!current()) return;
      const role = user()?.workspace_role;
      const refresh = snapshot.length === 0 && (role === WSRoleAdmin || role === WSRoleEditor);
      if (refresh) {
        await createWelcomePageIfNeeded();
        if (!current()) return;
      }
      if (current()) await rootData.completeInitialization(captured, refresh);
    })()
      .catch((error: unknown) => console.warn("Workspace initialization failed", error))
      .finally(() => {
        if (initializationOwner === captured) initializationOwner = null;
      });
  };
  createEffect(on([rootData.owner, firstLoginCheckDone, rootData.snapshotReady], initialize));

  // Update metadata in place so page edits retain loaded descendants and focus.
  async function refreshNodeMetadata(id: string) {
    const ws = wsApi();
    if (!ws || !findNodePath(nodes, id)) return;
    const identity = workspaceIdentity();
    try {
      const data = await ws.getNode(id);
      if (identity !== workspaceIdentity()) return;
      setNodesStore(
        produce((list) => {
          const path = findNodePath(list, id);
          const node = path?.[path.length - 1];
          if (node && node.modified <= data.modified) {
            // Cleared optional fields are omitted from JSON. Remove them while
            // retaining the loaded children and this store object's identity.
            for (const key of Object.keys(node)) {
              if (key !== "children" && !(key in data)) Reflect.deleteProperty(node, key);
            }
            Object.assign(node, data, { children: node.children });
          }
        }),
      );
      setBreadcrumbPath((path) => path.map((node) => (node.id === id && node.modified <= data.modified ? data : node)));
    } catch (err) {
      // A subsequent deletion event owns removal of a concurrently deleted node.
      if (err instanceof APIError && err.status === 404) return;
      if (identity === workspaceIdentity()) setLoadError(`${t("errors.failedToLoad")}: ${err}`);
    }
  }

  // React once per SSE event; request state must not retrigger the refresh.
  const { lastEvent } = useEventSource();
  createEffect(
    on(lastEvent, (evt) => {
      if (!evt) return;
      if (evt.type === EventNodeUpdated) {
        void refreshNodeMetadata(evt.node_id);
      } else if (evt.type === EventNodeCreated || evt.type === EventNodeDeleted || evt.type === EventNodeMoved) {
        loadNodes(true);
      }
    }),
  );

  // Move a node to a new parent (or root if newParentId === "0")
  async function moveNode(nodeId: string, newParentId: string): Promise<void> {
    const ws = wsApi();
    if (!ws) return;

    try {
      await ws.moveNode(nodeId, { new_parent_id: newParentId });

      let moveResult: ReturnType<typeof reconcileMovedNode>;
      batch(() => {
        setNodesStore(
          produce((list) => {
            moveResult = reconcileMovedNode(list as NodeResponse[], nodeId, newParentId);
          }),
        );

        if (!moveResult) return;
        const appliedMove = moveResult;
        const selectedId = selectedNodeId();
        const selectedData = selectedNodeData();
        const nextSelectedData = reconcileSelectedNodeData(selectedId, selectedData, nodeId, newParentId);
        setSelectedNodeData(nextSelectedData);
        setBreadcrumbPath((path) => {
          const nextPath = reconcileBreadcrumbPath(nodes, path, selectedId, appliedMove, newParentId);
          if (selectedId === nodeId && nextSelectedData) {
            return nextPath.map((node) => (node.id === nodeId ? nextSelectedData : node));
          }
          return nextPath;
        });
      });
      setSaveError(null);
    } catch (err) {
      setSaveError(`${t("errors.failedToMove")}: ${err}`);
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
    failedNodeId,
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
    updateNodeIcon,
    moveNode,
    clearErrors,
  };

  return <WorkspaceContext.Provider value={value}>{props.children}</WorkspaceContext.Provider>;
};

export function useWorkspace(): WorkspaceContextValue {
  const context = useContext(WorkspaceContext);
  if (!context) {
    throw new Error("useWorkspace must be used within a WorkspaceProvider");
  }
  return context;
}
