// Signals settled cookie identity and transitions to the trusted Go Mode Android main frame.

declare global {
  interface Window {
    goModeHost?: unknown;
    gomodeAuth?: { postMessage(message: string): void };
  }
}
export function isGoModeHost(): boolean {
  return window.goModeHost !== undefined || new URLSearchParams(window.location.search).get("goModeHost") === "1";
}
export function publishGoModeAuthChanging(): void {
  if (isGoModeHost()) window.gomodeAuth?.postMessage(JSON.stringify({ authChanging: true }));
}
export function publishGoModeAuthChanged(nativeAccess: boolean): void {
  if (isGoModeHost()) window.gomodeAuth?.postMessage(JSON.stringify({ authChanged: true, nativeAccess }));
}
