// Utility functions for asset URL handling in markdown content.

import type { AssetUrlMap } from '../../contexts/EditorContext';

/**
 * Rewrite asset URLs from local filenames to signed URLs.
 * Transforms: ![alt](filename.png) -> ![alt](https://...signed-url...)
 * Uses the asset URL map from the backend which contains signed URLs.
 */
export function rewriteAssetUrls(markdown: string, assetUrls: AssetUrlMap): string {
  if (!assetUrls || Object.keys(assetUrls).length === 0) return markdown;

  // Match markdown images with local filenames (not URLs or absolute paths)
  // Matches: ![...](filename.ext) but not ![...](http...) or ![...](/...)
  return markdown.replace(/!\[([^\]]*)\]\(([^/)][^)]*)\)/g, (match, alt, src) => {
    // Skip if it looks like a URL (contains ://)
    if (src.includes('://')) return match;
    // Skip if it starts with / (absolute path)
    if (src.startsWith('/')) return match;
    // Look up signed URL from asset map
    const signedUrl = assetUrls[src];
    if (signedUrl) {
      return `![${alt}](${signedUrl})`;
    }
    // Keep original if no signed URL found
    return match;
  });
}

/**
 * Reverse asset URL rewriting from signed URLs to local filenames.
 * Transforms: ![alt](https://...signed-url...) -> ![alt](filename.png)
 * Used when saving content back to the backend.
 */
export function reverseRewriteAssetUrls(markdown: string, assetUrls: AssetUrlMap): string {
  if (!assetUrls || Object.keys(assetUrls).length === 0) return markdown;

  // Build reverse map: signed URL -> filename
  const reverseMap: Record<string, string> = {};
  for (const [filename, url] of Object.entries(assetUrls)) {
    reverseMap[url] = filename;
  }

  // Match markdown images with signed URLs
  return markdown.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, (match, alt, src) => {
    const filename = reverseMap[src];
    if (filename) {
      return `![${alt}](${filename})`;
    }
    return match;
  });
}
