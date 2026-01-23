import { createAPIClient, APIError, type FetchFn } from './api.gen';

export { APIError };

/** Default retry configuration */
const RETRY_CONFIG = {
  maxRetries: 3,
  baseDelayMs: 1000,
  maxDelayMs: 30000,
};

/** Checks if the status code should trigger a retry */
function isRetryableStatus(status: number): boolean {
  return status === 429 || (status >= 500 && status < 600);
}

/** Parses Retry-After header value (seconds or HTTP date) */
function parseRetryAfter(header: string | null): number | null {
  if (!header) return null;
  // Try parsing as seconds
  const seconds = parseInt(header, 10);
  if (!isNaN(seconds)) {
    return seconds * 1000;
  }
  // Try parsing as HTTP date
  const date = Date.parse(header);
  if (!isNaN(date)) {
    return Math.max(0, date - Date.now());
  }
  return null;
}

/** Calculates delay for next retry with exponential backoff */
function getRetryDelay(attempt: number, retryAfterMs: number | null): number {
  if (retryAfterMs !== null) {
    return Math.min(retryAfterMs, RETRY_CONFIG.maxDelayMs);
  }
  // Exponential backoff: 1s, 2s, 4s...
  const delay = RETRY_CONFIG.baseDelayMs * Math.pow(2, attempt);
  return Math.min(delay, RETRY_CONFIG.maxDelayMs);
}

/** Wraps a fetch function with retry logic for 429 and 5xx errors */
function withRetry(fetchFn: FetchFn): FetchFn {
  return async (url: string, init?: RequestInit): Promise<Response> => {
    let attempt = 0;
    for (;;) {
      const response = await fetchFn(url, init);
      if (!isRetryableStatus(response.status) || attempt >= RETRY_CONFIG.maxRetries) {
        return response;
      }
      const retryAfterMs = parseRetryAfter(response.headers.get('Retry-After'));
      const delay = getRetryDelay(attempt, retryAfterMs);
      await new Promise((resolve) => setTimeout(resolve, delay));
      attempt++;
    }
  };
}

/**
 * Creates an authenticated fetch function for use with the API client.
 * @param getToken - Function that returns the current auth token
 * @param onUnauthorized - Callback when a 401 response is received
 */
export function createAuthFetch(getToken: () => string | null, onUnauthorized?: () => void): FetchFn {
  const baseFetch: FetchFn = async (url: string, init?: RequestInit) => {
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
  return withRetry(baseFetch);
}

/**
 * Creates an API client with authentication.
 * @param getToken - Function that returns the current auth token
 * @param onUnauthorized - Callback when a 401 response is received
 */
export function createApi(getToken: () => string | null, onUnauthorized?: () => void) {
  return createAPIClient(createAuthFetch(getToken, onUnauthorized));
}
