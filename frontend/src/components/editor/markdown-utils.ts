// Utility functions for asset URL handling in markdown content.

import type { AssetUrlMap } from '../../contexts/EditorContext';

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
