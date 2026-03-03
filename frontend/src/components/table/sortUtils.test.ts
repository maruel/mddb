// Unit tests for sort utility functions (zero mocks).

import { describe, it, expect } from 'vitest';
import type { Property, Sort } from '@sdk/types.gen';
import {
  usedPropertyNames,
  availableProperties,
  addSort,
  removeSort,
  toggleSortDirection,
  changeSortProperty,
} from './sortUtils';

const props: Property[] = [
  { name: 'Name', type: 'text' },
  { name: 'Age', type: 'number' },
  { name: 'Status', type: 'select' },
];

describe('usedPropertyNames', () => {
  it('returns empty set for empty sorts', () => {
    expect(usedPropertyNames([], -1).size).toBe(0);
  });

  it('returns all property names', () => {
    const sorts: Sort[] = [
      { property: 'Name', direction: 'asc' },
      { property: 'Age', direction: 'desc' },
    ];
    const used = usedPropertyNames(sorts, -1);
    expect(used).toEqual(new Set(['Name', 'Age']));
  });

  it('excludes the sort at excludeIndex', () => {
    const sorts: Sort[] = [
      { property: 'Name', direction: 'asc' },
      { property: 'Age', direction: 'desc' },
    ];
    const used = usedPropertyNames(sorts, 0);
    expect(used).toEqual(new Set(['Age']));
  });
});

describe('availableProperties', () => {
  it('returns all properties when no sorts', () => {
    const result = availableProperties(props, [], -1);
    expect(result.map((p) => p.name)).toEqual(['Name', 'Age', 'Status']);
  });

  it('excludes properties used by other sorts', () => {
    const sorts: Sort[] = [
      { property: 'Name', direction: 'asc' },
      { property: 'Age', direction: 'desc' },
    ];
    // For row 0: exclude Age (used by row 1), keep Name (own) + Status
    const result = availableProperties(props, sorts, 0);
    expect(result.map((p) => p.name)).toEqual(['Name', 'Status']);
  });

  it('returns empty when all properties used by other sorts', () => {
    const sorts: Sort[] = [
      { property: 'Name', direction: 'asc' },
      { property: 'Age', direction: 'asc' },
      { property: 'Status', direction: 'asc' },
    ];
    // excludeIndex -1: all used
    const result = availableProperties(props, sorts, -1);
    expect(result).toEqual([]);
  });

  it('returns all properties when empty properties list', () => {
    const result = availableProperties([], [{ property: 'Ghost', direction: 'asc' }], 0);
    expect(result).toEqual([]);
  });

  it('ignores orphaned sort properties not in properties list', () => {
    const sorts: Sort[] = [{ property: 'Deleted', direction: 'asc' }];
    // "Deleted" is not in props, so all props remain available
    const result = availableProperties(props, sorts, -1);
    expect(result.map((p) => p.name)).toEqual(['Name', 'Age', 'Status']);
  });
});

describe('addSort', () => {
  it('adds sort for first property when no sorts exist', () => {
    const result = addSort([], props);
    expect(result).toEqual([{ property: 'Name', direction: 'asc' }]);
  });

  it('adds sort for first unused property', () => {
    const sorts: Sort[] = [{ property: 'Name', direction: 'asc' }];
    const result = addSort(sorts, props);
    expect(result).toEqual([
      { property: 'Name', direction: 'asc' },
      { property: 'Age', direction: 'asc' },
    ]);
  });

  it('skips used properties', () => {
    const sorts: Sort[] = [
      { property: 'Name', direction: 'asc' },
      { property: 'Age', direction: 'desc' },
    ];
    const result = addSort(sorts, props);
    expect(result).toEqual([
      { property: 'Name', direction: 'asc' },
      { property: 'Age', direction: 'desc' },
      { property: 'Status', direction: 'asc' },
    ]);
  });

  it('returns null when all properties used', () => {
    const sorts: Sort[] = [
      { property: 'Name', direction: 'asc' },
      { property: 'Age', direction: 'asc' },
      { property: 'Status', direction: 'asc' },
    ];
    expect(addSort(sorts, props)).toBeNull();
  });

  it('returns null when properties list is empty', () => {
    expect(addSort([], [])).toBeNull();
  });

  it('always defaults to ascending direction', () => {
    const result = addSort([], props);
    expect(result![0]!.direction).toBe('asc');
  });
});

describe('removeSort', () => {
  it('removes the only sort', () => {
    const sorts: Sort[] = [{ property: 'Name', direction: 'asc' }];
    expect(removeSort(sorts, 0)).toEqual([]);
  });

  it('removes first of two sorts', () => {
    const sorts: Sort[] = [
      { property: 'Name', direction: 'asc' },
      { property: 'Age', direction: 'desc' },
    ];
    expect(removeSort(sorts, 0)).toEqual([{ property: 'Age', direction: 'desc' }]);
  });

  it('removes last of two sorts', () => {
    const sorts: Sort[] = [
      { property: 'Name', direction: 'asc' },
      { property: 'Age', direction: 'desc' },
    ];
    expect(removeSort(sorts, 1)).toEqual([{ property: 'Name', direction: 'asc' }]);
  });

  it('removes middle of three sorts', () => {
    const sorts: Sort[] = [
      { property: 'Name', direction: 'asc' },
      { property: 'Age', direction: 'desc' },
      { property: 'Status', direction: 'asc' },
    ];
    expect(removeSort(sorts, 1)).toEqual([
      { property: 'Name', direction: 'asc' },
      { property: 'Status', direction: 'asc' },
    ]);
  });

  it('does not mutate original array', () => {
    const sorts: Sort[] = [{ property: 'Name', direction: 'asc' }];
    const result = removeSort(sorts, 0);
    expect(sorts).toHaveLength(1);
    expect(result).toHaveLength(0);
  });
});

describe('toggleSortDirection', () => {
  it('toggles asc to desc', () => {
    const sorts: Sort[] = [{ property: 'Name', direction: 'asc' }];
    expect(toggleSortDirection(sorts, 0)).toEqual([{ property: 'Name', direction: 'desc' }]);
  });

  it('toggles desc to asc', () => {
    const sorts: Sort[] = [{ property: 'Name', direction: 'desc' }];
    expect(toggleSortDirection(sorts, 0)).toEqual([{ property: 'Name', direction: 'asc' }]);
  });

  it('toggles correct sort in multi-sort', () => {
    const sorts: Sort[] = [
      { property: 'Name', direction: 'asc' },
      { property: 'Age', direction: 'asc' },
    ];
    expect(toggleSortDirection(sorts, 1)).toEqual([
      { property: 'Name', direction: 'asc' },
      { property: 'Age', direction: 'desc' },
    ]);
  });

  it('returns null for out-of-bounds index', () => {
    expect(toggleSortDirection([], 0)).toBeNull();
    expect(toggleSortDirection([{ property: 'Name', direction: 'asc' }], 5)).toBeNull();
  });

  it('does not mutate original array', () => {
    const sorts: Sort[] = [{ property: 'Name', direction: 'asc' }];
    toggleSortDirection(sorts, 0);
    expect(sorts[0]!.direction).toBe('asc');
  });
});

describe('changeSortProperty', () => {
  it('changes property and preserves direction', () => {
    const sorts: Sort[] = [{ property: 'Name', direction: 'desc' }];
    expect(changeSortProperty(sorts, 0, 'Age')).toEqual([{ property: 'Age', direction: 'desc' }]);
  });

  it('changes correct sort in multi-sort', () => {
    const sorts: Sort[] = [
      { property: 'Name', direction: 'asc' },
      { property: 'Age', direction: 'desc' },
    ];
    expect(changeSortProperty(sorts, 1, 'Status')).toEqual([
      { property: 'Name', direction: 'asc' },
      { property: 'Status', direction: 'desc' },
    ]);
  });

  it('returns null for out-of-bounds index', () => {
    expect(changeSortProperty([], 0, 'Name')).toBeNull();
  });

  it('does not mutate original array', () => {
    const sorts: Sort[] = [{ property: 'Name', direction: 'asc' }];
    changeSortProperty(sorts, 0, 'Age');
    expect(sorts[0]!.property).toBe('Name');
  });
});
