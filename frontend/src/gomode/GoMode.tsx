// Adapts authenticated workspace selection and focus policy to shared conversation preparation.

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
import { defaultVoiceMessages, type VoiceMessages } from "@maruel/gomode/web/VoiceMessages";
import { useWorkspaceData } from "../contexts/WorkspaceDataContext";
import { useWorkspaceNavigation } from "../contexts/WorkspaceNavigationContext";
import { useAuth } from "../contexts/AuthContext";
import { useI18n } from "../i18n";
import { workspaceResourceEvents } from "./workspaceResourceEvents";
import { voiceAuthorization } from "./voiceAuthorization";
import type { McpResourceNotification } from "@maruel/gomode/web/McpClient";
import { createVoiceFocusPublisher } from "./voiceFocus";
import styles from "./GoMode.module.css";
import type { WorkspaceEvent } from "@sdk/types.gen";
import type { UpdateVoiceFocusRequest } from "@sdk/types.gen";
import { createApiClient } from "@sdk/api.gen";
import { createAuthFetch } from "../useApi";

interface GoModeContextValue {
  preparation: Accessor<FrontendPreparationScope | null>;
  shareWorkspaceEvents(events: Accessor<WorkspaceEvent | null>): void;
}
const GoModeContext = createContext<GoModeContextValue>();

export const GoModeProvider: ParentComponent = (props) => {
  const { frontendIdentity, identity, user } = useAuth();
  const { t } = useI18n();
  const data = useWorkspaceData();
  const navigation = useWorkspaceNavigation();
  const [preparation, setPreparation] = createSignal<FrontendPreparationScope | null>(null);
  const owner = createMemo(() => `${identity()}:${user()?.session_id ?? ""}:${user()?.workspace_id ?? ""}`);
  const [eventSource, setEventSource] = createSignal<{
    owner: string;
    events: Accessor<WorkspaceEvent | null>;
  } | null>(null);
  const workspaceEvents = () => {
    const current = eventSource();
    return current?.owner === owner() ? current.events : null;
  };
  const resourceEvents = createMemo<readonly McpResourceNotification[] | null>(() => {
    const event = workspaceEvents()?.();
    if (!event) return null;
    const workspace = user()?.workspace_id;
    return workspace ? workspaceResourceEvents(workspace, event) : null;
  });

  createEffect(
    on([frontendIdentity, owner], ([lease]) => {
      setPreparation(null);
      if (lease === null) return;
      const workspaceId = user()?.workspace_id;
      const authFetch = createAuthFetch(undefined, user()?.session_id);
      // Keep the client bound to this lease's workspace and cookie session.
      const workspace = workspaceId ? createApiClient(authFetch).ws(workspaceId) : null;
      const updateFocus = workspace
        ? (request: UpdateVoiceFocusRequest, signal: AbortSignal) => workspace.updateVoiceFocus(request, signal)
        : null;
      const scope = lease.createScope(
        updateFocus
          ? {
              selection: () => navigation.selectedNode()?.id ?? null,
              createPublisher: (sessionID, kind, release) =>
                createVoiceFocusPublisher(updateFocus, sessionID, kind === "conversation", release),
            }
          : undefined,
      );
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
                  if (invocation.sessionID !== null && invocation.isCurrent()) {
                    try {
                      await scope.publishContext(invocation.sessionID, invocation.signal);
                    } catch {
                      // Shared preparation owns the advisory outcome and recovery.
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
        scope.dispose();
      });
    }),
  );

  const context: GoModeContextValue = {
    preparation,
    shareWorkspaceEvents(events) {
      const registration = { owner: owner(), events };
      setEventSource(registration);
      onCleanup(() => {
        if (untrack(eventSource) === registration) setEventSource(null);
      });
    },
  };
  const messages = (): VoiceMessages => {
    const translated = { ...defaultVoiceMessages };
    for (const key of Object.keys(translated) as (keyof VoiceMessages)[]) {
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
                  gatewayHeaders={session ? { "X-MDDB-Session": session } : {}}
                  resourceEvents={workspaceEvents() ? resourceEvents : undefined}
                  messages={messages}
                  setupFailed={t("voice.setupFailed") || ""}
                  errorClass={styles.error}
                  onError={(error) => console.error("Could not configure mddb voice gateway", error)}
                />
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
