// Validates voice focus before navigation and observes completion in the current route-owned view.

import {
  createContext,
  createEffect,
  createMemo,
  createRoot,
  createSignal,
  on,
  onCleanup,
  untrack,
  useContext,
  type Accessor,
  type ParentComponent,
} from "solid-js";
import { useLocation, useNavigate } from "@solidjs/router";
import type { NodeResponse } from "@sdk/types.gen";
import type { FrontendVoiceToolContext } from "@maruel/gomode/web/FrontendVoiceTools";
import { useAuth } from "./AuthContext";
import { nodeUrl, parseNodeUrl } from "../utils/urls";

export interface WorkspaceNavigationView {
  selectedNodeId: Accessor<string | null>;
  selectedNodeData: Accessor<NodeResponse | null>;
  failedNodeId: Accessor<string | null>;
  beforeNavigate(): void;
}
interface WorkspaceNavigationContextValue {
  registerView(view: WorkspaceNavigationView): void;
  focusNode(id: string, context: FrontendVoiceToolContext): Promise<void>;
}
const WorkspaceNavigationContext = createContext<WorkspaceNavigationContextValue>();

export const WorkspaceNavigationProvider: ParentComponent = (props) => {
  const auth = useAuth();
  const location = useLocation();
  const navigate = useNavigate();
  const owner = createMemo(
    () =>
      `${auth.identity()}:${auth.user()?.id ?? ""}:${auth.user()?.session_id ?? ""}:${auth.user()?.workspace_id ?? ""}`,
  );
  const [view, setView] = createSignal<WorkspaceNavigationView | null>(null);
  let pending: { controller: AbortController; target: string; wsId: string; navigating: boolean } | null = null;
  let disposed = false;
  const matches = (wsId: string, id: string) => {
    const parsed = parseNodeUrl(location.pathname);
    return parsed?.wsId === wsId && parsed.nodeId === id;
  };
  createEffect(on(owner, () => pending?.controller.abort(), { defer: true }));
  createEffect(
    on(
      () => `${location.pathname}${location.search}${location.hash}`,
      () => {
        const current = pending;
        if (current !== null && (!current.navigating || !matches(current.wsId, current.target)))
          current.controller.abort();
      },
      { defer: true },
    ),
  );
  onCleanup(() => {
    disposed = true;
    pending?.controller.abort();
  });

  const value: WorkspaceNavigationContextValue = {
    registerView(current) {
      setView(current);
      onCleanup(() => {
        if (untrack(view) === current) setView(null);
      });
    },
    async focusNode(id, context) {
      pending?.controller.abort();
      const user = auth.user();
      const client = auth.wsApi();
      if (!user?.workspace_id || !client || disposed) throw new Error("Workspace unavailable.");
      const wsId = user.workspace_id;
      const captured = owner();
      const operation = { controller: new AbortController(), target: id, wsId, navigating: false };
      pending = operation;
      const signal = AbortSignal.any([context.signal, operation.controller.signal]);
      const current = () =>
        !disposed && !signal.aborted && context.isCurrent() && owner() === captured && pending === operation;
      try {
        let node: NodeResponse;
        try {
          node = await abortable(client.getNode(id), signal);
        } catch (error) {
          if (!current()) throw new Error("Focus canceled.", { cause: error });
          console.warn("Voice node lookup failed", error);
          throw new Error("Node unavailable.", { cause: error });
        }
        if (!current()) throw new Error("Focus canceled.");
        if (node.id !== id) throw new Error("Node unavailable.");
        const active = view();
        if (matches(user.workspace_id, id) && active?.selectedNodeId() === id && active.selectedNodeData()?.id === id)
          return;
        active?.beforeNavigate();
        if (!current()) throw new Error("Focus canceled.");
        operation.navigating = true;
        navigate(nodeUrl(user.workspace_id, user.workspace_name, node.id, node.title));
        await new Promise<void>((resolve, reject) => {
          createRoot((dispose) => {
            const stop = (error: Error | null) => {
              signal.removeEventListener("abort", cancel);
              dispose();
              if (error === null) resolve();
              else reject(error);
            };
            const cancel = () => stop(new Error("Focus canceled."));
            signal.addEventListener("abort", cancel, { once: true });
            createEffect(() => {
              if (!current()) {
                cancel();
                return;
              }
              if (!matches(wsId, id)) return;
              const mounted = view();
              if (mounted?.selectedNodeId() === id && mounted.selectedNodeData()?.id === id) stop(null);
              else if (mounted?.failedNodeId() === id) stop(new Error("Node unavailable."));
            });
          });
        });
        if (!current()) throw new Error("Focus canceled.");
      } finally {
        if (pending === operation) pending = null;
      }
    },
  };
  return <WorkspaceNavigationContext.Provider value={value}>{props.children}</WorkspaceNavigationContext.Provider>;
};

function abortable<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
  return new Promise((resolve, reject) => {
    const cancel = () => reject(new Error("Focus canceled."));
    if (signal.aborted) {
      cancel();
      return;
    }
    signal.addEventListener("abort", cancel, { once: true });
    void promise.then(resolve, reject).finally(() => signal.removeEventListener("abort", cancel));
  });
}

export function useWorkspaceNavigation(): WorkspaceNavigationContextValue {
  const context = useContext(WorkspaceNavigationContext);
  if (!context) throw new Error("useWorkspaceNavigation must be used within WorkspaceNavigationProvider");
  return context;
}
