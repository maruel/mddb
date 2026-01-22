import { createAPIClient, APIError, type FetchFn } from './api.gen';

export { APIError };

/**
 * Creates an authenticated fetch function for use with the API client.
 * @param getToken - Function that returns the current auth token
 * @param onUnauthorized - Callback when a 401 response is received
 */
export function createAuthFetch(getToken: () => string | null, onUnauthorized?: () => void): FetchFn {
  return async (url: string, init?: RequestInit) => {
    const token = getToken();
    const headers: HeadersInit = {
      ...init?.headers,
    };
    if (token) {
      (headers as Record<string, string>)['Authorization'] = `Bearer ${token}`;
    }
    const res = await fetch(url, { ...init, headers });
    if (res.status === 401 && onUnauthorized) {
      onUnauthorized();
    }
    return res;
  };
}

/**
 * Creates an API client with authentication.
 * @param getToken - Function that returns the current auth token
 * @param onUnauthorized - Callback when a 401 response is received
 */
export function createApi(getToken: () => string | null, onUnauthorized?: () => void) {
  return createAPIClient(createAuthFetch(getToken, onUnauthorized));
}
