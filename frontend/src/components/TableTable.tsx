// Notion-like table view with inline editing.

import { createSignal, For, Show } from 'solid-js';
import { type DataRecordResponse, type Filter, type Property, SortAsc, SortDesc } from '@sdk/types.gen';
import styles from './TableTable.module.css';
import { RowHandle, ContextMenu, type ContextMenuAction } from './shared';
import { TABLE_RECORD_MIME } from './table/TableRow';
import { TableCell } from './table/TableCell';
import { AddColumnDropdown } from './table/AddColumnDropdown';
import { FilterPanel } from './table/FilterPanel';
import { useI18n } from '../i18n';
import { useRecords, DEFAULT_VIEW_ID } from '../contexts';

import ArrowUpwardIcon from '@material-symbols/svg-400/outlined/arrow_upward.svg?solid';
import ArrowDownwardIcon from '@material-symbols/svg-400/outlined/arrow_downward.svg?solid';
import FilterAltIcon from '@material-symbols/svg-400/outlined/filter_alt.svg?solid';

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
  onUpdateColumn?: (index: number, column: Property) => void;
  onDeleteColumn?: (index: number) => void;
  onInsertColumn?: (beforeIndex: number) => void;
  onLoadMore?: () => void;
  hasMore?: boolean;
}

export default function TableTable(props: TableTableProps) {
  const { t } = useI18n();
  const { setSorts, setFilters, updateView, activeViewId, activeSorts, activeFilters } = useRecords();

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

  // Column header menu state
  const [columnMenu, setColumnMenu] = createSignal<{
    colIndex: number;
    x: number;
    y: number;
  } | null>(null);

  // Filter panel state
  const [filterPanel, setFilterPanel] = createSignal<{
    colIndex: number;
    column: Property;
    x: number;
    y: number;
  } | null>(null);

  // Inline column rename state
  const [renamingColumn, setRenamingColumn] = createSignal<number | null>(null);
  const [renameValue, setRenameValue] = createSignal('');

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

  // Column header click → show menu below the header
  const handleHeaderClick = (e: MouseEvent, colIndex: number) => {
    // Don't open menu when clicking the rename input
    if ((e.target as HTMLElement).tagName === 'INPUT') return;
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    setColumnMenu({ colIndex, x: rect.left, y: rect.bottom + 4 });
  };

  const getColumnActions = (colIndex: number): ContextMenuAction[] => {
    const column = props.columns[colIndex];
    const activeSort = column ? activeSorts().find((s) => s.property === column.name) : undefined;
    const activeFilter = column ? activeFilters().find((f) => f.property === column.name) : undefined;

    const actions: ContextMenuAction[] = [
      { id: 'rename', label: t('table.renameColumn') || 'Rename' },
      { id: 'sort-asc', label: t('table.sortAscending') || 'Sort Ascending' },
      { id: 'sort-desc', label: t('table.sortDescending') || 'Sort Descending' },
    ];

    if (activeSort) {
      actions.push({
        id: 'remove-sort',
        label: t('table.removeSort') || 'Remove sort',
        separator: true,
      });
    }

    actions.push({
      id: 'filter-by',
      label: activeFilter ? `${t('table.filterBy') || 'Filter by...'} \u2713` : t('table.filterBy') || 'Filter by...',
      separator: true,
    });

    if (props.onInsertColumn) {
      actions.push(
        { id: 'insert-left', label: t('table.insertColumnLeft') || 'Insert Left', separator: true },
        { id: 'insert-right', label: t('table.insertColumnRight') || 'Insert Right' }
      );
    }

    if (props.onDeleteColumn && props.columns.length > 1) {
      actions.push({
        id: 'delete-column',
        label: t('table.deleteColumn') || 'Delete column',
        danger: true,
        separator: true,
      });
    }

    return actions;
  };

  const applySort = (colIndex: number, direction: typeof SortAsc | typeof SortDesc) => {
    const column = props.columns[colIndex];
    if (!column) return;
    const existing = activeSorts();
    const existingIdx = existing.findIndex((s) => s.property === column.name);
    const newSorts =
      existingIdx >= 0
        ? existing.map((s, i) => (i === existingIdx ? { ...s, direction } : s))
        : [...existing, { property: column.name, direction }];
    setSorts(newSorts);
    const viewId = activeViewId();
    if (viewId && viewId !== DEFAULT_VIEW_ID) {
      updateView(viewId, { sorts: newSorts });
    }
  };

  const removeSort = (colIndex: number) => {
    const column = props.columns[colIndex];
    if (!column) return;
    const newSorts = activeSorts().filter((s) => s.property !== column.name);
    setSorts(newSorts);
    const viewId = activeViewId();
    if (viewId && viewId !== DEFAULT_VIEW_ID) {
      updateView(viewId, { sorts: newSorts });
    }
  };

  const applyFilter = (colIndex: number, filter: Filter) => {
    const column = props.columns[colIndex];
    if (!column) return;
    const existing = activeFilters();
    const idx = existing.findIndex((f) => f.property === column.name);
    const newFilters = idx >= 0 ? existing.map((f, i) => (i === idx ? filter : f)) : [...existing, filter];
    setFilters(newFilters);
    const viewId = activeViewId();
    if (viewId && viewId !== DEFAULT_VIEW_ID) {
      updateView(viewId, { filters: newFilters });
    }
  };

  const removeFilter = (colIndex: number) => {
    const column = props.columns[colIndex];
    if (!column) return;
    const newFilters = activeFilters().filter((f) => f.property !== column.name);
    setFilters(newFilters);
    const viewId = activeViewId();
    if (viewId && viewId !== DEFAULT_VIEW_ID) {
      updateView(viewId, { filters: newFilters });
    }
  };

  const handleColumnAction = (actionId: string) => {
    const state = columnMenu();
    if (!state) return;
    const column = props.columns[state.colIndex];
    setColumnMenu(null);
    if (!column) return;

    switch (actionId) {
      case 'rename':
        setRenameValue(column.name);
        setRenamingColumn(state.colIndex);
        break;
      case 'sort-asc':
        applySort(state.colIndex, SortAsc);
        break;
      case 'sort-desc':
        applySort(state.colIndex, SortDesc);
        break;
      case 'remove-sort':
        removeSort(state.colIndex);
        break;
      case 'filter-by':
        setFilterPanel({ colIndex: state.colIndex, column, x: state.x, y: state.y });
        break;
      case 'insert-left':
        props.onInsertColumn?.(state.colIndex);
        break;
      case 'insert-right':
        props.onInsertColumn?.(state.colIndex + 1);
        break;
      case 'delete-column':
        if (confirm(t('table.confirmDeleteColumn') || 'Delete this column and all its data?')) {
          props.onDeleteColumn?.(state.colIndex);
        }
        break;
    }
  };

  const commitRename = () => {
    const idx = renamingColumn();
    if (idx === null) return;
    const column = props.columns[idx];
    const newName = renameValue().trim();
    if (column && newName && newName !== column.name) {
      props.onUpdateColumn?.(idx, { ...column, name: newName });
    }
    setRenamingColumn(null);
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
                {(column, colIndex) => (
                  <th class={styles.headerCell} onClick={(e) => handleHeaderClick(e, colIndex())}>
                    <Show
                      when={renamingColumn() === colIndex()}
                      fallback={
                        <>
                          {column.name}
                          <Show when={column.required}>
                            <span class={styles.required}>*</span>
                          </Show>
                          <Show when={activeSorts().find((s) => s.property === column.name)}>
                            {(sort) => (
                              <span class={styles.sortIndicator} data-testid="sort-indicator">
                                <Show when={sort().direction === SortAsc} fallback={<ArrowDownwardIcon />}>
                                  <ArrowUpwardIcon />
                                </Show>
                              </span>
                            )}
                          </Show>
                          <Show when={activeFilters().find((f) => f.property === column.name)}>
                            <span class={styles.filterIndicator} data-testid="filter-indicator">
                              <FilterAltIcon />
                            </span>
                          </Show>
                        </>
                      }
                    >
                      <input
                        class={styles.renameInput}
                        value={renameValue()}
                        onInput={(e) => setRenameValue(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') commitRename();
                          if (e.key === 'Escape') setRenamingColumn(null);
                        }}
                        onBlur={commitRename}
                        ref={(el) => setTimeout(() => el?.select(), 0)}
                      />
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

      {/* Column header menu */}
      <Show when={columnMenu()}>
        {(state) => (
          <ContextMenu
            position={{ x: state().x, y: state().y }}
            actions={getColumnActions(state().colIndex)}
            onAction={handleColumnAction}
            onClose={() => setColumnMenu(null)}
          />
        )}
      </Show>

      {/* Filter panel */}
      <Show when={filterPanel()}>
        {(state) => (
          <FilterPanel
            column={state().column}
            position={{ x: state().x, y: state().y }}
            currentFilter={activeFilters().find((f) => f.property === state().column.name)}
            onApply={(f) => applyFilter(state().colIndex, f)}
            onRemove={() => removeFilter(state().colIndex)}
            onClose={() => setFilterPanel(null)}
          />
        )}
      </Show>
    </div>
  );
}
