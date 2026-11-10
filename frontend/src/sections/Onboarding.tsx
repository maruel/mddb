// Presents shared workspace preparation, retry, access requirements, and the resulting workspace redirect.

import { createEffect, Show } from "solid-js";
import { useNavigate } from "@solidjs/router";
import { useAuth } from "../contexts";
import { useWorkspaceData } from "../contexts/WorkspaceDataContext";
import { useI18n } from "../i18n";
import { workspaceUrl } from "../utils/urls";
import styles from "./Onboarding.module.css";

export default function Onboarding() {
  const { t } = useI18n();
  const { user } = useAuth();
  const data = useWorkspaceData();
  const navigate = useNavigate();
  createEffect(() => {
    const current = user();
    if (current?.workspace_id) navigate(workspaceUrl(current.workspace_id, current.workspace_name), { replace: true });
  });
  return (
    <div class={styles.container}>
      <Show when={data.preparationError()}>
        <div class={styles.error} role="alert">
          {data.preparationError()}
        </div>
        <button
          class={styles.retry}
          type="button"
          onClick={() =>
            void data.retryPreparation().catch((error: unknown) => console.warn("Workspace retry failed", error))
          }
        >
          {t("recovery.retry")}
        </button>
      </Show>
      <Show when={!data.preparationError()}>
        <Show
          when={data.needsWorkspace()}
          fallback={
            <>
              <div class={styles.status} role="status">
                {t("common.loading")}
              </div>
              <div class={styles.spinner} />
            </>
          }
        >
          <p role="status">{t("onboarding.noWorkspace")}</p>
        </Show>
      </Show>
    </div>
  );
}
