// Owns authenticated workspace preparation and browser voice across route layouts.

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
import type { VoiceOverlayMessages } from "@maruel/gomode/web/VoiceOverlay";
import { useWorkspaceData } from "../contexts/WorkspaceDataContext";
import { useWorkspaceNavigation } from "../contexts/WorkspaceNavigationContext";
import { useAuth } from "../contexts/AuthContext";
import { useI18n } from "../i18n";
import { voiceAuthorization } from "./voiceAuthorization";
import VoiceResourceUpdates from "./VoiceResourceUpdates";
import styles from "./GoMode.module.css";

interface GoModeContextValue {
  preparation: Accessor<FrontendPreparationScope | null>;
  watchResources(resources: Accessor<string[]>): void;
}
const GoModeContext = createContext<GoModeContextValue>();

export const GoModeProvider: ParentComponent = (props) => {
  const { frontendIdentity, identity, user } = useAuth();
  const { t } = useI18n();
  const data = useWorkspaceData();
  const navigation = useWorkspaceNavigation();
  const [preparation, setPreparation] = createSignal<FrontendPreparationScope | null>(null);
  const owner = createMemo(() => `${identity()}:${user()?.session_id ?? ""}:${user()?.workspace_id ?? ""}`);
  const [watched, setWatched] = createSignal<{ owner: string; resources: Accessor<string[]> } | null>(null);
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
                  return { nodeId: id, status: "focused" };
                },
              },
            ]
          : [],
      );
      createEffect(() => scope.setDataReady(!workspaceId || data.ready()));
      setPreparation(scope);
      onCleanup(() => scope.dispose());
    }),
  );

  const context: GoModeContextValue = {
    preparation,
    watchResources(watchedResources) {
      const registration = { owner: owner(), resources: watchedResources };
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
                    return url.href;
                  }}
                  gatewayHeaders={session ? { "X-MDDB-Session": session } : {}}
                  messages={messages}
                  setupFailed={t("voice.setupFailed") || ""}
                  errorClass={styles.error}
                  onError={(error) => console.error("Could not configure mddb voice gateway", error)}
                >
                  {(runtime) => <VoiceResourceUpdates runtime={runtime} resources={resources} />}
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
