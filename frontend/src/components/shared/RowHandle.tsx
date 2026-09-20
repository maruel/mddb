// Shared drag handle component rendering a 6-dot grip icon for draggable rows.
import styles from "./RowHandle.module.css";
import RowGripIcon from "./RowGripIcon.svg?solid";

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
 * Displays a 6-dot grip icon that appears on hover.
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
      class={`${styles.handle} ${props.class || ""}`}
      draggable="true"
      onDragStart={handleDragStart}
      onContextMenu={handleContextMenu}
      onClick={handleClick}
      aria-label="Drag handle"
      data-testid="row-handle"
    >
      <RowGripIcon class={styles.icon} />
    </div>
  );
}
