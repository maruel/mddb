// Table view with inline editing.

import { createSignal, createEffect, onCleanup, For, Show } from 'solid-js';
import { Dynamic } from 'solid-js/web';
import { type DataRecordResponse, type Filter, type Property, SortAsc, SortDesc } from '@sdk/types.gen';
import styles from './TableTable.module.css';
import { RowHandle, ContextMenu, type ContextMenuAction } from './shared';
import { TABLE_RECORD_MIME } from './table/TableRow';
import { TableCell } from './table/TableCell';
import { AddColumnDropdown } from './table/AddColumnDropdown';
import { FilterPanel } from './table/FilterPanel';
import { useI18n } from '../i18n';
import { useRecords, DEFAULT_VIEW_ID } from '../contexts';
import { useClickOutside } from '../composables/useClickOutside';

import ArrowUpwardIcon from '@material-symbols/svg-400/outlined/arrow_upward.svg?solid';
import ArrowDownwardIcon from '@material-symbols/svg-400/outlined/arrow_downward.svg?solid';
import FilterAltIcon from '@material-symbols/svg-400/outlined/filter_alt.svg?solid';
import CloseIcon from '@material-symbols/svg-400/outlined/close.svg?solid';
import OpenInFullIcon from '@material-symbols/svg-400/outlined/open_in_full.svg?solid';
import AbcIcon from '@material-symbols/svg-400/outlined/abc.svg?solid';
import NumbersIcon from '@material-symbols/svg-400/outlined/numbers.svg?solid';
import CheckBoxOutlineBlankIcon from '@material-symbols/svg-400/outlined/check_box_outline_blank.svg?solid';
import CalendarMonthIcon from '@material-symbols/svg-400/outlined/calendar_month.svg?solid';
import LabelIcon from '@material-symbols/svg-400/outlined/label.svg?solid';
import LinkIcon from '@material-symbols/svg-400/outlined/link.svg?solid';
import AlternateEmailIcon from '@material-symbols/svg-400/outlined/alternate_email.svg?solid';

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
  onReorderColumns?: (newOrder: Property[]) => void;
  onLoadMore?: () => void;
  hasMore?: boolean;
}

const DEFAULT_COL_WIDTH = 150;
const MIN_COL_WIDTH = 50;

const COLUMN_TYPE_ICONS: Record<string, SolidSVG> = {
  text: AbcIcon,
  number: NumbersIcon,
  checkbox: CheckBoxOutlineBlankIcon,
  date: CalendarMonthIcon,
  select: LabelIcon,
  multi_select: LabelIcon,
  url: LinkIcon,
  email: AlternateEmailIcon,
  phone: AlternateEmailIcon,
};

export default function TableTable(props: TableTableProps) {
  const { t } = useI18n();
  const { setSorts, setFilters, updateView, activeViewId, activeSorts, activeFilters, views } = useRecords();

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

  // Column resize state
  const [dragWidths, setDragWidths] = createSignal<Record<string, number>>({});
  const [resizing, setResizing] = createSignal<{
    colName: string;
    startX: number;
    startWidth: number;
  } | null>(null);

  const storedWidth = (colName: string): number | undefined => {
    const vid = activeViewId();
    const cols = views().find((v) => v.id === vid)?.columns;
    return cols?.find((vc) => vc.property === colName)?.width;
  };

  const colWidth = (colName: string): number => dragWidths()[colName] ?? storedWidth(colName) ?? DEFAULT_COL_WIDTH;

  // Column drag-to-reorder state
  const [draggingColName, setDraggingColName] = createSignal<string | null>(null);
  const [dragOverColName, setDragOverColName] = createSignal<string | null>(null);

  const handleColDragStart = (e: DragEvent, colName: string) => {
    if (resizing()) {
      e.preventDefault();
      return;
    }
    e.dataTransfer?.setData('text/plain', colName);
    setDraggingColName(colName);
  };

  const handleColDragOver = (e: DragEvent, colName: string) => {
    e.preventDefault();
    if (colName !== draggingColName()) setDragOverColName(colName);
  };

  const handleColDragEnd = () => {
    setDraggingColName(null);
    setDragOverColName(null);
  };

  const handleColDrop = (e: DragEvent, targetColName: string) => {
    e.preventDefault();
    const srcName = draggingColName();
    setDraggingColName(null);
    setDragOverColName(null);
    if (!srcName || srcName === targetColName || !props.onReorderColumns) return;
    const cols = [...props.columns];
    const srcIdx = cols.findIndex((c) => c.name === srcName);
    if (srcIdx < 0) return;
    const [moved] = cols.splice(srcIdx, 1) as [Property];
    const targetIdx = cols.findIndex((c) => c.name === targetColName);
    if (targetIdx < 0) return;
    cols.splice(targetIdx, 0, moved);
    props.onReorderColumns(cols);
  };

  // Column visibility
  const [hiddenDropdownOpen, setHiddenDropdownOpen] = createSignal(false);
  let hiddenDropdownRef: HTMLDivElement | undefined;
  useClickOutside(
    () => hiddenDropdownRef,
    () => setHiddenDropdownOpen(false)
  );

  const isColumnVisible = (colName: string): boolean => {
    const vid = activeViewId();
    const cols = views().find((v) => v.id === vid)?.columns;
    if (!cols || cols.length === 0) return true;
    const entry = cols.find((vc) => vc.property === colName);
    return entry ? entry.visible : true;
  };

  const visibleColumns = () => props.columns.filter((col) => isColumnVisible(col.name));
  const hiddenColumns = () => props.columns.filter((col) => !isColumnVisible(col.name));

  const hideColumn = (colName: string) => {
    const viewId = activeViewId();
    if (!viewId || viewId === DEFAULT_VIEW_ID) return;
    if (visibleColumns().length <= 1) return;
    const existing = views().find((v) => v.id === viewId)?.columns ?? [];
    const hasEntry = existing.some((vc) => vc.property === colName);
    const newCols = hasEntry
      ? existing.map((vc) => (vc.property === colName ? { ...vc, visible: false } : vc))
      : [...existing, { property: colName, visible: false }];
    updateView(viewId, { columns: newCols });
  };

  const showColumn = (colName: string) => {
    const viewId = activeViewId();
    if (!viewId || viewId === DEFAULT_VIEW_ID) return;
    const existing = views().find((v) => v.id === viewId)?.columns ?? [];
    const newCols = existing.map((vc) => (vc.property === colName ? { ...vc, visible: true } : vc));
    updateView(viewId, { columns: newCols });
  };

  const handleResizeStart = (e: MouseEvent, colName: string) => {
    e.preventDefault();
    e.stopPropagation();
    const th = (e.currentTarget as HTMLElement).closest('th');
    const startWidth = th?.getBoundingClientRect().width ?? colWidth(colName);
    setResizing({ colName, startX: e.clientX, startWidth });
  };

  createEffect(() => {
    if (!resizing()) return;
    const handleMouseMove = (e: MouseEvent) => {
      const s = resizing();
      if (!s) return;
      const newWidth = Math.max(MIN_COL_WIDTH, s.startWidth + e.clientX - s.startX);
      setDragWidths((prev) => ({ ...prev, [s.colName]: newWidth }));
    };
    const handleMouseUp = () => {
      const s = resizing();
      if (!s) return;
      const finalWidth = dragWidths()[s.colName] ?? s.startWidth;
      const viewId = activeViewId();
      if (viewId && viewId !== DEFAULT_VIEW_ID) {
        const existing = views().find((v) => v.id === viewId)?.columns ?? [];
        const hasEntry = existing.some((vc) => vc.property === s.colName);
        const newCols = hasEntry
          ? existing.map((vc) => (vc.property === s.colName ? { ...vc, width: finalWidth } : vc))
          : [...existing, { property: s.colName, width: finalWidth, visible: true }];
        updateView(viewId, { columns: newCols });
      }
      setResizing(null);
    };
    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
    onCleanup(() => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    });
  });

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

  // Left-click on column header → cycle sort (none → asc → desc → none)
  const handleHeaderClick = (e: MouseEvent, colIndex: number) => {
    if ((e.target as HTMLElement).tagName === 'INPUT') return;
    const column = props.columns[colIndex];
    if (!column) return;
    const existing = activeSorts().find((s) => s.property === column.name);
    if (!existing) {
      applySort(colIndex, SortAsc);
    } else if (existing.direction === SortAsc) {
      applySort(colIndex, SortDesc);
    } else {
      removeSort(colIndex);
    }
  };

  // Right-click on column header → show context menu
  const handleHeaderContextMenu = (e: MouseEvent, colIndex: number) => {
    if ((e.target as HTMLElement).tagName === 'INPUT') return;
    e.preventDefault();
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

    if (activeViewId() !== DEFAULT_VIEW_ID) {
      actions.push({
        id: 'hide-column',
        label: t('table.hideColumn') || 'Hide column',
        disabled: visibleColumns().length <= 1,
        separator: true,
      });
    }

    if (props.onInsertColumn) {
      actions.push(
        {
          id: 'insert-left',
          label: t('table.insertColumnLeft') || 'Insert Left',
          separator: !actions.find((a) => a.id === 'hide-column'),
        },
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
      case 'hide-column':
        hideColumn(column.name);
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

  // Navigate to an adjacent cell after Tab/Enter
  const moveFocus = (direction: 'next' | 'prev' | 'down') => {
    const current = editingCell();
    if (!current) return;
    const cols = visibleColumns();
    const rows = props.records;
    const colIdx = cols.findIndex((c) => c.name === current.columnId);
    const rowIdx = rows.findIndex((r) => r.id === current.recordId);
    if (colIdx < 0 || rowIdx < 0) return;

    let nextColIdx = colIdx;
    let nextRowIdx = rowIdx;

    if (direction === 'next') {
      if (colIdx < cols.length - 1) {
        nextColIdx = colIdx + 1;
      } else {
        nextColIdx = 0;
        nextRowIdx = rowIdx + 1;
      }
    } else if (direction === 'prev') {
      if (colIdx > 0) {
        nextColIdx = colIdx - 1;
      } else {
        nextColIdx = cols.length - 1;
        nextRowIdx = rowIdx - 1;
      }
    } else {
      nextRowIdx = rowIdx + 1;
    }

    if (nextRowIdx < 0 || nextRowIdx >= rows.length) return;
    const nextCol = cols[nextColIdx];
    const nextRow = rows[nextRowIdx];
    if (!nextCol || !nextRow) return;
    setEditingCell({ recordId: nextRow.id, columnId: nextCol.name });
  };

  const removeSortByName = (colName: string) => {
    const idx = props.columns.findIndex((c) => c.name === colName);
    if (idx >= 0) removeSort(idx);
  };

  const removeFilterByName = (colName: string) => {
    const idx = props.columns.findIndex((c) => c.name === colName);
    if (idx >= 0) removeFilter(idx);
  };

  return (
    <div class={styles.container}>
      <Show when={activeSorts().length > 0 || activeFilters().length > 0}>
        <div class={styles.chipsBar} data-testid="active-chips-bar">
          <For each={activeSorts()}>
            {(sort) => (
              <div class={styles.chip} data-testid={`sort-chip-${sort.property}`}>
                <span class={styles.chipIcon}>
                  <Show when={sort.direction === SortAsc} fallback={<ArrowDownwardIcon />}>
                    <ArrowUpwardIcon />
                  </Show>
                </span>
                <span class={styles.chipLabel}>{sort.property}</span>
                <button
                  class={styles.chipRemove}
                  onClick={() => removeSortByName(sort.property)}
                  title={t('table.removeSort') || 'Remove sort'}
                >
                  <CloseIcon />
                </button>
              </div>
            )}
          </For>
          <For each={activeFilters()}>
            {(filter) => {
              const prop = filter.property ?? '';
              return (
                <div class={`${styles.chip} ${styles.chipFilter}`} data-testid={`filter-chip-${prop}`}>
                  <span class={styles.chipIcon}>
                    <FilterAltIcon />
                  </span>
                  <span class={styles.chipLabel}>
                    {prop}
                    <Show when={filter.value !== undefined && filter.value !== ''}>
                      {': '}
                      {String(filter.value)}
                    </Show>
                  </span>
                  <button
                    class={styles.chipRemove}
                    onClick={() => removeFilterByName(prop)}
                    title={t('table.removeFilter') || 'Remove filter'}
                  >
                    <CloseIcon />
                  </button>
                </div>
              );
            }}
          </For>
        </div>
      </Show>
      <div class={styles.tableWrapper}>
        <table class={styles.table} classList={{ [`${styles.resizing}`]: !!resizing() }}>
          <thead>
            <tr class={styles.headerRow}>
              {/* Handle column header */}
              <th class={styles.handleHeader} />
              <Show when={props.onOpenRecord}>
                <th class={styles.expandHeader} />
              </Show>
              <Show when={props.onDeleteRecord}>
                <th class={styles.actionsHeader} />
              </Show>
              <For each={visibleColumns()}>
                {(column) => {
                  const realIndex = () => props.columns.indexOf(column);
                  return (
                    <th
                      class={styles.headerCell}
                      classList={{
                        [`${styles.colDragging}`]: draggingColName() === column.name,
                        [`${styles.colDragOver}`]: dragOverColName() === column.name,
                      }}
                      style={{ width: `${colWidth(column.name)}px`, 'min-width': `${MIN_COL_WIDTH}px` }}
                      draggable={!!props.onReorderColumns && realIndex() > 0}
                      onClick={(e) => handleHeaderClick(e, realIndex())}
                      onContextMenu={(e) => handleHeaderContextMenu(e, realIndex())}
                      onDragStart={(e) => handleColDragStart(e, column.name)}
                      onDragOver={(e) => realIndex() > 0 && handleColDragOver(e, column.name)}
                      onDrop={(e) => realIndex() > 0 && handleColDrop(e, column.name)}
                      onDragEnd={handleColDragEnd}
                    >
                      <Show
                        when={renamingColumn() === realIndex()}
                        fallback={
                          <>
                            <Show when={COLUMN_TYPE_ICONS[column.type]}>
                              {(Icon) => (
                                <span class={styles.typeIcon}>
                                  <Dynamic component={Icon()} />
                                </span>
                              )}
                            </Show>
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
                      <div
                        class={styles.resizeHandle}
                        classList={{ [`${styles.resizeHandleActive}`]: resizing()?.colName === column.name }}
                        onMouseDown={(e) => handleResizeStart(e, column.name)}
                      />
                    </th>
                  );
                }}
              </For>
              <Show when={props.onAddColumn}>
                {(onAddColumn) => (
                  <th class={styles.addColumnTh}>
                    <AddColumnDropdown onAddColumn={onAddColumn()} />
                  </th>
                )}
              </Show>
              <Show when={hiddenColumns().length > 0}>
                <th class={styles.hiddenColumnsCell}>
                  <div ref={(el) => (hiddenDropdownRef = el)} class={styles.hiddenColumnsWrapper}>
                    <button
                      class={styles.hiddenColumnsBtn}
                      data-testid="hidden-columns-btn"
                      onClick={() => setHiddenDropdownOpen(!hiddenDropdownOpen())}
                    >
                      {hiddenColumns().length} {t('table.hiddenColumns') || 'hidden'}
                    </button>
                    <Show when={hiddenDropdownOpen()}>
                      <div class={styles.hiddenColumnsDropdown} data-testid="hidden-columns-dropdown">
                        <For each={hiddenColumns()}>
                          {(col) => (
                            <div class={styles.hiddenColumnItem}>
                              <span>{col.name}</span>
                              <button onClick={() => showColumn(col.name)}>{t('table.showColumn') || 'Show'}</button>
                            </div>
                          )}
                        </For>
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
                  <Show when={props.onOpenRecord}>
                    {(onOpen) => (
                      <td class={styles.expandCell}>
                        <button
                          class={styles.expandBtn}
                          onClick={(e) => {
                            e.stopPropagation();
                            onOpen()(record.id);
                          }}
                          title={t('table.openRecord') || 'Open'}
                        >
                          <OpenInFullIcon />
                        </button>
                      </td>
                    )}
                  </Show>
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
                  <For each={visibleColumns()}>
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
                          onTabNext={() => moveFocus('next')}
                          onTabPrev={() => moveFocus('prev')}
                          onEnterDown={() => moveFocus('down')}
                          onUpdateOptions={(opts) => {
                            const idx = props.columns.indexOf(column);
                            if (idx >= 0) props.onUpdateColumn?.(idx, { ...column, options: opts });
                          }}
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
                  colSpan={
                    visibleColumns().length +
                    1 +
                    (props.onOpenRecord ? 1 : 0) +
                    (props.onDeleteRecord ? 1 : 0) +
                    (props.onAddColumn ? 1 : 0) +
                    (hiddenColumns().length > 0 ? 1 : 0)
                  }
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

      <div class={styles.statusBar}>
        {props.records.length} {t('table.recordCount') || 'records'}
      </div>

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
