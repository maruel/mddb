// Editor toolbar component with mode toggle and formatting buttons.

import { Show } from 'solid-js';
import type { EditorView } from 'prosemirror-view';
import { toggleMark, setBlockType, wrapIn, lift } from 'prosemirror-commands';
import { wrapInList } from 'prosemirror-schema-list';
import { nodes, marks } from './prosemirror-config';
import { useI18n } from '../../i18n';
import styles from './Editor.module.css';

export interface FormatState {
  isBold: boolean;
  isItalic: boolean;
  isCode: boolean;
  headingLevel: number | null;
  isBulletList: boolean;
  isOrderedList: boolean;
  isTaskList: boolean;
  isBlockquote: boolean;
  isCodeBlock: boolean;
}

interface EditorToolbarProps {
  editorMode: 'wysiwyg' | 'markdown';
  formatState: FormatState;
  view: EditorView | undefined;
  onSwitchToWysiwyg: () => void;
  onSwitchToMarkdown: () => void;
}

export default function EditorToolbar(props: EditorToolbarProps) {
  const { t } = useI18n();

  const modeButtonClass = (isActive: boolean) =>
    isActive ? `${styles.modeButton} ${styles.active}` : styles.modeButton;

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

  const toggleCode = () => {
    if (!props.view) return;
    toggleMark(marks.code)(props.view.state, props.view.dispatch);
    props.view.focus();
  };

  const setHeading = (level: number) => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    if (props.formatState.headingLevel === level) {
      setBlockType(nodes.paragraph)(state, dispatch);
    } else {
      setBlockType(nodes.heading, { level })(state, dispatch);
    }
    props.view.focus();
  };

  const toggleBulletList = () => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    if (props.formatState.isBulletList) {
      lift(state, dispatch);
    } else {
      wrapInList(nodes.bullet_list)(state, dispatch);
    }
    props.view.focus();
  };

  const toggleOrderedList = () => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    if (props.formatState.isOrderedList) {
      lift(state, dispatch);
    } else {
      wrapInList(nodes.ordered_list)(state, dispatch);
    }
    props.view.focus();
  };

  const toggleTaskList = () => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    const { $from } = state.selection;

    if (props.formatState.isTaskList) {
      // Convert task list item to regular list item
      for (let d = $from.depth; d > 0; d--) {
        const node = $from.node(d);
        if (node.type === nodes.list_item && node.attrs.checked !== null) {
          const tr = state.tr.setNodeMarkup($from.before(d), undefined, { checked: null });
          dispatch(tr);
          props.view.focus();
          return;
        }
      }
    } else if (props.formatState.isBulletList || props.formatState.isOrderedList) {
      // Convert regular list item to task list item
      for (let d = $from.depth; d > 0; d--) {
        const node = $from.node(d);
        if (node.type === nodes.list_item) {
          const tr = state.tr.setNodeMarkup($from.before(d), undefined, { checked: false });
          dispatch(tr);
          props.view.focus();
          return;
        }
      }
    } else {
      // Create new task list
      const listItem = nodes.list_item.create({ checked: false }, nodes.paragraph.create());
      const bulletList = nodes.bullet_list.create(null, listItem);
      const tr = state.tr.replaceSelectionWith(bulletList);
      dispatch(tr);
      props.view.focus();
    }
  };

  const toggleBlockquote = () => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    if (props.formatState.isBlockquote) {
      lift(state, dispatch);
    } else {
      wrapIn(nodes.blockquote)(state, dispatch);
    }
    props.view.focus();
  };

  const toggleCodeBlock = () => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    if (props.formatState.isCodeBlock) {
      setBlockType(nodes.paragraph)(state, dispatch);
    } else {
      setBlockType(nodes.code_block)(state, dispatch);
    }
    props.view.focus();
  };

  return (
    <div class={styles.editorToolbar}>
      <div class={styles.modeToggle}>
        <button
          class={modeButtonClass(props.editorMode === 'wysiwyg')}
          onClick={props.onSwitchToWysiwyg}
          title={t('editor.wysiwygMode') || 'Visual'}
          data-testid="editor-mode-visual"
        >
          {t('editor.wysiwygMode') || 'Visual'}
        </button>
        <button
          class={modeButtonClass(props.editorMode === 'markdown')}
          onClick={props.onSwitchToMarkdown}
          title={t('editor.markdownMode') || 'Markdown'}
          data-testid="editor-mode-markdown"
        >
          {t('editor.markdownMode') || 'Markdown'}
        </button>
      </div>

      <Show when={props.editorMode === 'wysiwyg'}>
        <div class={styles.formattingButtons}>
          <button class={formatButtonClass(props.formatState.isBold)} onClick={toggleBold} title="Bold (Ctrl+B)">
            B
          </button>
          <button class={formatButtonClass(props.formatState.isItalic)} onClick={toggleItalic} title="Italic (Ctrl+I)">
            I
          </button>
          <button class={formatButtonClass(props.formatState.isCode)} onClick={toggleCode} title="Code (Ctrl+`)">
            {'</>'}
          </button>
          <span class={styles.separator} />
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
            •
          </button>
          <button
            class={formatButtonClass(props.formatState.isOrderedList)}
            onClick={toggleOrderedList}
            title="Numbered List"
          >
            1.
          </button>
          <button class={formatButtonClass(props.formatState.isTaskList)} onClick={toggleTaskList} title="Task List">
            ☐
          </button>
          <button class={formatButtonClass(props.formatState.isBlockquote)} onClick={toggleBlockquote} title="Quote">
            "
          </button>
          <button class={formatButtonClass(props.formatState.isCodeBlock)} onClick={toggleCodeBlock} title="Code Block">
            {'{ }'}
          </button>
        </div>
      </Show>
    </div>
  );
}
