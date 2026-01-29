// Slash command menu overlay component for selecting block types.

import { createSignal, createEffect, For, Show, onMount, onCleanup } from 'solid-js';
import type { EditorView } from 'prosemirror-view';
import { useI18n } from '../../i18n';
import { useClickOutside } from '../../composables/useClickOutside';
import { filterCommands, type SlashCommand } from './slashCommands';
import { closeSlashMenu, slashMenuKey, type SlashMenuState } from './slashCommandPlugin';
import styles from './Editor.module.css';

interface SlashCommandMenuProps {
  view: EditorView;
  state: SlashMenuState;
}

export default function SlashCommandMenu(props: SlashCommandMenuProps) {
  const { t } = useI18n();
  const [selectedIndex, setSelectedIndex] = createSignal(0);
  const [adjustedPosition, setAdjustedPosition] = createSignal<{ top: number; left: number } | null>(null);
  let menuRef: HTMLDivElement | undefined;

  // Filter commands based on query
  const filteredCommands = () => filterCommands(props.state.query);

  // Reset selection and position when query changes or menu activates
  createEffect(() => {
    // Track query to trigger effect when it changes
    void props.state.query;
    void props.state.active;
    setSelectedIndex(0);
    setAdjustedPosition(null); // Reset adjusted position when menu state changes
  });

  // Adjust position after menu is rendered to prevent viewport overflow
  createEffect(() => {
    if (!props.state.active || !menuRef) return;

    // Wait for next frame to ensure the menu has been rendered
    requestAnimationFrame(() => {
      if (!menuRef) return;

      const menuRect = menuRef.getBoundingClientRect();
      const viewportHeight = window.innerHeight;
      const viewportWidth = window.innerWidth;

      let newTop = props.state.position.top;
      let newLeft = props.state.position.left;

      // If menu extends beyond viewport bottom, position it above the cursor
      if (newTop + menuRect.height > viewportHeight) {
        // Get the cursor coordinates to position above
        const coords = props.view.coordsAtPos(props.state.triggerPos);
        // Position menu above cursor (subtract menu height and a small gap)
        newTop = coords.top - menuRect.height - 4;

        // If still overflowing at top, just position at top of viewport with margin
        if (newTop < 0) {
          newTop = 8;
        }
      }

      // Prevent menu from extending beyond right edge
      if (newLeft + menuRect.width > viewportWidth) {
        newLeft = viewportWidth - menuRect.width - 8;
      }

      // Prevent menu from extending beyond left edge
      if (newLeft < 0) {
        newLeft = 8;
      }

      // Only update if position changed
      if (newTop !== props.state.position.top || newLeft !== props.state.position.left) {
        setAdjustedPosition({ top: newTop, left: newLeft });
      }
    });
  });

  // Scroll selected item into view when selection changes
  createEffect(() => {
    const index = selectedIndex();
    if (menuRef) {
      const selectedItem = menuRef.querySelector(`[data-index="${index}"]`);
      if (selectedItem) {
        selectedItem.scrollIntoView({ block: 'nearest' });
      }
    }
  });

  // Handle click outside to close menu
  useClickOutside(
    () => menuRef,
    () => {
      if (props.state.active) {
        closeSlashMenu(props.view);
      }
    }
  );

  // Handle keyboard navigation
  // Note: We check plugin state directly from ProseMirror rather than props
  // because props.state may be stale in event handlers (SolidJS closure issue)
  onMount(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Check plugin state directly from ProseMirror (authoritative source)
      const pluginState = slashMenuKey.getState(props.view.state);
      if (!pluginState?.active) return;

      const commands = filterCommands(pluginState.query);
      if (commands.length === 0) return;

      switch (e.key) {
        case 'ArrowDown':
          e.preventDefault();
          e.stopPropagation();
          setSelectedIndex((i) => (i + 1) % commands.length);
          break;
        case 'ArrowUp':
          e.preventDefault();
          e.stopPropagation();
          setSelectedIndex((i) => (i - 1 + commands.length) % commands.length);
          break;
        case 'Enter':
        case 'Tab':
          e.preventDefault();
          e.stopPropagation();
          executeCommand(commands[selectedIndex()]);
          break;
        case 'Escape':
          e.preventDefault();
          e.stopPropagation();
          closeSlashMenu(props.view);
          break;
      }
    };

    document.addEventListener('keydown', handleKeyDown, true);
    onCleanup(() => {
      document.removeEventListener('keydown', handleKeyDown, true);
    });
  });

  const executeCommand = (command: SlashCommand | undefined) => {
    if (!command) return;

    // Get current plugin state from ProseMirror (authoritative source)
    const pluginState = slashMenuKey.getState(props.view.state);
    if (!pluginState) return;

    const { triggerPos } = pluginState;
    const cursorPos = props.view.state.selection.from;

    // Execute the command (it will delete the "/" and query text)
    command.execute(props.view, triggerPos, cursorPos);

    // Focus back on editor
    props.view.focus();
  };

  const handleItemClick = (command: SlashCommand) => {
    executeCommand(command);
  };

  const handleItemMouseEnter = (index: number) => {
    setSelectedIndex(index);
  };

  // Use adjusted position if available, otherwise use original position
  const menuPosition = () => adjustedPosition() ?? props.state.position;

  return (
    <Show when={props.state.active}>
      <div
        ref={menuRef}
        class={styles.slashMenu}
        data-testid="slash-command-menu"
        style={{
          top: `${menuPosition().top}px`,
          left: `${menuPosition().left}px`,
        }}
      >
        <Show
          when={filteredCommands().length > 0}
          fallback={<div class={styles.slashMenuEmpty}>{t('slashMenu.noResults')}</div>}
        >
          <For each={filteredCommands()}>
            {(command, index) => (
              <div
                class={`${styles.slashMenuItem} ${index() === selectedIndex() ? styles.selected : ''}`}
                data-index={index()}
                onClick={() => handleItemClick(command)}
                onMouseEnter={() => handleItemMouseEnter(index())}
              >
                <span class={styles.slashMenuIcon}>{command.icon}</span>
                <span class={styles.slashMenuLabel}>{t(`slashMenu.${command.labelKey}`)}</span>
              </div>
            )}
          </For>
        </Show>
      </div>
    </Show>
  );
}
