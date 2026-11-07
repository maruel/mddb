// Publishes validated browser selections and subscribes the voice runtime to document changes.

import { createEffect, on, onCleanup, type Accessor } from "solid-js";
import type { WorkspaceEvent } from "@sdk/types.gen";
import type { BrowserVoiceShellProps } from "@maruel/gomode/web/BrowserVoiceShell";
import { voiceResourceNotice } from "./voiceResources";

type BrowserVoiceRuntime = Parameters<NonNullable<BrowserVoiceShellProps["children"]>>[0];

export default function VoiceResourceUpdates(props: {
  runtime: BrowserVoiceRuntime;
  resources: Accessor<string[]>;
  events?: Accessor<Accessor<WorkspaceEvent | null> | null>;
  selection?: Accessor<string | null>;
  publishFocus?: (sessionID: string, nodeID: string | null, signal?: AbortSignal) => Promise<void>;
}) {
  createEffect(() => {
    const sessionID = props.runtime.session.conversationID;
    const nodeID = props.selection?.() ?? null;
    const publish = props.publishFocus;
    if (!publish || !props.runtime.session.state.connected) return;
    const controller = new AbortController();
    void publish(sessionID, nodeID, controller.signal).catch((error: unknown) => {
      if (!controller.signal.aborted) console.warn("Could not publish selected voice node", error);
    });
    onCleanup(() => controller.abort());
  });
  createEffect(() => {
    const uris = props.resources();
    const runtime = props.runtime;
    const events = props.events?.();
    if (events) {
      // Reuse the screen's stream: two tabs with three HTTP/1 event streams
      // each would exhaust the browser's six sockets and stall navigation.
      createEffect(
        on(
          events,
          (event) => {
            if (!event || !runtime.session.state.connected) return;
            const uri = uris.find((value) => value.endsWith(`/nodes/${event.node_id}`));
            const listChanged = ["node_created", "node_deleted", "node_moved"].includes(event.type);
            const notice = voiceResourceNotice({ uri, listChanged });
            if (notice !== null) runtime.session.injectText(notice);
          },
          { defer: true },
        ),
      );
      return;
    }
    let active = true;
    const subscription = runtime.mcp.subscribeResources(
      {
        resourceSubscriptions: uris,
        resourcesListChanged: true,
        onError: (error: unknown) => console.warn("Go Mode resource subscription failed", error),
      },
      (notification) => {
        const notice = voiceResourceNotice(notification);
        if (active && notice !== null && runtime.session.state.connected) runtime.session.injectText(notice);
      },
    );
    onCleanup(() => {
      active = false;
      subscription.close();
    });
  });
  return null;
}
