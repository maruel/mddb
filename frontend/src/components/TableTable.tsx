// Notion-like table view with inline editing.

import { createSignal, For, Show } from 'solid-js';
import { type DataRecordResponse, type Property } from '@sdk/types.gen';
import styles from './TableTable.module.css';
import { RowHandle, ContextMenu, type ContextMenuAction } from './shared';
import { TABLE_RECORD_MIME } from './table/TableRow';
import { TableCell } from './table/TableCell';
import { AddColumnDropdown } from './table/AddColumnDropdown';
import { useI18n } from '../i18n';

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

  // Row context menu state
  const [menuState, setMenuState] = createSignal<{
    recordId: string;
    x: number;
    y: number;
  } | null>(null);

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
        shortcut: '\u2318D',
      });
    }

    if (props.onDeleteRecord) {
      actions.push({
        id: 'delete',
        label: t('table.deleteRecord') || 'Delete',
        shortcut: '\u232B',
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

  const handleCellSave = (recordId: string, columnName: string, value: string) => {
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
    updatedData[column.name] = value;

    props.onUpdateRecord(recordId, updatedData);
    setEditingCell(null);
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
              <Show when={props.onAddColumn}>{(onAddColumn) => <AddColumnDropdown onAddColumn={onAddColumn()} />}</Show>
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
                        {'\u2715'}
                      </button>
                    </td>
                  </Show>
                  <For each={props.columns}>
                    {(column) => {
                      const isEditing = () =>
                        editingCell()?.recordId === record.id && editingCell()?.columnId === column.name;

                      return (
                        <TableCell
                          record={record}
                          column={column}
                          isEditing={isEditing}
                          onStartEdit={() => setEditingCell({ recordId: record.id, columnId: column.name })}
                          onSave={(value) => handleCellSave(record.id, column.name, value)}
                          onCancel={() => setEditingCell(null)}
                        />
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
          <ContextMenu
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
