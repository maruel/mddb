// Slash command menu overlay component for selecting block types.

import { createSignal, createEffect, For, Show, onMount, onCleanup } from 'solid-js';
import { useNavigate } from '@solidjs/router';
import type { EditorView } from 'prosemirror-view';
import { schema, marks } from './prosemirror-config';
import { useI18n } from '../../i18n';
import { useAuth } from '../../contexts/AuthContext';
import { useWorkspace } from '../../contexts/WorkspaceContext';
import { useEditor } from '../../contexts/EditorContext';
import { useClickOutside } from '../../composables/useClickOutside';
import { nodeUrl } from '../../utils/urls';
import { filterCommands, type SlashCommand } from './slashCommands';
import { closeSlashMenu, slashMenuKey, type SlashMenuState } from './slashCommandPlugin';
import styles from './Editor.module.css';

interface SlashCommandMenuProps {
  view: EditorView;
  state: SlashMenuState;
  nodeId?: string;
}

export default function SlashCommandMenu(props: SlashCommandMenuProps) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const { user, wsApi } = useAuth();
  const { loadNode, fetchNodeChildren } = useWorkspace();
  const { flushAutoSave } = useEditor();
  const [selectedIndex, setSelectedIndex] = createSignal(0);
  const [adjustedPosition, setAdjustedPosition] = createSignal<{ top: number; left: number } | null>(null);
  let menuRef: HTMLDivElement | undefined;

  // Filter commands based on query (pass translate function for display text matching)
  const filteredCommands = () => filterCommands(props.state.query, t);

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

      // If position is (0, 0), it's likely invalid - recalculate from trigger position
      if (newTop === 0 && newLeft === 0) {
        try {
          const coords = props.view.coordsAtPos(props.state.triggerPos);
          newTop = coords.bottom + 4;
          newLeft = coords.left;
        } catch {
          // If we can't get coords, keep (0, 0) - will be hidden by Show condition below
        }
      }

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

      const commands = filterCommands(pluginState.query, t);
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

  const executeCommand = async (command: SlashCommand | undefined) => {
    if (!command) return;

    // Get current plugin state from ProseMirror (authoritative source)
    const pluginState = slashMenuKey.getState(props.view.state);
    if (!pluginState) return;

    const { triggerPos } = pluginState;
    const cursorPos = props.view.state.selection.from;

    // Handle async actions
    if (command.asyncAction === 'createSubpage') {
      const ws = wsApi();
      const u = user();
      const parentId = props.nodeId;
      if (!ws || !u || !parentId) return;

      // Delete the slash text first
      const tr = props.view.state.tr.delete(triggerPos, cursorPos);
      props.view.dispatch(tr);

      try {
        // Create the subpage
        const untitledTitle = t('slashMenu.untitledSubpage') || 'Untitled';
        const newPage = await ws.nodes.page.createPage(parentId, { title: untitledTitle });
        if (!newPage?.id) return;

        // Build the URL for the new page
        const wsId = u.workspace_id;
        const wsName = u.workspace_name;
        const url = nodeUrl(wsId || '', wsName, newPage.id, untitledTitle);

        // Insert a proper link node with link mark (not raw markdown text)
        const linkMark = marks.link.create({ href: url, title: null });
        const linkNode = schema.text(untitledTitle, [linkMark]);
        const insertTr = props.view.state.tr.insert(props.view.state.selection.from, linkNode);
        props.view.dispatch(insertTr);

        // Flush auto-save to persist the link immediately
        flushAutoSave();

        // Refresh parent's children in sidebar to show new subpage
        // (loadNodes only refreshes root nodes, fetchNodeChildren refreshes the parent's children)
        await fetchNodeChildren(parentId);

        // Load the new page data and navigate to it
        await loadNode(newPage.id);
        navigate(url);
      } catch (err) {
        console.error('Failed to create subpage:', err);
      }

      props.view.focus();
      return;
    }

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

  // Hide menu visually while position is being calculated (0, 0)
  const hasValidPosition = () => {
    const pos = menuPosition();
    return pos.top !== 0 || pos.left !== 0;
  };

  return (
    <Show when={props.state.active}>
      <div
        ref={menuRef}
        class={styles.slashMenu}
        data-testid="slash-command-menu"
        style={{
          top: `${menuPosition().top}px`,
          left: `${menuPosition().left}px`,
          visibility: hasValidPosition() ? 'visible' : 'hidden',
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
