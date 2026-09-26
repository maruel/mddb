// Classifies workspace SSE events for the page the editor has open.

/** How the editor should react to a node.updated event for the open page. */
export type ExternalUpdateDecision = "ignore" | "reload" | "conflict";

/**
 * Classifies a node.updated event for the page the editor has open.
 *
 * The editor stores the revision it already holds, so the SSE echo of its own
 * write (eventModified <= syncedModified) is ignored instead of prompting the
 * user. A newer revision reloads the page when the editor has nothing unsaved,
 * and otherwise reports a conflict so a reload does not discard local edits.
 */
export function classifyExternalUpdate(
  eventModified: number,
  syncedModified: number | null,
  hasUnsavedChanges: boolean,
): ExternalUpdateDecision {
  if (syncedModified !== null && eventModified <= syncedModified) return "ignore";
  return hasUnsavedChanges ? "conflict" : "reload";
}
