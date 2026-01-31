// Editor-specific block context menu component.
// Wraps the shared RowContextMenu with editor-specific actions.

import { createSignal, onMount, onCleanup, Show } from 'solid-js';
import type { EditorView } from 'prosemirror-view';
import { RowContextMenu, type ContextMenuAction } from '../shared/RowContextMenu';
import { useI18n } from '../../i18n';
import { deleteBlock, duplicateBlock, convertBlock, indentBlock, outdentBlock } from './blockCommands';
import type { BlockType } from './schema';
import { BLOCK_CONTEXT_MENU_EVENT, type BlockContextMenuDetail } from './BlockNodeView';
import styles from './BlockContextMenu.module.css';

export interface BlockContextMenuProps {
  /** The ProseMirror editor view */
  view: EditorView | undefined;
  /** CSS class for the container */
  class?: string;
}

/**
 * Block context menu component for the editor.
 * Listens for custom events from BlockNodeView and shows appropriate actions.
 */
export function BlockContextMenu(props: BlockContextMenuProps) {
  const { t } = useI18n();
  const [menuState, setMenuState] = createSignal<{
    position: { x: number; y: number };
    blockPos: number;
    selectedCount: number;
  } | null>(null);

  // Listen for context menu events from BlockNodeView
  const handleBlockContextMenu = (e: Event) => {
    const event = e as CustomEvent<BlockContextMenuDetail>;
    const { pos, x, y, selectedCount } = event.detail;

    setMenuState({
      position: { x, y },
      blockPos: pos,
      selectedCount,
    });
  };

  onMount(() => {
    const view = props.view;
    if (view) {
      view.dom.addEventListener(BLOCK_CONTEXT_MENU_EVENT, handleBlockContextMenu);
    }
  });

  onCleanup(() => {
    const view = props.view;
    if (view) {
      view.dom.removeEventListener(BLOCK_CONTEXT_MENU_EVENT, handleBlockContextMenu);
    }
  });

  /**
   * Get context menu actions based on current state.
   */
  const getActions = (): ContextMenuAction[] => {
    const state = menuState();
    if (!state) return [];

    const { selectedCount } = state;
    const isMulti = selectedCount > 1;

    const actions: ContextMenuAction[] = [
      {
        id: 'delete',
        label: isMulti
          ? t('editor.deleteBlocks') || `Delete ${selectedCount} blocks`
          : t('editor.deleteBlock') || 'Delete block',
        shortcut: '⌫',
        danger: true,
      },
      {
        id: 'duplicate',
        label: isMulti
          ? t('editor.duplicateBlocks') || `Duplicate ${selectedCount} blocks`
          : t('editor.duplicateBlock') || 'Duplicate block',
        shortcut: '⌘D',
      },
    ];

    // Only show convert options for single blocks
    if (!isMulti) {
      actions.push(
        {
          id: 'indent',
          label: t('editor.indent') || 'Indent',
          shortcut: 'Tab',
          separator: true,
        },
        {
          id: 'outdent',
          label: t('editor.outdent') || 'Outdent',
          shortcut: '⇧Tab',
        },
        {
          id: 'convertTo',
          label: t('editor.convertTo') || 'Convert to',
          separator: true,
        }
      );

      // Block type conversion options
      const blockTypes: { id: string; type: BlockType; label: string }[] = [
        { id: 'convert-paragraph', type: 'paragraph', label: t('editor.paragraph') || 'Paragraph' },
        { id: 'convert-heading1', type: 'heading', label: t('editor.heading') + ' 1' || 'Heading 1' },
        { id: 'convert-heading2', type: 'heading', label: t('editor.heading') + ' 2' || 'Heading 2' },
        { id: 'convert-bullet', type: 'bullet', label: t('editor.bulletList') || 'Bullet list' },
        { id: 'convert-number', type: 'number', label: t('editor.numberedList') || 'Numbered list' },
        { id: 'convert-task', type: 'task', label: t('editor.taskList') || 'Task list' },
        { id: 'convert-quote', type: 'quote', label: t('editor.blockquote') || 'Quote' },
        { id: 'convert-code', type: 'code', label: t('editor.codeBlock') || 'Code block' },
      ];

      for (const bt of blockTypes) {
        actions.push({
          id: bt.id,
          label: bt.label,
        });
      }
    }

    return actions;
  };

  /**
   * Handle action selection.
   */
  const handleAction = (actionId: string) => {
    const view = props.view;
    const state = menuState();
    if (!view || !state) {
      setMenuState(null);
      return;
    }

    const { blockPos } = state;
    const { state: editorState, dispatch } = view;

    switch (actionId) {
      case 'delete':
        deleteBlock(blockPos)(editorState, dispatch);
        break;
      case 'duplicate':
        duplicateBlock(blockPos)(editorState, dispatch);
        break;
      case 'indent':
        indentBlock(blockPos)(editorState, dispatch);
        break;
      case 'outdent':
        outdentBlock(blockPos)(editorState, dispatch);
        break;
      case 'convert-paragraph':
        convertBlock(blockPos, 'paragraph')(editorState, dispatch);
        break;
      case 'convert-heading1':
        convertBlock(blockPos, 'heading', { level: 1 })(editorState, dispatch);
        break;
      case 'convert-heading2':
        convertBlock(blockPos, 'heading', { level: 2 })(editorState, dispatch);
        break;
      case 'convert-bullet':
        convertBlock(blockPos, 'bullet')(editorState, dispatch);
        break;
      case 'convert-number':
        convertBlock(blockPos, 'number')(editorState, dispatch);
        break;
      case 'convert-task':
        convertBlock(blockPos, 'task', { checked: false })(editorState, dispatch);
        break;
      case 'convert-quote':
        convertBlock(blockPos, 'quote')(editorState, dispatch);
        break;
      case 'convert-code':
        convertBlock(blockPos, 'code')(editorState, dispatch);
        break;
      case 'convertTo':
        // This is just a label, do nothing
        return;
      default:
        // Unknown action
        break;
    }

    // Close menu after action
    setMenuState(null);
  };

  /**
   * Handle menu close.
   */
  const handleClose = () => {
    setMenuState(null);
  };

  return (
    <div class={`${styles.container} ${props.class || ''}`}>
      <Show when={menuState()}>
        {(state) => (
          <RowContextMenu
            position={state().position}
            actions={getActions()}
            onAction={handleAction}
            onClose={handleClose}
          />
        )}
      </Show>
    </div>
  );
}
