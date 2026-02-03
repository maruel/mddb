import { createEffect, createSignal, For, Show, type JSX, onCleanup } from 'solid-js';
import { Portal } from 'solid-js/web';
import styles from './RowContextMenu.module.css';

export interface ContextMenuAction {
  id: string;
  label: string;
  icon?: JSX.Element;
  shortcut?: string;
  disabled?: boolean;
  danger?: boolean; // Red text for destructive actions
  separator?: boolean; // Render separator before this item
}

export interface RowContextMenuProps {
  position: { x: number; y: number };
  actions: ContextMenuAction[];
  onAction: (actionId: string) => void;
  onClose: () => void;
}

/**
 * A shared context menu component for editor blocks and table rows.
 * Supports keyboard navigation, click-outside to close, and viewport boundary detection.
 */
export function RowContextMenu(props: RowContextMenuProps) {
  let menuRef: HTMLDivElement | undefined;
  const [focusedIndex, setFocusedIndex] = createSignal(0);
  const [adjustedPosition, setAdjustedPosition] = createSignal({ x: 0, y: 0 });

  // Get non-disabled action indices for keyboard navigation
  const getEnabledIndices = () =>
    props.actions
      .map((action, i) => ({ action, i }))
      .filter(({ action }) => !action.disabled)
      .map(({ i }) => i);

  // Adjust position to stay within viewport
  createEffect(() => {
    if (!menuRef) return;

    const rect = menuRef.getBoundingClientRect();
    const viewportWidth = window.innerWidth;
    const viewportHeight = window.innerHeight;
    const padding = 8;

    let x = props.position.x;
    let y = props.position.y;

    // Adjust horizontal position if menu would overflow right edge
    if (x + rect.width > viewportWidth - padding) {
      x = Math.max(padding, viewportWidth - rect.width - padding);
    }

    // Adjust vertical position if menu would overflow bottom edge
    if (y + rect.height > viewportHeight - padding) {
      y = Math.max(padding, viewportHeight - rect.height - padding);
    }

    setAdjustedPosition({ x, y });
  });

  // Handle click outside to close
  createEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef && !menuRef.contains(e.target as Node)) {
        props.onClose();
      }
    };

    // Use setTimeout to avoid immediately closing from the triggering right-click
    const timeoutId = setTimeout(() => {
      document.addEventListener('click', handleClickOutside);
      document.addEventListener('contextmenu', handleClickOutside);
    }, 0);

    onCleanup(() => {
      clearTimeout(timeoutId);
      document.removeEventListener('click', handleClickOutside);
      document.removeEventListener('contextmenu', handleClickOutside);
    });
  });

  // Handle keyboard navigation
  createEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const enabledIndices = getEnabledIndices();
      if (enabledIndices.length === 0) return;

      switch (e.key) {
        case 'Escape':
          e.preventDefault();
          props.onClose();
          break;

        case 'ArrowDown': {
          e.preventDefault();
          const currentPos = enabledIndices.indexOf(focusedIndex());
          const nextPos = currentPos >= 0 ? (currentPos + 1) % enabledIndices.length : 0;
          const nextIndex = enabledIndices[nextPos];
          if (nextIndex !== undefined) {
            setFocusedIndex(nextIndex);
          }
          break;
        }

        case 'ArrowUp': {
          e.preventDefault();
          const currentPos = enabledIndices.indexOf(focusedIndex());
          const prevPos =
            currentPos >= 0
              ? (currentPos - 1 + enabledIndices.length) % enabledIndices.length
              : enabledIndices.length - 1;
          const prevIndex = enabledIndices[prevPos];
          if (prevIndex !== undefined) {
            setFocusedIndex(prevIndex);
          }
          break;
        }

        case 'Enter':
        case ' ':
          e.preventDefault();
          {
            const action = props.actions[focusedIndex()];
            if (action && !action.disabled) {
              props.onAction(action.id);
              props.onClose();
            }
          }
          break;

        case 'Tab':
          // Prevent tabbing out of menu
          e.preventDefault();
          break;
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    onCleanup(() => document.removeEventListener('keydown', handleKeyDown));
  });

  // Focus menu on mount for keyboard accessibility
  createEffect(() => {
    menuRef?.focus();
  });

  const handleAction = (action: ContextMenuAction) => {
    if (action.disabled) return;
    props.onAction(action.id);
    props.onClose();
  };

  // Build classList object for each button (reactive in SolidJS)
  const getItemClassList = (action: ContextMenuAction, idx: () => number): Record<string, boolean | undefined> => ({
    [styles.item as string]: true,
    [styles.danger as string]: action.danger,
    [styles.disabled as string]: action.disabled,
    [styles.focused as string]: focusedIndex() === idx(),
  });

  return (
    <Portal>
      <div
        ref={menuRef}
        class={styles.menu}
        style={{
          left: `${adjustedPosition().x}px`,
          top: `${adjustedPosition().y}px`,
        }}
        role="menu"
        tabIndex={-1}
        aria-label="Context menu"
      >
        <For each={props.actions}>
          {(action, index) => (
            <>
              <Show when={action.separator}>
                <div class={styles.separator} role="separator" />
              </Show>
              <button
                classList={getItemClassList(action, index)}
                onClick={() => handleAction(action)}
                onMouseEnter={() => setFocusedIndex(index())}
                role="menuitem"
                disabled={action.disabled}
                tabIndex={-1}
                aria-disabled={action.disabled}
              >
                <Show when={action.icon}>
                  <span class={styles.icon}>{action.icon}</span>
                </Show>
                <span class={styles.label}>{action.label}</span>
                <Show when={action.shortcut}>
                  <span class={styles.shortcut}>{action.shortcut}</span>
                </Show>
              </button>
            </>
          )}
        </For>
      </div>
    </Portal>
  );
}
