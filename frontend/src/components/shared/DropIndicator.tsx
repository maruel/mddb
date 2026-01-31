import { Show } from 'solid-js';
import styles from './DropIndicator.module.css';

export interface DropIndicatorProps {
  /** Y position relative to container */
  y: number;
  /** Whether indicator is visible */
  visible: boolean;
  /** Optional: indentation level (multiplied by 24px) */
  indent?: number;
}

/**
 * Visual indicator shown between rows during drag-drop operations.
 * Displays a horizontal line with circular end caps to indicate drop position.
 */
export function DropIndicator(props: DropIndicatorProps) {
  return (
    <Show when={props.visible}>
      <div
        class={styles.indicator}
        style={{
          top: `${props.y}px`,
          left: `${(props.indent || 0) * 24}px`,
        }}
        role="presentation"
        aria-hidden="true"
      />
    </Show>
  );
}
