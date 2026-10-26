// SSE client context for receiving real-time workspace events.

import {
  createContext,
  useContext,
  createSignal,
  createEffect,
  onCleanup,
  type ParentComponent,
  type Accessor,
} from "solid-js";
import { useAuth } from "./AuthContext";
import type { WorkspaceEvent } from "@sdk/types.gen";
import { isLocalWriteRequest } from "../useApi";

interface EventSourceContextValue {
  lastEvent: Accessor<WorkspaceEvent | null>;
  connected: Accessor<boolean>;
}

const EventSourceCtx = createContext<EventSourceContextValue>();

// Module-level: persists across reconnects and workspace switches so we can
// detect when a new binary is deployed (revision changes on reconnect).
let knownRevision: string | null = null;

export const EventSourceProvider: ParentComponent = (props) => {
  const { user, identity } = useAuth();
  const [lastEvent, setLastEvent] = createSignal<WorkspaceEvent | null>(null);
  const [connected, setConnected] = createSignal(false);

  createEffect(() => {
    const wsId = user()?.workspace_id;
    identity();
    const userId = user()?.id;
    if (!wsId || typeof EventSource === "undefined") {
      setConnected(false);
      return;
    }

    const sid = user()?.session_id;
    const url = `/api/v1/workspaces/${wsId}/events${sid ? `?session_id=${encodeURIComponent(sid)}` : ""}`;
    const es = new EventSource(url);

    es.onopen = () => setConnected(true);
    es.onerror = () => setConnected(false);

    es.addEventListener("workspace", (e: MessageEvent) => {
      try {
        const evt = JSON.parse(e.data) as WorkspaceEvent;
        // Suppress only writes the UI explicitly owns, including uploads and
        // frontmatter. MCP tool calls remain visible even from the same tab.
        if (evt.actor_id === userId && isLocalWriteRequest(evt.request_id)) return;
        setLastEvent(evt);
      } catch (err) {
        console.error("Invalid workspace event", err);
      }
    });

    es.addEventListener("server", (e: MessageEvent) => {
      try {
        const { revision } = JSON.parse(e.data) as { revision: string };
        if (knownRevision === null) {
          knownRevision = revision;
        } else if (revision !== knownRevision) {
          window.location.reload();
        }
      } catch (err) {
        console.error("Invalid server event", err);
      }
    });

    onCleanup(() => {
      es.close();
      setConnected(false);
    });
  });

  return <EventSourceCtx.Provider value={{ lastEvent, connected }}>{props.children}</EventSourceCtx.Provider>;
};

export function useEventSource(): EventSourceContextValue {
  const ctx = useContext(EventSourceCtx);
  if (!ctx) {
    throw new Error("useEventSource must be used within an EventSourceProvider");
  }
  return ctx;
}
