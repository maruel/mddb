// Utility functions for extracting information from markdown content.

/** Map of node ID to title for resolving internal page links */
export type NodeTitleMap = Record<string, string>;

// --- Internal Page Link Utilities ---

/**
 * Pattern to match internal page links: [text](/w/{wsId}+{slug}/{nodeId}+{slug})
 * Captures: group 1 = text, group 2 = wsId, group 3 = nodeId
 */
const INTERNAL_LINK_PATTERN = /\[([^\]]*)\]\(\/w\/([^/+]+)(?:\+[^/]*)?\/([A-Za-z0-9]+)(?:\+[^)]*)?\)/g;

/**
 * Extract all linked node IDs from internal page links in markdown content.
 * Only extracts links that belong to the current workspace.
 *
 * @param markdown The markdown content to parse
 * @param currentWsId The current workspace ID to filter links by
 * @returns Array of unique node IDs referenced in the content
 */
export function extractLinkedNodeIds(markdown: string, currentWsId: string): string[] {
  if (!markdown || !currentWsId) return [];

  const seen = new Set<string>();
  const pattern = new RegExp(INTERNAL_LINK_PATTERN.source, 'g');
  let match;

  while ((match = pattern.exec(markdown)) !== null) {
    const wsId = match[2];
    const nodeId = match[3];
    // Only include links to the current workspace
    if (wsId === currentWsId && nodeId && !seen.has(nodeId)) {
      seen.add(nodeId);
    }
  }

  return Array.from(seen);
}
