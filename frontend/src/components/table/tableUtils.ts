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
