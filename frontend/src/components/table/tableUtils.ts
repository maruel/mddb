// Shared utilities for table views.

import type { DataRecordResponse } from '@sdk/types.gen';

/**
 * Updates a single field in a record's data and calls the update callback.
 * No-op if value unchanged or callback not provided.
 */
export function updateRecordField(
  record: DataRecordResponse,
  fieldName: string,
  newValue: string,
  onUpdate?: (id: string, data: Record<string, unknown>) => void
): void {
  if (record.data[fieldName] === newValue || !onUpdate) return;
  const newData = { ...record.data, [fieldName]: newValue };
  onUpdate(record.id, newData);
}

/**
 * Handles Enter key to blur input (submit on Enter).
 */
export function handleEnterBlur(e: KeyboardEvent): void {
  if (e.key === 'Enter') {
    (e.currentTarget as HTMLInputElement).blur();
  }
}

/**
 * Gets the display value for a record field, returning empty string if not found.
 */
export function getFieldValue(record: DataRecordResponse, fieldName: string | undefined): string {
  if (!fieldName) return '';
  const value = record.data[fieldName];
  return value !== null && value !== undefined ? String(value) : '';
}

/**
 * Gets the title (first column) value for a record.
 */
export function getRecordTitle(record: DataRecordResponse, columns: { name: string }[]): string {
  const firstCol = columns[0];
  return firstCol ? getFieldValue(record, firstCol.name) : '';
}

/**
 * Computes readable text color (#fff or #111) for a given hex background color.
 * Uses W3C relative luminance formula.
 */
export function chipTextColor(hexBg: string): string {
  const hex = hexBg.replace('#', '');
  if (hex.length !== 6) return '#111';
  const r = parseInt(hex.slice(0, 2), 16) / 255;
  const g = parseInt(hex.slice(2, 4), 16) / 255;
  const b = parseInt(hex.slice(4, 6), 16) / 255;
  const toLinear = (c: number) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
  const luminance = 0.2126 * toLinear(r) + 0.7152 * toLinear(g) + 0.0722 * toLinear(b);
  return luminance > 0.179 ? '#111' : '#fff';
}
