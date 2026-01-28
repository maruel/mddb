// Authentication context providing user state, token management, and API clients.

import {
  createContext,
  useContext,
  createSignal,
  createMemo,
  createEffect,
  onMount,
  type ParentComponent,
  type Accessor,
} from 'solid-js';
import { createApi, APIError, type Api } from '../useApi';
import type { UserResponse } from '@sdk/types.gen';

interface AuthContextValue {
  user: Accessor<UserResponse | null>;
  token: Accessor<string | null>;
  api: Accessor<Api>;
  wsApi: Accessor<ReturnType<Api['ws']> | null>;
  login: (token: string, user: UserResponse) => void;
  logout: () => Promise<void>;
  setUser: (user: UserResponse | null) => void;
  refreshUser: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue>();

export const AuthProvider: ParentComponent = (props) => {
  const [user, setUser] = createSignal<UserResponse | null>(null);
  const [token, setToken] = createSignal<string | null>(localStorage.getItem('mddb_token'));

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
    window.history.pushState(null, '', '/');
  };

  // Create API client with auth
  const api = createMemo(() => createApi(() => token(), logout));

  // Get workspace-scoped API client
  const wsApi = createMemo(() => {
    const wsID = user()?.workspace_id;
    return wsID ? api().ws(wsID) : null;
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

  // Handle OAuth token from URL on mount
  onMount(() => {
    const urlParams = new URLSearchParams(window.location.search);
    const urlToken = urlParams.get('token');
    if (urlToken) {
      localStorage.setItem('mddb_token', urlToken);
      setToken(urlToken);
      window.history.replaceState({}, document.title, window.location.pathname);
    }
  });

  // Fetch user when we have a token but no user data
  let fetchingUser = false;
  createEffect(() => {
    const tok = token();
    const u = user();
    if (tok && !u && !fetchingUser) {
      fetchingUser = true;
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
        } finally {
          fetchingUser = false;
        }
      })();
    }
  });

  const value: AuthContextValue = {
    user,
    token,
    api,
    wsApi,
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
