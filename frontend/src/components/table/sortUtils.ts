// Pure functions for sort rule manipulation.

import { SortAsc, SortDesc, type Property, type Sort } from '@sdk/types.gen';

/** Returns the set of property names used by sorts, excluding the sort at excludeIndex. */
export function usedPropertyNames(sorts: Sort[], excludeIndex: number): Set<string> {
  const used = new Set<string>();
  sorts.forEach((s, i) => {
    if (i !== excludeIndex) used.add(s.property);
  });
  return used;
}

/** Returns properties not used by other sorts (keeps the property at excludeIndex available). */
export function availableProperties(properties: Property[], sorts: Sort[], excludeIndex: number): Property[] {
  const used = usedPropertyNames(sorts, excludeIndex);
  return properties.filter((p) => !used.has(p.name));
}

/** Creates a new sort array with a sort added for the first unused property. Returns null if none available. */
export function addSort(sorts: Sort[], properties: Property[]): Sort[] | null {
  const unused = availableProperties(properties, sorts, -1);
  const first = unused[0];
  if (!first) return null;
  return [...sorts, { property: first.name, direction: SortAsc }];
}

/** Creates a new sort array with the sort at index removed. */
export function removeSort(sorts: Sort[], index: number): Sort[] {
  return sorts.filter((_, i) => i !== index);
}

/** Creates a new sort array with the sort at index having its direction toggled. */
export function toggleSortDirection(sorts: Sort[], index: number): Sort[] | null {
  const current = sorts[index];
  if (!current) return null;
  const newSorts = [...sorts];
  newSorts[index] = { property: current.property, direction: current.direction === SortAsc ? SortDesc : SortAsc };
  return newSorts;
}

/** Creates a new sort array with the sort at index changed to a different property. */
export function changeSortProperty(sorts: Sort[], index: number, propertyName: string): Sort[] | null {
  const current = sorts[index];
  if (!current) return null;
  const newSorts = [...sorts];
  newSorts[index] = { property: propertyName, direction: current.direction };
  return newSorts;
}
