// Spreadsheet-like table view for editing records inline.

import { createSignal, For, Show } from 'solid-js';
import {
  type DataRecordResponse,
  type Property,
  PropertyTypeCheckbox,
  PropertyTypeSelect,
  PropertyTypeNumber,
  PropertyTypeDate,
} from '../types.gen';
import styles from './TableTable.module.css';
import { useI18n } from '../i18n';

interface TableTableProps {
  tableId: string;
  columns: Property[];
  records: DataRecordResponse[];
  onAddRecord?: (data: Record<string, unknown>) => void;
  onUpdateRecord?: (recordId: string, data: Record<string, unknown>) => void;
  onDeleteRecord?: (recordId: string) => void;
  onLoadMore?: () => void;
  hasMore?: boolean;
}

export default function TableTable(props: TableTableProps) {
  const { t } = useI18n();
  const [editingCell, setEditingCell] = createSignal<{
    recordId: string;
    columnId: string;
  } | null>(null);
  const [editValue, setEditValue] = createSignal('');
  const [newRowData, setNewRowData] = createSignal<Record<string, unknown>>({});

  const getCellValue = (record: DataRecordResponse, columnName: string) => {
    const column = props.columns.find((c) => c.name === columnName);
    if (!column) return '';
    return record.data[column.name] ?? '';
  };

  const handleCellClick = (recordId: string, columnName: string) => {
    const col = props.columns.find((c) => c.name === columnName);
    const value = props.records.find((r) => r.id === recordId)?.data[col?.name ?? ''] ?? '';

    setEditingCell({ recordId, columnId: columnName });
    setEditValue(String(value));
  };

  const handleCellChange = (value: string) => {
    setEditValue(value);
  };

  const handleKeyDown = (e: KeyboardEvent, recordId: string, columnName: string) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleCellSave(recordId, columnName);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      setEditingCell(null);
    }
  };

  const handleCellSave = (recordId: string, columnName: string) => {
    const column = props.columns.find((c) => c.name === columnName);
    if (!column || !props.onUpdateRecord) return;

    const record = props.records.find((r) => r.id === recordId);
    if (!record) return;

    const updatedData = { ...record.data };
    updatedData[column.name] = editValue();

    props.onUpdateRecord(recordId, updatedData);
    setEditingCell(null);
  };

  const handleAddRecord = () => {
    if (props.onAddRecord && Object.keys(newRowData()).length > 0) {
      props.onAddRecord(newRowData());
      setNewRowData({});
    }
  };

  const renderCellContent = (record: DataRecordResponse, column: Property) => {
    const value = getCellValue(record, column.name);

    switch (column.type) {
      case 'checkbox':
        return value ? '✓' : '';
      case 'select':
      case 'multi_select':
        return String(value);
      case 'date':
        return value ? new Date(value as string).toLocaleDateString() : '';
      case 'number':
        return String(value);
      default:
        return String(value);
    }
  };

  const renderCellInput = (
    column: Property,
    initialValue: string,
    autoFocus = false,
    recordId?: string,
    columnName?: string
  ) => {
    // Helper to auto-focus input element
    const focusRef = (el: HTMLInputElement | HTMLSelectElement) => {
      if (autoFocus) {
        // Use setTimeout to ensure DOM is ready
        setTimeout(() => el.focus(), 0);
      }
    };

    // Keyboard handler for Enter (save) and Escape (cancel)
    const onKeyDown = recordId && columnName ? (e: KeyboardEvent) => handleKeyDown(e, recordId, columnName) : undefined;

    switch (column.type) {
      case PropertyTypeCheckbox:
        return (
          <input
            ref={focusRef}
            type="checkbox"
            checked={initialValue === 'true'}
            onChange={(e) => handleCellChange(String(e.target.checked))}
            onKeyDown={onKeyDown}
            class={styles.input}
          />
        );
      case PropertyTypeSelect:
        // Use dropdown if options are defined, otherwise text input
        if (column.options && column.options.length > 0) {
          return (
            <select
              ref={focusRef}
              value={initialValue}
              onChange={(e) => handleCellChange(e.target.value)}
              onKeyDown={onKeyDown}
              class={styles.input}
            >
              <option value="">--</option>
              <For each={column.options}>{(option) => <option value={option.id}>{option.name}</option>}</For>
            </select>
          );
        }
        return (
          <input
            ref={focusRef}
            type="text"
            value={initialValue}
            onInput={(e) => handleCellChange(e.target.value)}
            onKeyDown={onKeyDown}
            class={styles.input}
          />
        );
      case PropertyTypeNumber:
        return (
          <input
            ref={focusRef}
            type="number"
            value={initialValue}
            onInput={(e) => handleCellChange(e.target.value)}
            onKeyDown={onKeyDown}
            class={styles.input}
          />
        );
      case PropertyTypeDate:
        return (
          <input
            ref={focusRef}
            type="date"
            value={initialValue}
            onInput={(e) => handleCellChange(e.target.value)}
            onKeyDown={onKeyDown}
            class={styles.input}
          />
        );
      default:
        return (
          <input
            ref={focusRef}
            type="text"
            value={initialValue}
            onInput={(e) => handleCellChange(e.target.value)}
            onKeyDown={onKeyDown}
            class={styles.input}
          />
        );
    }
  };

  return (
    <div class={styles.container}>
      <div class={styles.tableWrapper}>
        <table class={styles.table}>
          <thead>
            <tr class={styles.headerRow}>
              <th class={styles.headerCell}>{t('common.actions')}</th>
              <For each={props.columns}>
                {(column) => (
                  <th class={styles.headerCell}>
                    {column.name}
                    <Show when={column.required}>
                      <span class={styles.required}>*</span>
                    </Show>
                  </th>
                )}
              </For>
            </tr>
          </thead>
          <tbody>
            <For each={props.records}>
              {(record) => (
                <tr class={styles.row}>
                  <td class={styles.actionsCell}>
                    <Show when={props.onDeleteRecord}>
                      <button
                        class={styles.deleteBtn}
                        onClick={() => props.onDeleteRecord?.(record.id)}
                        title={t('table.deleteRecord') || 'Delete record'}
                      >
                        ✕
                      </button>
                    </Show>
                  </td>
                  <For each={props.columns}>
                    {(column) => {
                      const isEditing = () =>
                        editingCell()?.recordId === record.id && editingCell()?.columnId === column.name;

                      return (
                        <td
                          class={styles.cell}
                          classList={{ [`${styles.editing}`]: isEditing() }}
                          onClick={() => handleCellClick(record.id, column.name)}
                        >
                          <Show
                            when={isEditing()}
                            fallback={<div class={styles.cellContent}>{renderCellContent(record, column)}</div>}
                          >
                            <div class={styles.editContainer}>
                              {renderCellInput(column, editValue(), true, record.id, column.name)}
                              <div class={styles.editActions}>
                                <button class={styles.saveBtn} onClick={() => handleCellSave(record.id, column.name)}>
                                  ✓
                                </button>
                                <button class={styles.cancelBtn} onClick={() => setEditingCell(null)}>
                                  ✕
                                </button>
                              </div>
                            </div>
                          </Show>
                        </td>
                      );
                    }}
                  </For>
                </tr>
              )}
            </For>
            <Show when={props.onAddRecord}>
              <tr class={styles.newRow}>
                <td class={styles.actionsCell}>
                  <button class={styles.addBtn} onClick={handleAddRecord}>
                    +
                  </button>
                </td>
                <For each={props.columns}>
                  {(column) => (
                    <td class={styles.cell}>{renderCellInput(column, String(newRowData()[column.name] ?? ''))}</td>
                  )}
                </For>
              </tr>
            </Show>
          </tbody>
        </table>
      </div>

      <Show when={props.records.length === 0}>
        <div class={styles.empty}>{t('table.noRecords')}</div>
      </Show>

      <Show when={props.hasMore}>
        <div class={styles.loadMore}>
          <button onClick={() => props.onLoadMore?.()}>{t('table.loadMore')}</button>
        </div>
      </Show>
    </div>
  );
}
