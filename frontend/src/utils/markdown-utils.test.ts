// Unit tests for markdown link conversion utilities.

import { describe, it, expect } from 'vitest';
import { relativeLinksToSpaUrls, spaUrlsToRelativeLinks, extractLinkedNodeIds } from './markdown-utils';

describe('relativeLinksToSpaUrls', () => {
  const wsId = 'IVQKBFTG000';

  it('converts sibling link', () => {
    expect(relativeLinksToSpaUrls('[Page](../NODE123/index.md)', wsId)).toBe('[Page](/w/@IVQKBFTG000/@NODE123)');
  });

  it('converts child link', () => {
    expect(relativeLinksToSpaUrls('[Page](NODE123/index.md)', wsId)).toBe('[Page](/w/@IVQKBFTG000/@NODE123)');
  });

  it('converts deep relative link', () => {
    expect(relativeLinksToSpaUrls('[Page](../../NODE123/index.md)', wsId)).toBe('[Page](/w/@IVQKBFTG000/@NODE123)');
  });

  it('converts multiple links', () => {
    const content = 'See [A](../AAA111/index.md) and [B](../BBB222/index.md)';
    expect(relativeLinksToSpaUrls(content, wsId)).toBe(
      'See [A](/w/@IVQKBFTG000/@AAA111) and [B](/w/@IVQKBFTG000/@BBB222)'
    );
  });

  it('preserves link text with spaces and special chars', () => {
    expect(relativeLinksToSpaUrls('[My Cool Page!](../NODE123/index.md)', wsId)).toBe(
      '[My Cool Page!](/w/@IVQKBFTG000/@NODE123)'
    );
  });

  it('preserves empty link text', () => {
    expect(relativeLinksToSpaUrls('[](../NODE123/index.md)', wsId)).toBe('[](/w/@IVQKBFTG000/@NODE123)');
  });

  it('leaves absolute links unchanged', () => {
    const content = '[Page](/some/NODE123/index.md)';
    expect(relativeLinksToSpaUrls(content, wsId)).toBe(content);
  });

  it('leaves http links unchanged', () => {
    const content = '[Page](https://example.com/NODE123/index.md)';
    expect(relativeLinksToSpaUrls(content, wsId)).toBe(content);
  });

  it('leaves non-index.md links unchanged', () => {
    const content = '[Page](../NODE123/readme.md)';
    expect(relativeLinksToSpaUrls(content, wsId)).toBe(content);
  });

  it('leaves image links unchanged', () => {
    const content = '![img](image.png)';
    expect(relativeLinksToSpaUrls(content, wsId)).toBe(content);
  });

  it('leaves invalid node IDs unchanged (lowercase)', () => {
    const content = '[Page](../some-slug/index.md)';
    expect(relativeLinksToSpaUrls(content, wsId)).toBe(content);
  });

  it('leaves invalid node IDs unchanged (has hyphens)', () => {
    const content = '[Page](../my-page/index.md)';
    expect(relativeLinksToSpaUrls(content, wsId)).toBe(content);
  });

  it('handles mixed content with non-link text', () => {
    const content = '# Title\n\nSome text [link](../ABC123/index.md) more text\n\n![img](photo.jpg)';
    expect(relativeLinksToSpaUrls(content, wsId)).toBe(
      '# Title\n\nSome text [link](/w/@IVQKBFTG000/@ABC123) more text\n\n![img](photo.jpg)'
    );
  });

  it('returns empty string unchanged', () => {
    expect(relativeLinksToSpaUrls('', wsId)).toBe('');
  });

  it('returns content unchanged with empty wsId', () => {
    expect(relativeLinksToSpaUrls('[P](../N/index.md)', '')).toBe('[P](../N/index.md)');
  });

  it('returns content unchanged when no index.md links', () => {
    const content = 'No links here, just text.';
    expect(relativeLinksToSpaUrls(content, wsId)).toBe(content);
  });
});

describe('spaUrlsToRelativeLinks', () => {
  const wsId = 'IVQKBFTG000';

  it('converts basic SPA link', () => {
    expect(spaUrlsToRelativeLinks('[Page](/w/@IVQKBFTG000/@NODE123)', wsId)).toBe('[Page](../NODE123/index.md)');
  });

  it('converts SPA link with slug', () => {
    expect(spaUrlsToRelativeLinks('[Page](/w/@IVQKBFTG000+my-ws/@NODE123+my-page)', wsId)).toBe(
      '[Page](../NODE123/index.md)'
    );
  });

  it('converts multiple SPA links', () => {
    const content = 'See [A](/w/@IVQKBFTG000/@AAA111) and [B](/w/@IVQKBFTG000/@BBB222)';
    expect(spaUrlsToRelativeLinks(content, wsId)).toBe('See [A](../AAA111/index.md) and [B](../BBB222/index.md)');
  });

  it('preserves cross-workspace links', () => {
    const content = '[Page](/w/@OTHERWSID/@NODE123)';
    expect(spaUrlsToRelativeLinks(content, wsId)).toBe(content);
  });

  it('preserves link text with special characters', () => {
    expect(spaUrlsToRelativeLinks('[My Cool Page!](/w/@IVQKBFTG000/@NODE123)', wsId)).toBe(
      '[My Cool Page!](../NODE123/index.md)'
    );
  });

  it('preserves empty link text', () => {
    expect(spaUrlsToRelativeLinks('[](/w/@IVQKBFTG000/@NODE123)', wsId)).toBe('[](../NODE123/index.md)');
  });

  it('leaves external links unchanged', () => {
    const content = '[Google](https://google.com)';
    expect(spaUrlsToRelativeLinks(content, wsId)).toBe(content);
  });

  it('leaves relative links unchanged (already converted)', () => {
    const content = '[Page](../NODE123/index.md)';
    expect(spaUrlsToRelativeLinks(content, wsId)).toBe(content);
  });

  it('handles mixed content', () => {
    const content = '# Title\n\n[link](/w/@IVQKBFTG000/@ABC123)\n\n![img](photo.jpg)';
    expect(spaUrlsToRelativeLinks(content, wsId)).toBe('# Title\n\n[link](../ABC123/index.md)\n\n![img](photo.jpg)');
  });

  it('returns empty string unchanged', () => {
    expect(spaUrlsToRelativeLinks('', wsId)).toBe('');
  });

  it('returns content unchanged with empty wsId', () => {
    expect(spaUrlsToRelativeLinks('[P](/w/@WS/@N)', '')).toBe('[P](/w/@WS/@N)');
  });

  it('returns content unchanged when no SPA links', () => {
    const content = 'No links here, just text.';
    expect(spaUrlsToRelativeLinks(content, wsId)).toBe(content);
  });
});

describe('round-trip', () => {
  const wsId = 'IVQKBFTG000';

  it('SPA -> relative -> SPA preserves content', () => {
    const original = 'See [Page](/w/@IVQKBFTG000/@NODE123) for details.';
    const relative = spaUrlsToRelativeLinks(original, wsId);
    const restored = relativeLinksToSpaUrls(relative, wsId);
    expect(restored).toBe(original);
  });

  it('relative -> SPA -> relative preserves content', () => {
    const original = 'See [Page](../NODE123/index.md) for details.';
    const spa = relativeLinksToSpaUrls(original, wsId);
    const restored = spaUrlsToRelativeLinks(spa, wsId);
    expect(restored).toBe(original);
  });

  it('round-trips multiple links', () => {
    const original = '[A](/w/@IVQKBFTG000/@AAA) and [B](/w/@IVQKBFTG000/@BBB)';
    const relative = spaUrlsToRelativeLinks(original, wsId);
    const restored = relativeLinksToSpaUrls(relative, wsId);
    expect(restored).toBe(original);
  });

  it('round-trips with cross-workspace link preserved', () => {
    const original = '[Local](/w/@IVQKBFTG000/@NODE1) and [Other](/w/@OTHERWS/@NODE2)';
    const relative = spaUrlsToRelativeLinks(original, wsId);
    expect(relative).toBe('[Local](../NODE1/index.md) and [Other](/w/@OTHERWS/@NODE2)');
    const restored = relativeLinksToSpaUrls(relative, wsId);
    expect(restored).toBe('[Local](/w/@IVQKBFTG000/@NODE1) and [Other](/w/@OTHERWS/@NODE2)');
  });
});

describe('extractLinkedNodeIds', () => {
  const wsId = 'IVQKBFTG000';

  it('returns empty array for empty content', () => {
    expect(extractLinkedNodeIds('', wsId)).toEqual([]);
  });

  it('returns empty array for empty wsId', () => {
    expect(extractLinkedNodeIds('[P](/w/@WS/@N)', '')).toEqual([]);
  });

  it('returns empty array for no links', () => {
    expect(extractLinkedNodeIds('Some text without links', wsId)).toEqual([]);
  });

  it('extracts single node ID', () => {
    expect(extractLinkedNodeIds('[P](/w/@IVQKBFTG000/@NODE123)', wsId)).toEqual(['NODE123']);
  });

  it('extracts from link with slugs', () => {
    expect(extractLinkedNodeIds('[P](/w/@IVQKBFTG000+ws/@NODE123+page)', wsId)).toEqual(['NODE123']);
  });

  it('extracts multiple unique IDs', () => {
    const content = '[A](/w/@IVQKBFTG000/@AAA) and [B](/w/@IVQKBFTG000/@BBB)';
    expect(extractLinkedNodeIds(content, wsId)).toEqual(['AAA', 'BBB']);
  });

  it('deduplicates IDs', () => {
    const content = '[A](/w/@IVQKBFTG000/@NODE1) and [B](/w/@IVQKBFTG000/@NODE1)';
    expect(extractLinkedNodeIds(content, wsId)).toEqual(['NODE1']);
  });

  it('skips cross-workspace links', () => {
    const content = '[A](/w/@IVQKBFTG000/@NODE1) and [B](/w/@OTHER/@NODE2)';
    expect(extractLinkedNodeIds(content, wsId)).toEqual(['NODE1']);
  });

  it('returns empty for only cross-workspace links', () => {
    expect(extractLinkedNodeIds('[P](/w/@OTHER/@NODE1)', wsId)).toEqual([]);
  });
});
