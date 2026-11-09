// Owns workspace preparation, resumable startup requests, and canonical root snapshots across routes.

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
import { WSRoleAdmin, WSRoleEditor, type NodeResponse, type UserResponse } from "@sdk/types.gen";
import { useAuth } from "./AuthContext";
import { useI18n } from "../i18n";
import type { Api } from "../useApi";
import { provisionFirstWorkspace } from "./firstLogin";

type PreparationMode = "read-only" | "workspace" | "refresh";
type PreparationStage = "roots" | "welcome" | "final-read";
interface OperationOwner {
  scope: string;
  controller: AbortController;
  completion: ReturnType<typeof Promise.withResolvers<NodeResponse[]>>;
}
interface WorkspacePreparation extends OperationOwner {
  kind: "workspace";
  user: UserResponse;
  client: ReturnType<Api["ws"]>;
  mode: PreparationMode;
  stage: PreparationStage;
}
type PreparationOperation =
  WorkspacePreparation | (OperationOwner & { kind: "provision"; user: UserResponse; api: Api });
type PreparationState =
  | { kind: "idle" | "needs-workspace" }
  | { kind: "running"; operation: PreparationOperation }
  | { kind: "failed"; operation: PreparationOperation; error: string };

interface WorkspaceDataContextValue {
  owner: Accessor<string | null>;
  nodes: NodeResponse[];
  setNodesStore: SetStoreFunction<NodeResponse[]>;
  loadingNodes: Accessor<boolean>;
  ready: Accessor<boolean>;
  needsWorkspace: Accessor<boolean>;
  preparationError: Accessor<string | null>;
  loadNodes: (force: boolean) => Promise<NodeResponse[]>;
}
const WorkspaceDataContext = createContext<WorkspaceDataContextValue>();

export const WorkspaceDataProvider: ParentComponent = (props) => {
  const auth = useAuth();
  const location = useLocation();
  const { t, ready: i18nReady } = useI18n();
  const scope = createMemo(() => {
    const user = auth.user();
    return user ? `${auth.identity()}:${user.id}:${user.session_id ?? ""}:${user.workspace_id ?? ""}` : null;
  });
  const owner = () => (auth.user()?.workspace_id ? scope() : null);
  const workspaceEntry = createMemo(() => {
    const path = location.pathname;
    return path === "/" || path === "/login" || path === "/onboarding" || path.startsWith("/w/");
  });
  const [nodes, setNodesStore] = createStore<NodeResponse[]>([]);
  const [snapshot, setSnapshot] = createSignal<{ owner: string; policy: "read-only" | "workspace" } | null>(null);
  const [state, setState] = createSignal<PreparationState>({ kind: "idle" });
  const ready = createMemo(() => owner() !== null && snapshot()?.owner === owner());
  let disposed = false;

  const current = (operation: PreparationOperation) => {
    const active = state();
    return (
      !disposed &&
      !operation.controller.signal.aborted &&
      scope() === operation.scope &&
      active.kind === "running" &&
      active.operation === operation
    );
  };
  const retire = () => {
    const active = state();
    if (active.kind === "running" || active.kind === "failed") active.operation.controller.abort();
    setState({ kind: "idle" });
  };
  const publish = (operation: WorkspacePreparation, roots: NodeResponse[]) => {
    if (!current(operation)) return [];
    const policy = operation.mode === "refresh" ? (snapshot()?.policy ?? "read-only") : operation.mode;
    batch(() => {
      setNodesStore(reconcile(roots));
      setSnapshot({ owner: operation.scope, policy });
      setState({ kind: "idle" });
    });
    return roots;
  };
  const run = async (operation: PreparationOperation): Promise<NodeResponse[]> => {
    const { user, controller } = operation;
    const signal = controller.signal;
    if (operation.kind === "provision") {
      const firstName = user.name.split(" ")[0] || user.name;
      const result = await provisionFirstWorkspace(
        user,
        operation.api,
        {
          organization: t(firstName ? "onboarding.defaultOrgName" : "onboarding.defaultOrgNameFallback", {
            name: firstName,
          }),
          workspace: t(firstName ? "onboarding.defaultWorkspaceName" : "onboarding.defaultWorkspaceNameFallback", {
            name: firstName,
          }),
        },
        signal,
      );
      if (!current(operation)) return [];
      if (result.workspace_id) {
        // The new workspace starts its own preparation. This operation must not
        // publish a result or clear its successor after changing the auth scope.
        auth.setUser(result);
      } else {
        setState({ kind: "needs-workspace" });
      }
      return [];
    }
    const client = operation.client;
    if (operation.stage === "roots") {
      const response = await client.listNodeChildren("0", signal);
      if (!current(operation)) return [];
      const roots = response.nodes ?? [];
      if (
        operation.mode !== "workspace" ||
        roots.length > 0 ||
        (user.workspace_role !== WSRoleAdmin && user.workspace_role !== WSRoleEditor)
      ) {
        return publish(operation, roots);
      }
      operation.stage = "welcome";
    }
    if (operation.stage === "welcome") {
      // Permission can change without replacing the session or workspace owner.
      const role = auth.user()?.workspace_role;
      if (workspaceEntry() && (role === WSRoleAdmin || role === WSRoleEditor)) {
        await client.createPage(
          "0",
          {
            title: t("welcome.welcomePageTitle"),
            content: t("welcome.welcomePageContent"),
            ensure_root_if_empty: true,
          },
          signal,
        );
        if (!current(operation)) return [];
      }
      operation.stage = "final-read";
    }
    const canonical = await client.listNodeChildren("0", signal);
    return publish(operation, canonical.nodes ?? []);
  };
  const start = (mode: PreparationMode, stage: PreparationStage): Promise<NodeResponse[]> => {
    const user = auth.user();
    const captured = scope();
    if (!user || captured === null || disposed) return Promise.resolve([]);
    const lifetime: OperationOwner = {
      scope: captured,
      controller: new AbortController(),
      completion: Promise.withResolvers<NodeResponse[]>(),
    };
    const operation: PreparationOperation = user.workspace_id
      ? { ...lifetime, kind: "workspace", user, client: auth.api().ws(user.workspace_id), mode, stage }
      : { ...lifetime, kind: "provision", user, api: auth.api() };
    setState({ kind: "running", operation });
    void (async () => {
      try {
        operation.completion.resolve(await run(operation));
      } catch (error) {
        if (current(operation)) {
          setState({ kind: "failed", operation, error: `${t("errors.failedToLoad")}: ${String(error)}` });
        }
        operation.completion.reject(error);
      }
    })();
    return operation.completion.promise;
  };
  const begin = (force: boolean): Promise<NodeResponse[]> => {
    const active = state();
    if (active.kind === "running") return active.operation.completion.promise;
    if (!i18nReady() && !ready()) return Promise.resolve([]);
    if (!force && ready()) return Promise.resolve([...nodes]);
    if (active.kind === "failed") {
      const operation = active.operation;
      retire();
      return operation.kind === "provision" ? start("workspace", "roots") : start(operation.mode, operation.stage);
    }
    if (!auth.user()?.workspace_id && !workspaceEntry()) return Promise.resolve([]);
    return start(ready() ? "refresh" : workspaceEntry() ? "workspace" : "read-only", "roots");
  };
  const observe = (preparation: Promise<NodeResponse[]>) => {
    void preparation.catch((error: unknown) => {
      // Retiring an owner deliberately aborts its pending request.
      if (error instanceof Error && error.name === "AbortError") return;
      console.warn("Workspace preparation failed", error);
    });
  };
  const prepare = () => {
    const user = auth.user();
    const captured = scope();
    if (captured === null || !user || !i18nReady()) return;
    if (!user.workspace_id && !workspaceEntry()) {
      setState({ kind: "needs-workspace" });
      return;
    }
    if (ready()) {
      if (!workspaceEntry() || snapshot()?.policy === "workspace") {
        return;
      }
      if (nodes.length > 0 || (user.workspace_role !== WSRoleAdmin && user.workspace_role !== WSRoleEditor)) {
        setSnapshot({ owner: captured, policy: "workspace" });
        return;
      }
      observe(start("workspace", "welcome"));
      return;
    }
    observe(begin(false));
  };
  createEffect(
    on([scope, workspaceEntry, i18nReady], ([captured, entry], previous) => {
      const replaced = !previous || previous[0] !== captured;
      const changedPolicy = previous && previous[1] !== entry;
      if (replaced || changedPolicy) retire();
      if (replaced) {
        batch(() => {
          setNodesStore(reconcile([]));
          setSnapshot(null);
        });
      }
      // Translation readiness can start idle work, but cannot restart a failed
      // stage or replace a request that already owns valid translated input.
      if (state().kind === "running" || state().kind === "failed") return;
      prepare();
    }),
  );
  onCleanup(() => {
    disposed = true;
    retire();
  });
  const value: WorkspaceDataContextValue = {
    owner,
    nodes,
    setNodesStore,
    loadingNodes: () => state().kind === "running",
    ready,
    needsWorkspace: () => state().kind === "needs-workspace",
    preparationError: () => {
      const active = state();
      return active.kind === "failed" ? active.error : null;
    },
    loadNodes: begin,
  };
  return <WorkspaceDataContext.Provider value={value}>{props.children}</WorkspaceDataContext.Provider>;
};

export function useWorkspaceData(): WorkspaceDataContextValue {
  const context = useContext(WorkspaceDataContext);
  if (!context) throw new Error("useWorkspaceData must be used within WorkspaceDataProvider");
  return context;
}
