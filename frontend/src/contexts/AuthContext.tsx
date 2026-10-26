// Owns cookie account state and native identity transitions without browser-readable credentials.

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
import type { UserResponse } from "@sdk/types.gen";
import { publishGoModeAuthChanging, publishGoModeAuthChanged } from "../gomode/host";
import { useI18n, type Locale } from "../i18n";

export interface AuthContextValue {
  user: Accessor<UserResponse | null>;
  identity: Accessor<number>;
  ready: Accessor<boolean>;
  api: Accessor<Api>;
  wsApi: Accessor<ReturnType<Api["ws"]> | null>;
  orgApi: Accessor<ReturnType<Api["org"]> | null>;
  login: (user: UserResponse) => void;
  logout: () => Promise<void>;
  /** Invalidate native identity before rediscovering the authoritative cookie. */
  reloadIdentity: () => void;
  setUser: (user: UserResponse | null) => void;
  refreshUser: () => Promise<void>;
}
export const AuthContext = createContext<AuthContextValue>();
const authEventKey = "mddb_auth_changed";

export const AuthProvider: ParentComponent = (props) => {
  const { setLocale, t } = useI18n();
  const [user, setUser] = createSignal<UserResponse | null>(null);
  const [ready, setReady] = createSignal(false);
  const [identity, setIdentity] = createSignal(0);
  const [logoutFailed, setLogoutFailed] = createSignal(false);
  let generation = 0;
  createEffect(() => {
    const lang = user()?.settings?.language;
    const locale: Locale = lang === "fr" || lang === "de" || lang === "es" ? lang : "en";
    setLocale(locale);
  });
  const changing = () => {
    generation++;
    publishGoModeAuthChanging();
    batch(() => {
      setIdentity(generation);
      setUser(null);
      setReady(false);
    });
    return generation;
  };
  const settle = (id: number, value: UserResponse | null) => {
    if (id !== generation) return;
    batch(() => {
      setUser(value);
      setReady(true);
    });
    publishGoModeAuthChanged(value !== null);
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
    setLogoutFailed(false);
    const id = changing();
    settle(id, value);
    signalChange();
  };
  const confirmLoggedOut = () => settle(changing(), null);
  const reloadIdentity = () => {
    changing();
    window.location.reload();
  };
  const logout = async () => {
    setLogoutFailed(false);
    const sid = user()?.session_id;
    const id = changing();
    try {
      await createApi(undefined, sid).browserLogout();
    } catch (err) {
      console.error("Failed to revoke browser session", err);
      // Discover the current cookie without revoking a possible replacement.
      // If recovery also fails, settle signed out locally so login/retry stays
      // usable; the surviving provider owns the visible uncertain-revoke error.
      let recovered: UserResponse | null = null;
      try {
        recovered = await createApi().getMe();
      } catch (recoveryError) {
        console.error("Failed to recover browser session", recoveryError);
      }
      if (id === generation) {
        settle(id, recovered);
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
    user,
    identity,
    ready,
    api,
    wsApi,
    orgApi,
    login,
    logout,
    reloadIdentity,
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
