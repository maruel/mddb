// Utility functions for asset URL handling in markdown content.

/**
 * Rewrite asset URLs from relative paths to org-scoped absolute paths.
 * Transforms: ![alt](assets/...) -> ![alt](/assets/{orgId}/...)
 * Also handles img src attributes for HTML in markdown.
 */
export function rewriteAssetUrls(markdown: string, orgId: string | undefined): string {
  if (!orgId) return markdown;
  // Handle markdown image syntax: ![alt](assets/...)
  let result = markdown.replace(/\]\(assets\//g, `](/assets/${orgId}/`);
  // Handle HTML img src: src="assets/..."
  result = result.replace(/src="assets\//g, `src="/assets/${orgId}/`);
  return result;
}

/**
 * Reverse asset URL rewriting from org-scoped absolute paths to relative paths.
 * Transforms: ![alt](/assets/{orgId}/...) -> ![alt](assets/...)
 */
export function reverseRewriteAssetUrls(markdown: string, orgId: string | undefined): string {
  if (!orgId) return markdown;
  const pattern = new RegExp(`/assets/${orgId}/`, 'g');
  return markdown.replace(pattern, 'assets/');
}
