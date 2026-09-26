// Builds the model-facing notice injected when a watched MCP resource changes.

import type { McpResourceNotification } from "@maruel/gomode/web/McpClient";

/**
 * Returns the instruction the voice model receives when a watched resource
 * changes, or null when nothing should be sent.
 *
 * The notice is addressed to the model rather than the user, so it stays in
 * English instead of following the interface language. A resource update is the
 * case that matters: the model otherwise keeps writing against the copy it read
 * before the change and can overwrite newer content.
 */
export function voiceResourceNotice(notification: McpResourceNotification): string | null {
  if (notification.uri !== undefined) {
    return "A document you are working on changed elsewhere. Re-read it before editing so you do not overwrite newer content.";
  }
  if (notification.listChanged) {
    return "The workspace changed: documents were added, removed, or moved. Relist or re-read before acting.";
  }
  return null;
}
