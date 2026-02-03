// Shared table row wrapper with drag handle and context menu.
// Used across TableTable, TableGrid, TableGallery, and TableBoard views.

import { createSignal, Show, type ParentProps } from 'solid-js';
import { Dynamic } from 'solid-js/web';
import { RowHandle, ContextMenu, type ContextMenuAction } from '../shared';
import { useI18n } from '../../i18n';
import styles from './TableRow.module.css';

/** MIME type for table record drag data */
export const TABLE_RECORD_MIME = 'application/x-table-record';

export interface TableRowProps extends ParentProps {
  /** The record ID */
  recordId: string;
  /** Callback when record should be deleted */
  onDelete?: (recordId: string) => void;
  /** Callback when record should be duplicated */
  onDuplicate?: (recordId: string) => void;
  /** Callback when record should be opened */
  onOpen?: (recordId: string) => void;
  /** Callback when drag starts (for reordering) */
  onDragStart?: (recordId: string, e: DragEvent) => void;
  /** Callback when dragging over this row */
  onDragOver?: (recordId: string, e: DragEvent) => void;
  /** Callback when dropping on this row */
  onDrop?: (recordId: string, e: DragEvent) => void;
  /** Whether this row is currently being dragged */
  isDragging?: boolean;
  /** Whether to show the drop indicator above this row */
  showDropIndicator?: boolean;
  /** Additional CSS class for the row container */
  class?: string;
  /** Element type: 'div' for card views, 'tr' for table view */
  as?: 'div' | 'tr';
}

/**
 * Table row wrapper component with integrated drag handle and context menu.
 * Provides consistent drag-and-drop behavior across all table views.
 */
export function TableRow(props: TableRowProps) {
  const { t } = useI18n();
  const [menuState, setMenuState] = createSignal<{ x: number; y: number } | null>(null);

  // Build context menu actions based on available callbacks
  const getActions = (): ContextMenuAction[] => {
    const actions: ContextMenuAction[] = [];

    if (props.onOpen) {
      actions.push({
        id: 'open',
        label: t('table.openRecord') || 'Open',
      });
    }

    if (props.onDuplicate) {
      actions.push({
        id: 'duplicate',
        label: t('table.duplicateRecord') || 'Duplicate',
        shortcut: '⌘D',
      });
    }

    if (props.onDelete) {
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

  const handleDragStart = (e: DragEvent, _rowId: string) => {
    e.dataTransfer?.setData(TABLE_RECORD_MIME, props.recordId);
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move';
    }
    props.onDragStart?.(props.recordId, e);
  };

  const handleContextMenu = (e: MouseEvent, _rowId: string) => {
    setMenuState({ x: e.clientX, y: e.clientY });
  };

  const handleAction = (actionId: string) => {
    switch (actionId) {
      case 'open':
        props.onOpen?.(props.recordId);
        break;
      case 'duplicate':
        props.onDuplicate?.(props.recordId);
        break;
      case 'delete':
        props.onDelete?.(props.recordId);
        break;
    }
    setMenuState(null);
  };

  const handleDragOver = (e: DragEvent) => {
    e.preventDefault();
    if (e.dataTransfer) {
      e.dataTransfer.dropEffect = 'move';
    }
    props.onDragOver?.(props.recordId, e);
  };

  const handleDrop = (e: DragEvent) => {
    e.preventDefault();
    props.onDrop?.(props.recordId, e);
  };

  const containerClass = () =>
    `${styles.row} row-with-handle ${props.isDragging ? styles.dragging : ''} ${props.class || ''}`.trim();

  return (
    <>
      <Show when={props.showDropIndicator}>
        <div class={styles.dropIndicator} />
      </Show>
      <Dynamic
        component={props.as === 'tr' ? 'tr' : 'div'}
        class={containerClass()}
        onDragOver={handleDragOver}
        onDrop={handleDrop}
      >
        <div class={styles.handleWrapper}>
          <RowHandle rowId={props.recordId} onDragStart={handleDragStart} onContextMenu={handleContextMenu} />
        </div>
        {props.children}
      </Dynamic>

      <Show when={menuState()}>
        {(state) => (
          <ContextMenu
            position={state()}
            actions={getActions()}
            onAction={handleAction}
            onClose={() => setMenuState(null)}
          />
        )}
      </Show>
    </>
  );
}
