// Detects the Go Mode Android shell and sends its trusted main frame an in-memory bearer token.

declare global {
  interface Window {
    goModeHost?: unknown;
    gomodeAuth?: { postMessage(message: string): void };
  }
}

export function isGoModeHost(): boolean {
  return window.goModeHost !== undefined || new URLSearchParams(window.location.search).get("goModeHost") === "1";
}

export function publishGoModeBearerToken(token: string | null): void {
  if (!isGoModeHost()) return;
  window.gomodeAuth?.postMessage(JSON.stringify({ bearerToken: token }));
}
