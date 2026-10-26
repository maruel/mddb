// Utilities for creating authenticated and retry-enabled API clients.

import { createApiClient, APIError, type FetchFn } from "@sdk/api.gen";

export { APIError };

// Keep recent UI writes so SSE echoes are recognized before and after their HTTP
// responses. This is bounded per document and does not register MCP requests.
const localWriteRequestIDs = new Set<string>();
const MAX_LOCAL_WRITE_REQUESTS = 1024;

/** Registers one UI write and attaches its echo-correlation ID to the headers. */
export function localWriteHeaders(input: HeadersInit | undefined): Headers {
  const id = Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) => byte.toString(16).padStart(2, "0")).join(
    "",
  );
  localWriteRequestIDs.add(id);
  if (localWriteRequestIDs.size > MAX_LOCAL_WRITE_REQUESTS) {
    const oldest = localWriteRequestIDs.values().next().value;
    if (oldest !== undefined) localWriteRequestIDs.delete(oldest);
  }
  const headers = new Headers(input);
  headers.set("X-Request-ID", id);
  return headers;
}

/** Reports whether this document's UI owns the write identified by an event. */
export function isLocalWriteRequest(id: string | undefined): boolean {
  return id !== undefined && localWriteRequestIDs.has(id);
}

/** Type alias for the API client returned by createApi */
export type Api = ReturnType<typeof createApi>;

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
      const retryAfterMs = parseRetryAfter(response.headers.get("Retry-After"));
      const delay = getRetryDelay(attempt, retryAfterMs);
      await new Promise((resolve) => setTimeout(resolve, delay));
      attempt++;
    }
  };
}

/**
 * Creates an authenticated fetch function for use with the API client.
 * @param onUnauthorized - Callback when a 401 response is received
 */
export function createAuthFetch(onUnauthorized?: () => void, expectedSession?: string): FetchFn {
  const baseFetch: FetchFn = async (url, init) => {
    const headers = new Headers(init?.headers);
    if (expectedSession) headers.set("X-MDDB-Session", expectedSession);
    const response = await fetch(url, { ...init, headers, credentials: "same-origin", cache: "no-store" });
    if (response.status === 401) onUnauthorized?.();
    return response;
  };
  return withRetry(baseFetch);
}

/**
 * Creates an API client with authentication.
 * @param onUnauthorized - Callback when a 401 response is received
 */
export function createApi(onUnauthorized?: () => void, expectedSession?: string) {
  const authFetch = createAuthFetch(onUnauthorized, expectedSession);
  return createApiClient((url, init) => {
    // Only the generated UI mutation methods own optimistic updates. Generic
    // authenticated fetches (including MCP transports) must remain observable.
    if (init?.method && ["DELETE", "PATCH", "POST", "PUT"].includes(init.method.toUpperCase())) {
      return authFetch(url, { ...init, headers: localWriteHeaders(init.headers) });
    }
    return authFetch(url, init);
  });
}
