// Owns canonical root-node snapshots and initial preparation across authenticated routes.

import {
  batch,
  createContext,
  createEffect,
  createMemo,
  createSignal,
  on,
  onCleanup,
  useContext,
  type Accessor,
  type ParentComponent,
} from "solid-js";
import { createStore, reconcile, type SetStoreFunction } from "solid-js/store";
import { useLocation } from "@solidjs/router";
import type { NodeResponse } from "@sdk/types.gen";
import { useAuth } from "./AuthContext";
import { useI18n } from "../i18n";

interface WorkspaceDataContextValue {
  owner: Accessor<string | null>;
  nodes: NodeResponse[];
  setNodesStore: SetStoreFunction<NodeResponse[]>;
  loadingNodes: Accessor<boolean>;
  snapshotReady: Accessor<boolean>;
  ready: Accessor<boolean>;
  loadError: Accessor<string | null>;
  setLoadError: (error: string | null) => void;
  loadNodes: (force: boolean) => Promise<NodeResponse[]>;
  completeInitialization: (owner: string, refresh: boolean) => Promise<void>;
}
const WorkspaceDataContext = createContext<WorkspaceDataContextValue>();

export const WorkspaceDataProvider: ParentComponent = (props) => {
  const auth = useAuth();
  const location = useLocation();
  const { t } = useI18n();
  const owner = createMemo(() => {
    const user = auth.user();
    return user?.workspace_id ? `${auth.identity()}:${user.id}:${user.session_id ?? ""}:${user.workspace_id}` : null;
  });
  const [nodes, setNodesStore] = createStore<NodeResponse[]>([]);
  const [loadedOwner, setLoadedOwner] = createSignal<string | null>(null);
  const [initializationPending, setInitializationPending] = createSignal(false);
  const [loadingNodes, setLoadingNodes] = createSignal(false);
  const [loadError, setLoadError] = createSignal<string | null>(null);
  const snapshotReady = () => owner() !== null && loadedOwner() === owner();
  const ready = () => snapshotReady() && !initializationPending();
  let pending: Promise<NodeResponse[]> | null = null;
  let initializationReadOwner: string | null = null;
  let generation = 0;
  let disposed = false;
  const workspaceEntry = (path: string) =>
    path === "/" || path === "/login" || path === "/onboarding" || path.startsWith("/w/");

  function loadNodes(force: boolean): Promise<NodeResponse[]> {
    const captured = owner();
    const client = auth.wsApi();
    if (captured === null || client === null || disposed) return Promise.resolve([]);
    if (pending !== null) return pending;
    if (!force && loadedOwner() === captured) return Promise.resolve([...nodes]);
    const request = generation;
    setLoadingNodes(true);
    const task = (async () => {
      try {
        const response = await client.listNodeChildren("0");
        if (disposed || generation !== request || owner() !== captured) return [];
        batch(() => {
          setNodesStore(reconcile(response.nodes));
          setLoadedOwner(captured);
          setLoadError(null);
          if (initializationReadOwner === captured) {
            initializationReadOwner = null;
            setInitializationPending(false);
          }
        });
        return response.nodes;
      } catch (error) {
        if (!disposed && generation === request && owner() === captured)
          setLoadError(`${t("errors.failedToLoad")}: ${error}`);
        throw error;
      } finally {
        if (!disposed && generation === request && owner() === captured) {
          pending = null;
          setLoadingNodes(false);
        }
      }
    })();
    pending = task;
    return task;
  }
  const readInitial = () => {
    void loadNodes(false).catch((error: unknown) => console.warn("Initial workspace read failed", error));
  };
  createEffect(
    on(owner, (current) => {
      generation++;
      pending = null;
      initializationReadOwner = null;
      batch(() => {
        setNodesStore(reconcile([]));
        setLoadedOwner(null);
        setLoadError(null);
        setLoadingNodes(false);
        setInitializationPending(current !== null && workspaceEntry(location.pathname));
      });
      if (current !== null && !initializationPending()) readInitial();
    }),
  );
  createEffect(
    on(
      () => location.pathname,
      (path) => {
        if (initializationPending() && !workspaceEntry(path)) {
          setInitializationPending(false);
          readInitial();
        }
      },
    ),
  );
  onCleanup(() => {
    disposed = true;
    generation++;
  });
  const value: WorkspaceDataContextValue = {
    owner,
    nodes,
    setNodesStore,
    loadingNodes,
    snapshotReady,
    ready,
    loadError,
    setLoadError,
    loadNodes,
    async completeInitialization(captured, refresh) {
      if (disposed || captured !== owner()) return;
      const request = generation;
      if (!refresh) {
        if (snapshotReady()) setInitializationPending(false);
        return;
      }
      // Reads already underway may predate the route's initialization writes.
      // Only the following fresh read, or its successful retry, can settle them.
      if (pending !== null) {
        try {
          await pending;
        } catch (error) {
          console.warn("Prior workspace read failed", error);
        }
      }
      if (disposed || generation !== request || captured !== owner()) return;
      initializationReadOwner = captured;
      await loadNodes(true);
    },
  };
  return <WorkspaceDataContext.Provider value={value}>{props.children}</WorkspaceDataContext.Provider>;
};

export function useWorkspaceData(): WorkspaceDataContextValue {
  const context = useContext(WorkspaceDataContext);
  if (!context) throw new Error("useWorkspaceData must be used within WorkspaceDataProvider");
  return context;
}
