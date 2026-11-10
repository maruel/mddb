// Owns workspace event streams, route recovery, heartbeat liveness, and retired callback isolation.

import {
  createContext,
  useContext,
  createSignal,
  createEffect,
  onCleanup,
  type ParentComponent,
  type Accessor,
} from "solid-js";
import { observeNetworkChanges } from "@maruel/gomode/web/NetworkChanges";
import { useAuth } from "./AuthContext";
import { EventResync, type WorkspaceEvent } from "@sdk/types.gen";
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
    let source: EventSource | null = null;
    let watchdog: ReturnType<typeof setTimeout> | null = null;
    let active = true;
    const close = () => {
      if (watchdog !== null) clearTimeout(watchdog);
      watchdog = null;
      const retired = source;
      source = null;
      retired?.close();
    };
    const connect = () => {
      close();
      setConnected(false);
      if (!active || !navigator.onLine) return;
      const es = new EventSource(url);
      source = es;
      const current = () => active && source === es;
      const alive = () => {
        if (!current()) return;
        if (watchdog !== null) clearTimeout(watchdog);
        // The server sends observable heartbeats every 30 seconds. Three missed
        // heartbeats replace a socket that never reports its broken route.
        watchdog = setTimeout(connect, 90_000);
      };
      es.onopen = () => {
        if (!current()) return;
        alive();
        setConnected(true);
        // Every open establishes the subscription boundary. Initial REST reads
        // can predate it too; refresh through the same draft-preserving path.
        setLastEvent({ type: EventResync, node_id: "0", actor_id: "0", modified: 0 });
      };
      es.onerror = () => {
        if (!current()) return;
        setConnected(false);
        // Native EventSource owns reported transport retries and terminal HTTP
        // failures. The watchdog only detects sockets that stop reporting activity.
        if (watchdog !== null) clearTimeout(watchdog);
        watchdog = null;
      };
      es.addEventListener("heartbeat", alive);
      es.addEventListener("workspace", (e: MessageEvent<string>) => {
        if (!current()) return;
        alive();
        try {
          const evt = JSON.parse(e.data) as WorkspaceEvent;
          // Suppress only writes the UI explicitly owns. MCP calls remain visible.
          if (evt.actor_id === userId && isLocalWriteRequest(evt.request_id)) return;
          setLastEvent(evt);
        } catch (err) {
          console.error("Invalid workspace event", err);
        }
      });
      es.addEventListener("server", (e: MessageEvent<string>) => {
        if (!current()) return;
        alive();
        try {
          const { revision } = JSON.parse(e.data) as { revision: string };
          if (knownRevision === null) knownRevision = revision;
          else if (revision !== knownRevision) window.location.reload();
        } catch (err) {
          console.error("Invalid server event", err);
        }
      });
      alive();
    };
    const stopObserving = observeNetworkChanges(connect);
    connect();
    onCleanup(() => {
      active = false;
      stopObserving();
      close();
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
