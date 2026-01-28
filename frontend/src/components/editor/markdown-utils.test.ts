// Unit tests for markdown utility functions.

import { describe, it, expect } from 'vitest';
import { rewriteAssetUrls, reverseRewriteAssetUrls } from './markdown-utils';

describe('rewriteAssetUrls', () => {
  it('rewrites markdown image paths to org-scoped paths', () => {
    const markdown = '![alt](assets/image.png)';
    expect(rewriteAssetUrls(markdown, 'org123')).toBe('![alt](/assets/org123/image.png)');
  });

  it('rewrites HTML img src paths', () => {
    const html = '<img src="assets/image.png">';
    expect(rewriteAssetUrls(html, 'org123')).toBe('<img src="/assets/org123/image.png">');
  });

  it('rewrites multiple asset paths', () => {
    const markdown = '![a](assets/a.png) ![b](assets/b.png)';
    const expected = '![a](/assets/org123/a.png) ![b](/assets/org123/b.png)';
    expect(rewriteAssetUrls(markdown, 'org123')).toBe(expected);
  });

  it('does not modify non-asset paths', () => {
    const markdown = '![alt](https://example.com/image.png)';
    expect(rewriteAssetUrls(markdown, 'org123')).toBe(markdown);
  });

  it('returns unchanged if orgId is undefined', () => {
    const markdown = '![alt](assets/image.png)';
    expect(rewriteAssetUrls(markdown, undefined)).toBe(markdown);
  });

  it('returns unchanged if orgId is empty string', () => {
    const markdown = '![alt](assets/image.png)';
    expect(rewriteAssetUrls(markdown, '')).toBe(markdown);
  });

  it('handles nested paths', () => {
    const markdown = '![alt](assets/folder/subfolder/image.png)';
    expect(rewriteAssetUrls(markdown, 'org123')).toBe('![alt](/assets/org123/folder/subfolder/image.png)');
  });
});

describe('reverseRewriteAssetUrls', () => {
  it('converts org-scoped paths back to relative paths', () => {
    const markdown = '![alt](/assets/org123/image.png)';
    expect(reverseRewriteAssetUrls(markdown, 'org123')).toBe('![alt](assets/image.png)');
  });

  it('converts multiple paths', () => {
    const markdown = '![a](/assets/org123/a.png) ![b](/assets/org123/b.png)';
    expect(reverseRewriteAssetUrls(markdown, 'org123')).toBe('![a](assets/a.png) ![b](assets/b.png)');
  });

  it('does not modify paths for other orgs', () => {
    const markdown = '![alt](/assets/other-org/image.png)';
    expect(reverseRewriteAssetUrls(markdown, 'org123')).toBe(markdown);
  });

  it('returns unchanged if orgId is undefined', () => {
    const markdown = '![alt](/assets/org123/image.png)';
    expect(reverseRewriteAssetUrls(markdown, undefined)).toBe(markdown);
  });

  it('returns unchanged if orgId is empty string', () => {
    const markdown = '![alt](/assets/org123/image.png)';
    expect(reverseRewriteAssetUrls(markdown, '')).toBe(markdown);
  });

  it('handles nested paths', () => {
    const markdown = '![alt](/assets/org123/folder/subfolder/image.png)';
    expect(reverseRewriteAssetUrls(markdown, 'org123')).toBe('![alt](assets/folder/subfolder/image.png)');
  });
});

describe('round-trip asset URL handling', () => {
  it('asset URLs survive rewrite and reverse', () => {
    const original = '![alt](assets/image.png)';
    const rewritten = rewriteAssetUrls(original, 'org123');
    expect(rewritten).toBe('![alt](/assets/org123/image.png)');
    const reversed = reverseRewriteAssetUrls(rewritten, 'org123');
    expect(reversed).toBe(original);
  });

  it('multiple assets survive round-trip', () => {
    const original = '![a](assets/a.png)\n\n![b](assets/b.png)';
    const rewritten = rewriteAssetUrls(original, 'org123');
    const reversed = reverseRewriteAssetUrls(rewritten, 'org123');
    expect(reversed).toBe(original);
  });
});
