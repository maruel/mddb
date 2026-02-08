// Utility functions for extracting information from markdown content.

/** Map of node ID to title for resolving internal page links */
export type NodeTitleMap = Record<string, string>;

// --- Internal Page Link Utilities ---

/**
 * Pattern to match relative file path links: [text](../nodeId/index.md) or [text](nodeId/index.md)
 * Captures: group 1 = text, group 2 = relative href ending in /index.md
 */
const RELATIVE_LINK_PATTERN = /\[([^\]]*)\]\(([^)]+\/index\.md)\)/g;

/**
 * Convert relative file path links to SPA URLs.
 * Called on the read path: API content (disk format) -> frontend SPA URLs.
 *
 * Extracts node IDs from directory names in paths like ../nodeId/index.md
 * and constructs /w/@wsId/@nodeId URLs.
 *
 * @param content Markdown content with relative file path links
 * @param wsId Current workspace ID
 * @returns Content with relative paths replaced by SPA URLs
 */
export function relativeLinksToSpaUrls(content: string, wsId: string): string {
  if (!content || !wsId || !content.includes('/index.md)')) return content;

  return content.replace(RELATIVE_LINK_PATTERN, (match, text, href) => {
    // Skip absolute or external links.
    if (href.startsWith('/') || href.startsWith('http')) return match;

    // Extract nodeId from directory name: "../nodeId/index.md" -> "nodeId"
    const lastSlash = href.lastIndexOf('/');
    if (lastSlash < 0) return match;
    const dir = href.substring(0, lastSlash);
    const secondLastSlash = dir.lastIndexOf('/');
    const nodeId = secondLastSlash >= 0 ? dir.substring(secondLastSlash + 1) : dir;

    // Validate: node IDs are uppercase alphanumeric (base32)
    if (!nodeId || !/^[A-Z0-9]+$/.test(nodeId)) return match;

    return `[${text}](/w/@${wsId}/@${nodeId})`;
  });
}

/**
 * Pattern to match SPA URL links: [text](/w/@wsId+slug/@nodeId+slug)
 * Captures: group 1 = text, group 2 = wsId, group 3 = nodeId
 */
const SPA_LINK_PATTERN = /\[([^\]]*)\]\(\/w\/@([^/+]+)(?:\+[^/]*)?\/@([A-Za-z0-9]+)(?:\+[^)]*)?\)/g;

/**
 * Convert SPA URL links to relative file path links.
 * Called on the write path: frontend SPA URLs -> API (disk format).
 *
 * Converts /w/@wsId/@nodeId links to ../nodeId/index.md format.
 * Cross-workspace links (different wsId) are left unchanged.
 *
 * @param content Markdown content with SPA URL links
 * @param wsId Current workspace ID
 * @returns Content with SPA URLs replaced by relative file paths
 */
export function spaUrlsToRelativeLinks(content: string, wsId: string): string {
  if (!content || !wsId || !content.includes('/w/@')) return content;

  return content.replace(new RegExp(SPA_LINK_PATTERN.source, 'g'), (match, text, linkWsId, nodeId) => {
    // Skip cross-workspace links.
    if (linkWsId !== wsId) return match;
    return `[${text}](../${nodeId}/index.md)`;
  });
}

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
  const pattern = new RegExp(SPA_LINK_PATTERN.source, 'g');
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
