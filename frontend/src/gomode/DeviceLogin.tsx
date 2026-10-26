// Hosts shared device login with mddb's translated labels and cookie-backed browser sessions.

import { createEffect, createSignal, Show } from "solid-js";
import { Navigate, A } from "@solidjs/router";
import {
  DeviceLoginDisplay,
  DeviceLoginApproval,
  type DeviceLoginMessages,
  type DeviceLoginRecoveryMessages,
} from "@maruel/gomode/web/DeviceLogin";
import { createApiClient, APIError } from "@sdk/api.gen";
import type { UserResponse } from "@sdk/types.gen";
import Auth from "../components/Auth";
import { useAuth } from "../contexts";
import { useI18n } from "../i18n";
import styles from "./DeviceLogin.module.css";

const endpoints = {
  start: "/api/v1/auth/device/start",
  poll: "/api/v1/auth/browser/device/poll",
  lookup: "/api/v1/auth/device/lookup",
  approve: "/api/v1/auth/device/approve",
};

function useMessages(): () => DeviceLoginMessages {
  const { t } = useI18n();
  return () => ({
    start: t("deviceLogin.start") || "",
    starting: t("deviceLogin.starting") || "",
    instructions: t("deviceLogin.instructions") || "",
    pending: t("deviceLogin.pending") || "",
    approved: t("deviceLogin.approved") || "",
    denied: t("deviceLogin.denied") || "",
    expired: t("deviceLogin.expired") || "",
    error: t("deviceLogin.error") || "",
    remaining: (seconds) =>
      t("deviceLogin.remaining", { time: `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}` }) ||
      "",
    code: t("deviceLogin.code") || "",
    lookup: t("deviceLogin.lookup") || "",
    approve: t("deviceLogin.approve") || "",
    deny: t("deviceLogin.deny") || "",
    warning: t("deviceLogin.warning") || "",
    approvalApproved: t("deviceLogin.approvalApproved") || "",
    approvalDenied: t("deviceLogin.approvalDenied") || "",
    approvalError: t("deviceLogin.approvalError") || "",
    approvalExpired: t("deviceLogin.approvalExpired") || "",
    device: t("deviceLogin.device") || "",
    address: t("deviceLogin.address") || "",
    requested: t("deviceLogin.requested") || "",
  });
}

function useRecoveryMessages(): () => DeviceLoginRecoveryMessages {
  const { t } = useI18n();
  return () => ({
    newCode: t("deviceLogin.newCode") || "",
    checkSignIn: t("deviceLogin.checkSignIn") || "",
    throttled: (seconds) => t("deviceLogin.throttled", { seconds: seconds ?? 600 }) || "",
    approvalThrottled: (seconds) => t("deviceLogin.approvalThrottled", { seconds: seconds ?? 600 }) || "",
    audit: t("deviceLogin.audit") || "",
    approvalAudit: t("deviceLogin.approvalAudit") || "",
    session: t("deviceLogin.session") || "",
    delivery: t("deviceLogin.delivery") || "",
    approvalDelivery: t("deviceLogin.approvalDelivery") || "",
    approvalNetwork: t("deviceLogin.approvalNetwork") || "",
    changed: t("deviceLogin.changed") || "",
    decided: t("deviceLogin.decided") || "",
    login: t("deviceLogin.login") || "",
    ineligible: t("deviceLogin.ineligible") || "",
    disabled: t("deviceLogin.disabled") || "",
    network: t("deviceLogin.network") || "",
  });
}

export function DeviceLoginRoute() {
  const { login, user, ready, refreshUser } = useAuth();
  const { t } = useI18n();
  const messages = useMessages();
  const recoveryMessages = useRecoveryMessages();
  const [initialized, setInitialized] = createSignal(false);
  createEffect(() => {
    if (ready()) setInitialized(true);
  });
  return (
    <Show when={initialized()}>
      <Show when={!user()} fallback={<Navigate href="/" />}>
        <main class={styles.shell}>
          <h1>{t("deviceLogin.start")}</h1>
          <DeviceLoginDisplay
            messages={messages}
            recoveryMessages={recoveryMessages}
            options={{
              endpoints,
              onCheckSession: () => {
                void refreshUser();
              },
              onSession: (response) => {
                if (
                  typeof response !== "object" ||
                  response === null ||
                  !("id" in response) ||
                  typeof response.id !== "string"
                )
                  throw new Error("invalid_session");
                login(response as UserResponse);
              },
            }}
          />
          <A href="/login">{t("auth.login")}</A>
        </main>
      </Show>
    </Show>
  );
}

export function DeviceApprovalRoute() {
  const { login, user, ready, logout, identity, reloadIdentity } = useAuth();
  const { t } = useI18n();
  const messages = useMessages();
  const recoveryMessages = useRecoveryMessages();
  const verifyApprover = async (sessionID: string, signal: AbortSignal) => {
    const epoch = identity();
    const accountID = user()?.id;
    const client = createApiClient(async (url, init) => {
      const response = await fetch(url, { ...init, signal, cache: "no-store", credentials: "same-origin" });
      // Authentication middleware returns plain-text 401s before the JSON API.
      if (response.status === 401) throw new APIError(401, "UNAUTHORIZED");
      return response;
    });
    try {
      const fresh = await client.getMe();
      if (signal.aborted || identity() !== epoch) return false;
      if (fresh.id !== accountID || fresh.session_id !== sessionID) {
        login(fresh);
        return false;
      }
      return true;
    } catch (err) {
      if (signal.aborted || identity() !== epoch) return false;
      if (err instanceof APIError && err.status === 401) {
        reloadIdentity();
        return false;
      }
      throw err;
    }
  };
  return (
    <Show when={ready()}>
      <Show keyed when={user()?.session_id} fallback={<Auth onLogin={login} hideDeviceLogin returnPath="/device" />}>
        {(sessionID) => (
          <main class={styles.shell}>
            <h1>{t("deviceLogin.lookup")}</h1>
            <p>{t("deviceLogin.account", { name: user()?.name || user()?.email || "", email: user()?.email || "" })}</p>
            <button
              class={styles.accountButton}
              type="button"
              onClick={() => {
                void logout().catch((err: unknown) => console.error("Failed to change approving account", err));
              }}
            >
              {t("deviceLogin.changeAccount")}
            </button>
            <DeviceLoginApproval
              endpoints={endpoints}
              messages={messages}
              recoveryMessages={recoveryMessages}
              headers={() => ({ "X-MDDB-Session": sessionID })}
              onAuthenticationRequired={reloadIdentity}
              verifyApprover={(signal) => verifyApprover(sessionID, signal)}
            />
          </main>
        )}
      </Show>
    </Show>
  );
}
