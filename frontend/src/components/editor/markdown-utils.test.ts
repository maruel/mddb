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
});

describe('reverseRewriteAssetUrls', () => {
  const assetUrls: AssetUrlMap = {
    'image.png': 'https://example.com/assets/ws/node/image.png?sig=abc&exp=123',
    'photo.jpg': 'https://example.com/assets/ws/node/photo.jpg?sig=def&exp=456',
  };

  it('converts signed URLs back to filenames', () => {
    const markdown = '![alt](https://example.com/assets/ws/node/image.png?sig=abc&exp=123)';
    expect(reverseRewriteAssetUrls(markdown, assetUrls)).toBe('![alt](image.png)');
  });

  it('converts multiple URLs', () => {
    const markdown =
      '![a](https://example.com/assets/ws/node/image.png?sig=abc&exp=123) ![b](https://example.com/assets/ws/node/photo.jpg?sig=def&exp=456)';
    expect(reverseRewriteAssetUrls(markdown, assetUrls)).toBe('![a](image.png) ![b](photo.jpg)');
  });

  it('does not modify URLs not in asset map', () => {
    const markdown = '![alt](https://other.com/image.png)';
    expect(reverseRewriteAssetUrls(markdown, assetUrls)).toBe(markdown);
  });

  it('returns unchanged if assetUrls is empty', () => {
    const markdown = '![alt](https://example.com/assets/ws/node/image.png?sig=abc&exp=123)';
    expect(reverseRewriteAssetUrls(markdown, {})).toBe(markdown);
  });
});

describe('round-trip asset URL handling', () => {
  const assetUrls: AssetUrlMap = {
    'image.png': 'https://example.com/signed-url',
  };

  it('asset URLs survive rewrite and reverse', () => {
    const original = '![alt](image.png)';
    const rewritten = rewriteAssetUrls(original, assetUrls);
    expect(rewritten).toBe('![alt](https://example.com/signed-url)');
    const reversed = reverseRewriteAssetUrls(rewritten, assetUrls);
    expect(reversed).toBe(original);
  });

  it('multiple assets survive round-trip', () => {
    const urls: AssetUrlMap = {
      'a.png': 'https://example.com/a',
      'b.png': 'https://example.com/b',
    };
    const original = '![a](a.png)\n\n![b](b.png)';
    const rewritten = rewriteAssetUrls(original, urls);
    const reversed = reverseRewriteAssetUrls(rewritten, urls);
    expect(reversed).toBe(original);
  });
});
