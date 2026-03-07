// SSE client context for receiving real-time workspace events.

import {
  createContext,
  useContext,
  createSignal,
  createEffect,
  onCleanup,
  type ParentComponent,
  type Accessor,
} from 'solid-js';
import { useAuth } from './AuthContext';
import type { WorkspaceEvent } from '@sdk/types.gen';

interface EventSourceContextValue {
  lastEvent: Accessor<WorkspaceEvent | null>;
  connected: Accessor<boolean>;
}

const EventSourceCtx = createContext<EventSourceContextValue>();

// Module-level: persists across reconnects and workspace switches so we can
// detect when a new binary is deployed (revision changes on reconnect).
let knownRevision: string | null = null;

export const EventSourceProvider: ParentComponent = (props) => {
  const { user, token } = useAuth();
  const [lastEvent, setLastEvent] = createSignal<WorkspaceEvent | null>(null);
  const [connected, setConnected] = createSignal(false);

  createEffect(() => {
    const wsId = user()?.workspace_id;
    const t = token();
    const userId = user()?.id;
    if (!wsId || !t || typeof EventSource === 'undefined') {
      setConnected(false);
      return;
    }

    const url = `/api/v1/workspaces/${wsId}/events?token=${encodeURIComponent(t)}`;
    const es = new EventSource(url);

    es.onopen = () => setConnected(true);
    es.onerror = () => setConnected(false);

    es.addEventListener('workspace', (e: MessageEvent) => {
      try {
        const evt = JSON.parse(e.data) as WorkspaceEvent;
        // Self-filter: ignore events caused by the current user.
        if (evt.actor_id === userId) return;
        setLastEvent(evt);
      } catch {
        // Ignore malformed events.
      }
    });

    es.addEventListener('server', (e: MessageEvent) => {
      try {
        const { revision } = JSON.parse(e.data) as { revision: string };
        if (knownRevision === null) {
          knownRevision = revision;
        } else if (revision !== knownRevision) {
          window.location.reload();
        }
      } catch {
        // Ignore malformed events.
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
    throw new Error('useEventSource must be used within an EventSourceProvider');
  }
  return ctx;
}
