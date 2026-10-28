// Subscribes the current browser voice runtime to mddb document changes.

import { createEffect, onCleanup, type Accessor } from "solid-js";
import type { BrowserVoiceShellProps } from "@maruel/gomode/web/BrowserVoiceShell";
import { voiceResourceNotice } from "./voiceResources";

type BrowserVoiceRuntime = Parameters<NonNullable<BrowserVoiceShellProps["children"]>>[0];

export default function VoiceResourceUpdates(props: { runtime: BrowserVoiceRuntime; resources: Accessor<string[]> }) {
  createEffect(() => {
    const uris = props.resources();
    const runtime = props.runtime;
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
