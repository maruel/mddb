// Onboarding screen for first-time users, using the shared first-workspace provisioner.

import { createEffect, createSignal, Show } from "solid-js";
import { useNavigate } from "@solidjs/router";
import { useAuth } from "../contexts";
import { provisionFirstWorkspace } from "../contexts/firstLogin";
import { useI18n } from "../i18n";
import { createApi } from "../useApi";
import { workspaceUrl } from "../utils/urls";
import styles from "./Onboarding.module.css";

/**
 * Onboarding handles the first-login flow for users who have authenticated
 * but don't yet have an organization or workspace.
 *
 * Creates or picks the first workspace and redirects to it.
 */
export default function Onboarding() {
  const { t, ready: i18nReady } = useI18n();
  const { user, token, login, setUser } = useAuth();
  const navigate = useNavigate();

  const [done, setDone] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);

  // Track whether we're already running to prevent duplicate executions
  const [running, setRunning] = createSignal(false);

  // Run first-login flow
  createEffect(() => {
    const u = user();
    const initialToken = token();
    if (!u || !initialToken || !i18nReady() || running() || done() || error()) return;

    // If user already has a workspace, redirect
    if (u.workspace_id) {
      navigate(workspaceUrl(u.workspace_id, u.workspace_name), { replace: true });
      return;
    }

    // Run async flow
    setRunning(true);
    (async () => {
      try {
        const firstName = u.name.split(" ")[0] || u.name;
        const result = await provisionFirstWorkspace(
          u.id,
          createApi(() => initialToken),
          {
            organization: (firstName
              ? t("onboarding.defaultOrgName", { name: firstName })
              : t("onboarding.defaultOrgNameFallback")) as string,
            workspace: (firstName
              ? t("onboarding.defaultWorkspaceName", { name: firstName })
              : t("onboarding.defaultWorkspaceNameFallback")) as string,
          },
        );
        if (token() !== initialToken) return;
        if (result.token) login(result.token, result.user);
        else setUser(result.user);
        if (result.user.workspace_id)
          navigate(workspaceUrl(result.user.workspace_id, result.user.workspace_name), { replace: true });
        setDone(true);
      } catch (err) {
        console.error("Onboarding error:", err);
        setError(String(err));
      } finally {
        setRunning(false);
      }
    })();
  });

  return (
    <div class={styles.container}>
      <Show when={error()}>
        <div class={styles.error}>{error()}</div>
      </Show>
      <Show when={!error()}>
        <div class={styles.status}>{t("common.loading") || "Loading..."}</div>
        <div class={styles.spinner} />
      </Show>
    </div>
  );
}
