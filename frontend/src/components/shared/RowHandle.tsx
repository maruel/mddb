import styles from './RowHandle.module.css';

export interface RowHandleProps {
  /** Unique identifier for the row (block position, record ID, etc.) */
  rowId: string;
  /** Called when drag starts - consumer sets up drag data */
  onDragStart: (e: DragEvent, rowId: string) => void;
  /** Called on right-click - consumer shows context menu */
  onContextMenu: (e: MouseEvent, rowId: string) => void;
  /** Optional: called on handle click (e.g., to select row) */
  onClick?: (e: MouseEvent, rowId: string) => void;
  /** Optional: additional CSS class */
  class?: string;
}

/**
 * A shared drag handle component used for table rows.
 * Inspired by Notion's 6-dot drag handle.
 */
export function RowHandle(props: RowHandleProps) {
  const handleDragStart = (e: DragEvent) => {
    props.onDragStart(e, props.rowId);
  };

  const handleContextMenu = (e: MouseEvent) => {
    e.preventDefault();
    props.onContextMenu(e, props.rowId);
  };

  const handleClick = (e: MouseEvent) => {
    props.onClick?.(e, props.rowId);
  };

  return (
    <div
      class={`${styles.handle} ${props.class || ''}`}
      draggable="true"
      onDragStart={handleDragStart}
      onContextMenu={handleContextMenu}
      onClick={handleClick}
      aria-label="Drag handle"
      data-testid="row-handle"
    >
      <svg viewBox="0 0 12 16" class={styles.icon} fill="currentColor" xmlns="http://www.w3.org/2000/svg">
        <circle cx="3" cy="3" r="1.5" />
        <circle cx="3" cy="8" r="1.5" />
        <circle cx="3" cy="13" r="1.5" />
        <circle cx="9" cy="3" r="1.5" />
        <circle cx="9" cy="8" r="1.5" />
        <circle cx="9" cy="13" r="1.5" />
      </svg>
    </div>
  );
}
