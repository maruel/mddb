// Displays invitation scope, preserves sign-in context, and joins with the authenticated account.

import { createResource, createSignal, createMemo, createEffect, on, Match, Show, Switch } from "solid-js";
import { useLocation, useNavigate, useParams, useSearchParams, A } from "@solidjs/router";
import { createApiClient } from "@sdk/api.gen";
import type { UserResponse } from "@sdk/types.gen";
import { APIError } from "../useApi";
import { useAuth, useAuthScope } from "../contexts";
import { useI18n } from "../i18n";
import { workspaceUrl, invitationReturnPath } from "../utils/urls";
import Auth from "./Auth";
import styles from "./AcceptInvitation.module.css";

const publicApi = createApiClient((url, init) => fetch(url, init));

export default function AcceptInvitation() {
  const { t, translateError } = useI18n();
  const { user, identity, ready, login, logout, setUser, api } = useAuth();
  const captureScope = useAuthScope();
  const params = useParams<{ kind: string }>();
  const [search] = useSearchParams<{ token: string; oauth_error?: string }>();
  const location = useLocation();
  const navigate = useNavigate();
  const [joining, setJoining] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [progress, setProgress] = createSignal<"pending" | "organization" | "granted">("pending");
  const owner = createMemo(() => [identity(), user()?.id, user()?.session_id, params.kind, search.token].join("\0"));
  createEffect(
    on(owner, () => {
      setProgress("pending");
      setError(null);
      setJoining(false);
    }),
  );
  const [invitation, { refetch }] = createResource(
    () => {
      const token = search.token;
      const kind = params.kind;
      return token && (kind === "org" || kind === "ws") ? { token, kind } : false;
    },
    async ({ token, kind }) => {
      try {
        const info =
          kind === "org"
            ? await publicApi.getOrgInvitationInfo({ Token: token })
            : await publicApi.getWSInvitationInfo({ Token: token });
        return { info, error: null, retry: false };
      } catch (err) {
        return {
          info: null,
          error: err,
          retry: !(err instanceof APIError && err.status === 404),
        };
      }
    },
  );
  const loadError = () => {
    const failure = invitation()?.error;
    if (!failure) return null;
    if (failure instanceof APIError) {
      return failure.status === 404
        ? t("invitations.unavailable") || "This invitation is unavailable or expired."
        : translateError(failure.code);
    }
    return t("invitations.loadFailed") || "Could not load the invitation. Try again.";
  };
  const roleLabel = () => {
    const labels: Record<string, string> = {
      "org:owner": "settings.roleOwner",
      "org:admin": "settings.roleAdmin",
      "org:member": "settings.roleMember",
      "ws:admin": "settings.roleAdmin",
      "ws:editor": "settings.roleEditor",
      "ws:viewer": "settings.roleViewer",
    };
    const role = invitation()?.info?.role || "";
    return t(labels[role] || "") || role;
  };
  const canJoin = () => {
    const info = invitation()?.info;
    const account = user();
    return (
      !!info &&
      !!account &&
      (progress() === "granted" || !info.email || (account.email_verified && account.email === info.email))
    );
  };

  const accept = async () => {
    const token = search.token;
    const kind = params.kind;
    const info = invitation()?.info;
    if (!token || !info || (kind !== "org" && kind !== "ws") || !canJoin() || joining()) return;
    const isCurrent = captureScope();
    const initialOwner = owner();
    const current = () => isCurrent() && owner() === initialOwner;
    setJoining(true);
    setError(null);
    try {
      let account: UserResponse | null = null;
      if (progress() !== "granted") {
        account =
          kind === "org" ? await api().acceptOrgInvitation({ token }) : await api().acceptWSInvitation({ token });
        if (!current()) return;
        setProgress("granted");
      }
      if (kind === "ws" && "workspace_id" in info) {
        const selected = await api().switchWorkspace({ ws_id: info.workspace_id });
        if (!current()) return;
        if (!selected.user) throw new Error("Missing workspace account response");
        account = selected.user;
      } else if (!account) {
        account = await api().getMe();
        if (!current()) return;
      }
      setUser(account);
      navigate(
        kind === "ws" && "workspace_id" in info
          ? workspaceUrl(info.workspace_id, info.workspace_name)
          : "/settings/user",
        { replace: true },
      );
    } catch (err) {
      if (!current()) return;
      if (progress() === "granted") {
        setError(t("invitations.openFailed") || "You joined, but the workspace could not open. Try again.");
        return;
      }
      // An interrupted response can follow committed membership changes. Refresh
      // access without treating explicitly uncertain persistence as completion.
      let account: UserResponse | null = null;
      let checkFailed = false;
      try {
        account = await api().getMe();
      } catch {
        checkFailed = true;
      }
      if (!current()) return;
      const details = err instanceof APIError ? err.details : undefined;
      const uncertain = details?.persistence_uncertain === true;
      const organizationPresent =
        details?.organization_membership_present === true ||
        !!account?.organizations?.some((membership) => membership.organization_id === info.organization_id);
      const workspacePresent =
        "workspace_id" in info &&
        (details?.workspace_membership_present === true ||
          !!account?.workspaces?.some((membership) => membership.workspace_id === info.workspace_id));
      if (!uncertain && (kind === "org" ? organizationPresent : workspacePresent)) {
        setProgress("granted");
        setError(t("invitations.openFailed") || "You joined, but the workspace could not open. Try again.");
      } else if (uncertain) {
        setProgress(organizationPresent ? "organization" : "pending");
        setError(
          t("invitations.persistenceUncertain") ||
            "Some access changes are visible, but saving could not be confirmed. Try again.",
        );
      } else if (organizationPresent && kind === "ws") {
        setProgress("organization");
        setError(
          t("invitations.organizationJoined") ||
            "You joined the organization, but workspace access is not complete. Try again.",
        );
      } else {
        setError(
          err instanceof APIError
            ? translateError(err.code)
            : t("invitations.failedToAccept") || "Could not accept invitation",
        );
      }
      if (checkFailed)
        setError(
          (message) => `${message} ${t("invitations.accessCheckFailed") || "Current access could not be checked."}`,
        );
      // Clear busy state before our own metadata refresh changes workspace scope.
      setJoining(false);
      if (account) setUser(account);
    } finally {
      if (current()) setJoining(false);
    }
  };

  return (
    <main class={styles.page}>
      <section class={styles.invitation}>
        <h1>{t("invitations.title")}</h1>
        <Switch>
          <Match when={!search.token || (params.kind !== "org" && params.kind !== "ws")}>
            <p role="alert">{t("invitations.unavailable")}</p>
          </Match>
          <Match when={invitation.loading || !ready()}>
            <p role="status">{t("common.loading")}</p>
          </Match>
          <Match when={loadError()}>
            <p role="alert">{loadError()}</p>
            <Show when={invitation()?.retry}>
              <button type="button" onClick={() => void refetch()}>
                {t("invitations.retry")}
              </button>
            </Show>
          </Match>
          <Match when={invitation()?.info}>
            <Show when={invitation()?.info}>
              {(info) => {
                const name = () => {
                  const current = info();
                  return "workspace_name" in current ? current.workspace_name : current.organization_name;
                };
                return (
                  <>
                    <h2>{name()}</h2>
                    <p>{t("invitations.grantsRole", { role: roleLabel() })}</p>
                    <p>
                      {info().email ? t("invitations.boundTo", { email: info().email }) : t("invitations.bearerHint")}
                    </p>
                    <p class={styles.hint}>
                      {t("invitations.expires", { date: new Date(info().expires_at * 1000).toLocaleDateString() })}
                    </p>
                    <Show
                      when={user()}
                      fallback={
                        <Auth
                          embedded
                          hideDeviceLogin
                          returnPath={location.pathname + location.search}
                          onLogin={login}
                        />
                      }
                    >
                      <p>{t("invitations.signedInAs", { email: user()?.email || "" })}</p>
                      <Show when={!canJoin()}>
                        <p role="alert">{t("invitations.verifiedRecipientRequired")}</p>
                        <A
                          href={`/settings/user?next=${encodeURIComponent(invitationReturnPath(location.pathname + location.search) || "")}`}
                        >
                          {t("invitations.manageAccount")}
                        </A>
                      </Show>
                      <Show when={search.oauth_error}>
                        <p role="alert">{t("invitations.providerLinkFailed")}</p>
                      </Show>
                      <Show when={error()}>
                        <p role="alert">{error()}</p>
                      </Show>
                      <div class={styles.actions}>
                        <button type="button" onClick={() => void accept()} disabled={!canJoin() || joining()}>
                          {joining()
                            ? t("common.loading")
                            : progress() === "granted"
                              ? t("invitations.openWorkspace")
                              : error()
                                ? t("invitations.retry")
                                : t("invitations.accept")}
                        </button>
                        <button type="button" onClick={() => void logout()} disabled={joining()}>
                          {t("invitations.changeAccount")}
                        </button>
                      </div>
                    </Show>
                  </>
                );
              }}
            </Show>
          </Match>
        </Switch>
      </section>
    </main>
  );
}
