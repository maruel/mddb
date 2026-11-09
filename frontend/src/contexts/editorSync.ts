// Classifies canonical page revisions and metadata timestamps without discarding local drafts.

/** How the editor should react to a node.updated event for the open page. */
export type ExternalUpdateDecision = "ignore" | "reload" | "conflict";

/** Distinguish content revisions even when persisted timestamps share a second. */
export function classifyExternalUpdate(
  eventModified: number,
  syncedModified: number | null,
  localWorkPending: boolean,
  eventRevision: string | null,
  syncedRevision: string | null,
): ExternalUpdateDecision {
  if (eventRevision !== null && syncedRevision !== null) {
    if (eventRevision === syncedRevision) return "ignore";
    return localWorkPending ? "conflict" : "reload";
  }
  if (syncedModified !== null && eventModified <= syncedModified) return "ignore";
  return localWorkPending ? "conflict" : "reload";
}
