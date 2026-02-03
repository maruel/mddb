// Editor-specific block context menu component.
// Wraps the shared ContextMenu with editor-specific actions.

import { createSignal, onMount, onCleanup, Show, type JSX } from 'solid-js';
import type { EditorView } from 'prosemirror-view';
import { ContextMenu, type ContextMenuAction } from '../shared/ContextMenu';
import { useI18n } from '../../i18n';
import { deleteBlock, duplicateBlock, convertBlock, indentBlock, outdentBlock } from './blockCommands';
import type { BlockType } from './schema';
import { BLOCK_CONTEXT_MENU_EVENT, type BlockContextMenuDetail } from './BlockNodeView';
import styles from './BlockContextMenu.module.css';

import ContentCopyIcon from '@material-symbols/svg-400/outlined/content_copy.svg?solid';
import FormatIndentIncreaseIcon from '@material-symbols/svg-400/outlined/format_indent_increase.svg?solid';
import FormatIndentDecreaseIcon from '@material-symbols/svg-400/outlined/format_indent_decrease.svg?solid';
import DeleteIcon from '@material-symbols/svg-400/outlined/delete.svg?solid';
import SubjectIcon from '@material-symbols/svg-400/outlined/subject.svg?solid';
import TitleIcon from '@material-symbols/svg-400/outlined/title.svg?solid';
import FormatListBulletedIcon from '@material-symbols/svg-400/outlined/format_list_bulleted.svg?solid';
import FormatListNumberedIcon from '@material-symbols/svg-400/outlined/format_list_numbered.svg?solid';
import ChecklistIcon from '@material-symbols/svg-400/outlined/checklist.svg?solid';
import FormatQuoteIcon from '@material-symbols/svg-400/outlined/format_quote.svg?solid';
import CodeIcon from '@material-symbols/svg-400/outlined/code.svg?solid';

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
        id: 'duplicate',
        label: isMulti
          ? t('editor.duplicateBlocks') || `Duplicate ${selectedCount} blocks`
          : t('editor.duplicateBlock') || 'Duplicate block',
        icon: <ContentCopyIcon />,
        shortcut: '⌘D',
      },
    ];

    // Only show convert options for single blocks
    if (!isMulti) {
      actions.push(
        {
          id: 'indent',
          label: t('editor.indent') || 'Indent',
          icon: <FormatIndentIncreaseIcon />,
          shortcut: 'Tab',
          separator: true,
        },
        {
          id: 'outdent',
          label: t('editor.outdent') || 'Outdent',
          icon: <FormatIndentDecreaseIcon />,
          shortcut: '⇧Tab',
        }
      );

      // Block type conversion options
      const blockTypes: { id: string; type: BlockType; label: string; icon: JSX.Element }[] = [
        {
          id: 'convert-paragraph',
          type: 'paragraph',
          label: t('editor.paragraph') || 'Paragraph',
          icon: <SubjectIcon />,
        },
        {
          id: 'convert-heading1',
          type: 'heading',
          label: (t('editor.heading') || 'Heading') + ' 1',
          icon: <TitleIcon />,
        },
        {
          id: 'convert-heading2',
          type: 'heading',
          label: (t('editor.heading') || 'Heading') + ' 2',
          icon: <TitleIcon />,
        },
        {
          id: 'convert-bullet',
          type: 'bullet',
          label: t('editor.bulletList') || 'Bullet list',
          icon: <FormatListBulletedIcon />,
        },
        {
          id: 'convert-number',
          type: 'number',
          label: t('editor.numberedList') || 'Numbered list',
          icon: <FormatListNumberedIcon />,
        },
        { id: 'convert-task', type: 'task', label: t('editor.taskList') || 'Task list', icon: <ChecklistIcon /> },
        { id: 'convert-quote', type: 'quote', label: t('editor.blockquote') || 'Quote', icon: <FormatQuoteIcon /> },
        { id: 'convert-code', type: 'code', label: t('editor.codeBlock') || 'Code block', icon: <CodeIcon /> },
      ];

      blockTypes.forEach((bt, i) => {
        actions.push({
          id: bt.id,
          label: bt.label,
          icon: bt.icon,
          separator: i === 0,
        });
      });
    }

    // Delete action always last with separator
    actions.push({
      id: 'delete',
      label: isMulti
        ? t('editor.deleteBlocks') || `Delete ${selectedCount} blocks`
        : t('editor.deleteBlock') || 'Delete block',
      icon: <DeleteIcon />,
      shortcut: '⌫',
      danger: true,
      separator: true,
    });

    return actions;
  };

  /**
   * Handle action selection.
   * Uses view.state and view.dispatch directly to avoid stale closures and binding issues.
   */
  const handleAction = (actionId: string) => {
    const view = props.view;
    const state = menuState();
    if (!view || !state) {
      setMenuState(null);
      return;
    }

    const { blockPos } = state;

    switch (actionId) {
      case 'delete':
        deleteBlock(blockPos)(view.state, view.dispatch);
        break;
      case 'duplicate':
        duplicateBlock(blockPos)(view.state, view.dispatch);
        break;
      case 'indent':
        indentBlock(blockPos)(view.state, view.dispatch);
        break;
      case 'outdent':
        outdentBlock(blockPos)(view.state, view.dispatch);
        break;
      case 'convert-paragraph':
        convertBlock(blockPos, 'paragraph')(view.state, view.dispatch);
        break;
      case 'convert-heading1':
        convertBlock(blockPos, 'heading', { level: 1 })(view.state, view.dispatch);
        break;
      case 'convert-heading2':
        convertBlock(blockPos, 'heading', { level: 2 })(view.state, view.dispatch);
        break;
      case 'convert-bullet':
        convertBlock(blockPos, 'bullet')(view.state, view.dispatch);
        break;
      case 'convert-number':
        convertBlock(blockPos, 'number')(view.state, view.dispatch);
        break;
      case 'convert-task':
        convertBlock(blockPos, 'task', { checked: false })(view.state, view.dispatch);
        break;
      case 'convert-quote':
        convertBlock(blockPos, 'quote')(view.state, view.dispatch);
        break;
      case 'convert-code':
        convertBlock(blockPos, 'code')(view.state, view.dispatch);
        break;
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
          <ContextMenu
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
