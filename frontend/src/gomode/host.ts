// Detects Go Mode's native shell and explicit hosted browser mode.

declare global {
  interface Window {
    goModeHost?: unknown;
  }
}

export function isGoModeHost(): boolean {
  return window.goModeHost !== undefined || new URLSearchParams(window.location.search).get("goModeHost") === "1";
}
