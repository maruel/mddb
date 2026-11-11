// Owns route-selected documents, subscription snapshots and guarded refreshes; commands distinguish failure from retirement.

import {
  createContext,
  createComputed,
  useContext,
  createSignal,
  createEffect,
  createMemo,
  batch,
  on,
  onCleanup,
  untrack,
  type ParentComponent,
  type Accessor,
} from "solid-js";
import { produce } from "solid-js/store";
import { useLocation, useNavigate } from "@solidjs/router";
import { useWorkspaceNavigation } from "./WorkspaceNavigationContext";
import { useAuth } from "./AuthContext";
import { useWorkspaceData } from "./WorkspaceDataContext";
import { useEventSource } from "./EventSourceContext";
import { useI18n } from "../i18n";
import {
  EventAspectMetadata,
  EventNodeCreated,
  EventNodeDeleted,
  EventNodeMoved,
  EventNodeUpdated,
  EventResync,
  type NodeResponse,
} from "@sdk/types.gen";
import { workspaceUrl } from "../utils/urls";
import { APIError } from "../useApi";
import { findNodePath, reconcileBreadcrumbPath, reconcileMovedNode, reconcileSelectedNodeData } from "./workspaceTree";

interface NodeLoadOptions {
  refresh: boolean;
  navigation?: boolean;
  // The editor owns draft admission; route and request lifetime are enforced here.
  canApply?: () => boolean;
}

interface NodeSnapshot {
  node: NodeResponse;
  // Only an accepted document read replaces the editor's publication.
  document: NodeResponse;
  // Ancestor failure settles a usable partial path; retirement leaves it pending.
  path: { kind: "pending" | "stale" | "settled"; nodes: NodeResponse[] };
}

interface NodeRead {
  id: string;
  controller: AbortController;
  promise: Promise<NodeResponse | undefined>;
  phase: "node" | "ancestry";
  options: NodeLoadOptions;
  roots: Promise<NodeResponse[]> | null;
  // Records metadata acknowledged during this read, not a second metadata copy.
  metadata: Set<string>;
}

type NodeSelection =
  | { kind: "empty"; snapshot: null }
  | { kind: "loading"; snapshot: NodeSnapshot | null; request: NodeRead }
  | { kind: "selected"; snapshot: NodeSnapshot }
  | { kind: "failed"; snapshot: NodeSnapshot | null; id: string };

interface WorkspaceContextValue {
  // Node tree
  nodes: NodeResponse[];

  // Selected node
  selectedNodeId: Accessor<string | null>;
  selectedNodeData: Accessor<NodeResponse | null>;
  selectedDocument: Accessor<NodeResponse | null>;
  updateSelectedNode: (node: NodeResponse) => void;

  // Breadcrumbs
  breadcrumbPath: Accessor<NodeResponse[]>;

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

  // Operations
  switchWorkspace: (wsId: string) => Promise<boolean>;
  createOrganization: (data: { name: string }) => Promise<void>;
  createWorkspace: (data: { name: string }) => Promise<string | null>;
  refreshRoots: () => Promise<void>;
  loadNode: (id: string, options: NodeLoadOptions) => Promise<NodeResponse | undefined>;
  fetchNodeChildren: (nodeId: string) => Promise<boolean>;
  removeNode: (nodeId: string) => void;
  updateNodeTitle: (nodeId: string, newTitle: string) => void;
  updateNodeIcon: (nodeId: string, newIcon: string) => void;
  moveNode: (nodeId: string, newParentId: string) => Promise<boolean>;

  // Clear errors
  clearErrors: () => void;
}

const WorkspaceContext = createContext<WorkspaceContextValue>();

export const WorkspaceProvider: ParentComponent = (props) => {
  const { t } = useI18n();
  const navigate = useNavigate();
  const location = useLocation();
  const navigation = useWorkspaceNavigation();
  const { user, identity: authIdentity, setUser, api, wsApi } = useAuth();

  const rootData = useWorkspaceData();
  const { nodes, setNodesStore, loadingNodes } = rootData;
  const metadataRequests = new Map<string, AbortController>();
  const retireMetadata = (id: string) => {
    metadataRequests.get(id)?.abort();
    metadataRequests.delete(id);
  };
  const retireAllMetadata = () => {
    for (const id of metadataRequests.keys()) retireMetadata(id);
  };
  const retireSubtreeMetadata = (nodeId: string) => {
    for (const id of metadataRequests.keys()) {
      if (findNodePath(nodes, id)?.some((node) => node.id === nodeId)) retireMetadata(id);
    }
  };
  let disposed = false;
  let streamEpoch = 0;
  let deferredNodeRefresh: { id: string; identity: string; options: NodeLoadOptions } | null = null;
  const [admittedNodeId, setAdmittedNodeId] = createSignal<string | null>(null);
  onCleanup(() => {
    disposed = true;
    deferredNodeRefresh = null;
    retireAllMetadata();
    retireNodeRead();
  });

  // Selected-node and editor ownership remain local to this route.
  const [selection, setSelection] = createSignal<NodeSelection>({ kind: "empty", snapshot: null });
  const selectedNodeData = createMemo(() => selection().snapshot?.node ?? null);
  const selectedDocument = createMemo(() => selection().snapshot?.document ?? null);
  const selectedNodeId = createMemo(() => selectedNodeData()?.id ?? null);
  const breadcrumbPath = createMemo(() => selection().snapshot?.path.nodes ?? []);
  const loadingNodeId = createMemo(() => {
    const state = selection();
    return state.kind === "loading" ? state.request.id : null;
  });
  const failedNodeId = createMemo(() => {
    const state = selection();
    return state.kind === "failed" ? state.id : null;
  });
  const settleSelection = (snapshot: NodeSnapshot | null): NodeSelection =>
    snapshot ? { kind: "selected", snapshot } : { kind: "empty", snapshot: null };
  const retireNodeRead = () => {
    const state = selection();
    if (state.kind !== "loading") return;
    setSelection(settleSelection(state.snapshot));
    state.request.controller.abort();
  };
  const updateSnapshot = (update: (snapshot: NodeSnapshot) => NodeSnapshot) => {
    setSelection((state) => (state.snapshot ? { ...state, snapshot: update(state.snapshot) } : state));
  };
  const applySelectedNode = (node: NodeResponse) => {
    updateSnapshot((snapshot) =>
      snapshot.node.id === node.id
        ? {
            ...snapshot,
            node,
            path: { ...snapshot.path, nodes: snapshot.path.nodes.map((item) => (item.id === node.id ? node : item)) },
          }
        : snapshot,
    );
  };
  const updateSelectedNode = (node: NodeResponse) => {
    const currentNode = selectedNodeData();
    if (currentNode?.id !== node.id) return;
    if ((currentNode.parent_id ?? "0") !== (node.parent_id ?? "0"))
      reconcileLineage(() => applySelectedNode(node), false);
    else applySelectedNode(node);
  };
  const setBreadcrumbPath = (update: (path: NodeResponse[]) => NodeResponse[]) => {
    const before = breadcrumbPath();
    const after = update(before);
    const publish = () => {
      const leaf = after.at(-1);
      const selected = selectedNodeData();
      if (leaf && selected && leaf.id === selected.id && (leaf.parent_id ?? "0") !== (selected.parent_id ?? "0"))
        applySelectedNode({ ...selected, parent_id: leaf.parent_id });
      updateSnapshot((snapshot) => ({
        ...snapshot,
        path: { ...snapshot.path, nodes: after.map((node) => (node.id === snapshot.node.id ? snapshot.node : node)) },
      }));
    };
    if (
      before.length !== after.length ||
      before.some(
        (node, index) => node.id !== after[index]?.id || (node.parent_id ?? "0") !== (after[index].parent_id ?? "0"),
      )
    )
      reconcileLineage(publish, false);
    else publish();
  };

  // All acknowledged structural writers pass here. An ancestry candidate loses
  // ownership before reconciliation; replacement work starts after publication.
  function reconcileLineage(publish: () => void, readLeaf: boolean) {
    const active = selection();
    const request = active.kind === "loading" ? active.request : null;
    retireNodeRead();
    publish();
    if (request?.phase === "node") {
      void loadNode(request.id, request.options).catch((error: unknown) => console.warn("Node loading failed", error));
      return;
    }
    const snapshot = selection().snapshot;
    if (!snapshot) return;
    const parentChanged = (active.snapshot?.node.parent_id ?? "0") !== (snapshot.node.parent_id ?? "0");
    if (!request && !parentChanged) return;
    const path = snapshot.path.nodes;
    const complete =
      path.at(-1)?.id === snapshot.node.id &&
      (path[0]?.parent_id ?? "0") === "0" &&
      path.every((node, index) => index === 0 || node.parent_id === path[index - 1]?.id);
    if (!readLeaf && complete) {
      updateSnapshot((currentSnapshot) => ({ ...currentSnapshot, path: { ...currentSnapshot.path, kind: "settled" } }));
      return;
    }
    updateSnapshot((currentSnapshot) => ({
      ...currentSnapshot,
      path: {
        kind: readLeaf ? "stale" : "pending",
        nodes: parentChanged ? [currentSnapshot.node] : currentSnapshot.path.nodes,
      },
    }));
    const selected = selectedNodeData();
    if (selected)
      void loadNode(selected.id, { refresh: false }).catch((error: unknown) =>
        console.warn("Node loading failed", error),
      );
  }
  const retainMetadata = (id: string) => {
    const active = selection();
    if (active.kind === "loading") active.request.metadata.add(id);
  };

  // Operation-specific loading states
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
      creatingNode(),
  );

  const workspaceIdentity = createMemo(() => `${authIdentity()}:${user()?.id ?? ""}:${user()?.workspace_id ?? ""}`);
  const captureWorkspace = () => {
    const initialIdentity = workspaceIdentity();
    return () => !disposed && initialIdentity === workspaceIdentity();
  };

  // A subscription boundary invalidates every part of an in-flight snapshot.
  async function readSubscribedSnapshot<T>(read: () => Promise<T>, current: () => boolean): Promise<T | undefined> {
    while (current()) {
      const epoch = streamEpoch;
      try {
        const result = await read();
        if (!current()) return undefined;
        if (epoch === streamEpoch) return result;
      } catch (error) {
        if (!current()) return undefined;
        if (epoch === streamEpoch) throw error;
        console.warn("Pre-subscription workspace read failed", error);
      }
    }
    return undefined;
  }

  createEffect(
    on([loadingNodeId, switchingWorkspace, admittedNodeId, selectedNodeId], () => {
      const refresh = deferredNodeRefresh;
      if (!refresh) return;
      if (disposed || refresh.identity !== workspaceIdentity() || refresh.id !== selectedNodeId()) {
        deferredNodeRefresh = null;
        return;
      }
      if (loadingNodeId() !== null || switchingWorkspace() || admittedNodeId() !== refresh.id) return;
      deferredNodeRefresh = null;
      void loadNode(refresh.id, refresh.options).catch((error: unknown) => console.warn("Node loading failed", error));
    }),
  );

  createComputed(
    on([rootData.owner, loadingNodes], ([owner, readingRoots], previous) => {
      if (readingRoots || (previous && owner !== previous[0])) retireAllMetadata();
      const active = selection();
      if (!readingRoots) return;
      if (active.kind === "loading") {
        const roots = rootData.prepareWorkspace();
        // Reads admitted under this exact preparation keep their candidate.
        // A later canonical preparation retires it, even during a refresh.
        if (active.request.roots === roots || (!rootData.ready() && active.request.roots === null)) {
          active.request.roots = roots;
          return;
        }
      }
      reconcileLineage(() => undefined, true);
    }),
  );

  createComputed(
    on(
      workspaceIdentity,
      () => {
        deferredNodeRefresh = null;
        setAdmittedNodeId(null);
        retireNodeRead();
        batch(() => {
          setSelection({ kind: "empty", snapshot: null });
          setSwitchingWorkspace(false);
          setCreatingNode(false);
          setLoadError(null);
        });
      },
      { defer: true },
    ),
  );
  createComputed(on(() => location.pathname, retireNodeRead, { defer: true }));

  // Clear route operation errors
  const clearErrors = () => {
    setLoadError(null);
    setSaveError(null);
  };

  // Helper to update node title in local state (sidebar and breadcrumbs)
  const updateNodeTitle = (nodeId: string, newTitle: string) => {
    retireMetadata(nodeId);
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
    const selected = selectedNodeData();
    if (selected?.id === nodeId) applySelectedNode({ ...selected, title: newTitle });
    setBreadcrumbPath((path) => path.map((node) => (node.id === nodeId ? { ...node, title: newTitle } : node)));
    retainMetadata(nodeId);
  };

  const updateNodeIcon = (nodeId: string, newIcon: string) => {
    retireMetadata(nodeId);
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
    const selected = selectedNodeData();
    if (selected?.id === nodeId) applySelectedNode({ ...selected, icon: newIcon });
    retainMetadata(nodeId);
  };

  // Remove a node from the store (used after deletion)
  const removeNode = (nodeId: string) => {
    const parent = findNodePath(nodes, nodeId)?.at(-2);
    if (parent?.children?.length === 1) retireMetadata(parent.id);
    retireSubtreeMetadata(nodeId);
    reconcileLineage(
      () =>
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
        ),
      true,
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

  // Creation acknowledges its ID separately from activation so callers can
  // retry a failed switch without creating another workspace.
  async function createWorkspace(data: { name: string }): Promise<string | null> {
    const u = user();
    if (!u?.organization_id || disposed) return null;
    const current = captureWorkspace();
    try {
      const ws = await api().org(u.organization_id).createWorkspace({ name: data.name });
      return current() ? ws.id : null;
    } catch (error) {
      if (!current()) return null;
      throw error;
    }
  }

  async function switchWorkspace(wsId: string) {
    const current = captureWorkspace();
    if (!(await navigation.beforeNavigate()) || !current()) return false;
    const client = api();
    try {
      setSwitchingWorkspace(true);
      const data = await client.switchWorkspace({ ws_id: wsId });
      if (!current()) return false;
      if (!data.user) {
        throw new Error("No user data returned");
      }
      setUser(data.user);
      // Navigate to the new workspace root to clear any stale node ID from the URL.
      const u = user();
      if (u?.workspace_id) {
        navigate(workspaceUrl(u.workspace_id, u.workspace_name));
      }
      return true;
    } catch (error) {
      if (!current()) return false;
      throw error;
    } finally {
      if (current()) setSwitchingWorkspace(false);
    }
  }

  const refreshRoots = async () => {
    await rootData.refreshRoots();
  };

  function loadNode(id: string, options: NodeLoadOptions): Promise<NodeResponse | undefined> {
    const ws = wsApi();
    if (!ws || disposed) return Promise.resolve(undefined);
    if (options.navigation) setAdmittedNodeId(id);
    if (options.refresh && ((loadingNodeId() !== null && loadingNodeId() !== id) || switchingWorkspace())) {
      if (selectedNodeId() === id) deferredNodeRefresh = { id, identity: workspaceIdentity(), options };
      return Promise.resolve(undefined);
    }
    const previous = selection();
    const cached = previous.snapshot?.node.id === id ? previous.snapshot : null;
    const failedTarget = previous.kind === "failed" && previous.id === id;
    if (!options.refresh) {
      if (previous.kind === "loading" && previous.request.id === id) return previous.request.promise;
      if (!failedTarget && cached?.path.kind === "settled") {
        retireNodeRead();
        batch(() => {
          setSelection(settleSelection(cached));
          setLoadError(null);
        });
        return Promise.resolve(cached.node);
      }
    }
    const refreshIntent = options.refresh ? { id, identity: workspaceIdentity(), options } : null;
    if (refreshIntent) deferredNodeRefresh = refreshIntent;
    retireNodeRead();
    const resume = !options.refresh && !failedTarget ? cached : null;
    const identity = workspaceIdentity();
    const pathname = location.pathname;
    const controller = new AbortController();
    const request: NodeRead = {
      id,
      controller,
      promise: Promise.resolve().then(() => read()),
      phase: resume ? "ancestry" : "node",
      options,
      roots: loadingNodes() || rootData.preparationError() !== null ? rootData.prepareWorkspace() : null,
      metadata: new Set(),
    };
    const current = () => {
      const state = selection();
      return (
        !disposed &&
        !controller.signal.aborted &&
        state.kind === "loading" &&
        state.request === request &&
        identity === workspaceIdentity() &&
        pathname === location.pathname
      );
    };
    setSelection({ kind: "loading", snapshot: previous.snapshot, request });
    const read = async (): Promise<NodeResponse | undefined> => {
      try {
        if (!current()) return undefined;
        // A cancelled ancestor read leaves its selected document usable. Resume
        // only ancestry so returning to it cannot replace an admitted draft.
        const nodeData = resume
          ? resume.node
          : await readSubscribedSnapshot(() => ws.getNode(id, controller.signal), current);
        if (!nodeData || !current()) return undefined;
        const nodeEpoch = streamEpoch;
        const admitted = !options.canApply || options.canApply();
        if (!admitted && selectedNodeId() !== id) return undefined;
        // Same-parent refresh keeps known membership until the complete path is
        // replaced. Removing and restoring ancestors would reopen collapsed UI.
        const retainedPath = cached && cached.node.parent_id === nodeData.parent_id ? cached.path.nodes : [nodeData];
        batch(() => {
          if (!resume && admitted) {
            setSelection({
              kind: "loading",
              snapshot: {
                node: nodeData,
                document: nodeData,
                path: { kind: "pending", nodes: retainedPath.map((item) => (item.id === id ? nodeData : item)) },
              },
              request,
            });
          }
          setLoadError(null);
        });
        request.phase = "ancestry";
        // Document admission is independent of canonical roots. Only ancestry
        // waits for an older root snapshot; preparation failure owns its retry
        // and cannot discard the document or prevent independent parent reads.
        if (loadingNodes()) {
          try {
            await rootData.prepareWorkspace();
          } catch {
            // Canonical preparation owns this failure and its explicit retry.
          }
          if (!current()) return undefined;
        }
        // Retry the whole ancestry across a subscription boundary, including
        // the leaf parent link. A guarded body still admits canonical hierarchy.
        let initial: { node: NodeResponse; epoch: number } | undefined =
          resume?.path.kind === "stale" ? undefined : { node: nodeData, epoch: nodeEpoch };
        const client = ws;
        async function readPath() {
          const epoch = streamEpoch;
          const leaf = initial?.epoch === epoch ? initial.node : await client.getNode(id, controller.signal);
          initial = undefined;
          const result: NodeResponse[] = [leaf];
          let parentId = leaf.parent_id;
          while (parentId && parentId !== "0") {
            if (!current() || epoch !== streamEpoch) break;
            try {
              const parent = await client.getNode(parentId, controller.signal);
              result.unshift(parent);
              parentId = parent.parent_id;
            } catch (error) {
              // Initial navigation can show a partial path. Refresh failure
              // retains previously published complete membership for retry.
              if (options.refresh || cached?.path.kind === "settled" || epoch !== streamEpoch) throw error;
              break;
            }
          }
          return result;
        }
        const path = await readSubscribedSnapshot(readPath, current);
        if (!path || !current()) return undefined;
        const lineage = path.at(-1);
        if (lineage && !admitted) {
          const selected = selectedNodeData();
          if (selected)
            applySelectedNode({
              ...selected,
              parent_id: lineage.parent_id,
              title: request.metadata.has(id) ? selected.title : lineage.title,
              icon: request.metadata.has(id) ? selected.icon : lineage.icon,
            });
        } else if (lineage && (lineage.parent_id ?? "0") !== (selectedNodeData()?.parent_id ?? "0")) {
          const selected = selectedNodeData();
          if (selected) applySelectedNode({ ...selected, parent_id: lineage.parent_id });
        }
        // A lazy snapshot can introduce an ancestor that was absent when this
        // read began. Its acknowledged parent link supersedes the captured one.
        if (
          path.some((item) => {
            const known = request.metadata.has(item.id) ? findNodePath(nodes, item.id)?.at(-1) : null;
            return known && (known.parent_id ?? "0") !== (item.parent_id ?? "0");
          })
        ) {
          reconcileLineage(() => undefined, true);
          return undefined;
        }
        updateSnapshot((snapshot) => ({
          ...snapshot,
          node: snapshot.node,
          path: {
            kind: "settled",
            nodes: path.map((item) => {
              if (item.id === id) return snapshot.node;
              const metadata = request.metadata.has(item.id) ? findNodePath(nodes, item.id)?.at(-1) : null;
              return metadata ? { ...item, title: metadata.title, icon: metadata.icon } : item;
            }),
          },
        }));
        if (deferredNodeRefresh === refreshIntent) deferredNodeRefresh = null;
        setSelection(settleSelection(selection().snapshot));
        return admitted ? (selectedNodeData() ?? undefined) : undefined;
      } catch (err) {
        if (current()) {
          if (deferredNodeRefresh === refreshIntent) deferredNodeRefresh = null;
          setSelection({ kind: "failed", snapshot: selection().snapshot, id });
          setLoadError(`${t("errors.failedToLoad")}: ${String(err)}`);
          throw err;
        }
        return undefined;
      } finally {
        if (current()) setSelection(settleSelection(selection().snapshot));
      }
    };
    return request.promise;
  }

  async function fetchNodeChildren(nodeId: string): Promise<boolean> {
    const ws = wsApi();
    if (!ws || disposed) return false;
    const current = captureWorkspace();

    try {
      const data = await readSubscribedSnapshot(() => ws.listNodeChildren(nodeId), current);
      if (!data) return false;

      const children = data.nodes?.filter((node): node is NodeResponse => Boolean(node)) || [];

      // This snapshot replaces descendant metadata as well as the parent's
      // child-presence flag. Older detail reads no longer own either write.
      retireSubtreeMetadata(nodeId);
      const previousChildren = findNodePath(nodes, nodeId)?.at(-1)?.children ?? [];
      const hierarchy =
        previousChildren.some((child) => !children.some((item) => item.id === child.id)) ||
        children.some((child) => {
          const known = breadcrumbPath().find((item) => item.id === child.id) ?? findNodePath(nodes, child.id)?.at(-1);
          return known && (known.parent_id ?? "0") !== (child.parent_id ?? "0");
        });
      const publish = () => {
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
        for (const child of children) retainMetadata(child.id);
      };
      if (hierarchy) reconcileLineage(publish, true);
      else publish();
      return true;
    } catch (err) {
      if (!current()) return false;
      throw err;
    }
  }

  // Update metadata in place so page edits retain loaded descendants and focus.
  async function refreshNodeMetadata(id: string, authoritative: boolean) {
    const ws = wsApi();
    if (!ws || !findNodePath(nodes, id)) return;
    retireMetadata(id);
    const controller = new AbortController();
    metadataRequests.set(id, controller);
    const owner = rootData.owner();
    const current = () =>
      !disposed &&
      !controller.signal.aborted &&
      metadataRequests.get(id) === controller &&
      rootData.owner() === owner &&
      findNodePath(nodes, id) !== null;
    try {
      // A root snapshot that began earlier must publish before this detail read.
      if (loadingNodes()) {
        try {
          await rootData.prepareWorkspace();
        } catch {
          // The canonical owner publishes preparation failure and owns retry.
          // No detail request started, so there is no route error to publish.
          return;
        }
      }
      if (!current()) return;
      const data = await readSubscribedSnapshot(() => ws.getNode(id, controller.signal), current);
      if (!data) return;
      const known = findNodePath(nodes, id)?.at(-1);
      const hierarchy =
        known &&
        (authoritative || known.modified <= data.modified) &&
        (known.parent_id ?? "0") !== (data.parent_id ?? "0");
      const publish = () => {
        setNodesStore(
          produce((list) => {
            const path = findNodePath(list, id);
            const node = path?.[path.length - 1];
            if (node && (authoritative || node.modified <= data.modified)) {
              // Cleared optional fields are omitted from JSON. Remove them while
              // retaining the loaded children and this store object's identity.
              for (const key of Object.keys(node)) {
                if (key !== "children" && !(key in data)) Reflect.deleteProperty(node, key);
              }
              Object.assign(node, data, { children: node.children });
            }
          }),
        );
        setBreadcrumbPath((path) =>
          path.map((node) => (node.id === id && (authoritative || node.modified <= data.modified) ? data : node)),
        );
        retainMetadata(id);
      };
      if (hierarchy) reconcileLineage(publish, false);
      else publish();
    } catch (err) {
      // A subsequent deletion event owns removal of a concurrently deleted node.
      if (err instanceof APIError && err.status === 404) return;
      if (current()) setLoadError(`${t("errors.failedToLoad")}: ${String(err)}`);
    } finally {
      if (metadataRequests.get(id) === controller) metadataRequests.delete(id);
    }
  }

  // React once per SSE event; request state must not retrigger the refresh.
  const { lastEvent } = useEventSource();
  createEffect(
    on(lastEvent, (evt) => {
      if (!evt) return;
      if (evt.type === EventResync) streamEpoch++;
      if (evt.type === EventNodeUpdated) {
        void refreshNodeMetadata(evt.node_id, evt.aspects?.includes(EventAspectMetadata) ?? false);
      } else if (
        evt.type === EventResync ||
        evt.type === EventNodeCreated ||
        evt.type === EventNodeDeleted ||
        evt.type === EventNodeMoved
      ) {
        if (evt.type === EventNodeMoved && evt.parent_id !== undefined) applyMove(evt.node_id, evt.parent_id);
        batch(() => {
          const roots = rootData.refreshRoots();
          if (evt.type === EventResync) {
            const active = selection();
            // Bind only this subscription's root transition. The snapshot
            // reader retries the candidate across the stream boundary itself.
            if (active.kind === "loading") active.request.roots = roots;
            const id = selectedNodeId();
            if (id && loadingNodeId() === null) {
              void loadNode(id, { refresh: true, canApply: () => false }).catch((error: unknown) =>
                console.warn("Breadcrumb refresh failed", error),
              );
            }
          }
          // Canonical preparation owns the visible error and its retry action.
          void roots.catch((error: unknown) => {
            if (error instanceof Error && error.name === "AbortError") return;
            console.warn("Workspace refresh failed", error);
          });
        });
      }
    }),
  );

  // Move a node to a new parent (or root if newParentId === "0")
  async function moveNode(nodeId: string, newParentId: string): Promise<boolean> {
    const ws = wsApi();
    if (!ws || disposed) return false;
    const current = captureWorkspace();

    try {
      await ws.moveNode(nodeId, { new_parent_id: newParentId });
      if (!current()) return false;
      applyMove(nodeId, newParentId);
      return true;
    } catch (err) {
      if (!current()) return false;
      throw err;
    }
  }
  function applyMove(nodeId: string, newParentId: string) {
    reconcileLineage(
      () =>
        untrack(() => {
          retireSubtreeMetadata(nodeId);
          const sourcePath = findNodePath(nodes, nodeId);
          if (sourcePath) {
            const sourceParent = sourcePath.at(-2);
            if (sourceParent?.children?.length === 1) retireMetadata(sourceParent.id);
            retireMetadata(newParentId);
          }
          let moveResult: ReturnType<typeof reconcileMovedNode>;
          batch(() => {
            setNodesStore(
              produce((list) => {
                moveResult = reconcileMovedNode(list, nodeId, newParentId);
              }),
            );

            if (!moveResult) {
              const selected = selectedNodeData();
              if (selected?.id === nodeId) {
                applySelectedNode({ ...selected, parent_id: newParentId });
                updateSnapshot((snapshot) => ({ ...snapshot, path: { kind: "pending", nodes: [snapshot.node] } }));
              }
              return;
            }
            const appliedMove = moveResult;
            const selectedId = selectedNodeId();
            const selectedData = selectedNodeData();
            const nextSelectedData = reconcileSelectedNodeData(selectedId, selectedData, nodeId, newParentId);
            if (nextSelectedData) applySelectedNode(nextSelectedData);
            setBreadcrumbPath((path) => {
              const nextPath = reconcileBreadcrumbPath(nodes, path, selectedId, appliedMove, newParentId);
              if (selectedId === nodeId && nextSelectedData) {
                return nextPath.map((node) => (node.id === nodeId ? nextSelectedData : node));
              }
              return nextPath;
            });
          });
        }),
      false,
    );
  }

  const value: WorkspaceContextValue = {
    nodes,
    selectedNodeId,
    selectedNodeData,
    selectedDocument,
    updateSelectedNode,
    breadcrumbPath,
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
    switchWorkspace,
    createOrganization,
    createWorkspace,
    refreshRoots,
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
