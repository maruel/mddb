// Shows account sessions, expiry, and individual sign-out actions.

import { createEffect, createResource, createSignal, For, on, Show } from "solid-js";
import { useAuth, useAuthScope } from "../../contexts";
import { Button } from "../shared";
import styles from "./SessionsSection.module.css";
import { useI18n } from "../../i18n";

export default function SessionsSection() {
  const auth = useAuth();
  const { t } = useI18n();
  const captureScope = useAuthScope();
  const [sessions, { refetch }] = createResource(
    () => auth.user()?.session_id,
    async (sessionID) => ({ sessionID, response: await auth.api().listSessions() }),
  );
  const [error, setError] = createSignal(false);
  const [busy, setBusy] = createSignal("");
  createEffect(
    on(
      () => auth.user()?.session_id,
      () => {
        setError(false);
        setBusy("");
      },
    ),
  );
  async function revoke(id: string, current: boolean) {
    if (sessions()?.sessionID !== auth.user()?.session_id) return;
    const isCurrent = captureScope();
    if (current && auth.user()?.session_id !== id) return;
    const api = auth.api();
    setBusy(id);
    setError(false);
    try {
      if (current) {
        await auth.logout();
      } else {
        await api.revokeSession({ session_id: id });
        if (isCurrent()) await refetch();
      }
    } catch {
      if (isCurrent()) setError(true);
    } finally {
      if (isCurrent()) setBusy("");
    }
  }
  return (
    <section class={styles.section}>
      <h3>{t("deviceLogin.sessions")}</h3>
      <Show when={sessions.loading}>
        <p role="status">{t("common.loading")}</p>
      </Show>
      <Show when={sessions.error || error()}>
        <p role="alert">{t("deviceLogin.sessionsError")}</p>
      </Show>
      <ul>
        <For each={sessions()?.sessionID === auth.user()?.session_id ? sessions()?.response.sessions : []}>
          {(session) => (
            <li data-testid={`session-${session.id}`}>
              <strong>{session.device_info}</strong>
              <p>{t("deviceLogin.expires", { time: new Date(session.expires_at * 1000).toLocaleString() })}</p>
              <Button
                class={styles.action}
                type="button"
                disabled={busy() !== ""}
                onClick={() => void revoke(session.id, session.is_current)}
              >
                {t("deviceLogin.signOutSession")}
              </Button>
            </li>
          )}
        </For>
      </ul>
    </section>
  );
}
