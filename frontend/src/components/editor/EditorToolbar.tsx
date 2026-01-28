// Editor toolbar component with mode toggle and formatting buttons.

import { Show } from 'solid-js';
import type { EditorView } from 'prosemirror-view';
import { schema } from 'prosemirror-markdown';
import { toggleMark, setBlockType, wrapIn, lift } from 'prosemirror-commands';
import { wrapInList } from 'prosemirror-schema-list';
import { useI18n } from '../../i18n';
import styles from './Editor.module.css';

export interface FormatState {
  isBold: boolean;
  isItalic: boolean;
  isCode: boolean;
  headingLevel: number | null;
  isBulletList: boolean;
  isOrderedList: boolean;
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
    toggleMark(schema.marks.strong)(props.view.state, props.view.dispatch);
    props.view.focus();
  };

  const toggleItalic = () => {
    if (!props.view) return;
    toggleMark(schema.marks.em)(props.view.state, props.view.dispatch);
    props.view.focus();
  };

  const toggleCode = () => {
    if (!props.view) return;
    toggleMark(schema.marks.code)(props.view.state, props.view.dispatch);
    props.view.focus();
  };

  const setHeading = (level: number) => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    if (props.formatState.headingLevel === level) {
      setBlockType(schema.nodes.paragraph)(state, dispatch);
    } else {
      setBlockType(schema.nodes.heading, { level })(state, dispatch);
    }
    props.view.focus();
  };

  const toggleBulletList = () => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    if (props.formatState.isBulletList) {
      lift(state, dispatch);
    } else {
      wrapInList(schema.nodes.bullet_list)(state, dispatch);
    }
    props.view.focus();
  };

  const toggleOrderedList = () => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    if (props.formatState.isOrderedList) {
      lift(state, dispatch);
    } else {
      wrapInList(schema.nodes.ordered_list)(state, dispatch);
    }
    props.view.focus();
  };

  const toggleBlockquote = () => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    if (props.formatState.isBlockquote) {
      lift(state, dispatch);
    } else {
      wrapIn(schema.nodes.blockquote)(state, dispatch);
    }
    props.view.focus();
  };

  const toggleCodeBlock = () => {
    if (!props.view) return;
    const { state, dispatch } = props.view;
    if (props.formatState.isCodeBlock) {
      setBlockType(schema.nodes.paragraph)(state, dispatch);
    } else {
      setBlockType(schema.nodes.code_block)(state, dispatch);
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
