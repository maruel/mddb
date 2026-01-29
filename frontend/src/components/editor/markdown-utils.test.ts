// Unit tests for markdown utility functions.

import { describe, it, expect } from 'vitest';
import {
  rewriteAssetUrls,
  reverseRewriteAssetUrls,
  extractLinkedNodeIds,
  rewriteInternalLinkTitles,
  type NodeTitleMap,
} from './markdown-utils';
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

describe('extractLinkedNodeIds', () => {
  it('returns empty array for empty content', () => {
    expect(extractLinkedNodeIds('', 'ws123')).toEqual([]);
  });

  it('returns empty array if no workspace ID provided', () => {
    expect(extractLinkedNodeIds('[link](/w/ws123/node456)', '')).toEqual([]);
  });

  it('extracts single internal link', () => {
    const markdown = 'Check [my page](/w/ws123+workspace/node456+my-page) here';
    expect(extractLinkedNodeIds(markdown, 'ws123')).toEqual(['node456']);
  });

  it('extracts multiple internal links', () => {
    const markdown = 'See [page1](/w/ws123/node1+title) and [page2](/w/ws123/node2)';
    expect(extractLinkedNodeIds(markdown, 'ws123')).toEqual(['node1', 'node2']);
  });

  it('deduplicates repeated links', () => {
    const markdown = '[a](/w/ws123/same+a) and [b](/w/ws123/same+b)';
    expect(extractLinkedNodeIds(markdown, 'ws123')).toEqual(['same']);
  });

  it('filters by current workspace', () => {
    const markdown = '[same ws](/w/ws123/node1) [other ws](/w/other/node2)';
    expect(extractLinkedNodeIds(markdown, 'ws123')).toEqual(['node1']);
  });

  it('ignores external URLs', () => {
    const markdown = '[google](https://google.com) [internal](/w/ws123/node1)';
    expect(extractLinkedNodeIds(markdown, 'ws123')).toEqual(['node1']);
  });

  it('handles links without slug', () => {
    const markdown = '[page](/w/ws123/node1)';
    expect(extractLinkedNodeIds(markdown, 'ws123')).toEqual(['node1']);
  });
});

describe('rewriteInternalLinkTitles', () => {
  const nodeTitles: NodeTitleMap = {
    node1: 'Updated Title 1',
    node2: 'Updated Title 2',
  };

  it('returns unchanged for empty content', () => {
    expect(rewriteInternalLinkTitles('', nodeTitles, 'ws123')).toBe('');
  });

  it('returns unchanged if no titles map', () => {
    const markdown = '[Old](/w/ws123/node1)';
    expect(rewriteInternalLinkTitles(markdown, {}, 'ws123')).toBe(markdown);
  });

  it('rewrites link display text to current title', () => {
    const markdown = '[Old Title](/w/ws123+ws/node1+old-title)';
    expect(rewriteInternalLinkTitles(markdown, nodeTitles, 'ws123')).toBe(
      '[Updated Title 1](/w/ws123+ws/node1+old-title)'
    );
  });

  it('rewrites multiple links', () => {
    const markdown = 'See [A](/w/ws123/node1) and [B](/w/ws123/node2)';
    expect(rewriteInternalLinkTitles(markdown, nodeTitles, 'ws123')).toBe(
      'See [Updated Title 1](/w/ws123/node1) and [Updated Title 2](/w/ws123/node2)'
    );
  });

  it('preserves URL exactly', () => {
    const markdown = '[Old](/w/ws123+my-workspace/node1+old-slug)';
    const result = rewriteInternalLinkTitles(markdown, nodeTitles, 'ws123');
    expect(result).toContain('/w/ws123+my-workspace/node1+old-slug');
  });

  it('does not rewrite links to other workspaces', () => {
    const markdown = '[Other](/w/other-ws/node1+slug)';
    expect(rewriteInternalLinkTitles(markdown, nodeTitles, 'ws123')).toBe(markdown);
  });

  it('keeps original if node not in title map', () => {
    const markdown = '[Unknown](/w/ws123/unknown-node)';
    expect(rewriteInternalLinkTitles(markdown, nodeTitles, 'ws123')).toBe(markdown);
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
