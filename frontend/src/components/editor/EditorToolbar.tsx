// Floating editor toolbar with formatting buttons (appears on text selection).

import { Show, createSignal, createEffect, on } from 'solid-js';
import type { EditorView } from 'prosemirror-view';
import { toggleMark } from 'prosemirror-commands';
import { marks } from './prosemirror-config';
import { convertBlocks } from './blockCommands';
import { getSelectedBlockPositions } from './blockDragPlugin';
import type { BlockType, BlockAttrs } from './schema';
import styles from './Editor.module.css';

import FormatBoldIcon from '@material-symbols/svg-400/outlined/format_bold.svg?solid';
import FormatItalicIcon from '@material-symbols/svg-400/outlined/format_italic.svg?solid';
import FormatUnderlinedIcon from '@material-symbols/svg-400/outlined/format_underlined.svg?solid';
import FormatStrikethroughIcon from '@material-symbols/svg-400/outlined/format_strikethrough.svg?solid';
import CodeIcon from '@material-symbols/svg-400/outlined/code.svg?solid';
import FormatListBulletedIcon from '@material-symbols/svg-400/outlined/format_list_bulleted.svg?solid';
import FormatListNumberedIcon from '@material-symbols/svg-400/outlined/format_list_numbered.svg?solid';
import ChecklistIcon from '@material-symbols/svg-400/outlined/checklist.svg?solid';
import FormatQuoteIcon from '@material-symbols/svg-400/outlined/format_quote.svg?solid';
import TerminalIcon from '@material-symbols/svg-400/outlined/terminal.svg?solid';

export interface FormatState {
  isBold: boolean;
  isItalic: boolean;
  isUnderline: boolean;
  isStrikethrough: boolean;
  isCode: boolean;
  headingLevel: number | null;
  isBulletList: boolean;
  isOrderedList: boolean;
  isTaskList: boolean;
  isBlockquote: boolean;
  isCodeBlock: boolean;
}

interface EditorToolbarProps {
  formatState: FormatState;
  view: EditorView | undefined;
  position?: { top: number; bottom: number; left: number } | null;
  editorElement?: HTMLDivElement;
}

export default function EditorToolbar(props: EditorToolbarProps) {
  let toolbarRef: HTMLDivElement | undefined;
  const [above, setAbove] = createSignal(false);
  const [clampedLeft, setClampedLeft] = createSignal<number | null>(null);

  // Check if toolbar fits below selection (flip above if not) and clamp horizontally
  createEffect(
    on(
      () => props.position,
      () => {
        if (!props.position || !toolbarRef) {
          setAbove(false);
          setClampedLeft(null);
          return;
        }
        // Use requestAnimationFrame to measure after render
        requestAnimationFrame(() => {
          if (!toolbarRef || !props.position) return;
          const rect = toolbarRef.getBoundingClientRect();

          // Vertical: flip above if would overflow bottom
          const wouldOverflow = props.position.bottom + rect.height + 8 > window.innerHeight;
          setAbove(wouldOverflow);

          // Horizontal: clamp so toolbar stays within viewport
          // The toolbar is centered (translateX(-50%)), so we need halfWidth for bounds
          const halfWidth = rect.width / 2;

          // Use the editor element bounds (accounts for sidebar)
          const editorRect = props.editorElement?.getBoundingClientRect();
          const editorLeft = editorRect?.left ?? 0;
          const editorRight = editorRect?.right ?? window.innerWidth;

          const minLeft = editorLeft + halfWidth;
          const maxLeft = editorRight - halfWidth;
          const left = props.position.left;

          if (left < minLeft) {
            setClampedLeft(minLeft);
          } else if (left > maxLeft) {
            setClampedLeft(maxLeft);
          } else {
            setClampedLeft(null); // No clamping needed
          }
        });
      }
    )
  );

  const formatButtonClass = (isActive: boolean) =>
    isActive ? `${styles.formatButton} ${styles.isActive}` : styles.formatButton;

  const toggleBold = () => {
    if (!props.view) return;
    toggleMark(marks.strong)(props.view.state, props.view.dispatch);
    props.view.focus();
  };

  const toggleItalic = () => {
    if (!props.view) return;
    toggleMark(marks.em)(props.view.state, props.view.dispatch);
    props.view.focus();
  };

  const toggleUnderline = () => {
    if (!props.view) return;
    toggleMark(marks.underline)(props.view.state, props.view.dispatch);
    props.view.focus();
  };

  const toggleStrikethrough = () => {
    if (!props.view) return;
    toggleMark(marks.strikethrough)(props.view.state, props.view.dispatch);
    props.view.focus();
  };

  const toggleCode = () => {
    if (!props.view) return;
    toggleMark(marks.code)(props.view.state, props.view.dispatch);
    props.view.focus();
  };

  // Helper to apply block type conversion to all selected blocks
  const setBlockType = (type: BlockType, attrs: Partial<BlockAttrs> = {}) => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    const positions = getSelectedBlockPositions(state);

    // If no blocks selected (e.g. empty selection), use current block
    if (positions.length === 0) {
      const { $from } = state.selection;
      // Start depth 1 because depth 0 is doc
      for (let d = $from.depth; d > 0; d--) {
        if ($from.node(d).isBlock) {
          convertBlocks([$from.before(d)], type, attrs)(state, dispatch);
          break;
        }
      }
    } else {
      convertBlocks(positions, type, attrs)(state, dispatch);
    }

    props.view.focus();
  };

  const setHeading = (level: number) => {
    if (props.formatState.headingLevel === level) {
      setBlockType('paragraph');
    } else {
      setBlockType('heading', { level });
    }
  };

  const toggleBulletList = () => {
    if (props.formatState.isBulletList) {
      setBlockType('paragraph');
    } else {
      setBlockType('bullet');
    }
  };

  const toggleOrderedList = () => {
    if (props.formatState.isOrderedList) {
      setBlockType('paragraph');
    } else {
      setBlockType('number');
    }
  };

  const toggleTaskList = () => {
    if (props.formatState.isTaskList) {
      setBlockType('paragraph');
    } else {
      setBlockType('task', { checked: false });
    }
  };

  const toggleBlockquote = () => {
    if (props.formatState.isBlockquote) {
      setBlockType('paragraph');
    } else {
      setBlockType('quote');
    }
  };

  const toggleCodeBlock = () => {
    if (props.formatState.isCodeBlock) {
      setBlockType('paragraph');
    } else {
      setBlockType('code');
    }
  };

  return (
    <Show when={props.position}>
      <div
        ref={(el) => (toolbarRef = el)}
        class={`${styles.floatingToolbar} ${above() ? styles.above : ''}`}
        data-testid="floating-toolbar"
        style={{
          top: `${above() ? props.position?.top : props.position?.bottom}px`,
          left: `${clampedLeft() ?? props.position?.left ?? 0}px`,
        }}
      >
        <div class={styles.toolbarRow}>
          <button class={formatButtonClass(props.formatState.isBold)} onClick={toggleBold} title="Bold (Ctrl+B)">
            <FormatBoldIcon />
          </button>
          <button class={formatButtonClass(props.formatState.isItalic)} onClick={toggleItalic} title="Italic (Ctrl+I)">
            <FormatItalicIcon />
          </button>
          <button
            class={formatButtonClass(props.formatState.isUnderline)}
            onClick={toggleUnderline}
            title="Underline (Ctrl+U)"
          >
            <FormatUnderlinedIcon />
          </button>
          <button
            class={formatButtonClass(props.formatState.isStrikethrough)}
            onClick={toggleStrikethrough}
            title="Strikethrough (Ctrl+Shift+X)"
          >
            <FormatStrikethroughIcon />
          </button>
          <button class={formatButtonClass(props.formatState.isCode)} onClick={toggleCode} title="Code (Ctrl+`)">
            <CodeIcon />
          </button>
        </div>
        <div class={styles.toolbarRow}>
          <button
            class={formatButtonClass(props.formatState.headingLevel === 1)}
            onClick={() => setHeading(1)}
            title="Heading 1"
          >
            H1
          </button>
          <button
            class={formatButtonClass(props.formatState.headingLevel === 2)}
            onClick={() => setHeading(2)}
            title="Heading 2"
          >
            H2
          </button>
          <button
            class={formatButtonClass(props.formatState.headingLevel === 3)}
            onClick={() => setHeading(3)}
            title="Heading 3"
          >
            H3
          </button>
          <span class={styles.separator} />
          <button
            class={formatButtonClass(props.formatState.isBulletList)}
            onClick={toggleBulletList}
            title="Bullet List"
          >
            <FormatListBulletedIcon />
          </button>
          <button
            class={formatButtonClass(props.formatState.isOrderedList)}
            onClick={toggleOrderedList}
            title="Numbered List"
          >
            <FormatListNumberedIcon />
          </button>
          <button class={formatButtonClass(props.formatState.isTaskList)} onClick={toggleTaskList} title="Task List">
            <ChecklistIcon />
          </button>
          <button class={formatButtonClass(props.formatState.isBlockquote)} onClick={toggleBlockquote} title="Quote">
            <FormatQuoteIcon />
          </button>
          <button class={formatButtonClass(props.formatState.isCodeBlock)} onClick={toggleCodeBlock} title="Code Block">
            <TerminalIcon />
          </button>
        </div>
      </div>
    </Show>
  );
}
