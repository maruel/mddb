// Owns cookie account state, identity reconciliation policy, page-bound frontend preparation, and in-place or reloading session transitions.

import {
  createContext,
  useContext,
  createSignal,
  createMemo,
  createEffect,
  batch,
  onMount,
  onCleanup,
  type ParentComponent,
  type Accessor,
  Show,
} from "solid-js";
import { createApi, APIError, type Api } from "../useApi";
import { createApiClient } from "@sdk/api.gen";
import type { UserResponse } from "@sdk/types.gen";
import { createFrontendPreparationClient, type FrontendPreparation } from "@maruel/gomode/web/FrontendVoiceTools";
import { useI18n, type Locale } from "../i18n";

export interface AuthContextValue {
  user: Accessor<UserResponse | null>;
  identity: Accessor<number>;
  frontendIdentity: Accessor<FrontendPreparation | null>;
  ready: Accessor<boolean>;
  api: Accessor<Api>;
  wsApi: Accessor<ReturnType<Api["ws"]> | null>;
  orgApi: Accessor<ReturnType<Api["org"]> | null>;
  login: (user: UserResponse) => void;
  logout: () => Promise<void>;
  /** Invalidate native identity before rediscovering the authoritative cookie. */
  reloadIdentity: () => void;
  /** Refresh the cookie identity in place, or reload when the account or session is unusable. */
  refreshIdentity: (signal?: AbortSignal) => Promise<boolean>;
  setUser: (user: UserResponse | null) => void;
  refreshUser: () => Promise<void>;
}
export const AuthContext = createContext<AuthContextValue>();
const authEventKey = "mddb_auth_changed";

// A probe of the cookie credential, either resolved to an account or failed.
export type IdentityProbe = { user: UserResponse } | { error: unknown };

// What a cookie identity probe means for the account the page is rendering.
export type IdentityReconciliation =
  | { status: "current"; user: UserResponse }
  | { status: "changed"; user: UserResponse }
  | { status: "expired"; user: null }
  | { status: "unavailable"; user: null; error: unknown };

// Reports whether the server answered for good that the credential has no usable account.
function rejectedIdentity(error: unknown): boolean {
  return error instanceof APIError && (error.status === 401 || error.status === 404);
}

// Decides how a probed cookie identity replaces the rendered one: the same account
// refreshes in place, another account or a rejected identity needs a fresh document,
// and any other failure leaves the rendered identity untouched.
export function reconcileIdentity(shown: UserResponse | null, probe: IdentityProbe): IdentityReconciliation {
  if ("error" in probe) {
    return rejectedIdentity(probe.error)
      ? { status: "expired", user: null }
      : { status: "unavailable", user: null, error: probe.error };
  }
  return { status: shown !== null && probe.user.id === shown.id ? "current" : "changed", user: probe.user };
}

function reconciledUser(reconciliation: IdentityReconciliation): UserResponse | null {
  switch (reconciliation.status) {
    case "current":
    case "changed":
      return reconciliation.user;
    case "expired":
    case "unavailable":
      return null;
  }
}

export const AuthProvider: ParentComponent = (props) => {
  const nativeFrontend = createFrontendPreparationClient();
  const { setLocale, t } = useI18n();
  const [user, setUser] = createSignal<UserResponse | null>(null);
  const [ready, setReady] = createSignal(false);
  const [identity, setIdentity] = createSignal(0);
  const [logoutFailed, setLogoutFailed] = createSignal(false);
  let generation = 0;
  let disposed = false;
  const [frontendIdentity, setFrontendIdentity] = createSignal<FrontendPreparation | null>(null);
  let nativeIdentity: FrontendPreparation | null = null;
  const publishNativeAuth = (message: Record<string, unknown>) => {
    if (disposed) return;
    nativeIdentity?.dispose();
    nativeIdentity = nativeFrontend.publishAuth(message);
    setFrontendIdentity(message.authChanged === true && message.nativeAccess === true ? nativeIdentity : null);
  };
  onCleanup(() => {
    disposed = true;
    nativeIdentity?.dispose();
  });
  createEffect(() => {
    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- API or persisted data can omit fields despite the generated types.
    const lang = user()?.settings?.language;
    const locale: Locale = lang === "fr" || lang === "de" || lang === "es" ? lang : "en";
    setLocale(locale);
  });
  const changing = () => {
    generation++;
    publishNativeAuth({ authChanging: true });
    batch(() => {
      setIdentity(generation);
      setUser(null);
      setReady(false);
    });
    return generation;
  };
  const settle = (id: number, value: UserResponse | null) => {
    if (disposed || id !== generation) return;
    batch(() => {
      publishNativeAuth({ authChanged: true, nativeAccess: value !== null });
      setUser(value);
      setReady(true);
    });
  };
  const signalChange = () => localStorage.setItem(authEventKey, crypto.randomUUID());
  const sessionID = createMemo(() => user()?.session_id);
  const api = createMemo(() => {
    const id = identity();
    const sid = sessionID();
    return createApi(() => {
      if (id === generation) void load();
    }, sid);
  });
  const wsApi = createMemo(() => {
    const id = user()?.workspace_id;
    return id ? api().ws(id) : null;
  });
  const orgApi = createMemo(() => {
    const id = user()?.organization_id;
    return id ? api().org(id) : null;
  });
  const login = (value: UserResponse) => {
    if (disposed) return;
    setLogoutFailed(false);
    const id = changing();
    settle(id, value);
    signalChange();
  };
  const confirmLoggedOut = () => settle(changing(), null);
  const reloadIdentity = () => {
    if (disposed) return;
    changing();
    window.location.reload();
  };
  const refreshIdentity = async (signal?: AbortSignal): Promise<boolean> => {
    if (disposed) return false;
    const shown = user();
    const request = generation;
    let probe: IdentityProbe;
    try {
      const fresh = await createApiClient(async (url, init) => {
        const response = await fetch(url, { ...init, signal, cache: "no-store", credentials: "same-origin" });
        // Authentication middleware can reject before a JSON API response exists.
        if (response.status === 401) throw new APIError(401, "UNAUTHORIZED");
        return response;
      }).getMe();
      probe = { user: fresh };
    } catch (error) {
      probe = { error };
    }
    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- Callbacks and asynchronous work can change this state after narrowing.
    if (disposed || signal?.aborted || generation !== request) return false;
    const reconciliation = reconcileIdentity(shown, probe);
    if (reconciliation.status === "unavailable") throw reconciliation.error;
    if (reconciliation.status !== "current") {
      reloadIdentity();
      return false;
    }
    const sameSession = shown?.session_id === reconciliation.user.session_id;
    if (sameSession && ready()) {
      setUser(reconciliation.user);
      return true;
    }
    const id = ++generation;
    batch(() => {
      if (!sameSession && ready()) publishNativeAuth({ authChanging: true });
      setIdentity(id);
      settle(id, reconciliation.user);
    });
    return true;
  };
  const logout = async () => {
    if (disposed) return;
    setLogoutFailed(false);
    const shown = user();
    const sid = shown?.session_id;
    const id = changing();
    try {
      await createApi(undefined, sid).browserLogout();
    } catch (err) {
      console.error("Failed to revoke browser session", err);
      // Discover the current cookie without revoking a possible replacement. A
      // rejected or unreachable probe leaves no account to adopt, so settle signed
      // out locally and keep login and retry usable; the surviving provider owns the
      // visible uncertain-revoke error.
      let probe: IdentityProbe;
      try {
        probe = { user: await createApi().getMe() };
      } catch (recoveryError) {
        console.error("Failed to recover browser session", recoveryError);
        probe = { error: recoveryError };
      }
      if (id === generation) {
        settle(id, reconciledUser(reconcileIdentity(shown, probe)));
        setLogoutFailed(true);
      }
      throw err;
    }
    if (id === generation) {
      settle(id, null);
      signalChange();
    }
  };
  const refreshUser = async () => {
    if (disposed) return;
    const id = generation;
    try {
      const value = await createApi(() => {
        // Middleware can reject authentication before a JSON API response exists.
        if (id === generation) confirmLoggedOut();
      }).getMe();
      if (id !== generation) return;
      if (user()?.id !== value.id || user()?.session_id !== value.session_id) {
        const next = changing();
        settle(next, value);
      } else setUser(value);
    } catch (err) {
      if (id !== generation) return;
      if (err instanceof APIError && err.status === 401) {
        const next = changing();
        settle(next, null);
      } else console.error("Failed to refresh user", err);
    }
  };
  const load = async () => {
    if (disposed) return;
    const id = changing();
    try {
      const value = await createApi().getMe();
      settle(id, value);
    } catch (err) {
      if (!(err instanceof APIError && err.status === 401)) console.error("Failed to load user", err);
      settle(id, null);
    }
  };
  onMount(() => {
    localStorage.removeItem("mddb_token");
    const params = new URLSearchParams(window.location.search);
    if ((window.location.pathname === "/" || window.location.pathname === "/login") && params.has("token")) {
      params.delete("token");
      const q = params.toString();
      window.history.replaceState(
        {},
        document.title,
        window.location.pathname + (q ? `?${q}` : "") + window.location.hash,
      );
    }
    const storage = (event: StorageEvent) => {
      if (event.storageArea === localStorage && (event.key === authEventKey || event.key === null)) void load();
    };
    const focus = () => {
      void refreshUser();
    };
    window.addEventListener("storage", storage);
    window.addEventListener("focus", focus);
    onCleanup(() => {
      generation++;
      window.removeEventListener("storage", storage);
      window.removeEventListener("focus", focus);
    });
    void load();
  });
  const contextValue: AuthContextValue = {
    frontendIdentity,
    user,
    identity,
    ready,
    api,
    wsApi,
    orgApi,
    login,
    logout,
    reloadIdentity,
    refreshIdentity,
    setUser,
    refreshUser,
  };
  return (
    <AuthContext.Provider value={contextValue}>
      <Show when={logoutFailed()}>
        <p role="alert">{t("auth.logoutFailed")}</p>
      </Show>
      {props.children}
    </AuthContext.Provider>
  );
};
export function useAuth(): AuthContextValue {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used within an AuthProvider");
  return value;
}

/** Capture an operation's account/workspace lifetime before awaiting or scheduling work. */
export function useAuthScope(): () => () => boolean {
  const { identity, user } = useAuth();
  let disposed = false;
  onCleanup(() => {
    disposed = true;
  });
  return () => {
    const epoch = identity();
    const workspace = user()?.workspace_id;
    return () => !disposed && identity() === epoch && user()?.workspace_id === workspace;
  };
}
