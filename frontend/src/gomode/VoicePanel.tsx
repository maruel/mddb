// Hosts the authenticated Go Mode voice controls when mddb advertises a voice gateway.

import { createEffect, onCleanup, onMount } from "solid-js";
import VoiceOverlay, { defaultVoiceOverlayMessages, type VoiceOverlayMessages } from "@maruel/gomode/web/VoiceOverlay";
import { configureMcpClient, mcpClient, type McpResourceSubscription } from "@maruel/gomode/web/McpClient";
import { configureVoiceGateway, voiceSession } from "@maruel/gomode/web/VoiceSession";
import { notifications } from "@maruel/gomode/web/notifications";
import { useAuth } from "../contexts/AuthContext";
import { useI18n } from "../i18n";
import type { VoiceEndpoints } from "./BrowserVoiceShell";
import { voiceResourceNotice } from "./voiceResources";
import styles from "./VoicePanel.module.css";

export default function VoicePanel(props: { endpoints: VoiceEndpoints; resourceSubscriptions?: string[] }) {
  const { token } = useAuth();
  const { t } = useI18n();
  // Translate every label gomode defines, so a gomode upgrade that adds labels
  // needs only dictionary entries; VoicePanel.test.ts requires them.
  const messages = (): VoiceOverlayMessages => {
    const out = { ...defaultVoiceOverlayMessages };
    for (const key of Object.keys(out) as (keyof VoiceOverlayMessages)[]) {
      out[key] = t(`voice.${key}`) || out[key];
    }
    return out;
  };

  onMount(() => {
    configureMcpClient(props.endpoints.mcpEndpoint, "mddb-frontend", token);
    configureVoiceGateway(props.endpoints.gatewayURL, null, token);
  });

  // Follow the host's watched resources so the model is told when a document it
  // is working on changes, instead of overwriting newer content. Re-subscribe
  // only when the watched set actually changes.
  let subscription: McpResourceSubscription | null = null;
  let subscriptionKey = "";
  createEffect(() => {
    const uris = props.resourceSubscriptions ?? [];
    const key = uris.join("\n");
    if (key === subscriptionKey) return;
    subscriptionKey = key;
    subscription?.close();
    subscription = mcpClient.subscribeResources(
      {
        resourceSubscriptions: uris,
        resourcesListChanged: true,
        onError: (error: unknown) => console.warn("Go Mode resource subscription failed", error),
      },
      (notification) => {
        const notice = voiceResourceNotice(notification);
        if (notice !== null && voiceSession.state.connected) voiceSession.injectText(notice);
      },
    );
  });

  onCleanup(() => {
    subscription?.close();
    voiceSession.disconnect();
    notifications.setVoiceActive(false);
  });

  return (
    <div class={styles.shell}>
      <VoiceOverlay messages={messages} />
    </div>
  );
}
