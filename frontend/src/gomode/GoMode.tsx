// Owns authenticated workspace preparation, validated conversation focus, and browser voice across route layouts.

import {
  createContext,
  createEffect,
  createMemo,
  createSignal,
  on,
  onCleanup,
  Show,
  untrack,
  useContext,
  type Accessor,
  type ParentComponent,
} from "solid-js";
import type { FrontendPreparationScope } from "@maruel/gomode/web/FrontendVoiceTools";
import BrowserVoiceShell from "@maruel/gomode/web/BrowserVoiceShell";
import { useHostMode } from "@maruel/gomode/web/HostMode";
import type { VoiceOverlayMessages } from "@maruel/gomode/web/VoiceOverlay";
import { useWorkspaceData } from "../contexts/WorkspaceDataContext";
import { useWorkspaceNavigation } from "../contexts/WorkspaceNavigationContext";
import { useAuth } from "../contexts/AuthContext";
import { useI18n } from "../i18n";
import { voiceAuthorization } from "./voiceAuthorization";
import VoiceResourceUpdates from "./VoiceResourceUpdates";
import { createVoiceFocusPublisher, type VoiceFocusPublisher } from "./voiceFocus";
import styles from "./GoMode.module.css";
import type { WorkspaceEvent } from "@sdk/types.gen";
import type { UpdateVoiceFocusRequest } from "@sdk/types.gen";
import { createApiClient } from "@sdk/api.gen";
import { createAuthFetch } from "../useApi";

interface GoModeContextValue {
  preparation: Accessor<FrontendPreparationScope | null>;
  watchResources(resources: Accessor<string[]>, events?: Accessor<WorkspaceEvent | null>): void;
}
const GoModeContext = createContext<GoModeContextValue>();

export const GoModeProvider: ParentComponent = (props) => {
  const host = useHostMode();
  const { frontendIdentity, identity, user } = useAuth();
  const { t } = useI18n();
  const data = useWorkspaceData();
  const navigation = useWorkspaceNavigation();
  const [preparation, setPreparation] = createSignal<FrontendPreparationScope | null>(null);
  const [publisher, setPublisher] = createSignal<VoiceFocusPublisher | null>(null);
  const owner = createMemo(() => `${identity()}:${user()?.session_id ?? ""}:${user()?.workspace_id ?? ""}`);
  const [watched, setWatched] = createSignal<{
    owner: string;
    resources: Accessor<string[]>;
    events?: Accessor<WorkspaceEvent | null>;
  } | null>(null);
  const workspaceEvents = () => {
    const current = watched();
    return current?.owner === owner() ? (current.events ?? null) : null;
  };
  const resources = createMemo(
    () => {
      const current = watched();
      return current?.owner === owner() ? current.resources() : [];
    },
    [],
    { equals: (a, b) => a.length === b.length && a.every((uri, i) => uri === b[i]) },
  );

  createEffect(
    on([frontendIdentity, owner], ([lease]) => {
      setPreparation(null);
      if (lease === null) return;
      const scope = lease.createScope();
      const workspaceId = user()?.workspace_id;
      const captured = owner();
      const authFetch = createAuthFetch(undefined, user()?.session_id);
      // Each publication captures its cancellation at the generated client's
      // fetch boundary without changing unrelated workspace requests.
      const updateFocus = workspaceId
        ? (request: UpdateVoiceFocusRequest, signal: AbortSignal) =>
            createApiClient((url, init) =>
              authFetch(url, { ...init, signal: AbortSignal.any([signal, ...(init?.signal ? [init.signal] : [])]) }),
            )
              .ws(workspaceId)
              .updateVoiceFocus(request)
        : null;
      const focus = updateFocus ? createVoiceFocusPublisher(updateFocus, false) : null;
      const browserFocus = updateFocus ? createVoiceFocusPublisher(updateFocus, true) : null;
      setPublisher(browserFocus);
      scope.setTools(
        workspaceId
          ? [
              {
                declaration: {
                  name: "focus_node",
                  description:
                    "Open a document or table in the current workspace using its stable nodeId from service tools. This selects the node in the user’s frontend without switching workspaces.",
                  parameters: {
                    type: "object",
                    properties: { nodeId: { type: "string", minLength: 1 } },
                    required: ["nodeId"],
                    additionalProperties: false,
                  },
                },
                async execute(args, invocation) {
                  const id = args.nodeId;
                  if (typeof id !== "string" || id.length === 0 || id.trim() !== id)
                    throw new Error("Invalid node ID.");
                  await navigation.focusNode(id, invocation);
                  // Native focus belongs to this invocation's conversation.
                  // Manual native selection needs a shell conversation-lifetime
                  // signal; retaining the last invocation ID would leak focus
                  // into a retired conversation. Legacy bridge3 has no ID.
                  if (invocation.sessionID !== null && invocation.isCurrent()) {
                    try {
                      const invocationFocus = untrack(host.browserVoiceEnabled) ? browserFocus : focus;
                      await invocationFocus?.publish(
                        invocation.sessionID,
                        id,
                        () => untrack(owner) === captured && invocation.isCurrent(),
                        invocation.signal,
                      );
                    } catch (error) {
                      console.warn("Could not publish native voice focus", error);
                    }
                  }
                  return { nodeId: id, status: "focused" };
                },
              },
            ]
          : [],
      );
      createEffect(() => scope.setDataReady(!workspaceId || data.ready()));
      setPreparation(scope);
      onCleanup(() => {
        focus?.dispose();
        browserFocus?.dispose();
        setPublisher(null);
        scope.dispose();
      });
    }),
  );

  const context: GoModeContextValue = {
    preparation,
    watchResources(watchedResources, events) {
      const registration = { owner: owner(), resources: watchedResources, events };
      setWatched(registration);
      onCleanup(() => {
        if (untrack(watched) === registration) setWatched(null);
      });
    },
  };
  const messages = (defaults: Readonly<VoiceOverlayMessages>): VoiceOverlayMessages => {
    const translated = { ...defaults };
    for (const key of Object.keys(translated) as (keyof VoiceOverlayMessages)[]) {
      translated[key] = t(`voice.${key}`) || translated[key];
    }
    return translated;
  };

  return (
    <GoModeContext.Provider value={context}>
      <div class={styles.viewport}>
        <div class={styles.routes}>{props.children}</div>
        <Show when={user()?.workspace_id ? preparation() : null} keyed>
          {(scope) => {
            const session = user()?.session_id;
            const workspaceId = user()?.workspace_id;
            const staticFocus = publisher();
            const staticOwner = owner();
            const publishFocus = async (sessionID: string, nodeID: string | null, signal?: AbortSignal) => {
              await staticFocus?.publish(sessionID, nodeID, () => staticOwner === untrack(owner), signal);
            };
            return (
              <div class={styles.shell}>
                <BrowserVoiceShell
                  service="mddb"
                  toolGroup="workspace"
                  clientName="mddb-frontend"
                  preparation={scope}
                  authorize={(endpoint) => voiceAuthorization(endpoint, t("voice.authorizationFailed"), session)}
                  mcpEndpoint={(endpoint) => {
                    const url = new URL(endpoint);
                    if (session) url.searchParams.set("session_id", session);
                    if (workspaceId) url.searchParams.set("workspaceId", workspaceId);
                    return url.href;
                  }}
                  prepareSession={(sessionID, signal) =>
                    publishFocus(sessionID, navigation.selectedNode()?.id ?? null, signal)
                  }
                  gatewayHeaders={session ? { "X-MDDB-Session": session } : {}}
                  messages={messages}
                  setupFailed={t("voice.setupFailed") || ""}
                  errorClass={styles.error}
                  onError={(error) => console.error("Could not configure mddb voice gateway", error)}
                >
                  {(runtime) => (
                    <VoiceResourceUpdates
                      runtime={runtime}
                      resources={resources}
                      events={workspaceEvents}
                      selection={() => navigation.selectedNode()?.id ?? null}
                      publishFocus={publishFocus}
                    />
                  )}
                </BrowserVoiceShell>
              </div>
            );
          }}
        </Show>
      </div>
    </GoModeContext.Provider>
  );
};

export function useGoMode(): GoModeContextValue {
  const context = useContext(GoModeContext);
  if (!context) throw new Error("useGoMode must be used within GoModeProvider");
  return context;
}
