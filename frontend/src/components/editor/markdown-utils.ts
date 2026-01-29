// Utility functions for asset URL handling in markdown content.

import type { AssetUrlMap } from '../../contexts/EditorContext';

/** Map of node ID to title for resolving internal page links */
export type NodeTitleMap = Record<string, string>;

/**
 * Check if a source looks like a local filename (not a URL or absolute path).
 */
function isLocalFilename(src: string): boolean {
  // Skip if it looks like a URL (contains ://)
  if (src.includes('://')) return false;
  // Skip if it starts with / (absolute path)
  if (src.startsWith('/')) return false;
  return true;
}

/**
 * Rewrite asset URLs from local filenames to signed URLs.
 * Transforms: ![alt](filename.png) -> ![alt](/assets/.../filename.png?sig=...)
 * Also transforms: [text](filename.pdf) -> [text](/assets/.../filename.pdf?sig=...)
 * Uses the asset URL map from the backend which contains signed URLs.
 */
export function rewriteAssetUrls(markdown: string, assetUrls: AssetUrlMap): string {
  if (!assetUrls || Object.keys(assetUrls).length === 0) return markdown;

  // Match markdown images with local filenames (not URLs or absolute paths)
  // Matches: ![...](filename.ext) but not ![...](http...) or ![...](/...)
  let result = markdown.replace(/!\[([^\]]*)\]\(([^/)][^)]*)\)/g, (match, alt, src) => {
    if (!isLocalFilename(src)) return match;
    // Look up signed URL from asset map
    const signedUrl = assetUrls[src];
    if (signedUrl) {
      return `![${alt}](${signedUrl})`;
    }
    // Keep original if no signed URL found
    return match;
  });

  // Match markdown links with local filenames that are in the asset map
  // This handles linked files like PDFs
  result = result.replace(/\[([^\]]+)\]\(([^/)][^)]*)\)/g, (match, text, href) => {
    if (!isLocalFilename(href)) return match;
    // Only convert if this filename is in the asset map
    const signedUrl = assetUrls[href];
    if (signedUrl) {
      return `[${text}](${signedUrl})`;
    }
    return match;
  });

  return result;
}

// Regex to extract filename from signed asset URLs: /assets/{wsID}/{nodeID}/{filename}?...
const ASSET_URL_PATTERN = /^\/assets\/[^/]+\/[^/]+\/([^?]+)/;

/**
 * Extract filename from a signed asset URL.
 * Returns the filename or null if the URL doesn't match the pattern.
 */
function extractFilenameFromAssetUrl(url: string): string | null {
  const match = url.match(ASSET_URL_PATTERN);
  if (match && match[1]) {
    // Decode URL-encoded characters (e.g., %20 -> space)
    return decodeURIComponent(match[1]);
  }
  return null;
}

/**
 * Reverse asset URL rewriting from signed URLs to local filenames.
 * Transforms: ![alt](/assets/wsId/nodeId/filename.png?sig=...) -> ![alt](filename.png)
 * Used when saving content back to the backend.
 *
 * Handles both:
 * 1. URLs in the assetUrls map (exact match)
 * 2. Signed asset URLs matching /assets/{wsID}/{nodeID}/{filename}?... pattern
 */
export function reverseRewriteAssetUrls(markdown: string, assetUrls: AssetUrlMap): string {
  // Build reverse map: signed URL -> filename
  const reverseMap: Record<string, string> = {};
  for (const [filename, url] of Object.entries(assetUrls || {})) {
    reverseMap[url] = filename;
  }

  // Helper to convert a URL to filename
  const urlToFilename = (url: string): string | null => {
    // First try exact match in reverse map
    if (reverseMap[url]) {
      return reverseMap[url];
    }
    // Then try extracting from signed asset URL pattern
    return extractFilenameFromAssetUrl(url);
  };

  // Match markdown images: ![alt](url)
  let result = markdown.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, (match, alt, src) => {
    const filename = urlToFilename(src);
    if (filename) {
      return `![${alt}](${filename})`;
    }
    return match;
  });

  // Match markdown links: [text](url)
  // Only convert links that point to signed asset URLs
  result = result.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (match, text, href) => {
    // Only process asset URLs (not regular links)
    const filename = extractFilenameFromAssetUrl(href);
    if (filename) {
      return `[${text}](${filename})`;
    }
    return match;
  });

  return result;
}

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

/**
 * Rewrite internal page link display text to show current titles.
 * Transforms: [OldTitle](/w/{wsId}+{slug}/{nodeId}+{slug}) -> [CurrentTitle](/w/{wsId}+{slug}/{nodeId}+{slug})
 *
 * The URL is preserved exactly as-is; only the display text is updated.
 * Links to nodes not in the title map are left unchanged.
 *
 * @param markdown The markdown content to transform
 * @param nodeTitles Map of node ID to current title
 * @param currentWsId The current workspace ID to filter links by
 * @returns Markdown with link display text updated to current titles
 */
export function rewriteInternalLinkTitles(markdown: string, nodeTitles: NodeTitleMap, currentWsId: string): string {
  if (!markdown || !currentWsId || !nodeTitles || Object.keys(nodeTitles).length === 0) {
    return markdown;
  }

  // Pattern captures: full match, text, wsId, nodeId (and anything after)
  return markdown.replace(
    /\[([^\]]*)\]\((\/w\/([^/+]+)(?:\+[^/]*)?\/([A-Za-z0-9]+)(?:\+[^)]*)?)\)/g,
    (match, _text, url, wsId, nodeId) => {
      // Only rewrite links to the current workspace
      if (wsId !== currentWsId) {
        return match;
      }
      // Look up the current title
      const currentTitle = nodeTitles[nodeId];
      if (currentTitle !== undefined) {
        return `[${currentTitle}](${url})`;
      }
      // Keep original if no title found (page might be deleted)
      return match;
    }
  );
}

/**
 * Reverse of rewriteInternalLinkTitles - used when saving content.
 * Since we don't modify the markdown when saving (titles stay in storage),
 * this function is provided for consistency but currently just returns the input.
 *
 * The design approach is:
 * - Storage: markdown keeps whatever title was there when the link was created
 * - Display: titles are resolved at render time from the title map
 * - Save: original markdown is preserved (no rewriting in storage)
 */
export function reverseInternalLinkTitles(markdown: string): string {
  // Currently a no-op: we preserve original markdown on save
  return markdown;
}
