// Authentication context providing user state, token management, and API clients.

import {
  createContext,
  useContext,
  createSignal,
  createMemo,
  createEffect,
  on,
  onMount,
  type ParentComponent,
  type Accessor,
} from 'solid-js';
import { createApi, APIError, type Api } from '../useApi';
import type { UserResponse } from '@sdk/types.gen';

interface AuthContextValue {
  user: Accessor<UserResponse | null>;
  token: Accessor<string | null>;
  /** True once initial auth check is complete (token validated or no token present) */
  ready: Accessor<boolean>;
  api: Accessor<Api>;
  wsApi: Accessor<ReturnType<Api['ws']> | null>;
  orgApi: Accessor<ReturnType<Api['org']> | null>;
  login: (token: string, user: UserResponse) => void;
  logout: () => Promise<void>;
  setUser: (user: UserResponse | null) => void;
  refreshUser: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue>();

export const AuthProvider: ParentComponent = (props) => {
  const [user, setUser] = createSignal<UserResponse | null>(null);
  const [token, setToken] = createSignal<string | null>(localStorage.getItem('mddb_token'));
  const [ready, setReady] = createSignal(false);

  const logout = async () => {
    const currentToken = token();
    if (currentToken) {
      try {
        const logoutApi = createApi(
          () => currentToken,
          () => {}
        );
        await logoutApi.auth.logout();
      } catch {
        // Ignore errors - proceed with local logout even if server call fails
      }
    }
    localStorage.removeItem('mddb_token');
    setToken(null);
    setUser(null);
    // Note: Navigation after logout is handled by calling components
  };

  // Create API client with auth
  const api = createMemo(() => createApi(() => token(), logout));

  // Get workspace-scoped API client
  const wsApi = createMemo(() => {
    const wsID = user()?.workspace_id;
    return wsID ? api().ws(wsID) : null;
  });

  // Get organization-scoped API client
  const orgApi = createMemo(() => {
    const orgID = user()?.organization_id;
    return orgID ? api().org(orgID) : null;
  });

  const login = (newToken: string, userData: UserResponse) => {
    localStorage.setItem('mddb_token', newToken);
    setToken(newToken);
    setUser(userData);
  };

  const refreshUser = async () => {
    try {
      const data = await api().auth.getMe();
      setUser(data);
    } catch (err) {
      console.error('Failed to refresh user', err);
      if (err instanceof APIError && err.status === 401) {
        localStorage.removeItem('mddb_token');
        setToken(null);
        setUser(null);
      }
    }
  };

  // Handle OAuth token from URL on mount and fetch user data
  onMount(async () => {
    const urlParams = new URLSearchParams(window.location.search);
    const urlToken = urlParams.get('token');
    if (urlToken) {
      localStorage.setItem('mddb_token', urlToken);
      setToken(urlToken);
      window.history.replaceState({}, document.title, window.location.pathname);
    }

    // Determine which token to use - URL token takes priority over localStorage
    // Note: We read the token directly from the sources, not from the signal,
    // because the signal update might not be visible in this synchronous context
    const effectiveToken = urlToken || localStorage.getItem('mddb_token');
    if (effectiveToken) {
      try {
        const data = await api().auth.getMe();
        setUser(data);
      } catch (err) {
        console.error('Failed to load user', err);
        if (err instanceof APIError && err.status === 401) {
          localStorage.removeItem('mddb_token');
          setToken(null);
        }
      }
    }

    // Auth check complete - mark as ready
    setReady(true);
  });

  // Fetch user when token changes after mount (e.g., login)
  // Using on() with defer: true to skip initial execution (handled in onMount)
  createEffect(
    on(
      () => [token(), user()] as const,
      ([tok, u]) => {
        if (tok && !u) {
          (async () => {
            try {
              const data = await api().auth.getMe();
              setUser(data);
            } catch (err) {
              console.error('Failed to load user', err);
              if (err instanceof APIError && err.status === 401) {
                localStorage.removeItem('mddb_token');
                setToken(null);
              }
            }
          })();
        }
      },
      { defer: true }
    )
  );

  const value: AuthContextValue = {
    user,
    token,
    ready,
    api,
    wsApi,
    orgApi,
    login,
    logout,
    setUser,
    refreshUser,
  };

  return <AuthContext.Provider value={value}>{props.children}</AuthContext.Provider>;
};

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}
