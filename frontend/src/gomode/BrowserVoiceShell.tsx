// Configures the authenticated Go Mode voice overlay for mddb workspace pages.

import { createSignal, onCleanup, onMount, Show, type Component } from "solid-js";
import { useI18n } from "../i18n";
import { isGoModeHost } from "./host";
import styles from "./BrowserVoiceShell.module.css";

export type VoiceEndpoints = { gatewayURL: string; mcpEndpoint: string };
type LoadedPanel = { component: Component<{ endpoints: VoiceEndpoints }>; endpoints: VoiceEndpoints };

function record(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

/** A missing voice URL means this host has not enabled voice. */
export function resolveVoiceEndpoints(value: unknown, origin: string): VoiceEndpoints | null {
  const manifest = record(value);
  const shell = record(manifest?.webShell);
  const gateway = record(shell?.voiceGateway);
  if (typeof gateway?.url !== "string" || gateway.url === "") return null;
  if (manifest?.service !== "mddb") throw new Error("Unexpected Go Mode service");

  const gatewayURL = new URL(gateway.url, origin);
  if (gatewayURL.origin !== origin || !["http:", "https:"].includes(gatewayURL.protocol)) {
    throw new Error("Voice gateway must be same-origin");
  }

  const groups = shell?.toolGroups;
  if (!Array.isArray(groups)) throw new Error("Go Mode manifest has no MCP groups");
  const workspace = groups.map(record).find((group) => group?.name === "workspace");
  if (typeof workspace?.endpoint !== "string") throw new Error("Go Mode manifest has no workspace MCP endpoint");
  const mcpEndpoint = new URL(workspace.endpoint, origin);
  if (mcpEndpoint.origin !== origin || !["http:", "https:"].includes(mcpEndpoint.protocol)) {
    throw new Error("Workspace MCP endpoint must be same-origin");
  }
  return { gatewayURL: gatewayURL.href, mcpEndpoint: mcpEndpoint.href };
}

/** Ignore missing manifests and HTML SPA fallbacks; only advertised voice is actionable. */
export async function voiceEndpointsFromResponse(response: Response, origin: string): Promise<VoiceEndpoints | null> {
  if (!response.ok || !response.headers.get("content-type")?.toLowerCase().includes("application/json")) return null;
  let manifest: unknown;
  try {
    manifest = (await response.json()) as unknown;
  } catch {
    return null;
  }
  return resolveVoiceEndpoints(manifest, origin);
}

export default function BrowserVoiceShell() {
  const { t } = useI18n();
  const [panel, setPanel] = createSignal<LoadedPanel | null>(null);
  const [setupFailed, setSetupFailed] = createSignal(false);

  onMount(() => {
    if (isGoModeHost()) return;
    const controller = new AbortController();
    let active = true;
    async function configure() {
      const response = await fetch("/.well-known/gomode.json", {
        cache: "no-store",
        signal: controller.signal,
      });
      const endpoints = await voiceEndpointsFromResponse(response, window.location.origin);
      if (!active || endpoints === null) return;
      const module = await import("./VoicePanel");
      if (active) setPanel({ component: module.default, endpoints });
    }
    void configure().catch((error: unknown) => {
      if (!active) return;
      console.error("Could not configure mddb voice gateway", error);
      setSetupFailed(true);
    });
    onCleanup(() => {
      active = false;
      controller.abort();
    });
  });

  return (
    <>
      <Show when={panel()} keyed>
        {({ component: Panel, endpoints }) => <Panel endpoints={endpoints} />}
      </Show>
      <Show when={setupFailed()}>
        <p class={styles.error} role="alert">
          {t("voice.setupFailed")}
        </p>
      </Show>
    </>
  );
}
