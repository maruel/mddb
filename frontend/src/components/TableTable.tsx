// Notion-like table view with inline editing.

import { createSignal, For, Show } from 'solid-js';
import {
  type DataRecordResponse,
  type Property,
  type PropertyType,
  PropertyTypeCheckbox,
  PropertyTypeSelect,
  PropertyTypeNumber,
  PropertyTypeDate,
} from '@sdk/types.gen';
import styles from './TableTable.module.css';
import { RowHandle, RowContextMenu, type ContextMenuAction } from './shared';
import { TABLE_RECORD_MIME } from './table/TableRow';
import { useI18n } from '../i18n';

// Column types available for adding
const COLUMN_TYPES: { type: PropertyType; labelKey: string }[] = [
  { type: 'text', labelKey: 'table.typeText' },
  { type: 'number', labelKey: 'table.typeNumber' },
  { type: 'checkbox', labelKey: 'table.typeCheckbox' },
  { type: 'date', labelKey: 'table.typeDate' },
  { type: 'select', labelKey: 'table.typeSelect' },
  { type: 'url', labelKey: 'table.typeUrl' },
  { type: 'email', labelKey: 'table.typeEmail' },
];

interface TableTableProps {
  tableId: string;
  columns: Property[];
  records: DataRecordResponse[];
  onAddRecord?: (data: Record<string, unknown>) => void;
  onUpdateRecord?: (recordId: string, data: Record<string, unknown>) => void;
  onDeleteRecord?: (recordId: string) => void;
  onDuplicateRecord?: (recordId: string) => void;
  onOpenRecord?: (recordId: string) => void;
  onAddColumn?: (column: Property) => void;
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
  const [editCancelled, setEditCancelled] = createSignal(false);

  // Row context menu state
  const [menuState, setMenuState] = createSignal<{
    recordId: string;
    x: number;
    y: number;
  } | null>(null);

  // Column adding state
  const [showAddColumn, setShowAddColumn] = createSignal(false);
  const [newColumnName, setNewColumnName] = createSignal('');
  const [newColumnType, setNewColumnType] = createSignal<PropertyType>('text');

  const handleAddColumn = () => {
    const name = newColumnName().trim();
    if (!name || !props.onAddColumn) return;

    const newColumn: Property = {
      name,
      type: newColumnType(),
      required: false,
    };

    props.onAddColumn(newColumn);
    setNewColumnName('');
    setNewColumnType('text');
    setShowAddColumn(false);
  };

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

  // Save on blur or Enter, cancel on Escape
  const handleCellBlur = (recordId: string, columnName: string) => {
    // Don't save if we just cancelled via Escape
    if (editCancelled()) {
      setEditCancelled(false);
      return;
    }
    handleCellSave(recordId, columnName);
  };

  const handleKeyDown = (e: KeyboardEvent, recordId: string, columnName: string) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleCellSave(recordId, columnName);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      setEditCancelled(true);
      setEditingCell(null);
    } else if (e.key === 'Tab') {
      // Save current and move to next/prev cell
      handleCellSave(recordId, columnName);
      // Browser will handle focus movement
    }
  };

  const handleCellSave = (recordId: string, columnName: string) => {
    // Check if edit was cancelled or we're no longer in edit mode
    if (editCancelled() || !editingCell()) {
      return;
    }

    const column = props.columns.find((c) => c.name === columnName);
    if (!column || !props.onUpdateRecord) {
      setEditingCell(null);
      return;
    }

    const record = props.records.find((r) => r.id === recordId);
    if (!record) {
      setEditingCell(null);
      return;
    }

    const updatedData = { ...record.data };
    updatedData[column.name] = editValue();

    props.onUpdateRecord(recordId, updatedData);
    setEditingCell(null);
  };

  // Add a new empty row
  const handleAddRow = () => {
    if (props.onAddRecord && props.columns.length > 0) {
      props.onAddRecord({});
    }
  };

  // Row handle handlers
  const handleRowDragStart = (e: DragEvent, recordId: string) => {
    e.dataTransfer?.setData(TABLE_RECORD_MIME, recordId);
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move';
    }
  };

  const handleRowDragOver = (e: DragEvent) => {
    e.preventDefault();
    if (e.dataTransfer) {
      e.dataTransfer.dropEffect = 'move';
    }
  };

  const handleRowContextMenu = (e: MouseEvent, recordId: string) => {
    setMenuState({ recordId, x: e.clientX, y: e.clientY });
  };

  const getRowActions = (): ContextMenuAction[] => {
    const actions: ContextMenuAction[] = [];

    if (props.onOpenRecord) {
      actions.push({
        id: 'open',
        label: t('table.openRecord') || 'Open',
      });
    }

    if (props.onDuplicateRecord) {
      actions.push({
        id: 'duplicate',
        label: t('table.duplicateRecord') || 'Duplicate',
        shortcut: '⌘D',
      });
    }

    if (props.onDeleteRecord) {
      actions.push({
        id: 'delete',
        label: t('table.deleteRecord') || 'Delete',
        shortcut: '⌫',
        danger: true,
        separator: actions.length > 0,
      });
    }

    return actions;
  };

  const handleRowAction = (actionId: string) => {
    const state = menuState();
    if (!state) return;

    switch (actionId) {
      case 'open':
        props.onOpenRecord?.(state.recordId);
        break;
      case 'duplicate':
        props.onDuplicateRecord?.(state.recordId);
        break;
      case 'delete':
        props.onDeleteRecord?.(state.recordId);
        break;
    }
    setMenuState(null);
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

  // Render input for editing cells - saves on blur
  // Use ref to track input and read value on save (avoids re-render on every keystroke)
  let inputRef: HTMLInputElement | HTMLSelectElement | undefined;

  const renderCellInput = (column: Property, initialValue: string, recordId: string, columnName: string) => {
    const focusRef = (el: HTMLInputElement | HTMLSelectElement) => {
      inputRef = el;
      setTimeout(() => {
        el.focus();
        if (el instanceof HTMLInputElement && el.type === 'text') el.select();
      }, 0);
    };

    // Read current value from input ref for saving
    const getCurrentValue = () => {
      if (!inputRef) return initialValue;
      if (inputRef instanceof HTMLInputElement && inputRef.type === 'checkbox') {
        return String(inputRef.checked);
      }
      return inputRef.value;
    };

    const onKeyDown = (e: KeyboardEvent) => {
      // Sync current value from input before handling key press
      if (e.key === 'Enter' || e.key === 'Tab') {
        setEditValue(getCurrentValue());
      }
      handleKeyDown(e, recordId, columnName);
    };

    // Update editValue from input ref before save
    const syncAndSave = () => {
      setEditValue(getCurrentValue());
      handleCellSave(recordId, columnName);
    };

    const syncAndBlur = () => {
      setEditValue(getCurrentValue());
      handleCellBlur(recordId, columnName);
    };

    switch (column.type) {
      case PropertyTypeCheckbox:
        return (
          <input
            ref={focusRef}
            type="checkbox"
            checked={initialValue === 'true'}
            onChange={syncAndSave}
            onKeyDown={onKeyDown}
            class={styles.input}
          />
        );
      case PropertyTypeSelect:
        if (column.options && column.options.length > 0) {
          return (
            <select
              ref={focusRef}
              value={initialValue}
              onChange={syncAndSave}
              onBlur={syncAndBlur}
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
            onBlur={syncAndBlur}
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
            onBlur={syncAndBlur}
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
            onBlur={syncAndBlur}
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
            onBlur={syncAndBlur}
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
              {/* Handle column header */}
              <th class={styles.handleHeader} />
              <Show when={props.onDeleteRecord}>
                <th class={styles.actionsHeader} />
              </Show>
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
              <Show when={props.onAddColumn}>
                <th class={styles.addColumnCell}>
                  <div class={styles.addColumnWrapper}>
                    <button
                      class={styles.addColumnBtn}
                      onClick={() => setShowAddColumn(!showAddColumn())}
                      title={t('table.addColumn') || 'Add Column'}
                    >
                      +
                    </button>
                    <Show when={showAddColumn()}>
                      <div class={styles.addColumnDropdown}>
                        <input
                          type="text"
                          placeholder={t('table.columnName') || 'Column Name'}
                          value={newColumnName()}
                          onInput={(e) => setNewColumnName(e.target.value)}
                          class={styles.columnNameInput}
                          autofocus
                        />
                        <select
                          value={newColumnType()}
                          onChange={(e) => setNewColumnType(e.target.value as PropertyType)}
                          class={styles.columnTypeSelect}
                        >
                          <For each={COLUMN_TYPES}>
                            {(ct) => <option value={ct.type}>{t(ct.labelKey) || ct.type}</option>}
                          </For>
                        </select>
                        <div class={styles.addColumnActions}>
                          <button class={styles.addColumnConfirm} onClick={handleAddColumn}>
                            {t('common.confirm') || 'Add'}
                          </button>
                          <button class={styles.addColumnCancel} onClick={() => setShowAddColumn(false)}>
                            {t('common.cancel') || 'Cancel'}
                          </button>
                        </div>
                      </div>
                    </Show>
                  </div>
                </th>
              </Show>
            </tr>
          </thead>
          <tbody>
            <For each={props.records}>
              {(record) => (
                <tr class={styles.row} onDragOver={handleRowDragOver}>
                  {/* Handle cell */}
                  <td class={styles.handleCell}>
                    <RowHandle
                      rowId={record.id}
                      onDragStart={handleRowDragStart}
                      onContextMenu={handleRowContextMenu}
                    />
                  </td>
                  <Show when={props.onDeleteRecord}>
                    <td class={styles.actionsCell}>
                      <button
                        class={styles.deleteBtn}
                        onClick={() => props.onDeleteRecord?.(record.id)}
                        title={t('table.deleteRecord') || 'Delete'}
                      >
                        ✕
                      </button>
                    </td>
                  </Show>
                  <For each={props.columns}>
                    {(column) => {
                      const isEditing = () =>
                        editingCell()?.recordId === record.id && editingCell()?.columnId === column.name;

                      return (
                        <td
                          class={`${styles.cell}${isEditing() ? ` ${styles.editing}` : ''}`}
                          onClick={() => !isEditing() && handleCellClick(record.id, column.name)}
                        >
                          <Show
                            when={isEditing()}
                            fallback={<div class={styles.cellContent}>{renderCellContent(record, column)}</div>}
                          >
                            {renderCellInput(column, editValue(), record.id, column.name)}
                          </Show>
                        </td>
                      );
                    }}
                  </For>
                </tr>
              )}
            </For>
            {/* Add new row - click to add */}
            <Show when={props.onAddRecord && props.columns.length > 0}>
              <tr class={styles.newRow}>
                <td
                  class={styles.newRowPlaceholder}
                  colSpan={props.columns.length + 1 + (props.onDeleteRecord ? 1 : 0) + (props.onAddColumn ? 1 : 0)}
                  onClick={handleAddRow}
                >
                  + {t('table.addRecord') || 'New'}
                </td>
              </tr>
            </Show>
          </tbody>
        </table>
      </div>

      <Show when={props.columns.length === 0}>
        <div class={styles.empty}>
          {t('table.noColumns')}
          <Show when={props.onAddColumn}>
            <span> {t('table.addColumnFirst') || 'Click + to add a column.'}</span>
          </Show>
        </div>
      </Show>

      <Show when={props.hasMore}>
        <div class={styles.loadMore}>
          <button onClick={() => props.onLoadMore?.()}>{t('table.loadMore')}</button>
        </div>
      </Show>

      {/* Row context menu */}
      <Show when={menuState()}>
        {(state) => (
          <RowContextMenu
            position={{ x: state().x, y: state().y }}
            actions={getRowActions()}
            onAction={handleRowAction}
            onClose={() => setMenuState(null)}
          />
        )}
      </Show>
    </div>
  );
}
