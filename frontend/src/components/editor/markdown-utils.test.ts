// Unit tests for markdown utility functions.

import { describe, it, expect } from 'vitest';
import { rewriteAssetUrls, reverseRewriteAssetUrls } from './markdown-utils';
import type { AssetUrlMap } from '../../contexts/EditorContext';

describe('rewriteAssetUrls', () => {
  const assetUrls: AssetUrlMap = {
    'image.png': 'https://example.com/assets/ws/node/image.png?sig=abc&exp=123',
    'photo.jpg': 'https://example.com/assets/ws/node/photo.jpg?sig=def&exp=456',
    'abc123-image.png': 'https://example.com/assets/ws/node/abc123-image.png?sig=xyz&exp=789',
  };

  it('rewrites local image filenames to signed URLs', () => {
    const markdown = '![alt](image.png)';
    expect(rewriteAssetUrls(markdown, assetUrls)).toBe(
      '![alt](https://example.com/assets/ws/node/image.png?sig=abc&exp=123)'
    );
  });

  it('rewrites multiple local images', () => {
    const markdown = '![a](image.png) ![b](photo.jpg)';
    const expected =
      '![a](https://example.com/assets/ws/node/image.png?sig=abc&exp=123) ![b](https://example.com/assets/ws/node/photo.jpg?sig=def&exp=456)';
    expect(rewriteAssetUrls(markdown, assetUrls)).toBe(expected);
  });

  it('does not modify external URLs', () => {
    const markdown = '![alt](https://cdn.example.com/image.png)';
    expect(rewriteAssetUrls(markdown, assetUrls)).toBe(markdown);
  });

  it('does not modify absolute paths', () => {
    const markdown = '![alt](/some/path/image.png)';
    expect(rewriteAssetUrls(markdown, assetUrls)).toBe(markdown);
  });

  it('returns unchanged if assetUrls is empty', () => {
    const markdown = '![alt](image.png)';
    expect(rewriteAssetUrls(markdown, {})).toBe(markdown);
  });

  it('keeps filename if not in asset map', () => {
    const markdown = '![alt](unknown.png)';
    expect(rewriteAssetUrls(markdown, assetUrls)).toBe(markdown);
  });

  it('handles filenames with hash prefixes', () => {
    const markdown = '![alt](abc123-image.png)';
    expect(rewriteAssetUrls(markdown, assetUrls)).toBe(
      '![alt](https://example.com/assets/ws/node/abc123-image.png?sig=xyz&exp=789)'
    );
  });

  it('preserves alt text', () => {
    const markdown = '![My Image Alt](image.png)';
    expect(rewriteAssetUrls(markdown, assetUrls)).toBe(
      '![My Image Alt](https://example.com/assets/ws/node/image.png?sig=abc&exp=123)'
    );
  });

  it('rewrites links to asset filenames (for PDFs)', () => {
    const urls: AssetUrlMap = {
      'report.pdf': '/assets/ws/node/report.pdf?sig=abc&exp=123',
    };
    const markdown = '[Download Report](report.pdf)';
    expect(rewriteAssetUrls(markdown, urls)).toBe('[Download Report](/assets/ws/node/report.pdf?sig=abc&exp=123)');
  });

  it('does not modify links to non-asset files', () => {
    const markdown = '[link](other.pdf)';
    expect(rewriteAssetUrls(markdown, assetUrls)).toBe(markdown);
  });

  it('does not modify external links', () => {
    const urls: AssetUrlMap = {
      'file.pdf': '/assets/ws/node/file.pdf?sig=abc&exp=123',
    };
    const markdown = '[link](https://example.com/file.pdf)';
    expect(rewriteAssetUrls(markdown, urls)).toBe(markdown);
  });
});

describe('reverseRewriteAssetUrls', () => {
  const assetUrls: AssetUrlMap = {
    'image.png': 'https://example.com/assets/ws/node/image.png?sig=abc&exp=123',
    'photo.jpg': 'https://example.com/assets/ws/node/photo.jpg?sig=def&exp=456',
  };

  it('converts signed URLs back to filenames using asset map', () => {
    const markdown = '![alt](https://example.com/assets/ws/node/image.png?sig=abc&exp=123)';
    expect(reverseRewriteAssetUrls(markdown, assetUrls)).toBe('![alt](image.png)');
  });

  it('converts multiple URLs', () => {
    const markdown =
      '![a](https://example.com/assets/ws/node/image.png?sig=abc&exp=123) ![b](https://example.com/assets/ws/node/photo.jpg?sig=def&exp=456)';
    expect(reverseRewriteAssetUrls(markdown, assetUrls)).toBe('![a](image.png) ![b](photo.jpg)');
  });

  it('does not modify external URLs not matching asset pattern', () => {
    const markdown = '![alt](https://other.com/image.png)';
    expect(reverseRewriteAssetUrls(markdown, assetUrls)).toBe(markdown);
  });

  it('extracts filename from signed asset URL even if not in asset map', () => {
    // This handles newly uploaded images whose signed URL isn't in the map yet
    const markdown = '![alt](/assets/wsId/nodeId/newimage.png?sig=xyz&exp=999)';
    expect(reverseRewriteAssetUrls(markdown, {})).toBe('![alt](newimage.png)');
  });

  it('extracts filename with URL-encoded characters', () => {
    const markdown = '![alt](/assets/ws/node/my%20image.png?sig=abc&exp=123)';
    expect(reverseRewriteAssetUrls(markdown, {})).toBe('![alt](my image.png)');
  });

  it('converts links with signed asset URLs to filenames', () => {
    const markdown = '[document](/assets/ws/node/report.pdf?sig=abc&exp=123)';
    expect(reverseRewriteAssetUrls(markdown, {})).toBe('[document](report.pdf)');
  });

  it('does not modify regular links', () => {
    const markdown = '[link](https://example.com/page)';
    expect(reverseRewriteAssetUrls(markdown, {})).toBe(markdown);
  });
});

describe('round-trip asset URL handling', () => {
  it('image URLs survive rewrite and reverse', () => {
    const assetUrls: AssetUrlMap = {
      'image.png': '/assets/ws/node/image.png?sig=abc&exp=123',
    };
    const original = '![alt](image.png)';
    const rewritten = rewriteAssetUrls(original, assetUrls);
    expect(rewritten).toBe('![alt](/assets/ws/node/image.png?sig=abc&exp=123)');
    const reversed = reverseRewriteAssetUrls(rewritten, assetUrls);
    expect(reversed).toBe(original);
  });

  it('multiple assets survive round-trip', () => {
    const urls: AssetUrlMap = {
      'a.png': '/assets/ws/node/a.png?sig=1',
      'b.png': '/assets/ws/node/b.png?sig=2',
    };
    const original = '![a](a.png)\n\n![b](b.png)';
    const rewritten = rewriteAssetUrls(original, urls);
    const reversed = reverseRewriteAssetUrls(rewritten, urls);
    expect(reversed).toBe(original);
  });

  it('link URLs survive round-trip', () => {
    const assetUrls: AssetUrlMap = {
      'doc.pdf': '/assets/ws/node/doc.pdf?sig=abc&exp=123',
    };
    const original = '[document](doc.pdf)';
    const rewritten = rewriteAssetUrls(original, assetUrls);
    expect(rewritten).toBe('[document](/assets/ws/node/doc.pdf?sig=abc&exp=123)');
    const reversed = reverseRewriteAssetUrls(rewritten, assetUrls);
    expect(reversed).toBe(original);
  });

  it('newly uploaded images are extracted even without asset map', () => {
    // Simulates: upload returns signed URL, we insert it, then serialize
    // The signed URL should be converted back to just the filename
    const markdown = '![newfile](/assets/wsId/nodeId/newfile.png?sig=xyz&exp=999)';
    const reversed = reverseRewriteAssetUrls(markdown, {});
    expect(reversed).toBe('![newfile](newfile.png)');
  });
});
